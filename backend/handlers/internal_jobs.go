package handlers

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"jobscout/config"
	"jobscout/database"
	"jobscout/dedup"
	"jobscout/metrics"
	"jobscout/models"
	"jobscout/pulse"
	"jobscout/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// keywordRegexCache memoises the compiled word-boundary regex for each
// niche keyword so we don't rebuild it on every ingest. Regex
// compilation is the hot path's biggest CPU cost without it.
var (
	keywordRegexMu    sync.RWMutex
	keywordRegexCache = make(map[string]*regexp.Regexp)
)

// compileKeyword turns a free-form keyword into a case-insensitive,
// punctuation-tolerant, word-boundary anchored regex.
//
//	"AutoCAD"          -> (?i)\bautocad\b
//	"Machine Learning" -> (?i)\bmachine[\s\-_/]+learning\b
//	"C++"              -> (?i)\bc\+\+        (no trailing \b — '+' isn't a \w)
//
// The trailing `\b` is conditional on the keyword ending in a word
// character; without that guard, a keyword like "C++" would never
// match because `+\b` requires the boundary to be between a word and
// non-word char.
func compileKeyword(raw string) *regexp.Regexp {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return nil
	}
	keywordRegexMu.RLock()
	if r, ok := keywordRegexCache[key]; ok {
		keywordRegexMu.RUnlock()
		return r
	}
	keywordRegexMu.RUnlock()

	// Split on whitespace so multi-word keywords stay contiguous but
	// tolerate any amount of internal whitespace / hyphens / slashes.
	parts := strings.Fields(key)
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	body := strings.Join(parts, `[\s\-_/]+`)
	leading := ""
	trailing := ""
	if len(key) > 0 && isWordChar(key[0]) {
		leading = `\b`
	}
	if len(key) > 0 && isWordChar(key[len(key)-1]) {
		trailing = `\b`
	}
	pattern := `(?i)` + leading + body + trailing
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Pathological keyword (mostly from QuoteMeta producing a valid
		// pattern that's still too long). Fall back to a literal
		// substring matcher wrapped in a forgiving regex.
		re = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(key))
	}
	keywordRegexMu.Lock()
	keywordRegexCache[key] = re
	keywordRegexMu.Unlock()
	return re
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// effectiveThreshold returns the niche's MinContextMatches, defaulting
// to 2 when unset. Centralised so the rejection log and the JSON
// response always agree on what threshold was applied.
func effectiveThreshold(n *models.NicheProfile) int {
	if n == nil || n.MinContextMatches <= 0 {
		return 2
	}
	return n.MinContextMatches
}

// IngestPayload is the wire format the Python scraper POSTs to
// /api/internal/jobs. Fields mirror what the scraper extracts.
type IngestPayload struct {
	Title    string   `json:"title"`
	Company  string   `json:"company"`
	Location string   `json:"location"`
	URL      string   `json:"url"`
	Salary   string   `json:"salary,omitempty"`
	Deadline string   `json:"deadline,omitempty"` // RFC3339 or empty
	Tags     []string `json:"tags,omitempty"`
	Sources  []string `json:"sources,omitempty"`
	Type     string   `json:"type,omitempty"`
	Level    string   `json:"level,omitempty"`
	// TaskID, when set, links the inserted/merged Job back to the
	// user-defined ScrapeTask that produced it (via a ScrapeResult row).
	TaskID string `json:"taskId,omitempty"`
	// NicheID, when set, scopes the validation layer: the body is
	// scanned for the niche's ContextKeywords and rejected if fewer than
	// MinContextMatches hit. The scraper sends `body` for validation.
	NicheID string `json:"nicheId,omitempty"`
	// Body is the raw extracted page text used for ContextKeyword
	// validation. Optional; if absent, validation falls back to title +
	// tags + company.
	Body string `json:"body,omitempty"`
}

// validateNiche enforces the ContextKeyword threshold described in the
// Phase 4 spec: at least MinContextMatches keywords from the niche must
// appear in the candidate text. Returns (niche, ok, hits, missing).
//
// When the niche cannot be resolved (e.g. unknown id), validation is
// skipped (ok=true) so legacy / un-niched flows keep working.
func validateNiche(p *IngestPayload) (*models.NicheProfile, bool, int, []string) {
	if p.NicheID == "" {
		return nil, true, 0, nil
	}
	id, err := uuid.Parse(p.NicheID)
	if err != nil {
		return nil, true, 0, nil
	}
	var niche *models.NicheProfile
	if database.DB != nil {
		var n models.NicheProfile
		if err := database.DB.First(&n, "id = ?", id).Error; err != nil {
			return nil, true, 0, nil
		}
		niche = &n
	} else {
		niche = store.MemoryNiches.GetProfile(id)
		if niche == nil {
			return nil, true, 0, nil
		}
	}
	if len(niche.ContextKeywords) == 0 {
		return niche, true, 0, nil
	}
	threshold := niche.MinContextMatches
	if threshold <= 0 {
		threshold = 2
	}

	// Build the haystack from every text field the scraper provides.
	// We deliberately don't strip punctuation here — the per-keyword
	// regex already tolerates internal punctuation via [\s\-_/]+.
	hay := strings.Join([]string{
		p.Title, p.Company, p.Body, strings.Join(p.Tags, " "),
	}, "\n")

	hits := 0
	matched := map[string]bool{}
	missing := make([]string, 0, len(niche.ContextKeywords))
	for _, kw := range niche.ContextKeywords {
		key := strings.ToLower(strings.TrimSpace(kw))
		if key == "" || matched[key] {
			continue
		}
		re := compileKeyword(kw)
		if re != nil && re.MatchString(hay) {
			matched[key] = true
			hits++
			continue
		}
		missing = append(missing, kw)
	}
	return niche, hits >= threshold, hits, missing
}

// RequireInternalToken guards the internal ingestion endpoint with a shared
// secret sent by the Python worker via the X-Internal-Token header.
func RequireInternalToken(c *fiber.Ctx) error {
	want := config.Load().InternalAPIToken
	got := c.Get("X-Internal-Token")
	if want == "" || got != want {
		return c.Status(fiber.StatusUnauthorized).
			JSON(fiber.Map{"error": "invalid internal token"})
	}
	return c.Next()
}

// IngestJob accepts one job record from the scraper, cleans it,
// fingerprints it via SHA256(company|title), and either:
//
//   - INSERT a fresh row on first sight
//   - MERGE an existing row when the (company, title) similarity hash
//     collides: append the novel source / posting URL, increment
//     merge_count, and broadcast a pulse Event so the UI updates live.
func IngestJob(c *fiber.Ctx) error {
	if database.DB == nil {
		// DB-less fallback: persist into the in-memory job store so the
		// homepage feed and Scraper Health counters reflect real scraping
		// activity even without Postgres running.
		var p IngestPayload
		if err := c.BodyParser(&p); err != nil {
			return c.Status(fiber.StatusBadRequest).
				JSON(fiber.Map{"error": "invalid payload: " + err.Error()})
		}
		title := dedup.Clean(p.Title)
		companyRaw := dedup.Clean(p.Company)
		if title == "" || companyRaw == "" {
			return c.Status(fiber.StatusBadRequest).
				JSON(fiber.Map{"error": "title and company are required"})
		}

		// Niche validation: drop jobs that don't satisfy ContextKeyword threshold.
		niche, ok, hits, missing := validateNiche(&p)
		if niche != nil && !ok {
			threshold := effectiveThreshold(niche)
			reason := fmt.Sprintf(
				"Rejected: %q @ %s · Found %d/%d keywords. Missing: %v",
				title, companyRaw, hits, threshold, missing,
			)
			metrics.Record(parseUUIDPtr(p.TaskID), &niche.ID, metrics.Rejected, reason)
			log.Printf("[ingest] %s", reason)
			pulse.Broadcast("alert",
				fmt.Sprintf("Rejected %q · niche %s · %d/%d · missing %v",
					title, niche.Name, hits, threshold, missing))
			return c.Status(fiber.StatusAccepted).
				JSON(fiber.Map{"status": "rejected", "reason": "context-keyword-threshold", "hits": hits, "threshold": threshold, "missing": missing})
		}

		hash := dedup.SourceHash(companyRaw, title)

		var deadline *time.Time
		if p.Deadline != "" {
			if t, err := time.Parse(time.RFC3339, p.Deadline); err == nil {
				deadline = &t
			}
		}

		job := models.Job{
			Title:      title,
			Company:    companyRaw,
			Location:   dedup.Clean(p.Location),
			URL:        dedup.Clean(p.URL),
			Salary:     dedup.Clean(p.Salary),
			PostedAt:   time.Now().UTC(),
			Deadline:   deadline,
			Sources:    pq.StringArray(p.Sources),
			Tags:       pq.StringArray(p.Tags),
			Type:       p.Type,
			Level:      p.Level,
			SourceHash: hash,
			MergedURLs: pq.StringArray{p.URL},
			MatchScore: 75,
			IsActive:   true,
		}
		if niche != nil {
			id := niche.ID
			job.NicheID = &id
		}
		stored, merged := store.MemoryJobs.Upsert(job)

		var nicheIDPtr *uuid.UUID
		if niche != nil {
			nid := niche.ID
			nicheIDPtr = &nid
		}
		metrics.Record(parseUUIDPtr(p.TaskID), nicheIDPtr, metrics.Success,
			fmt.Sprintf("%s @ %s", stored.Title, stored.Company))

		if niche != nil {
			pulse.Broadcast("scrape",
				fmt.Sprintf("Scraping %s niche · %s @ %s (ctx %d)", niche.Name, stored.Title, stored.Company, hits))
		} else if merged {
			pulse.Broadcast("merge", fmt.Sprintf("Merged %s @ %s · %d sources", stored.Title, stored.Company, len(stored.Sources)))
		} else {
			pulse.Broadcast("scrape", fmt.Sprintf("New job · %s @ %s", stored.Title, stored.Company))
		}
		if p.TaskID != "" {
			if tid, err := uuid.Parse(p.TaskID); err == nil && !merged {
				// Only count brand-new jobs against the originating task.
				store.MemoryTargets.IncrementResultCount(tid)
			}
		}
		return c.Status(fiber.StatusAccepted).
			JSON(fiber.Map{"status": "accepted", "merged": merged, "id": stored.ID.String()})
	}

	var p IngestPayload
	if err := c.BodyParser(&p); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid payload: " + err.Error()})
	}

	title := dedup.Clean(p.Title)
	companyRaw := dedup.Clean(p.Company)
	companyNorm := dedup.NormalizeCompany(companyRaw)
	url := dedup.Clean(p.URL)
	if title == "" || companyRaw == "" || url == "" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "title, company and url are required"})
	}

	// Niche validation (DB path): drop jobs that fail the ContextKeyword threshold.
	niche, ok, hits, missing := validateNiche(&p)
	if niche != nil && !ok {
		threshold := effectiveThreshold(niche)
		reason := fmt.Sprintf(
			"Rejected: %q @ %s · Found %d/%d keywords. Missing: %v",
			title, companyRaw, hits, threshold, missing,
		)
		metrics.Record(parseUUIDPtr(p.TaskID), &niche.ID, metrics.Rejected, reason)
		log.Printf("[ingest] %s", reason)
		pulse.Broadcast("alert",
			fmt.Sprintf("Rejected %q · niche %s · %d/%d · missing %v",
				title, niche.Name, hits, threshold, missing))
		return c.Status(fiber.StatusAccepted).
			JSON(fiber.Map{"status": "rejected", "reason": "context-keyword-threshold", "hits": hits, "threshold": threshold, "missing": missing})
	}

	hash := dedup.SourceHash(companyRaw, title)
	now := time.Now().UTC()

	var deadline *time.Time
	if p.Deadline != "" {
		if t, err := time.Parse(time.RFC3339, p.Deadline); err == nil {
			deadline = &t
		}
	}

	// Look for an existing record by similarity hash.
	var existing models.Job
	err := database.DB.Where("source_hash = ?", hash).First(&existing).Error

	switch {
	case err == nil:
		resp := mergeIntoExisting(c, &existing, p, url, deadline, now, companyNorm, title, hash)
		linkTaskToJob(p.TaskID, existing.ID)
		return resp

	case errors.Is(err, gorm.ErrRecordNotFound):
		return insertNew(c, p, title, companyNorm, url, hash, deadline, now, niche, hits)

	default:
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "db error: " + err.Error()})
	}
}

// linkTaskToJob records a ScrapeResult row connecting an ingested Job to
// the ScrapeTask that found it, and bumps the task's result counter.
// Silently no-ops if no task id was supplied or the id is malformed.
func linkTaskToJob(taskIDRaw string, jobID uuid.UUID) {
	if taskIDRaw == "" || database.DB == nil {
		return
	}
	taskID, err := uuid.Parse(taskIDRaw)
	if err != nil {
		return
	}
	// Deduplicate the (task_id, job_id) link — a task may re-scrape the
	// same role on subsequent runs; we only want one ScrapeResult row.
	var existing int64
	database.DB.Model(&models.ScrapeResult{}).
		Where("task_id = ? AND job_id = ?", taskID, jobID).
		Count(&existing)
	if existing > 0 {
		return
	}
	if err := database.DB.Create(&models.ScrapeResult{
		TaskID: taskID,
		JobID:  jobID,
	}).Error; err != nil {
		log.Printf("[ingest] linkTaskToJob failed: %v", err)
		return
	}
	database.DB.Model(&models.ScrapeTask{}).
		Where("id = ?", taskID).
		UpdateColumn("result_count", gorm.Expr("result_count + 1"))
}

// mergeIntoExisting handles the duplicate-prevention path. If the incoming
// payload introduces a new source or posting URL, we fold it in and bump
// merge_count; otherwise we just touch last_scraped_at.
func mergeIntoExisting(
	c *fiber.Ctx,
	existing *models.Job,
	p IngestPayload,
	url string,
	deadline *time.Time,
	now time.Time,
	companyNorm, title, hash string,
) error {
	mergedURLs := stringSet(existing.MergedURLs)
	mergedSources := stringSet(existing.Sources)

	addedURL := false
	if url != "" && !mergedURLs[url] {
		mergedURLs[url] = true
		addedURL = true
	}
	addedSource := false
	for _, s := range p.Sources {
		if s == "" || mergedSources[s] {
			continue
		}
		mergedSources[s] = true
		addedSource = true
	}

	updates := map[string]any{
		"last_scraped_at": now,
		"is_active":       true,
	}
	if deadline != nil {
		updates["deadline"] = deadline
	}

	if addedURL || addedSource {
		updates["merged_urls"] = pq.StringArray(setToSlice(mergedURLs))
		updates["sources"] = pq.StringArray(setToSlice(mergedSources))
		updates["merge_count"] = existing.MergeCount + 1
	}

	if err := database.DB.Model(existing).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "update failed: " + err.Error()})
	}

	if addedURL || addedSource {
		log.Printf("[dedup] Merged · company=%q title=%q sources=%v urls=%d",
			companyNorm, title, setToSlice(mergedSources), len(mergedURLs))
		pulse.Broadcast("merge", fmt.Sprintf(
			"Merged %s @ %s (×%d)", title, companyNorm, existing.MergeCount+1,
		))
	} else {
		log.Printf("[dedup] Duplicate Prevented · company=%q title=%q hash=%s",
			companyNorm, title, hash[:10])
	}
	// Record as a successful scrape either way — the Python side reached
	// the target, extracted a valid job, and the dedup engine handled it.
	metrics.Record(parseUUIDPtr(p.TaskID), existing.NicheID, metrics.Success,
		fmt.Sprintf("merge: %s @ %s", title, companyNorm))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "merged",
		"id":     existing.ID.String(),
		"merge":  addedURL || addedSource,
	})
}

// insertNew creates a brand-new job record on first sight.
func insertNew(
	c *fiber.Ctx,
	p IngestPayload,
	title, companyNorm, url, hash string,
	deadline *time.Time,
	now time.Time,
	niche *models.NicheProfile,
	ctxHits int,
) error {
	job := models.Job{
		Title:         title,
		Company:       companyNorm,
		Location:      dedup.Clean(p.Location),
		URL:           url,
		Salary:        dedup.Clean(p.Salary),
		PostedAt:      now,
		Deadline:      deadline,
		Sources:       pq.StringArray(p.Sources),
		Tags:          pq.StringArray(p.Tags),
		Type:          dedup.Clean(p.Type),
		Level:         dedup.Clean(p.Level),
		SourceHash:    hash,
		MergedURLs:    pq.StringArray{url},
		MergeCount:    0,
		IsActive:      true,
		LastScrapedAt: &now,
	}
	if niche != nil {
		id := niche.ID
		job.NicheID = &id
	}
	if err := database.DB.Create(&job).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "insert failed: " + err.Error()})
	}
	log.Printf("[dedup] Inserted · company=%q title=%q id=%s",
		companyNorm, title, job.ID)
	var nicheIDPtr *uuid.UUID
	if niche != nil {
		nid := niche.ID
		nicheIDPtr = &nid
	}
	metrics.Record(parseUUIDPtr(p.TaskID), nicheIDPtr, metrics.Success,
		fmt.Sprintf("%s @ %s", title, companyNorm))

	if niche != nil {
		pulse.Broadcast("scrape",
			fmt.Sprintf("Scraping %s niche · %s @ %s (ctx %d)", niche.Name, title, companyNorm, ctxHits))
	} else {
		pulse.Broadcast("scrape", fmt.Sprintf("New job · %s @ %s", title, companyNorm))
	}
	linkTaskToJob(p.TaskID, job.ID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status": "created",
		"id":     job.ID.String(),
	})
}

// parseUUIDPtr safely converts a raw taskId string to *uuid.UUID. Returns
// nil on empty string or parse errors so metric recording never throws.
func parseUUIDPtr(raw string) *uuid.UUID {
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

// --- tiny set helpers ---------------------------------------------------

func stringSet(in pq.StringArray) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		out[s] = true
	}
	return out
}

func setToSlice(in map[string]bool) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	return out
}

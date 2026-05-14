package handlers

import (
	"strings"
	"time"

	"jobscout/database"
	"jobscout/models"
	"jobscout/store"

	"github.com/gofiber/fiber/v2"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// MergedRef mirrors the `merged` entries in the frontend Job type.
type MergedRef struct {
	Source  string    `json:"source"`
	URL     string    `json:"url"`
	FoundAt time.Time `json:"foundAt"`
}

// JobResponse is the JSON shape the frontend expects. It extends the DB model
// with the display-only `merged` field (not persisted yet).
type JobResponse struct {
	ID         string      `json:"id"`
	JobUUID    string      `json:"jobUuid"`
	Title      string      `json:"title"`
	Company    string      `json:"company"`
	Location   string      `json:"location"`
	URL        string      `json:"url,omitempty"`
	Salary     string      `json:"salary,omitempty"`
	PostedAt   time.Time   `json:"postedAt"`
	// Deadline is a pointer so jobs with no scraped deadline serialize as
	// JSON null (not the year-0001 zero value) and the frontend can tell
	// "no deadline known" from "deadline already passed".
	Deadline   *time.Time  `json:"deadline,omitempty"`
	MatchScore int         `json:"matchScore"`
	Sources    []string    `json:"sources"`
	Tags       []string    `json:"tags"`
	Type       string      `json:"type"`
	Level      string      `json:"level"`
	IsActive   bool        `json:"isActive"`
	MergeCount int         `json:"mergeCount"`
	NicheID    string      `json:"nicheId,omitempty"`
	Merged     []MergedRef `json:"merged,omitempty"`
}

// ListJobs returns active jobs, optionally filtered by query params:
//
//	q        — full-text-ish search across title, company, and tags (ILIKE)
//	role     — ILIKE match against title
//	location — ILIKE match against location
//
// An empty querystring returns every active job (Constraint #2).
// When no DB is available, the in-memory mock set is filtered with the
// same semantics so the dev UX stays consistent.
func ListJobs(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	role := strings.TrimSpace(c.Query("role"))
	location := strings.TrimSpace(c.Query("location"))
	nicheID := strings.TrimSpace(c.Query("nicheId"))

	if database.DB != nil {
		var rows []models.Job
		tx := buildJobsQuery(database.DB, q, role, location)
		if nicheID != "" {
			tx = tx.Where("niche_id = ?", nicheID)
		}
		if err := tx.Order("posted_at desc").Find(&rows).Error; err == nil {
			return c.JSON(toJobResponses(rows))
		}
	}
	// DB-less path: serve whatever has been ingested into the in-memory
	// job store this session. Returns an empty array (NOT mock data) when
	// no scraping has happened yet.
	return c.JSON(toJobResponses(store.MemoryJobs.List(q, role, location, nicheID)))
}

// toJobResponses converts model rows into the camelCase API shape the
// frontend expects, including the `merged` array used by job-row.tsx.
func toJobResponses(rows []models.Job) []JobResponse {
	out := make([]JobResponse, 0, len(rows))
	for _, r := range rows {
		nicheID := ""
		if r.NicheID != nil {
			nicheID = r.NicheID.String()
		}
		out = append(out, JobResponse{
			ID:         "JOB-" + r.ID.String()[:6],
			JobUUID:    r.ID.String(),
			Title:      r.Title,
			Company:    r.Company,
			Location:   r.Location,
			URL:        r.URL,
			Salary:     r.Salary,
			PostedAt:   r.PostedAt,
			Deadline:   r.Deadline,
			MatchScore: r.MatchScore,
			// Casting a nil pq.StringArray yields a nil []string which Go's
			// json encoder serialises as `null`. The frontend treats these
			// fields as iterable arrays (`.slice`, `.map`, `.includes`),
			// so we coerce to empty slices to keep the contract array-shaped.
			Sources:    nonNilStrings(r.Sources),
			Tags:       nonNilStrings(r.Tags),
			Type:       r.Type,
			Level:      r.Level,
			IsActive:   r.IsActive,
			MergeCount: r.MergeCount,
			NicheID:    nicheID,
			Merged:     buildMergedRefs(r.Sources, r.MergedURLs, r.LastScrapedAt),
		})
	}
	return out
}

// buildMergedRefs zips the `sources` and `merged_urls` slices into the
// MergedRef shape the frontend already expects (used by the "Merged ×N"
// expandable badge in `job-row.tsx`). When a job has only one source and
// one URL the result is a single-element slice, which the UI silently hides.
func buildMergedRefs(sources, urls pq.StringArray, foundAt *time.Time) []MergedRef {
	t := time.Time{}
	if foundAt != nil {
		t = *foundAt
	}
	n := len(sources)
	if len(urls) > n {
		n = len(urls)
	}
	if n == 0 {
		return nil
	}
	out := make([]MergedRef, 0, n)
	for i := 0; i < n; i++ {
		var src, url string
		if i < len(sources) {
			src = sources[i]
		}
		if i < len(urls) {
			url = urls[i]
		}
		if src == "" && url == "" {
			continue
		}
		out = append(out, MergedRef{Source: src, URL: url, FoundAt: t})
	}
	return out
}

// buildJobsQuery applies the search filters onto a base "is_active = true"
// query. Each parameter is optional.
func buildJobsQuery(db *gorm.DB, q, role, location string) *gorm.DB {
	tx := db.Model(&models.Job{}).Where("is_active = ?", true)
	if q != "" {
		like := "%" + q + "%"
		// title ILIKE %q% OR company ILIKE %q%
		//   OR EXISTS(unnest(tags) t WHERE t ILIKE %q%)
		tx = tx.Where(
			"title ILIKE ? OR company ILIKE ? OR EXISTS (SELECT 1 FROM unnest(tags) t WHERE t ILIKE ?)",
			like, like, like,
		)
	}
	if role != "" {
		tx = tx.Where("title ILIKE ?", "%"+role+"%")
	}
	if location != "" {
		tx = tx.Where("location ILIKE ?", "%"+location+"%")
	}
	return tx
}

// nonNilStrings returns an empty slice for a nil pq.StringArray so the
// JSON encoder emits `[]` instead of `null` for collection fields.
// Frontend code calls `.slice/.map/.includes` on these arrays, so a null
// would crash a render (TanStack Router error boundary on the homepage).
func nonNilStrings(arr pq.StringArray) []string {
	if arr == nil {
		return []string{}
	}
	return []string(arr)
}

package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"jobscout/database"
	"jobscout/models"
	"jobscout/pulse"
	"jobscout/scraper"
	"jobscout/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// NicheProfileResponse augments NicheProfile with derived fields used by
// the Niche Manager UI (e.g. the live source count).
type NicheProfileResponse struct {
	models.NicheProfile
	SourceCount int `json:"sourceCount"`
}

// ListNiches handles GET /api/niches.
func ListNiches(c *fiber.Ctx) error {
	if database.DB != nil {
		var rows []models.NicheProfile
		if err := database.DB.Order("name asc").Find(&rows).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).
				JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]NicheProfileResponse, 0, len(rows))
		for _, r := range rows {
			var n int64
			database.DB.Model(&models.NicheSource{}).Where("niche_id = ?", r.ID).Count(&n)
			out = append(out, NicheProfileResponse{NicheProfile: r, SourceCount: int(n)})
		}
		return c.JSON(out)
	}
	rows := store.MemoryNiches.ListProfiles()
	out := make([]NicheProfileResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, NicheProfileResponse{
			NicheProfile: r,
			SourceCount:  len(store.MemoryNiches.ListSources(r.ID)),
		})
	}
	return c.JSON(out)
}

// nicheBody is the create/update payload from the UI.
//
// Pointer fields let PATCH callers send a partial body — `null`/omitted
// values mean "leave this field alone" instead of "blank it out". Without
// the pointer on Description, partial updates (like editing only the
// MinContextMatches field) would silently wipe the description.
type nicheBody struct {
	Name              string    `json:"name"`
	Description       *string   `json:"description"`
	SeedKeywords      []string  `json:"seedKeywords"`
	ContextKeywords   []string  `json:"contextKeywords"`
	MinContextMatches int       `json:"minContextMatches"`
}

// CreateNiche handles POST /api/niches.
func CreateNiche(c *fiber.Ctx) error {
	var body nicheBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "name is required"})
	}
	if body.MinContextMatches < 0 {
		body.MinContextMatches = 0
	}
	if body.MinContextMatches == 0 {
		body.MinContextMatches = 2
	}

	desc := ""
	if body.Description != nil {
		desc = strings.TrimSpace(*body.Description)
	}
	profile := models.NicheProfile{
		Name:              body.Name,
		Description:       desc,
		SeedKeywords:      pq.StringArray(cleanKeywordList(body.SeedKeywords)),
		ContextKeywords:   pq.StringArray(cleanKeywordList(body.ContextKeywords)),
		MinContextMatches: body.MinContextMatches,
	}

	if database.DB != nil {
		// Atomic: if any secondary write ever gets added (e.g. seeding
		// sources), it must either all commit or all roll back so the
		// UI never sees a half-created niche.
		txErr := database.DB.Transaction(func(tx *gorm.DB) error {
			return tx.Create(&profile).Error
		})
		if txErr != nil {
			return c.Status(fiber.StatusBadRequest).
				JSON(fiber.Map{"error": txErr.Error()})
		}
	} else {
		profile = store.MemoryNiches.CreateProfile(profile)
	}
	pulse.Broadcast("scrape", fmt.Sprintf("Niche %q created", profile.Name))
	return c.Status(fiber.StatusCreated).JSON(profile)
}

// UpdateNiche handles PATCH /api/niches/:id.
func UpdateNiche(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body nicheBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	apply := func(p *models.NicheProfile) {
		if strings.TrimSpace(body.Name) != "" {
			p.Name = strings.TrimSpace(body.Name)
		}
		// Only overwrite description when the client explicitly sent the
		// field; absent => keep existing value. This is what makes
		// "edit only the threshold" not nuke the description.
		if body.Description != nil {
			p.Description = strings.TrimSpace(*body.Description)
		}
		if body.SeedKeywords != nil {
			p.SeedKeywords = pq.StringArray(cleanKeywordList(body.SeedKeywords))
		}
		if body.ContextKeywords != nil {
			p.ContextKeywords = pq.StringArray(cleanKeywordList(body.ContextKeywords))
		}
		if body.MinContextMatches > 0 {
			p.MinContextMatches = body.MinContextMatches
		}
	}

	if database.DB != nil {
		var p models.NicheProfile
		txErr := database.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.First(&p, "id = ?", id).Error; err != nil {
				return err
			}
			apply(&p)
			return tx.Save(&p).Error
		})
		if txErr != nil {
			if errors.Is(txErr, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": txErr.Error()})
		}
		return c.JSON(p)
	}
	updated := store.MemoryNiches.UpdateProfile(id, apply)
	if updated == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(updated)
}

// DeleteNiche handles DELETE /api/niches/:id.
//
// Cascade: a niche owns three downstream things — NicheSource rows
// (URL links shown in the manager), ScrapeTask rows created as mirrors
// of those sources (so the Health Matrix can pause/resume/restart them),
// and ScrapeTask rows bound via the niche selector on /targets. All
// three are removed so the user doesn't end up with orphan scrapers
// that keep hitting URLs whose niche profile is gone.
func DeleteNiche(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if database.DB != nil {
		txErr := database.DB.Transaction(func(tx *gorm.DB) error {
			// Order matters: drop children first so an FK-safe Postgres
			// schema would also accept this even if we later add ON
			// DELETE RESTRICT constraints.
			if err := tx.Where("niche_id = ?", id).Delete(&models.ScrapeTask{}).Error; err != nil {
				return err
			}
			if err := tx.Where("niche_id = ?", id).Delete(&models.NicheSource{}).Error; err != nil {
				return err
			}
			res := tx.Delete(&models.NicheProfile{}, "id = ?", id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
		if txErr != nil {
			if errors.Is(txErr, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": txErr.Error()})
		}
		pulse.Broadcast("archive", fmt.Sprintf("Niche %s deleted · sources + tasks cleared", id.String()[:6]))
		return c.JSON(fiber.Map{"status": "deleted"})
	}
	if !store.MemoryNiches.DeleteProfile(id) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	// Memory cascade: kill every ScrapeTask bound to this niche so the
	// Health Matrix immediately stops listing them.
	store.MemoryTargets.DeleteByNiche(id)
	pulse.Broadcast("archive", fmt.Sprintf("Niche %s deleted · sources + tasks cleared", id.String()[:6]))
	return c.JSON(fiber.Map{"status": "deleted"})
}

// --- Niche sources ----------------------------------------------------------

// ListNicheSources handles GET /api/niches/:id/sources.
func ListNicheSources(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if database.DB != nil {
		var rows []models.NicheSource
		if err := database.DB.Where("niche_id = ?", id).Order("created_at desc").Find(&rows).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(rows)
	}
	return c.JSON(store.MemoryNiches.ListSources(id))
}

// AddNicheSource handles POST /api/niches/:id/sources. We also persist a
// matching ScrapeTask so the existing dispatcher / health matrix keep
// treating these URLs the same as user-added Custom Links.
func AddNicheSource(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	body.URL = normaliseScrapeURL(body.URL)
	if body.URL == "" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "url must be http(s)"})
	}
	src := models.NicheSource{
		NicheID: id,
		URL:     body.URL,
		Label:   strings.TrimSpace(body.Label),
	}
	// Mirror as a ScrapeTask so the global Refresh + Health Matrix pick it up.
	task := models.ScrapeTask{
		Type:     models.TaskTypeDirectURL,
		Value:    body.URL,
		Status:   models.StatusPending,
		IsActive: true,
		NicheID:  &id,
	}
	if database.DB != nil {
		// Make sure the niche actually exists.
		var p models.NicheProfile
		if err := database.DB.First(&p, "id = ?", id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "niche not found"})
		}
		if err := database.DB.Create(&src).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		_ = database.DB.Create(&task).Error
	} else {
		if store.MemoryNiches.GetProfile(id) == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "niche not found"})
		}
		src = store.MemoryNiches.CreateSource(src)
		task = store.MemoryTargets.Create(task)
	}
	// Trigger an immediate first scrape.
	scraper.Default.TriggerTask(&task)
	pulse.Broadcast("scrape", fmt.Sprintf("New niche source · %s", hostnameOf(body.URL)))
	return c.Status(fiber.StatusCreated).JSON(src)
}

// DeleteNicheSource handles DELETE /api/niches/:id/sources/:sourceId.
//
// AddNicheSource writes two rows for every URL: a NicheSource (managed
// from the /niches page) plus a mirror ScrapeTask (managed from the
// Health Matrix). Without this cascade, deleting the source from /niches
// would leave the mirror task scraping the URL forever and showing up
// in the Health Matrix as a phantom "FAILED · 100% err rate" pipeline.
//
// We match the mirror task by (niche_id, type=DIRECT_URL, value=url) —
// since we don't store an explicit FK between NicheSource and
// ScrapeTask. Multiple mirror tasks would all be removed if anyone ever
// created duplicates via curl.
func DeleteNicheSource(c *fiber.Ctx) error {
	sourceID, err := uuid.Parse(c.Params("sourceId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if database.DB != nil {
		// Look up the source first so we know the (nicheID, URL) tuple
		// for the cascade. We do this inside the transaction so a
		// concurrent admin couldn't, say, swap the URL between read
		// and delete.
		txErr := database.DB.Transaction(func(tx *gorm.DB) error {
			var src models.NicheSource
			if err := tx.First(&src, "id = ?", sourceID).Error; err != nil {
				return err
			}
			if err := tx.Where(
				"niche_id = ? AND type = ? AND value = ?",
				src.NicheID, models.TaskTypeDirectURL, src.URL,
			).Delete(&models.ScrapeTask{}).Error; err != nil {
				return err
			}
			return tx.Delete(&models.NicheSource{}, "id = ?", sourceID).Error
		})
		if txErr != nil {
			if errors.Is(txErr, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": txErr.Error()})
		}
		pulse.Broadcast("archive", "Niche source removed · mirror scrape task cleared")
		return c.JSON(fiber.Map{"status": "deleted"})
	}
	// In-memory path: snapshot the source so we know its URL before we
	// drop it, then cascade into the targets store.
	src := store.MemoryNiches.GetSource(sourceID)
	if src == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	store.MemoryTargets.DeleteByNicheAndURL(src.NicheID, src.URL)
	if !store.MemoryNiches.DeleteSource(sourceID) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	pulse.Broadcast("archive", "Niche source removed · mirror scrape task cleared")
	return c.JSON(fiber.Map{"status": "deleted"})
}

// RunNiche handles POST /api/niches/:id/run — the "Run" button on the
// Niche Manager. Dispatches every SeedKeyword + bound target + NicheSource
// in a single batch and returns the count the UI can display as a toast.
func RunNiche(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	ok, n := scraper.Default.TriggerNiche(id)
	if !ok {
		return c.Status(fiber.StatusConflict).
			JSON(fiber.Map{"error": "scraper busy, globally paused, or niche has no targets"})
	}
	return c.Status(fiber.StatusAccepted).
		JSON(fiber.Map{"status": "dispatched", "targets": n})
}

// --- helpers ---------------------------------------------------------------

func cleanKeywordList(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func hostnameOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return raw
}

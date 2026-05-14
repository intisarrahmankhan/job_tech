package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"jobscout/database"
	"jobscout/dedup"
	"jobscout/models"
	"jobscout/pulse"
	"jobscout/scraper"
	"jobscout/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TargetPayload is the JSON body accepted by POST /api/targets. The Type
// determines how Value is validated (KEYWORD trims, DIRECT_URL parses).
type TargetPayload struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Frequency string `json:"frequency,omitempty"` // optional, default "6h"
	NicheID   string `json:"nicheId,omitempty"`   // optional FK
}

var allowedFrequencies = map[string]bool{
	"hourly": true,
	"6h":     true,
	"daily":  true,
}

// ListTargets returns every user-defined scrape target ordered newest-first.
// Falls back to the in-memory store when Postgres isn't connected so the
// Targeting Dashboard stays usable on dev machines.
func ListTargets(c *fiber.Ctx) error {
	if database.DB == nil {
		return c.JSON(store.MemoryTargets.List())
	}
	var rows []models.ScrapeTask
	if err := database.DB.Order("created_at desc").Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rows)
}

// CreateTarget validates + inserts a new ScrapeTask, then immediately fires
// the first scrape and broadcasts the action on the pulse channel.
func CreateTarget(c *fiber.Ctx) error {
	var p TargetPayload
	if err := c.BodyParser(&p); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid payload: " + err.Error()})
	}

	taskType := models.ScrapeTaskType(strings.ToUpper(strings.TrimSpace(p.Type)))
	value := dedup.Clean(p.Value)
	switch taskType {
	case models.TaskTypeKeyword:
		if value == "" {
			return c.Status(fiber.StatusBadRequest).
				JSON(fiber.Map{"error": "keyword cannot be empty"})
		}
	case models.TaskTypeDirectURL:
		fixed := normaliseScrapeURL(value)
		if fixed == "" {
			return c.Status(fiber.StatusBadRequest).
				JSON(fiber.Map{"error": "value must be a valid http(s) URL"})
		}
		value = fixed
	default:
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "type must be KEYWORD or DIRECT_URL"})
	}

	frequency := strings.TrimSpace(p.Frequency)
	if frequency == "" {
		frequency = "6h"
	}
	if !allowedFrequencies[frequency] {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "frequency must be hourly|6h|daily"})
	}

	task := models.ScrapeTask{
		Type:      taskType,
		Value:     value,
		Frequency: frequency,
		Status:    models.StatusPending,
		IsActive:  true,
	}
	if strings.TrimSpace(p.NicheID) != "" {
		if nid, err := uuid.Parse(strings.TrimSpace(p.NicheID)); err == nil {
			task.NicheID = &nid
		}
	}
	if database.DB != nil {
		if err := database.DB.Create(&task).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).
				JSON(fiber.Map{"error": "insert failed: " + err.Error()})
		}
	} else {
		// DB-less fallback: keep the target in the in-memory store so the
		// Targeting Dashboard still works on dev machines.
		task = store.MemoryTargets.Create(task)
	}

	// Real-time pulse line + immediate first scrape (best effort —
	// concurrency limits in ScraperManager may defer it).
	pulse.Broadcast("scrape", fmt.Sprintf(
		"New Target Added: Scraping %q now…", task.Value,
	))
	scraper.Default.TriggerTask(&task)

	return c.Status(fiber.StatusCreated).JSON(task)
}

// RunTarget re-triggers a single ScrapeTask. Used by the RESTART button on
// the Scraper Health Matrix and the manual run button on the Targeting
// Dashboard.
func RunTarget(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid id"})
	}

	var task *models.ScrapeTask
	if database.DB != nil {
		var t models.ScrapeTask
		if err := database.DB.First(&t, "id = ?", id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).
				JSON(fiber.Map{"error": "target not found"})
		}
		task = &t
	} else {
		task = store.MemoryTargets.Get(id)
		if task == nil {
			return c.Status(fiber.StatusNotFound).
				JSON(fiber.Map{"error": "target not found"})
		}
	}

	// TriggerTask can refuse for three distinct reasons; report each
	// precisely so the UI can show a meaningful toast instead of a
	// generic "already in progress" (which used to be the only message
	// even when the global Pause All flag was the actual cause).
	if scraper.Default.IsPaused() {
		return c.Status(fiber.StatusConflict).
			JSON(fiber.Map{"error": "scraping is globally paused — resume via START ALL first", "reason": "globally-paused"})
	}
	if !task.IsActive {
		return c.Status(fiber.StatusConflict).
			JSON(fiber.Map{"error": "this target is paused — resume it before running", "reason": "target-paused"})
	}
	if !scraper.Default.TriggerTask(task) {
		return c.Status(fiber.StatusConflict).
			JSON(fiber.Map{"error": "another scraper run is already in progress", "reason": "busy"})
	}
	pulse.Broadcast("scrape", fmt.Sprintf("Re-running target %q", task.Value))
	return c.JSON(fiber.Map{"status": "queued", "id": task.ID.String()})
}

// SetTargetActive toggles a single target's IsActive flag (Pause/Resume on
// the Scraper Health Matrix). Path: POST /api/targets/:id/(pause|resume).
// The action is taken from the `action` URL param.
func SetTargetActive(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid id"})
	}
	// Derive the action from the path suffix since the route uses literal
	// segments (/pause | /resume) instead of a regex param.
	action := "pause"
	if strings.HasSuffix(c.Path(), "/resume") {
		action = "resume"
	}
	active := action == "resume"

	if database.DB != nil {
		res := database.DB.Model(&models.ScrapeTask{}).
			Where("id = ?", id).
			Update("is_active", active)
		if res.RowsAffected == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
		if res.Error != nil {
			return c.Status(fiber.StatusInternalServerError).
				JSON(fiber.Map{"error": res.Error.Error()})
		}
	} else {
		if !store.MemoryTargets.SetActive(id, active) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
	}

	pulse.Broadcast("scrape", fmt.Sprintf("Target %s · %sd", id.String()[:6], action))
	return c.JSON(fiber.Map{"status": action + "d", "id": id.String(), "isActive": active})
}

// DeleteTarget removes a target by ID. Operates against the in-memory
// store when the database is unavailable.
func DeleteTarget(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid id"})
	}
	if database.DB == nil {
		if !store.MemoryTargets.Delete(id) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
		return c.JSON(fiber.Map{"status": "deleted", "id": id.String()})
	}
	res := database.DB.Delete(&models.ScrapeTask{}, "id = ?", id)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) || res.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if res.Error != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": res.Error.Error()})
	}
	return c.JSON(fiber.Map{"status": "deleted", "id": id.String()})
}

// isValidURL returns true for http(s) URLs with a host. Validation runs
// here in addition to the frontend so the API contract stays honest even
// if called by curl / external scripts.
func isValidURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// normaliseScrapeURL trims, prepends https:// if no scheme is present,
// then re-validates. Returns the cleaned URL or "" if it still isn't a
// valid http(s) URL after the fix-up.
//
// Why: users routinely paste hostnames like "linkedin.com/jobs" without
// the scheme. The scraper would then fail with "Cannot navigate to
// invalid URL" at runtime. We canonicalise at create time so the DB
// only ever stores well-formed URLs.
func normaliseScrapeURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		s = "https://" + s
	}
	if !isValidURL(s) {
		return ""
	}
	return s
}

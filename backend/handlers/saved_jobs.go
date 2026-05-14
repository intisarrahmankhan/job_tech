package handlers

import (
	"errors"
	"strings"
	"sync"

	"jobscout/database"
	"jobscout/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SavedJobs persist bookmarks so they survive a browser cache clear. The
// app has no real auth; the client sends a stable X-Scout-User header
// (profile email when signed in, else a localStorage UUID). That value
// is treated as an opaque identifier — we never email it, look it up,
// or expose it. It's a "bring-your-own-id" scheme that keeps bookmarks
// sticky across devices if the user logs in with the same email.

const userKeyHeader = "X-Scout-User"

func requireUserKey(c *fiber.Ctx) (string, error) {
	key := strings.TrimSpace(c.Get(userKeyHeader))
	if key == "" {
		return "", errors.New("missing X-Scout-User header")
	}
	if len(key) > 200 {
		key = key[:200]
	}
	return key, nil
}

// ListSavedJobs returns the saved-job ids for the calling user.
// GET /api/saved-jobs
func ListSavedJobs(c *fiber.Ctx) error {
	key, err := requireUserKey(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if database.DB != nil {
		var rows []models.SavedJob
		if err := database.DB.Where("user_key = ?", key).Find(&rows).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(rows)
	}
	return c.JSON(memorySaved.list(key))
}

// SaveJobPayload accepts either the full UUID in `jobId` or the display
// short code in `displayId` (e.g. "JOB-7F3A21") so the frontend can send
// whichever it has handy.
type SaveJobPayload struct {
	JobID     string `json:"jobId"`
	DisplayID string `json:"displayId"`
}

// SaveJob persists a bookmark.
// POST /api/saved-jobs  { jobId | displayId }
func SaveJob(c *fiber.Ctx) error {
	key, err := requireUserKey(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	var p SaveJobPayload
	if err := c.BodyParser(&p); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	jobID, resolveErr := resolveJobID(p.JobID, p.DisplayID)
	if resolveErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": resolveErr.Error()})
	}

	if database.DB != nil {
		saved := models.SavedJob{UserKey: key, JobID: jobID}
		// Upsert: the unique index on (user_key, job_id) turns re-saves
		// into a no-op so double-clicks don't explode with 500s.
		err := database.DB.
			Where("user_key = ? AND job_id = ?", key, jobID).
			FirstOrCreate(&saved).Error
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(saved)
	}
	row := memorySaved.add(key, jobID)
	return c.Status(fiber.StatusCreated).JSON(row)
}

// UnsaveJob removes a bookmark.
// DELETE /api/saved-jobs/:jobId
func UnsaveJob(c *fiber.Ctx) error {
	key, err := requireUserKey(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	jobID, resolveErr := resolveJobID(c.Params("jobId"), "")
	if resolveErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": resolveErr.Error()})
	}
	if database.DB != nil {
		if err := database.DB.Where("user_key = ? AND job_id = ?", key, jobID).
			Delete(&models.SavedJob{}).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "deleted"})
	}
	memorySaved.remove(key, jobID)
	return c.JSON(fiber.Map{"status": "deleted"})
}

// resolveJobID accepts either a plain UUID or the display short code
// ("JOB-7F3A21") and returns the full UUID. The short code holds the
// first 6 hex chars of the UUID, so we do a prefix match against jobs.
func resolveJobID(rawUUID, displayID string) (uuid.UUID, error) {
	if s := strings.TrimSpace(rawUUID); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			return id, nil
		}
		// Allow the display form in the `jobId` field too.
		displayID = s
	}
	d := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(displayID), "JOB-"))
	if d == "" {
		return uuid.Nil, errors.New("jobId or displayId is required")
	}
	// Look the UUID up by its leading 6 chars.
	if database.DB != nil {
		var job models.Job
		if err := database.DB.Select("id").
			Where("lower(id::text) LIKE ?", strings.ToLower(d)+"%").
			First(&job).Error; err == nil {
			return job.ID, nil
		}
		if errors.Is(database.DB.Error, gorm.ErrRecordNotFound) {
			return uuid.Nil, errors.New("job not found")
		}
	}
	// Fall back to parsing `d` as a full UUID if the client sent it with
	// a "JOB-" prefix by mistake.
	if id, err := uuid.Parse(d); err == nil {
		return id, nil
	}
	return uuid.Nil, errors.New("job not found for display id " + displayID)
}

// --- in-memory fallback ----------------------------------------------------

type memSavedStore struct {
	mu sync.RWMutex
	// user_key -> set(job_id)
	rows map[string]map[uuid.UUID]models.SavedJob
}

func (s *memSavedStore) list(key string) []models.SavedJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.SavedJob, 0, len(s.rows[key]))
	for _, v := range s.rows[key] {
		out = append(out, v)
	}
	return out
}

func (s *memSavedStore) add(key string, jobID uuid.UUID) models.SavedJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows == nil {
		s.rows = map[string]map[uuid.UUID]models.SavedJob{}
	}
	if s.rows[key] == nil {
		s.rows[key] = map[uuid.UUID]models.SavedJob{}
	}
	if existing, ok := s.rows[key][jobID]; ok {
		return existing
	}
	row := models.SavedJob{ID: uuid.New(), UserKey: key, JobID: jobID}
	s.rows[key][jobID] = row
	return row
}

func (s *memSavedStore) remove(key string, jobID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows[key], jobID)
}

var memorySaved = &memSavedStore{}

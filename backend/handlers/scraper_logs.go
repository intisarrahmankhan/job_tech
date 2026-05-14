package handlers

import (
	"strconv"

	"jobscout/database"
	"jobscout/models"
	"jobscout/scraper"

	"github.com/gofiber/fiber/v2"
)

// ListScraperLogs returns the most recent scraper failure entries so the
// Health Matrix can render a "last error" panel. DB-less mode returns an
// empty array (logs are only persisted with Postgres connected).
//
// Query params:
//   - limit  int, default 50, max 200
//   - taskId string, restricts to a single task's logs
func ListScraperLogs(c *fiber.Ctx) error {
	if database.DB == nil {
		return c.JSON([]models.ScraperLog{})
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 200 {
				v = 200
			}
			limit = v
		}
	}
	q := database.DB.Order("created_at DESC").Limit(limit)
	if taskID := c.Query("taskId"); taskID != "" {
		q = q.Where("task_id = ?", taskID)
	}
	var rows []models.ScraperLog
	if err := q.Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "scraper_logs query failed: " + err.Error()})
	}
	return c.JSON(rows)
}

// KillScraper forcibly terminates the in-flight scraper subprocess. Used
// by the Health Matrix Restart button when a previous run hung past its
// context deadline. Returns 200 with `{killed:true|false}`.
func KillScraper(c *fiber.Ctx) error {
	killed := scraper.Default.Kill()
	return c.JSON(fiber.Map{"killed": killed})
}

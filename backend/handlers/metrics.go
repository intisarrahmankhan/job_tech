package handlers

import (
	"strconv"
	"time"

	"jobscout/metrics"

	"github.com/gofiber/fiber/v2"
)

// ScraperMetrics exposes the Health Matrix rollup at GET /api/scraper/metrics.
// Query `since=30m` / `1h` / `24h` (default: 24h) controls the window; 0 = all time.
// Response shape: { taskId -> { total, success, failure, rejected, errRate } }.
func ScraperMetrics(c *fiber.Ctx) error {
	since := parseSince(c.Query("since", "24h"))
	return c.JSON(fiber.Map{
		"byTask":  metrics.RollupByTask(since),
		"byNiche": metrics.RollupByNiche(since),
		"since":   since.String(),
	})
}

// parseSince accepts Go duration strings ("30m", "1h", "24h"). "0" or
// empty string means all-time. Invalid values fall back to 24h.
func parseSince(raw string) time.Duration {
	if raw == "" {
		return 24 * time.Hour
	}
	if raw == "0" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		// Bare number = hours, for convenience.
		return time.Duration(n) * time.Hour
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return 24 * time.Hour
}

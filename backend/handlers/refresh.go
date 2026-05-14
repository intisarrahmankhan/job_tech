package handlers

import (
	"strings"

	"jobscout/pulse"
	"jobscout/scraper"

	"github.com/gofiber/fiber/v2"
)

// RefreshScraper is mounted at POST /api/refresh. It triggers a one-shot
// scraper run via the singleton ScraperManager and returns 202 Accepted.
// If a run is already in progress, returns 409 Conflict.
func RefreshScraper(c *fiber.Ctx) error {
	if !scraper.Default.Trigger() {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"status":  "already-running",
			"message": "scraper is already running",
		})
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":  "started",
		"message": "scraper run queued",
	})
}

// ScraperStatus is mounted at GET /api/refresh/status — small probe used
// by the scraper-health page and useful for debugging.
func ScraperStatus(c *fiber.Ctx) error {
	return c.JSON(scraper.Default.Status())
}

// SetScraperPaused flips the global pause flag on the ScraperManager.
// Path: POST /api/scraper/(pause|resume).
//
// Resume also dispatches a one-shot batch run so "Start All" actually
// starts scraping (not merely "permits future scraping"). If a run is
// already in flight or there are no active targets, Trigger returns
// false and we just emit the resume pulse — no error to the client.
func SetScraperPaused(c *fiber.Ctx) error {
	pause := !strings.HasSuffix(c.Path(), "/resume")
	action := "pause"
	if !pause {
		action = "resume"
	}
	scraper.Default.SetPaused(pause)

	dispatched := false
	if pause {
		pulse.Broadcast("alert", "Scraping paused globally")
	} else {
		pulse.Broadcast("scrape", "Scraping resumed globally · dispatching batch run")
		dispatched = scraper.Default.Trigger()
	}
	return c.JSON(fiber.Map{
		"status":     action + "d",
		"paused":     pause,
		"dispatched": dispatched,
	})
}

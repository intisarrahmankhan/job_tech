package routes

import (
	"jobscout/handlers"

	"github.com/gofiber/fiber/v2"
)

// Register wires every HTTP route onto the provided Fiber app.
func Register(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/jobs", handlers.ListJobs)
	api.Post("/refresh", handlers.RefreshScraper)
	api.Get("/refresh/status", handlers.ScraperStatus)

	// User-defined scraping targets (the Targeting Dashboard).
	api.Get("/targets", handlers.ListTargets)
	api.Post("/targets", handlers.CreateTarget)
	api.Post("/targets/:id/run", handlers.RunTarget)
	api.Post("/targets/:id/pause", handlers.SetTargetActive)
	api.Post("/targets/:id/resume", handlers.SetTargetActive)
	api.Delete("/targets/:id", handlers.DeleteTarget)

	// Global scraper pause/resume kill-switch (Scraper Health Matrix).
	api.Post("/scraper/pause", handlers.SetScraperPaused)
	api.Post("/scraper/resume", handlers.SetScraperPaused)
	api.Post("/scraper/kill", handlers.KillScraper)

	// Per-target + per-niche telemetry rollup (ERR RATE column etc.).
	api.Get("/scraper/metrics", handlers.ScraperMetrics)
	api.Get("/scraper/logs", handlers.ListScraperLogs)

	// Admin · dry-run scraper validator (no DB writes).
	api.Post("/admin/scrapers/test", handlers.TestScraper)

	// Saved / bookmarked jobs (persisted, survives cache clears when the
	// client sends a stable X-Scout-User header).
	api.Get("/saved-jobs", handlers.ListSavedJobs)
	api.Post("/saved-jobs", handlers.SaveJob)
	api.Delete("/saved-jobs/:jobId", handlers.UnsaveJob)

	// Niche profiles + sources (Niche Manager page).
	api.Get("/niches", handlers.ListNiches)
	api.Post("/niches", handlers.CreateNiche)
	api.Patch("/niches/:id", handlers.UpdateNiche)
	api.Delete("/niches/:id", handlers.DeleteNiche)
	api.Post("/niches/:id/run", handlers.RunNiche)
	api.Get("/niches/:id/sources", handlers.ListNicheSources)
	api.Post("/niches/:id/sources", handlers.AddNicheSource)
	api.Delete("/niches/:id/sources/:sourceId", handlers.DeleteNicheSource)

	internal := api.Group("/internal", handlers.RequireInternalToken)
	internal.Post("/jobs", handlers.IngestJob)

	// WebSocket pulse stream consumed by the System Pulse sidebar.
	app.Use("/ws", handlers.PulseUpgrader)
	app.Get("/ws/pulse", handlers.WSPulse)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
}

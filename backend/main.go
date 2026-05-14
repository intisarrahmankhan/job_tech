package main

import (
	"log"
	"net/url"
	"strings"

	"jobscout/config"
	"jobscout/database"
	"jobscout/queue"
	"jobscout/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	cfg := config.Load()

	// Best-effort DB connection + auto-migration. API still works without it.
	database.Connect()

	// Start Asynq worker + scheduler. No-op network failures are logged.
	stopQueue := queue.Start()
	defer stopQueue()

	app := fiber.New(fiber.Config{AppName: "JobScout API"})
	app.Use(logger.New())
	// CORS — any explicitly listed origin OR any localhost/127.0.0.1 port
	// (Vite happily picks 5174/5175 when 5173 is taken, so an exact-match
	// allowlist is too brittle during dev).
	explicit := splitAndTrim(cfg.FrontendOrigins)
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			return isAllowedOrigin(origin, explicit)
		},
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Internal-Token",
	}))

	routes.Register(app)

	log.Printf("[api] listening on :%s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("[api] fatal: %v", err)
	}
}

func splitAndTrim(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// isAllowedOrigin returns true for any explicitly-listed origin or any
// localhost/127.0.0.1 origin regardless of port — the latter is what
// keeps `npm run dev` working when Vite drifts to 5174/5175/etc.
func isAllowedOrigin(origin string, explicit []string) bool {
	for _, o := range explicit {
		if o == origin {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]"
}

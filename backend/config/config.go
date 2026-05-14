package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// Config holds every runtime setting. Values come from the process environment,
// optionally seeded from a `.env` file when present (godotenv).
type Config struct {
	Port              string
	DatabaseURL       string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	InternalAPIToken  string
	ScraperCmd        string
	ScraperScript     string
	FrontendOrigins   string
}

var (
	once   sync.Once
	cached *Config
)

// Load reads .env (best-effort) and returns a populated Config.
// Subsequent calls return the cached value.
func Load() *Config {
	once.Do(func() {
		// Look for .env in the current dir; ignore if missing.
		if err := godotenv.Load(); err != nil {
			log.Printf("[config] no .env file loaded (%v) — using process env only", err)
		} else {
			log.Println("[config] .env loaded")
		}

		cached = &Config{
			Port:             envOr("PORT", "8000"),
			DatabaseURL:      envOr("DATABASE_URL", "host=localhost user=postgres password=postgres dbname=jobscout port=5432 sslmode=disable TimeZone=UTC"),
			RedisAddr:        envOr("REDIS_ADDR", "localhost:6379"),
			RedisPassword:    os.Getenv("REDIS_PASSWORD"),
			RedisDB:          envInt("REDIS_DB", 0),
			InternalAPIToken: envOr("INTERNAL_API_TOKEN", "dev-internal-token"),
			ScraperCmd:       envOr("SCRAPER_CMD", "python"),
			ScraperScript:    envOr("SCRAPER_SCRIPT", "../scrapers/scraper.py"),
			FrontendOrigins:  envOr("FRONTEND_ORIGIN", "http://localhost:8080,http://localhost:5173,http://localhost:3000"),
		}
	})
	return cached
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

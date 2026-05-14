package queue

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"jobscout/config"
	"jobscout/database"
	"jobscout/models"
	"jobscout/pulse"
	"jobscout/scraper"

	"github.com/hibiken/asynq"
)

// HandleScraperRun is invoked on the `scraper:run` task. It shells out to the
// Python scraper exactly once per invocation. The scraper itself POSTs each
// extracted job to POST /api/internal/jobs, keeping Go and Python decoupled.
func HandleScraperRun(ctx context.Context, _ *asynq.Task) error {
	// Respect the global Pause All kill-switch — the scheduled job should
	// no-op while scraping is paused, otherwise the user pauses on the UI
	// but the periodic scheduler keeps slamming targets every 6 hours.
	if scraper.Default.IsPaused() {
		log.Printf("[worker] scraper:run skipped — globally paused")
		pulse.Broadcast("alert", "Scheduled scrape skipped — paused globally")
		return nil
	}

	cfg := config.Load()
	log.Printf("[worker] scraper:run · cmd=%q script=%q", cfg.ScraperCmd, cfg.ScraperScript)
	pulse.Broadcast("scrape", "Scraper started")

	cmd := exec.CommandContext(ctx, cfg.ScraperCmd, cfg.ScraperScript)
	cmd.Env = append(cmd.Env,
		"INTERNAL_API_TOKEN="+cfg.InternalAPIToken,
		"JOBSCOUT_API="+fmt.Sprintf("http://localhost:%s", cfg.Port),
	)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		log.Printf("[worker] scraper stdout:\n%s", string(out))
	}
	if err != nil {
		pulse.Broadcast("alert", fmt.Sprintf("Scraper failed: %v", err))
		return fmt.Errorf("scraper exited with error: %w", err)
	}
	log.Printf("[worker] scraper:run · completed")
	pulse.Broadcast("scrape", "Scraper completed")
	return nil
}

// JanitorGracePeriod controls how long after a deadline a job is still
// shown on the active feed. Set via the JANITOR_GRACE env var (Go duration
// string like "72h", "24h", "0"). Defaults to 72 hours so users can still
// see/apply for "just-closed" listings for ~3 days. Set to 0 to deactivate
// the moment the deadline passes.
var JanitorGracePeriod = func() time.Duration {
	if raw := os.Getenv("JANITOR_GRACE"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			return d
		}
	}
	return 72 * time.Hour
}()

// HandleDeadlineJanitor deactivates jobs whose deadline + grace period
// is in the past. Jobs with a NULL deadline are never auto-archived —
// many career pages don't list a closing date and we'd rather leave a
// stale listing visible than silently delete a still-open role.
func HandleDeadlineJanitor(ctx context.Context, _ *asynq.Task) error {
	if database.DB == nil {
		log.Printf("[janitor] skipped — no DB connection")
		return nil
	}
	cutoff := time.Now().UTC().Add(-JanitorGracePeriod)
	res := database.DB.WithContext(ctx).
		Model(&models.Job{}).
		Where("deadline IS NOT NULL AND deadline < ? AND is_active = ?", cutoff, true).
		Update("is_active", false)
	if res.Error != nil {
		pulse.Broadcast("alert", fmt.Sprintf("Janitor failed: %v", res.Error))
		return fmt.Errorf("janitor update failed: %w", res.Error)
	}
	log.Printf("[janitor] deactivated %d expired job(s) (grace=%s)", res.RowsAffected, JanitorGracePeriod)
	if res.RowsAffected > 0 {
		pulse.Broadcast("archive", fmt.Sprintf(
			"Archived %d expired job(s) past %s grace window", res.RowsAffected, JanitorGracePeriod,
		))
	}
	return nil
}

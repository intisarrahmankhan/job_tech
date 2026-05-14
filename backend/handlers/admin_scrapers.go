package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"jobscout/config"
	"jobscout/database"
	"jobscout/models"
	"jobscout/pulse"
	"jobscout/scraper"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// dryRunRequest is the payload accepted by POST /api/admin/scrapers/test.
type dryRunRequest struct {
	URL          string `json:"url"`
	NicheID      string `json:"nicheId"`
	SeedKeywords string `json:"seedKeywords"`
}

// dryRunPassed describes a job that survived the niche-context filter.
type dryRunPassed struct {
	Title      string `json:"title"`
	Company    string `json:"company"`
	URL        string `json:"url"`
	MatchScore int    `json:"matchScore"`
	Threshold  int    `json:"threshold"`
}

// dryRunFailed describes a job rejected by the niche-context filter.
type dryRunFailed struct {
	Title     string   `json:"title"`
	Company   string   `json:"company"`
	URL       string   `json:"url"`
	Hits      int      `json:"hits"`
	Threshold int      `json:"threshold"`
	Missing   []string `json:"missing"`
	Reason    string   `json:"reason"`
}

// dryRunResponse is what the frontend dry-run page renders.
type dryRunResponse struct {
	Status     string         `json:"status"`
	URL        string         `json:"url,omitempty"`
	NicheID    string         `json:"nicheId,omitempty"`
	NicheName  string         `json:"nicheName,omitempty"`
	TotalJobs  int            `json:"totalJobs"`
	Passed     []dryRunPassed `json:"passed"`
	Failed     []dryRunFailed `json:"failed"`
	Stderr     string         `json:"stderr,omitempty"`
	DurationMs int64          `json:"durationMs"`
	ExitCode   int            `json:"exitCode"`
}

// TestScraper handles POST /api/admin/scrapers/test.
//
// Lifecycle:
//  1. Spawn scraper.py in --dry-run mode with the supplied URL + niche.
//  2. Capture stdout (strict JSON array) and stderr (structured events)
//     in separate buffers; abort after 90 seconds via context.
//  3. Validate every parsed job against the niche's ContextKeywords.
//  4. Return a structured response so the UI can render passed/failed
//     tables side by side. NOTHING is persisted to PostgreSQL.
func TestScraper(c *fiber.Ctx) error {
	var req dryRunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid payload: " + err.Error()})
	}
	req.URL = strings.TrimSpace(req.URL)
	req.NicheID = strings.TrimSpace(req.NicheID)
	req.SeedKeywords = strings.TrimSpace(req.SeedKeywords)
	if req.URL == "" && req.SeedKeywords == "" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "url or seedKeywords is required"})
	}
	if req.NicheID == "" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "nicheId is required"})
	}
	if _, err := uuid.Parse(req.NicheID); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "nicheId must be a UUID"})
	}

	cfg := config.Load()

	// Resolve the script to an absolute path before spawning. If the
	// file is missing we return 502 with a meaningful error so the
	// dry-run UI shows "scraper script not found at /abs/path" instead
	// of the cryptic "exit status 2" Python emits in that case.
	scriptPath, workDir, scriptErr := scraper.ResolveScript(cfg)
	if scriptErr != nil {
		log.Printf("[admin/test] %v", scriptErr)
		return c.Status(fiber.StatusInternalServerError).JSON(dryRunResponse{
			Status:   "failed",
			URL:      req.URL,
			NicheID:  req.NicheID,
			Stderr:   scriptErr.Error(),
			ExitCode: -1,
		})
	}

	pulse.Broadcast("scrape", fmt.Sprintf("Dry-run · spawning scraper for %s", truncate(req.URL, 60)))
	log.Printf("[admin/test] url=%q nicheId=%s seed=%q", req.URL, req.NicheID, req.SeedKeywords)

	ctx, cancel := context.WithTimeout(c.UserContext(), 90*time.Second)
	defer cancel()

	argv := []string{scriptPath, "--dry-run", "--niche-id", req.NicheID, "--detail-pages"}
	if req.URL != "" {
		argv = append(argv, "--url", req.URL)
	}
	if req.SeedKeywords != "" {
		argv = append(argv, "--seed-keywords", req.SeedKeywords)
	}

	cmd := exec.CommandContext(ctx, cfg.ScraperCmd, argv...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"INTERNAL_API_TOKEN="+cfg.InternalAPIToken,
		"JOBSCOUT_API="+fmt.Sprintf("http://localhost:%s", cfg.Port),
	)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	stderrText := stderrBuf.String()
	stdoutText := stdoutBuf.String()
	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else if errors.Is(err, context.DeadlineExceeded) {
			exitCode = 124 // GNU timeout convention
		} else {
			exitCode = -1
		}

		// Persist the failure so the Health Matrix has something to
		// render. We deliberately keep this 'best-effort' — never let a
		// log-write failure mask the original error from the user.
		if database.DB != nil {
			nid, _ := uuid.Parse(req.NicheID)
			entry := &models.ScraperLog{
				URL:       req.URL,
				NicheID:   &nid,
				Stage:     "dry-run",
				Error:     truncate(err.Error(), 2000),
				Details:   truncate(stderrText, 16*1024),
				ExitCode:  exitCode,
				CreatedAt: time.Now().UTC(),
			}
			if dbErr := database.DB.Create(entry).Error; dbErr != nil {
				log.Printf("[admin/test] log persist failed: %v", dbErr)
			}
		}

		pulse.Broadcast("alert", fmt.Sprintf(
			"Dry-run failed (exit %d): %s",
			exitCode, summariseStderr(stderrText, err),
		))
		return c.Status(fiber.StatusBadGateway).JSON(dryRunResponse{
			Status:     "failed",
			URL:        req.URL,
			NicheID:    req.NicheID,
			Stderr:     truncate(stderrText, 16*1024),
			DurationMs: duration.Milliseconds(),
			ExitCode:   exitCode,
		})
	}

	// Parse strict JSON array from stdout. scraper.py emits exactly one
	// JSON array per run on its own line. We scan stdout bottom-up,
	// trying to JSON-parse each line that begins with `[` and ends
	// with `]`; this avoids the trap of `strings.LastIndex("[")`
	// landing inside a nested `sources` array within the payload.
	var jobs []IngestPayload
	parsed := false
	stdoutTrim := strings.TrimSpace(stdoutText)
	if stdoutTrim != "" {
		lines := strings.Split(stdoutTrim, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
				continue
			}
			if err := json.Unmarshal([]byte(line), &jobs); err == nil {
				parsed = true
				break
			}
		}
		if !parsed {
			log.Printf("[admin/test] stdout JSON parse failed; first 300 bytes: %s",
				truncate(stdoutTrim, 300))
			return c.Status(fiber.StatusBadGateway).JSON(dryRunResponse{
				Status:     "failed",
				URL:        req.URL,
				NicheID:    req.NicheID,
				Stderr:     truncate(stderrText, 16*1024),
				DurationMs: duration.Milliseconds(),
				ExitCode:   0,
			})
		}
	}

	pulse.Broadcast("scrape", fmt.Sprintf(
		"Dry-run · scraper produced %d candidates · validating", len(jobs),
	))

	passed := make([]dryRunPassed, 0, len(jobs))
	failed := make([]dryRunFailed, 0, len(jobs))
	var nicheName string

	for i := range jobs {
		j := &jobs[i]
		if j.NicheID == "" {
			j.NicheID = req.NicheID
		}
		niche, ok, hits, missing := validateNiche(j)
		if niche != nil {
			nicheName = niche.Name
		}
		threshold := 0
		if niche != nil {
			threshold = niche.MinContextMatches
			if threshold <= 0 {
				threshold = 2
			}
		}
		if !ok {
			failed = append(failed, dryRunFailed{
				Title:     j.Title,
				Company:   j.Company,
				URL:       j.URL,
				Hits:      hits,
				Threshold: threshold,
				Missing:   missing,
				Reason:    fmt.Sprintf("only %d of %d required keywords matched", hits, threshold),
			})
			continue
		}
		passed = append(passed, dryRunPassed{
			Title:      j.Title,
			Company:    j.Company,
			URL:        j.URL,
			MatchScore: hits,
			Threshold:  threshold,
		})
	}

	pulse.Broadcast("scrape", fmt.Sprintf(
		"Dry-run · complete · %d/%d passed", len(passed), len(jobs),
	))
	return c.JSON(dryRunResponse{
		Status:     "completed",
		URL:        req.URL,
		NicheID:    req.NicheID,
		NicheName:  nicheName,
		TotalJobs:  len(jobs),
		Passed:     passed,
		Failed:     failed,
		Stderr:     truncate(stderrText, 16*1024),
		DurationMs: duration.Milliseconds(),
		ExitCode:   exitCode,
	})
}

// summariseStderr extracts the most actionable line from a structured
// stderr buffer so the pulse alert says "Timeout waiting for selector"
// instead of "exit status 1".
func summariseStderr(stderr string, fallback error) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		if fallback != nil {
			return fallback.Error()
		}
		return ""
	}
	lines := strings.Split(stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var ev map[string]any
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				if e, ok := ev["error"].(string); ok && e != "" {
					return e
				}
			}
		}
		return line
	}
	if fallback != nil {
		return fallback.Error()
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

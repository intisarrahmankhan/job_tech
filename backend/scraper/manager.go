// Package scraper exposes a small ScraperManager that knows how to invoke
// the Python scrapers in /scrapers via os/exec. Used by the manual
// "Refresh" endpoint as well as the periodic Asynq schedule.
//
// Design notes (post-RCA, 2026-05):
//   - The Go side ALWAYS spawns the Python entry point with explicit CLI
//     flags (`--url`, `--seed-keywords`, `--niche-id`, `--task-id`).
//     Environment variables only carry secrets (INTERNAL_API_TOKEN) and
//     the backend URL (JOBSCOUT_API). This keeps the Python contract
//     analyzable: `python scraper.py --help` documents every input.
//   - We capture stdout into a buffer so the admin/test endpoint can
//     parse the strict-JSON-array output. Stderr is split: every line
//     is streamed to the Go logger AND accumulated into a 16 KB ring so
//     the failure-path can persist it to the scraper_logs table.
//   - The currently-running cmd is exposed via PID() so the Health
//     Matrix Restart button can forcibly kill a stuck subprocess
//     before launching a fresh one.
package scraper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"jobscout/config"
	"jobscout/database"
	"jobscout/metrics"
	"jobscout/models"
	"jobscout/pulse"
	"jobscout/store"

	"github.com/google/uuid"
)

// Target is a minimal description of what to scrape. Bridges the DB-backed
// ScrapeTask record and the parameters passed to the Python subprocess.
type Target struct {
	ID      string
	Type    string // KEYWORD | DIRECT_URL
	Value   string
	NicheID string // optional — links the run to a NicheProfile
}

// Manager coordinates manual scraper triggers. A single in-flight run is
// allowed at any time; concurrent calls return ErrAlreadyRunning to prevent
// the user from spawning multiple scrapers by spamming the refresh button.
//
// The `paused` flag is the global kill-switch consumed by the Pause/Start
// buttons in the Scraper Health Matrix; when set, every Trigger call is
// rejected and any periodic schedule no-ops.
type Manager struct {
	running atomic.Bool
	paused  atomic.Bool
	mu      sync.Mutex
	lastRun time.Time
	lastErr error
	lastPID int

	// cancelMu guards cancelFn + activeCmd, the cancel hook for the
	// active subprocess context plus a reference to the Cmd itself.
	// We store these so SetPaused(true) can yank an in-flight Python
	// run instead of letting it complete, and so Kill() can target the
	// PID directly when context cancellation is too gentle (e.g. Python
	// stuck in a Playwright wait_for_selector loop).
	cancelMu  sync.Mutex
	cancelFn  context.CancelFunc
	activeCmd *exec.Cmd

	// Single-use sentinel: once the manager has confirmed Playwright
	// Chromium is installed, we skip the per-run preflight.
	playwrightReady atomic.Bool
}

// SetPaused toggles the global pause flag. Returns the previous value.
// When pausing, also cancels any in-flight scraper subprocess so the
// user's "Pause All" click stops scraping *now* rather than after the
// current target finishes.
func (m *Manager) SetPaused(p bool) bool {
	prev := m.paused.Swap(p)
	if p {
		m.cancelMu.Lock()
		if m.cancelFn != nil {
			m.cancelFn()
		}
		m.cancelMu.Unlock()
	}
	return prev
}

// IsPaused reports whether scraping is globally paused.
func (m *Manager) IsPaused() bool { return m.paused.Load() }

// Kill forcibly terminates the in-flight Python subprocess (if any).
// Returns true if a process was actually killed. Used by the Restart
// button when a previous run hung past its context deadline.
func (m *Manager) Kill() bool {
	m.cancelMu.Lock()
	cmd := m.activeCmd
	cancel := m.cancelFn
	m.cancelMu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	if cancel != nil {
		cancel()
	}
	if err := cmd.Process.Kill(); err != nil {
		log.Printf("[scraper-mgr] kill pid=%d failed: %v", cmd.Process.Pid, err)
		return false
	}
	log.Printf("[scraper-mgr] killed pid=%d on operator request", cmd.Process.Pid)
	pulse.Broadcast("alert", fmt.Sprintf("Scraper PID %d killed by operator", cmd.Process.Pid))
	return true
}

// Singleton — the API is small enough that a package-level instance is fine.
var Default = &Manager{}

// Status is a snapshot of the manager's state, returned by the /api/refresh
// status probe (and useful for the scraper-health UI later).
type Status struct {
	Running bool      `json:"running"`
	Paused  bool      `json:"paused"`
	LastRun time.Time `json:"lastRun"`
	LastErr string    `json:"lastError,omitempty"`
	LastPID int       `json:"lastPid,omitempty"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Status{
		Running: m.running.Load(),
		Paused:  m.paused.Load(),
		LastRun: m.lastRun,
		LastPID: m.lastPID,
	}
	if m.lastErr != nil {
		s.LastErr = m.lastErr.Error()
	}
	return s
}

// Trigger kicks off a manual refresh across **every active user-defined
// target**. It does NOT fall back to the bundled mock fixture — if the
// user hasn't configured anything, the run is a no-op (with a pulse
// notice). This is the contract the homepage and Scraper Health
// "Refresh / Start All" buttons rely on, so phantom example.test jobs
// never enter the feed.
//
// Returns false if a run is already in progress or scraping is paused.
func (m *Manager) Trigger() bool {
	if m.paused.Load() {
		return false
	}

	targets := m.collectActiveTargets()
	if len(targets) == 0 {
		pulse.Broadcast("alert", "No active targets — add a Custom Link on /targets first.")
		return false
	}
	if !m.running.CompareAndSwap(false, true) {
		return false
	}
	go m.runBatch(targets)
	return true
}

// collectActiveTargets pulls the list of DIRECT_URL targets the user has
// configured. KEYWORD targets are intentionally excluded from the global
// Refresh batch — they get fired individually via TriggerTask from the
// Targeting Dashboard.
func (m *Manager) collectActiveTargets() []*Target {
	var out []*Target
	if database.DB != nil {
		var rows []models.ScrapeTask
		if err := database.DB.
			Where("is_active = ? AND type = ?", true, models.TaskTypeDirectURL).
			Find(&rows).Error; err == nil {
			for _, r := range rows {
				out = append(out, &Target{ID: r.ID.String(), Type: string(r.Type), Value: r.Value, NicheID: nilOrUUIDString(r.NicheID)})
			}
		}
		return out
	}
	for _, t := range store.MemoryTargets.List() {
		if t.IsActive && t.Type == models.TaskTypeDirectURL {
			out = append(out, &Target{ID: t.ID.String(), Type: string(t.Type), Value: t.Value, NicheID: nilOrUUIDString(t.NicheID)})
		}
	}
	return out
}

func nilOrUUIDString(p *uuid.UUID) string {
	if p == nil {
		return ""
	}
	return p.String()
}

// TriggerNiche fans out scrapes for a niche:
//
//   - every SeedKeyword becomes a KEYWORD dispatch
//   - every NicheSource (and any ScrapeTask pre-bound via NicheID) becomes a DIRECT_URL dispatch
//
// All resulting targets are scraped sequentially inside one batch so the
// concurrency invariant ("one scraper process at a time") is preserved.
// Returns false if globally paused or another run is already in flight.
func (m *Manager) TriggerNiche(nicheID uuid.UUID) (bool, int) {
	if m.paused.Load() {
		return false, 0
	}
	targets := m.collectNicheTargets(nicheID)
	if len(targets) == 0 {
		pulse.Broadcast("alert",
			"Niche has no targets yet — add seed keywords or links first.")
		return false, 0
	}
	if !m.running.CompareAndSwap(false, true) {
		return false, 0
	}
	pulse.Broadcast("scrape",
		fmt.Sprintf("Dispatching %d target(s) for niche run", len(targets)))
	go m.runBatch(targets)
	return true, len(targets)
}

// collectNicheTargets pulls every scrape input the dispatcher should fire
// for a niche. It merges three sources so the user doesn't need to keep
// them in sync manually:
//
//  1. ScrapeTask rows whose NicheID matches (keyword or URL targets bound via
//     the niche dropdown on /targets).
//  2. NicheSource rows (career-page URLs added on /niches).
//  3. Virtual KEYWORD targets synthesised from the niche's SeedKeywords
//     so the general scrapers (LinkedIn / BDJobs) run for each term.
//
// Dedup invariant: AddNicheSource writes BOTH a NicheSource and a
// mirror ScrapeTask. Iterating the two tables naively would scrape
// every niche URL twice. We track every URL produced by step 1 in a
// set and skip duplicates in step 2.
func (m *Manager) collectNicheTargets(nicheID uuid.UUID) []*Target {
	nid := nicheID.String()
	var out []*Target
	seenURLs := make(map[string]bool)

	if database.DB != nil {
		// 1. Existing tasks bound to this niche.
		var tasks []models.ScrapeTask
		if err := database.DB.Where("niche_id = ? AND is_active = ?", nicheID, true).Find(&tasks).Error; err == nil {
			for _, t := range tasks {
				if t.Type == models.TaskTypeDirectURL {
					seenURLs[t.Value] = true
				}
				out = append(out, &Target{
					ID: t.ID.String(), Type: string(t.Type), Value: t.Value, NicheID: nid,
				})
			}
		}
		// 2. NicheSource rows as virtual DIRECT_URL targets — but only
		//    those NOT already covered by a mirror ScrapeTask above.
		var sources []models.NicheSource
		if err := database.DB.Where("niche_id = ? AND is_active = ?", nicheID, true).Find(&sources).Error; err == nil {
			for _, s := range sources {
				if seenURLs[s.URL] {
					continue
				}
				seenURLs[s.URL] = true
				out = append(out, &Target{
					ID: s.ID.String(), Type: string(models.TaskTypeDirectURL), Value: s.URL, NicheID: nid,
				})
			}
		}
		// 3. Seed keywords as virtual KEYWORD targets.
		var n models.NicheProfile
		if err := database.DB.First(&n, "id = ?", nicheID).Error; err == nil {
			for _, kw := range n.SeedKeywords {
				out = append(out, &Target{
					ID: uuid.New().String(), Type: string(models.TaskTypeKeyword), Value: kw, NicheID: nid,
				})
			}
		}
		return out
	}

	// In-memory fallback — same dedup logic.
	for _, t := range store.MemoryTargets.List() {
		if t.NicheID != nil && *t.NicheID == nicheID && t.IsActive {
			if t.Type == models.TaskTypeDirectURL {
				seenURLs[t.Value] = true
			}
			out = append(out, &Target{
				ID: t.ID.String(), Type: string(t.Type), Value: t.Value, NicheID: nid,
			})
		}
	}
	for _, s := range store.MemoryNiches.ListSources(nicheID) {
		if !s.IsActive {
			continue
		}
		if seenURLs[s.URL] {
			continue
		}
		seenURLs[s.URL] = true
		out = append(out, &Target{
			ID: s.ID.String(), Type: string(models.TaskTypeDirectURL), Value: s.URL, NicheID: nid,
		})
	}
	if profile := store.MemoryNiches.GetProfile(nicheID); profile != nil {
		for _, kw := range profile.SeedKeywords {
			out = append(out, &Target{
				ID: uuid.New().String(), Type: string(models.TaskTypeKeyword), Value: kw, NicheID: nid,
			})
		}
	}
	return out
}

// TriggerTask launches the scraper parameterized for a single user-defined
// ScrapeTask. The DB record's status is flipped to "running" up-front so
// the UI shows the spinner immediately. Returns false if a run is already
// in progress, scraping is globally paused, or the target itself is paused
// (IsActive == false).
func (m *Manager) TriggerTask(task *models.ScrapeTask) bool {
	if task == nil {
		return false
	}
	if m.paused.Load() || !task.IsActive {
		return false
	}
	if !m.running.CompareAndSwap(false, true) {
		return false
	}
	if database.DB != nil {
		database.DB.Model(task).Updates(map[string]any{
			"status":     models.StatusRunning,
			"last_error": "",
		})
	} else {
		store.MemoryTargets.UpdateStatus(task.ID, models.StatusRunning, "", nil)
	}
	tgt := &Target{
		ID:      task.ID.String(),
		Type:    string(task.Type),
		Value:   task.Value,
		NicheID: nilOrUUIDString(task.NicheID),
	}
	go func() {
		defer m.running.Store(false)
		m.run(tgt)
	}()
	return true
}

// runBatch executes a sequence of targets in order, holding the running
// flag across the whole batch so the UI's "running" spinner stays solid.
// Used by the global Refresh button so one click = one synchronous pass
// over every active DIRECT_URL target.
func (m *Manager) runBatch(targets []*Target) {
	defer m.running.Store(false)
	pulse.Broadcast("scrape", fmt.Sprintf("Manual refresh · scraping %d target(s)", len(targets)))
	for _, t := range targets {
		// Honour the global pause flag between targets so a mid-batch
		// "Pause All" click drops the remainder of the queue immediately.
		if m.paused.Load() {
			pulse.Broadcast("alert", "Batch aborted — scraping paused globally")
			log.Printf("[scraper-mgr] batch aborted: globally paused")
			return
		}
		// Mark this target as running for the row spinner in the matrix.
		if tid, err := uuid.Parse(t.ID); err == nil {
			if database.DB != nil {
				database.DB.Model(&models.ScrapeTask{}).
					Where("id = ?", tid).
					Updates(map[string]any{"status": models.StatusRunning, "last_error": ""})
			} else {
				store.MemoryTargets.UpdateStatus(tid, models.StatusRunning, "", nil)
			}
		}
		m.run(t)
	}
	pulse.Broadcast("scrape", "Manual refresh · batch complete")
}

// argvForTarget converts a Target into the CLI flags scraper.py expects.
// Centralised here so the contract Go ↔ Python lives in exactly one place.
//
// Returns an error when the target cannot be turned into a runnable argv
// (unknown Type, empty Value). Bailing out *before* spawning prevents the
// well-known "exit status 2" failure mode where scraper.py runs with no
// `--url` / `--seed-keywords` and silently exits — the Health Matrix
// would then surface a meaningless `exit status 2` instead of the real
// reason. The script path is required to be absolute by ResolveScript
// so Python never returns its own exit-2 (file-not-found).
func argvForTarget(scriptPath string, target *Target) ([]string, error) {
	if target == nil {
		return nil, fmt.Errorf("nil target")
	}
	value := strings.TrimSpace(target.Value)
	if value == "" {
		return nil, fmt.Errorf("target value is empty (Type=%q)", target.Type)
	}
	argv := []string{scriptPath}
	switch strings.ToUpper(strings.TrimSpace(target.Type)) {
	case string(models.TaskTypeDirectURL):
		argv = append(argv, "--url", normaliseURL(value))
	case string(models.TaskTypeKeyword):
		argv = append(argv, "--seed-keywords", value)
	default:
		return nil, fmt.Errorf("unknown target type %q (expected DIRECT_URL or KEYWORD)", target.Type)
	}
	if target.NicheID != "" {
		argv = append(argv, "--niche-id", target.NicheID)
	}
	if target.ID != "" {
		argv = append(argv, "--task-id", target.ID)
	}
	// Detail-page enrichment is always on for live runs because the Niche
	// validator needs the full body text to score ContextKeywords. The
	// dry-run admin endpoint enables it explicitly via its own argv.
	argv = append(argv, "--detail-pages")
	return argv, nil
}

// normaliseURL prepends `https://` if a URL was stored without a scheme.
// We hit this in two flows:
//
//   - users typing `linkedin.com/jobs` in the Targets dashboard;
//   - legacy ScrapeTask rows created before the API validators ran.
//
// Without the scheme Playwright's `page.goto` raises
// `"Cannot navigate to invalid URL"` and the run reports zero jobs.
// Adding the scheme is safe: the API-layer validators still reject
// non-URL inputs at create time.
func normaliseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return s
	}
	return "https://" + s
}

// ResolveScript returns the absolute path to scraper.py + the directory
// to run it from. Exits with a typed error if the file is missing so the
// caller can surface the actual reason in the Health Matrix instead of
// the cryptic `exit status 2` Python emits when it can't open a script.
//
// We honour `cfg.ScraperScript` as supplied (absolute, relative, env
// override) but always anchor it to a stable absolute path before
// passing it to os/exec so the subprocess's cwd never causes the path
// to dangle.
func ResolveScript(cfg *config.Config) (string, string, error) {
	raw := strings.TrimSpace(cfg.ScraperScript)
	if raw == "" {
		return "", "", fmt.Errorf("SCRAPER_SCRIPT is empty")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", "", fmt.Errorf("resolve %q: %w", raw, err)
	}
	if info, statErr := os.Stat(abs); statErr != nil || info.IsDir() {
		return "", "", fmt.Errorf("scraper script not found at %q (set SCRAPER_SCRIPT in backend/.env)", abs)
	}
	return abs, filepath.Dir(abs), nil
}

// EnsurePlaywright runs `python -m playwright install chromium` once per
// process lifetime (best-effort, capped at 5 minutes). Failures are
// logged but not propagated — the actual scraper run will surface the
// canonical "Executable doesn't exist" error if the install really
// failed, which is more actionable than masking it here.
func (m *Manager) EnsurePlaywright(cfg *config.Config) {
	if m.playwrightReady.Load() {
		return
	}
	// Idempotent at the Python layer too: scraper.py keeps a sentinel
	// file next to itself, so this is doubly cheap on the happy path.
	// Use the absolute path so a wrong working dir in the parent Go
	// process doesn't make Python install Chromium for the wrong venv.
	_, scriptDir, err := ResolveScript(cfg)
	if err != nil {
		log.Printf("[scraper-mgr] playwright preflight skipped: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, cfg.ScraperCmd, "-m", "playwright", "install", "chromium")
	cmd.Dir = scriptDir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[scraper-mgr] playwright install failed (%v); details: %s",
			err, strings.TrimSpace(string(out)))
		return
	}
	m.playwrightReady.Store(true)
	log.Printf("[scraper-mgr] playwright chromium ready")
}

// run executes a single Target. The lifecycle pulse events (spawn,
// extract, validate, commit) are emitted by the Python side as JSON
// stderr lines; we forward each one to the System Pulse so the UI
// updates in real time.
func (m *Manager) run(target *Target) {
	cfg := config.Load()
	label := "Manual refresh"
	if target != nil {
		label = fmt.Sprintf("Target %s · %q", target.Type, target.Value)
	}

	// Resolve the script to an absolute path *before* doing anything
	// else: if the file isn't where we expect, Python would exit 2
	// with the cryptic "can't open file" — exactly the regression that
	// drove this rewrite. Fail fast with a useful message.
	scriptPath, workDir, scriptErr := ResolveScript(cfg)
	if scriptErr != nil {
		m.recordCrash(target, "spawn", scriptErr, "")
		m.finish(target, scriptErr)
		return
	}

	// Build argv up front so we can catch malformed targets (empty Value
	// or unknown Type) BEFORE wasting a subprocess on them.
	argv, argvErr := argvForTarget(scriptPath, target)
	if argvErr != nil {
		m.recordCrash(target, "spawn", argvErr, "")
		m.finish(target, argvErr)
		return
	}

	pulse.Broadcast("scrape", label+" · scraper spawned")
	log.Printf("[scraper-mgr] launching %s %v (%s)", cfg.ScraperCmd, argv, label)

	// One-shot Playwright preflight — saves the cryptic "Executable
	// doesn't exist" error on fresh checkouts.
	m.EnsurePlaywright(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.cancelMu.Lock()
	m.cancelFn = cancel
	m.cancelMu.Unlock()
	defer func() {
		m.cancelMu.Lock()
		m.cancelFn = nil
		m.activeCmd = nil
		m.cancelMu.Unlock()
		cancel()
	}()

	cmd := exec.CommandContext(ctx, cfg.ScraperCmd, argv...)
	// cwd = absolute script dir so the bundled `.playwright_ready`
	// sentinel and any relative file accesses inside scraper.py find
	// the right neighbours regardless of where Go was launched from.
	cmd.Dir = workDir

	// Inherit PATH/PYTHONHOME etc — overriding cmd.Env to a minimal slice
	// causes Python on Windows to fail with cryptic exit-1 errors.
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"INTERNAL_API_TOKEN="+cfg.InternalAPIToken,
		"JOBSCOUT_API="+fmt.Sprintf("http://localhost:%s", cfg.Port),
	)
	cmd.Env = env

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		m.recordCrash(target, "spawn", err, "")
		m.finish(target, fmt.Errorf("start: %w", err))
		return
	}
	m.cancelMu.Lock()
	m.activeCmd = cmd
	m.cancelMu.Unlock()
	m.mu.Lock()
	m.lastPID = cmd.Process.Pid
	m.mu.Unlock()
	log.Printf("[scraper-mgr] started pid=%d %s", cmd.Process.Pid, label)

	// Drain stdout (we don't need its contents in the live path —
	// scraper.py POSTs jobs directly to /api/internal/jobs — but we
	// still drain so the buffer doesn't block).
	go drainAndLog("stdout", stdoutPipe)

	// Drain stderr line-by-line. Every JSON line becomes a pulse event;
	// non-JSON lines are still logged. We accumulate up to 16 KB so a
	// failure persists meaningful context to scraper_logs.
	stderrBuf := newRingBuffer(16 * 1024)
	go forwardStderr(stderrPipe, stderrBuf, target)

	err := cmd.Wait()
	stderrText := stderrBuf.String()
	if err != nil {
		reason := summariseError(err, stderrText)
		m.recordCrash(target, "spawn", fmt.Errorf("%s", reason), stderrText)
		// Promote the rich reason into the Status struct + the per-task
		// row so the UI shows e.g. "Timeout waiting for selector"
		// instead of the bare "exit status 1".
		m.finish(target, fmt.Errorf("%s", reason))
		return
	}
	m.finish(target, nil)
}

// recordCrash persists the captured stderr to scraper_logs and emits a
// pulse alert with the human-readable reason so both the DB and the UI
// have first-class visibility into the failure.
func (m *Manager) recordCrash(target *Target, stage string, err error, stderr string) {
	reason := err.Error()
	pulse.Broadcast("alert", fmt.Sprintf("Scraper failed [%s]: %s", stage, reason))
	if database.DB == nil {
		log.Printf("[scraper-mgr] crash · stage=%s reason=%s\n%s",
			stage, reason, truncate(stderr, 2000))
		return
	}
	taskID := ""
	var nicheID *uuid.UUID
	url := ""
	if target != nil {
		taskID = target.ID
		url = target.Value
		if target.NicheID != "" {
			if nid, perr := uuid.Parse(target.NicheID); perr == nil {
				nicheID = &nid
			}
		}
	}
	entry := &models.ScraperLog{
		TaskID:    taskID,
		NicheID:   nicheID,
		URL:       url,
		Stage:     stage,
		Error:     truncate(reason, 2000),
		Details:   truncate(stderr, 16*1024),
		ExitCode:  exitCodeFromErr(err),
		CreatedAt: time.Now().UTC(),
	}
	if dbErr := database.DB.Create(entry).Error; dbErr != nil {
		log.Printf("[scraper-mgr] failed to persist scraper_log: %v", dbErr)
	}
}

func (m *Manager) finish(target *Target, err error) {
	m.mu.Lock()
	m.lastRun = time.Now().UTC()
	m.lastErr = err
	m.mu.Unlock()

	if target != nil {
		now := time.Now().UTC()
		status := models.StatusHealthy
		errMsg := ""
		if err != nil {
			status = models.StatusFailed
			errMsg = err.Error()
		}
		if database.DB != nil {
			updates := map[string]any{
				"last_run_at": &now,
				"status":      status,
				"last_error":  errMsg,
			}
			database.DB.Model(&models.ScrapeTask{}).
				Where("id = ?", target.ID).
				Updates(updates)
		} else if tid, parseErr := uuid.Parse(target.ID); parseErr == nil {
			store.MemoryTargets.UpdateStatus(tid, status, errMsg, &now)
		}
	}

	// Record a telemetry data point for the Health Matrix error rate. Ingest-
	// layer events (success / rejected per job) are recorded in internal_jobs.go.
	var taskIDPtr, nicheIDPtr *uuid.UUID
	if target != nil {
		if tid, perr := uuid.Parse(target.ID); perr == nil {
			taskIDPtr = &tid
		}
		if target.NicheID != "" {
			if nid, perr := uuid.Parse(target.NicheID); perr == nil {
				nicheIDPtr = &nid
			}
		}
	}

	if err != nil {
		metrics.Record(taskIDPtr, nicheIDPtr, metrics.Failure, err.Error())
		log.Printf("[scraper-mgr] failed: %v", err)
		return
	}

	// Batch-level success pulse.
	if target != nil && nicheIDPtr != nil {
		summary := summariseNicheBatch(*nicheIDPtr, target.Value)
		if summary != "" {
			pulse.Broadcast("scrape", summary)
		}
	}
	label := "Manual refresh"
	if target != nil {
		label = fmt.Sprintf("Target %q", target.Value)
	}
	pulse.Broadcast("scrape", label+" · completed")
	log.Printf("[scraper-mgr] completed")
}

// summariseNicheBatch reads the last 2-minute niche rollup and returns
// a short pulse-friendly line, e.g. "Niche Architecture · 3 saved · 5 rejected".
func summariseNicheBatch(nicheID uuid.UUID, targetValue string) string {
	rollup := metrics.RollupByNiche(2 * time.Minute)
	agg, ok := rollup[nicheID.String()]
	if !ok || (agg.Success == 0 && agg.Rejected == 0) {
		return ""
	}

	name := targetValue
	if database.DB != nil {
		var n models.NicheProfile
		if err := database.DB.First(&n, "id = ?", nicheID).Error; err == nil {
			name = n.Name
		}
	} else if n := store.MemoryNiches.GetProfile(nicheID); n != nil {
		name = n.Name
	}
	return metrics.FormatRollup("Niche "+name, agg.Success, agg.Rejected, agg.Failure)
}

// ---------------------------------------------------------------------------
// stdout / stderr plumbing

// drainAndLog reads pipe lines into the Go logger. Used for stdout; we
// don't currently parse it because scraper.py POSTs its results directly.
func drainAndLog(label string, r io.Reader) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		log.Printf("[scraper-%s] %s", label, line)
	}
}

// forwardStderr parses each stderr line. JSON lines (`{"stage":..}`)
// become System Pulse events scoped to the stage. Plain-text lines are
// logged. Everything is appended to the rolling buffer.
func forwardStderr(r io.Reader, buf *ringBuffer, target *Target) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		buf.Write([]byte(line + "\n"))
		log.Printf("[scraper-stderr] %s", line)
		if msg, kind, ok := pulseFromStderrLine(line, target); ok {
			pulse.Broadcast(kind, msg)
		}
	}
}

// pulseFromStderrLine inspects a structured stderr event from
// scraper.py and converts it into a (msg, kind) pair suitable for
// pulse.Broadcast. Returns ok=false for unparseable lines.
func pulseFromStderrLine(line string, target *Target) (string, string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return "", "", false
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return "", "", false
	}
	stage, _ := ev["stage"].(string)
	if stage == "" {
		return "", "", false
	}
	prefix := "Scrape"
	if target != nil && target.Value != "" {
		prefix = fmt.Sprintf("[%s]", truncate(target.Value, 40))
	}
	if errStr, ok := ev["error"].(string); ok && errStr != "" {
		return fmt.Sprintf("%s %s · %s", prefix, stage, errStr), "alert", true
	}
	switch stage {
	case "spawn":
		return fmt.Sprintf("%s spawn · launching browser", prefix), "scrape", true
	case "navigate":
		if u, ok := ev["url"].(string); ok {
			return fmt.Sprintf("%s navigate · %s", prefix, truncate(u, 80)), "scrape", true
		}
	case "extract":
		jobs, _ := ev["jobs"].(float64)
		return fmt.Sprintf("%s extract · %d jobs", prefix, int(jobs)), "scrape", true
	case "validate":
		title, _ := ev["title"].(string)
		reason, _ := ev["reason"].(string)
		return fmt.Sprintf("%s validate · rejected %q (%s)", prefix, truncate(title, 40), reason), "alert", true
	case "commit":
		return fmt.Sprintf("%s commit · %s", prefix, line), "merge", true
	case "done":
		acc, _ := ev["accepted"].(float64)
		rej, _ := ev["rejected"].(float64)
		return fmt.Sprintf("%s done · %d accepted · %d rejected", prefix, int(acc), int(rej)), "scrape", true
	}
	return fmt.Sprintf("%s %s", prefix, stage), "scrape", true
}

// summariseError extracts the most useful sentence from the captured
// stderr to surface in the Health Matrix `lastError` column. Falls back
// to err.Error() when the buffer offers nothing better.
func summariseError(err error, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return err.Error()
	}
	// Prefer the last structured `error` field.
	scanner := bufio.NewScanner(strings.NewReader(stderr))
	var last string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var ev map[string]any
			if perr := json.Unmarshal([]byte(line), &ev); perr == nil {
				if e, ok := ev["error"].(string); ok && e != "" {
					last = e
					continue
				}
			}
		}
		last = line
	}
	if last == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s (%s)", last, err.Error())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if asErr := strings.Contains(err.Error(), "exit status "); asErr {
		var code int
		fmt.Sscanf(err.Error(), "exit status %d", &code)
		return code
	}
	if exitErrAs(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func exitErrAs(err error, target **exec.ExitError) bool {
	for cur := err; cur != nil; cur = unwrap(cur) {
		if v, ok := cur.(*exec.ExitError); ok {
			*target = v
			return true
		}
	}
	return false
}

func unwrap(err error) error {
	type w interface{ Unwrap() error }
	if ww, ok := err.(w); ok {
		return ww.Unwrap()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tiny ring buffer for stderr capture.

type ringBuffer struct {
	mu   sync.Mutex
	cap  int
	data []byte
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{cap: capacity, data: make([]byte, 0, capacity)}
}

func (b *ringBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) >= b.cap {
		b.data = append(b.data[:0], p[len(p)-b.cap:]...)
		return len(p), nil
	}
	if len(b.data)+len(p) > b.cap {
		drop := len(b.data) + len(p) - b.cap
		b.data = b.data[drop:]
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *ringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(bytes.TrimRight(b.data, "\x00"))
}

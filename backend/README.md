# JobScout Backend

Go (Fiber v2) + GORM + PostgreSQL + Asynq. Manages user-defined scrape
targets, spawns the Python Playwright scraper, validates the results
against niche profiles, and serves the React frontend.

## Layout

```
backend/
  main.go                     # Fiber bootstrap + CORS
  routes/routes.go            # HTTP route registration
  handlers/
    jobs.go                   # GET /api/jobs (DB-backed; no mock)
    refresh.go                # POST /api/refresh (manual scraper trigger)
    targets.go                # /api/targets/* (Targeting Dashboard)
    niches.go                 # /api/niches/* (Niche Manager)
    internal_jobs.go          # POST /api/internal/jobs (scraper ingest + niche validator)
    admin_scrapers.go         # POST /api/admin/scrapers/test (dry-run validator)
    scraper_logs.go           # GET /api/scraper/logs, POST /api/scraper/kill
    metrics.go                # GET /api/scraper/metrics (rollup)
    saved_jobs.go             # /api/saved-jobs/*
    ws.go                     # WS /ws/pulse (system pulse stream)
  scraper/manager.go          # Singleton ScraperManager — spawns scraper.py
  database/database.go        # GORM connect + AutoMigrate
  models/models.go            # Domain models (Job, ScrapeTask, NicheProfile, ...)
  pulse/pulse.go              # In-process WebSocket pulse hub
  metrics/metrics.go          # Telemetry rollup (success/failure/rejected)
  dedup/dedup.go              # SHA256 source-hash for cross-board merge
  store/                      # In-memory fallback store (DB-less mode)
  queue/                      # Asynq scheduler
```

## Setup

```powershell
cd backend
go mod tidy
copy .env.example .env        # edit DATABASE_URL + INTERNAL_API_TOKEN
go run .
```

Server listens on `http://localhost:8000`.

## .env Vars (see `.env.example`)

| Var | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8000` | API listen port |
| `DATABASE_URL` | `postgres://...` | GORM DSN; if missing, API runs in DB-less mode using the in-memory store |
| `INTERNAL_API_TOKEN` | `dev-internal-token` | Shared secret with `scrapers/` |
| `SCRAPER_CMD` | `python` | Python executable; can be `.venv/Scripts/python.exe` on Windows |
| `SCRAPER_SCRIPT` | `../scrapers/scraper.py` | Path to the worker |
| `FRONTEND_ORIGIN` | `http://localhost:5173,...` | CSV allowlist; any localhost port also passes |

## Quickstart smoke

```powershell
# from repo root
go test ./backend/handlers/ -run TestCompileKeyword -v
go run ./backend
```

Then:

```powershell
# manual dry-run via the admin endpoint
$body = @{
  url = "https://realpython.github.io/fake-jobs/"
  nicheId = "<your-niche-uuid>"
} | ConvertTo-Json
Invoke-RestMethod -Method POST -Uri "http://localhost:8000/api/admin/scrapers/test" -ContentType "application/json" -Body $body
```

## Endpoints (selected)

| Method | Path | Notes |
| --- | --- | --- |
| GET    | `/api/jobs` | List jobs (filterable by `q`, `role`, `location`, `nicheId`) |
| POST   | `/api/refresh` | Trigger global manual refresh |
| GET    | `/api/refresh/status` | Singleton ScraperManager status (running/paused/lastError/lastPid) |
| POST   | `/api/scraper/pause` / `/scraper/resume` | Global kill-switch |
| POST   | `/api/scraper/kill` | Force-kill the in-flight Python subprocess |
| GET    | `/api/scraper/logs` | Captured stderr per failure (latest 25 by default) |
| GET    | `/api/scraper/metrics?since=24h` | Per-task / per-niche rollup |
| POST   | `/api/admin/scrapers/test` | Dry-run scraper + niche validator (no DB writes) |
| GET/POST/DELETE | `/api/targets`, `/api/niches`, `/api/saved-jobs` | CRUD endpoints |
| WS     | `/ws/pulse` | Live system pulse |

The internal ingest endpoint (`POST /api/internal/jobs`) requires the
`X-Internal-Token` header — the Python scraper sets it from
`INTERNAL_API_TOKEN`, which must match this side.

# JobScout Scrapers

`scraper.py` is a single, async Playwright worker that the Go backend
spawns via `os/exec`. It supports two modes:

| Mode | Trigger | Behaviour |
| --- | --- | --- |
| **Live** | `python scraper.py --url <u> --niche-id <id>` (no `--dry-run`) | Extracts jobs, then POSTs each one to `${JOBSCOUT_API}/api/internal/jobs` so the Go ingest pipeline can validate + persist. |
| **Dry-run** | `--dry-run` | Prints a strict JSON array on stdout. No POST, no DB writes. Used by the `/admin/scrapers` UI and by `test_pipeline.py`. |

Both modes emit one structured JSON event per stage on **stderr** (e.g.
`{"stage":"navigate","url":"…"}`); the Go side surfaces these directly
into the System Pulse and into `scraper_logs` on failure.

## Setup

```powershell
cd scrapers
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
python -m playwright install chromium
```

The first scraper run will also auto-install Chromium if the marker
file `.playwright_ready` is missing (so a fresh checkout self-heals).

## CLI

| Flag | Purpose |
| --- | --- |
| `--url URL` | Direct page to scrape. |
| `--seed-keywords "kw1,kw2"` | Search-driven mode (resolves to keyword-search URLs). |
| `--niche-id UUID` | Forwarded to ingest so the niche-context filter applies. |
| `--task-id UUID` | Forwarded to ingest so the result links back to the originating ScrapeTask. |
| `--dry-run` | Print only; never POST. |
| `--detail-pages` | Visit each apply URL to harvest body text (slower, but unlocks better keyword matching). |
| `--max-jobs N` | Hard cap per target (default 25). |

## Smoke test

```powershell
.\.venv\Scripts\python test_pipeline.py
```

Expected:

```
[smoke] PASS: extracted 5 jobs (sample: 'Senior Python Developer' @ 'Payne, Roberts and Davis')
```

## Environment

| Var | Required when | Purpose |
| --- | --- | --- |
| `JOBSCOUT_API` | live mode | Go API base URL (default `http://localhost:8000`). |
| `INTERNAL_API_TOKEN` | live mode | Shared secret matching backend `.env`. |

Env vars are NOT used to pass target data — every scrape parameter
goes through the documented CLI flags so `python scraper.py --help`
remains the source of truth.

## Stealth

Each spawn picks a random User-Agent / viewport / locale from a curated
allowlist and runs an init script that masks the most common headless
tells (`navigator.webdriver`, missing chrome runtime, plugin count of
zero). Rotating these per spawn makes simple fingerprint checks miss.

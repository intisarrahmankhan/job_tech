"""JobScout — Live Playwright Scraper (Async).

This module is the single source of truth for every scrape JobScout
runs.  Invoked by the Go backend via `python scraper.py ...`, it always
behaves as follows:

  1. Parse CLI flags (no env-var-only mode is supported any more).
  2. Spin up a headless Chromium with stealth settings.
  3. Run the State Machine:
        SEARCH   - resolve --seed-keywords into a list of result URLs
        EXTRACT  - visit each URL, harvest the listing
        DETAIL   - (optional) follow each posting URL for full body text
        EMIT     - dump a strict JSON array on stdout
  4. In production mode, additionally POST every job to
     `${JOBSCOUT_API}/api/internal/jobs` so the Go ingest pipeline can
     validate it against the niche profile and persist it.

Zero-mock guarantee: this file does not synthesise any job data.  If a
page produces nothing or the network fails, stdout is `[]` and stderr
contains a structured `{"stage":..,"error":..}` JSON line so the Go
caller can surface the failure in the Health Matrix without parsing
loose tracebacks.

Self-heal: if Playwright's bundled Chromium is missing the very first
time the script runs, we shell out to `python -m playwright install
chromium` once, then retry. The marker is recorded in
`.playwright_ready` next to this file so subsequent runs skip the
check.

CLI:
    --url URL                    Direct page to scrape.
    --seed-keywords "kw,kw2"     Comma-separated search terms.
    --niche-id UUID              Forwarded into each emitted record.
    --task-id UUID               Forwarded into each emitted record.
    --dry-run                    Print JSON only; do not POST to backend.
    --detail-pages               After listing, follow each apply URL
                                 to fetch the full body text (slow but
                                 unlocks better Niche-keyword matching).
    --max-jobs N                 Hard cap (default 25).

Env vars (read only when not --dry-run):
    JOBSCOUT_API         Backend base URL (default http://localhost:8000)
    INTERNAL_API_TOKEN   Shared secret for X-Internal-Token header
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import random
import re
import subprocess
import sys
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Iterable
from urllib.parse import urljoin, urlparse

import httpx
from dotenv import load_dotenv
from playwright.async_api import (
    Browser,
    BrowserContext,
    Error as PlaywrightError,
    Page,
    TimeoutError as PlaywrightTimeoutError,
    async_playwright,
)


# ---------------------------------------------------------------------------
# Stealth configuration
#
# These values are intentionally drawn from a curated allowlist rather
# than randomised across the full UA universe — every string here is a
# real, currently-shipping browser signature so the User-Agent and
# `navigator.userAgent` match what the rest of the headers imply.

USER_AGENTS: tuple[str, ...] = (
    # Chrome 124 / Win 11 / x64
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    # Chrome 124 / macOS 14
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    # Chrome 123 / Linux
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
    # Edge 124 / Win 11
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
    # Firefox 125 / Win 11
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) "
    "Gecko/20100101 Firefox/125.0",
    # Firefox 125 / macOS
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.4; rv:125.0) "
    "Gecko/20100101 Firefox/125.0",
)

VIEWPORTS: tuple[tuple[int, int], ...] = (
    (1366, 768),
    (1440, 900),
    (1536, 864),
    (1600, 900),
    (1920, 1080),
)

LOCALES: tuple[str, ...] = ("en-US", "en-GB", "en-CA")

# Injected before any document script executes; masks the most common
# headless tells (`navigator.webdriver`, missing chrome runtime, plugin
# count of 0). Not a substitute for a paid stealth plugin but enough to
# get past the cheap fingerprint checks on most career-page CDNs.
STEALTH_INIT_SCRIPT = """
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
Object.defineProperty(navigator, 'languages', {
    get: () => ['en-US', 'en'],
});
Object.defineProperty(navigator, 'plugins', {
    get: () => [1, 2, 3, 4, 5],
});
window.chrome = window.chrome || { runtime: {} };
const origQuery = navigator.permissions && navigator.permissions.query;
if (origQuery) {
    navigator.permissions.query = (parameters) =>
        parameters.name === 'notifications'
            ? Promise.resolve({ state: Notification.permission })
            : origQuery(parameters);
}
"""


# ---------------------------------------------------------------------------
# Data model


@dataclass
class JobRecord:
    """Wire-shape exchanged with the Go ingest endpoint.

    Only `title`, `company` and `url` are mandatory.  Fields left at
    their default sentinel are stripped before POSTing so the backend's
    optional-field omitempty tags actually fire.
    """

    title: str
    company: str
    url: str
    location: str = ""
    body: str = ""
    salary: str = ""
    deadline: str | None = None
    sources: list[str] = field(default_factory=list)
    tags: list[str] = field(default_factory=list)
    type: str = "Onsite"
    level: str = "Mid"
    nicheId: str | None = None
    taskId: str | None = None


# ---------------------------------------------------------------------------
# Structured stderr logger
#
# The Go side reads stderr line-by-line; every line it sees that is
# valid JSON gets surfaced into the System Pulse and the scraper_logs
# table.  This wrapper guarantees that everything we emit is parseable.


def log_event(stage: str, **fields: Any) -> None:
    """Print a single JSON event line to stderr."""
    payload = {"stage": stage, "ts": time.time(), **fields}
    try:
        sys.stderr.write(json.dumps(payload, default=str) + "\n")
        sys.stderr.flush()
    except Exception:
        # Never raise from a logger.
        pass


# ---------------------------------------------------------------------------
# Browser preflight
#
# `playwright install chromium` is a few-hundred MB download; we don't
# want to do it on every spawn. A `.playwright_ready` sentinel next to
# this file caches the success.

READY_SENTINEL = Path(__file__).resolve().parent / ".playwright_ready"


def ensure_chromium_installed() -> None:
    """Run `python -m playwright install chromium` if we haven't yet.

    Idempotent — guarded by a sentinel file. Errors are surfaced as
    structured stderr events but not raised; the actual `launch()` call
    will produce the canonical Playwright error if the install really
    failed and we want that error to reach the Go side intact.
    """
    if READY_SENTINEL.exists():
        return
    log_event("preflight", message="installing chromium (one-time)")
    try:
        proc = subprocess.run(
            [sys.executable, "-m", "playwright", "install", "chromium"],
            check=False,
            capture_output=True,
            text=True,
            timeout=600,  # 10 minutes for the download on slow links
        )
        if proc.returncode == 0:
            READY_SENTINEL.write_text("ok\n", encoding="utf-8")
            log_event("preflight", message="chromium ready")
        else:
            log_event(
                "preflight",
                error="playwright install chromium failed",
                exit_code=proc.returncode,
                stderr=proc.stderr[-1000:],
            )
    except Exception as exc:  # pragma: no cover - environmental
        log_event("preflight", error=f"install hook crashed: {exc}")


# ---------------------------------------------------------------------------
# Selector strategies
#
# Each entry is `(name, selectors)` ordered most-specific first. We try
# every selector until at least one returns a non-empty list of cards.
# Hosts with truly bespoke layouts can be added here without touching
# the rest of the pipeline.

SITE_STRATEGIES: tuple[tuple[str, dict[str, str]], ...] = (
    # Real Python's fake-jobs fixture (used by /scrapers/test_pipeline.py
    # and recommended in the README as the smoke target).
    (
        "realpython.github.io",
        {
            "card": "div.card-content",
            "title": "h2.title",
            "company": "h3.subtitle.company",
            "location": "p.location",
            "apply": "footer.card-footer a:nth-of-type(2)",
        },
    ),
    # LinkedIn public job results page
    (
        "linkedin.com",
        {
            "card": "div.base-card.job-search-card, li.result-card",
            "title": "h3.base-search-card__title, h3.result-card__title",
            "company": "h4.base-search-card__subtitle, h4.result-card__subtitle",
            "location": "span.job-search-card__location",
            "apply": "a.base-card__full-link, a.result-card__full-card-link",
        },
    ),
    # Lever ATS
    (
        "lever.co",
        {
            "card": "div.posting",
            "title": "h5[data-qa='posting-name'], h5",
            "company": "span.sort-by-team, div.posting-categories span",
            "location": "span.sort-by-location",
            "apply": "a.posting-title",
        },
    ),
    # Greenhouse ATS
    (
        "greenhouse.io",
        {
            "card": "div.opening",
            "title": "a",
            "company": "span.department",
            "location": "span.location",
            "apply": "a",
        },
    ),
    # Workable ATS
    (
        "workable.com",
        {
            "card": "li.job",
            "title": "h2, h3, [data-ui='job-title']",
            "company": "[data-ui='job-department']",
            "location": "[data-ui='job-location']",
            "apply": "a",
        },
    ),
)

# Generic fallback used when no host-specific strategy matches — uses
# heuristics ("h2/h3 inside something that looks like a card") so we
# emit *something* even on never-seen-before sites.
GENERIC_STRATEGY: dict[str, str] = {
    "card": "article, li.job, div.job, div.posting, div.opening, div[class*='job-' i], div[class*='card' i]",
    "title": "h1, h2, h3, h4, h5, [class*='title' i]",
    "company": "[class*='company' i], [class*='employer' i], [class*='department' i], h4, h5",
    "location": "[class*='location' i], [class*='city' i]",
    "apply": "a[href*='apply' i], a[href*='job' i], a.posting-title, a",
}


def pick_strategy(url: str) -> dict[str, str]:
    host = urlparse(url).hostname or ""
    host = host.lower()
    for needle, strat in SITE_STRATEGIES:
        if needle in host:
            return strat
    return GENERIC_STRATEGY


# ---------------------------------------------------------------------------
# Browser orchestration


async def new_stealth_context(browser: Browser) -> BrowserContext:
    ua = random.choice(USER_AGENTS)
    vp = random.choice(VIEWPORTS)
    locale = random.choice(LOCALES)
    context = await browser.new_context(
        user_agent=ua,
        viewport={"width": vp[0], "height": vp[1]},
        locale=locale,
        timezone_id=random.choice(["UTC", "America/New_York", "Europe/London"]),
        java_script_enabled=True,
        ignore_https_errors=True,
        extra_http_headers={
            "Accept-Language": f"{locale},en;q=0.9",
            "Accept": (
                "text/html,application/xhtml+xml,application/xml;q=0.9,"
                "image/avif,image/webp,*/*;q=0.8"
            ),
            "Sec-Ch-Ua": '"Chromium";v="124", "Not.A/Brand";v="99"',
            "Sec-Ch-Ua-Mobile": "?0",
            "Sec-Ch-Ua-Platform": '"Windows"',
            "Sec-Fetch-Dest": "document",
            "Sec-Fetch-Mode": "navigate",
            "Sec-Fetch-Site": "none",
            "Sec-Fetch-User": "?1",
            "Upgrade-Insecure-Requests": "1",
        },
    )
    await context.add_init_script(STEALTH_INIT_SCRIPT)
    log_event("context", ua=ua, viewport=list(vp), locale=locale)
    return context


# ---------------------------------------------------------------------------
# Extraction


async def safe_text(card, selector: str) -> str:
    try:
        el = await card.query_selector(selector)
        if not el:
            return ""
        text = await el.inner_text()
        return (text or "").strip()
    except PlaywrightError:
        return ""


async def safe_attr(card, selector: str, attr: str) -> str:
    try:
        el = await card.query_selector(selector)
        if not el:
            return ""
        v = await el.get_attribute(attr)
        return (v or "").strip()
    except PlaywrightError:
        return ""


def host_company_guess(url: str) -> str:
    host = urlparse(url).hostname or ""
    host = host.replace("www.", "")
    if not host:
        return ""
    base = host.split(".")[0]
    return base.replace("-", " ").title()


async def extract_listing(
    page: Page,
    url: str,
    *,
    max_jobs: int,
) -> list[JobRecord]:
    """Run the EXTRACT phase against the currently-loaded page."""
    strategy = pick_strategy(url)

    # Wait for at least the first card to materialise. We avoid hard
    # `time.sleep`s — `wait_for_selector` resolves as soon as something
    # matches.  Total wall-clock budget here is 12 seconds.
    try:
        await page.wait_for_selector(strategy["card"], timeout=12_000, state="visible")
    except PlaywrightTimeoutError:
        log_event(
            "extract",
            url=url,
            error="timeout waiting for card selector",
            selector=strategy["card"],
        )
        # Even on timeout, try once more with the generic strategy in
        # case the host strategy was wrong. If the generic strategy was
        # already in play, fall through to the empty-result branch.
        if strategy is not GENERIC_STRATEGY:
            strategy = GENERIC_STRATEGY
            try:
                await page.wait_for_selector(
                    strategy["card"], timeout=4_000, state="visible"
                )
            except PlaywrightTimeoutError:
                return []
        else:
            return []

    cards = await page.query_selector_all(strategy["card"])
    if not cards:
        log_event("extract", url=url, message="no cards matched", selector=strategy["card"])
        return []

    company_fallback = host_company_guess(url)
    out: list[JobRecord] = []
    for card in cards[:max_jobs]:
        title = await safe_text(card, strategy["title"])
        if not title:
            continue
        company = await safe_text(card, strategy["company"]) or company_fallback
        location = await safe_text(card, strategy["location"])
        href = await safe_attr(card, strategy["apply"], "href")
        if href:
            href = urljoin(url, href)
        else:
            href = url  # last-resort so the row still has SOME url
        out.append(
            JobRecord(
                title=title[:140],
                company=company[:120],
                location=location[:80],
                url=href,
                sources=[urlparse(url).hostname or url],
            )
        )
    log_event("extract", url=url, jobs=len(out))
    return out


async def enrich_with_detail(
    context: BrowserContext,
    job: JobRecord,
    *,
    timeout_ms: int = 12_000,
) -> None:
    """Visit the apply URL and harvest body text for context-keyword matching.

    Failures are non-fatal: we just leave job.body empty and emit a log
    event so the Go side can see that detail enrichment skipped.
    """
    if not job.url or job.url.startswith("javascript:"):
        return
    page = await context.new_page()
    try:
        await page.goto(job.url, timeout=timeout_ms, wait_until="domcontentloaded")
        body_text: str = await page.evaluate(
            "() => document.body ? document.body.innerText : ''"
        )
        job.body = (body_text or "")[:8000]
        # Pull a few opportunistic tags out of the body text so the
        # ingest-side ContextKeyword scan has more to bite into.
        if job.body:
            tag_pool = []
            for kw in ("Remote", "Hybrid", "Onsite", "Full-time", "Part-time", "Contract"):
                if re.search(rf"\b{re.escape(kw)}\b", job.body, re.IGNORECASE):
                    tag_pool.append(kw)
            if tag_pool:
                job.tags = list(dict.fromkeys([*job.tags, *tag_pool]))
                if "Remote" in tag_pool:
                    job.type = "Remote"
                elif "Hybrid" in tag_pool:
                    job.type = "Hybrid"
    except PlaywrightTimeoutError:
        log_event("detail", url=job.url, error="timeout")
    except PlaywrightError as exc:
        log_event("detail", url=job.url, error=str(exc))
    finally:
        try:
            await page.close()
        except PlaywrightError:
            pass


# ---------------------------------------------------------------------------
# Search phase
#
# For now `--seed-keywords` resolves into a single Google search URL per
# keyword. The result-page extractor still runs a generic strategy
# against the SERP, which is rarely useful, but the SEARCH path exists
# so future board-specific search builders (LinkedIn, BDJobs, Indeed)
# can plug in without touching the orchestrator.

SEARCH_TEMPLATES: tuple[str, ...] = (
    "https://www.google.com/search?q={q}+jobs",
)


def keyword_to_urls(keywords: Iterable[str]) -> list[str]:
    out: list[str] = []
    for kw in keywords:
        kw = kw.strip()
        if not kw:
            continue
        for tmpl in SEARCH_TEMPLATES:
            out.append(tmpl.format(q=kw.replace(" ", "+")))
    return out


# ---------------------------------------------------------------------------
# Backend POST


def post_jobs(
    api_base: str,
    token: str,
    jobs: list[JobRecord],
) -> tuple[int, int]:
    """POST every job to the Go ingest endpoint.

    Returns (accepted, rejected). Counts a 4xx as rejected and a 5xx as
    failed (logged as an event so the Go side records the network
    failure separately from validation rejections).
    """
    accepted = 0
    rejected = 0
    if not jobs:
        return 0, 0
    with httpx.Client(timeout=20.0) as client:
        for j in jobs:
            payload = {
                k: v for k, v in asdict(j).items() if v not in (None, "", [], ())
            }
            try:
                resp = client.post(
                    f"{api_base}/api/internal/jobs",
                    json=payload,
                    headers={"X-Internal-Token": token},
                )
            except httpx.HTTPError as exc:
                log_event(
                    "commit",
                    url=j.url,
                    error=f"network: {exc}",
                )
                continue
            if 200 <= resp.status_code < 300:
                accepted += 1
            elif resp.status_code in (400, 422, 202):
                # 202 from the validate-only path means "rejected" too
                try:
                    body = resp.json()
                    if body.get("status") == "rejected":
                        rejected += 1
                        log_event(
                            "validate",
                            title=j.title,
                            reason=body.get("reason", ""),
                            missing=body.get("missing", []),
                        )
                    else:
                        accepted += 1
                except ValueError:
                    rejected += 1
            else:
                log_event(
                    "commit",
                    url=j.url,
                    error=f"http {resp.status_code}",
                    body=resp.text[:300],
                )
    return accepted, rejected


# ---------------------------------------------------------------------------
# Orchestration


async def run_pipeline(args: argparse.Namespace) -> list[JobRecord]:
    """Drive the full SEARCH → EXTRACT → DETAIL state machine."""
    log_event("spawn", url=args.url, niche_id=args.niche_id, dry_run=args.dry_run)

    targets: list[str] = []
    if args.url:
        targets.append(args.url)
    if args.seed_keywords:
        targets.extend(
            keyword_to_urls(s.strip() for s in args.seed_keywords.split(","))
        )
    if not targets:
        log_event("spawn", error="no --url or --seed-keywords supplied")
        return []

    ensure_chromium_installed()
    all_jobs: list[JobRecord] = []

    async with async_playwright() as pw:
        try:
            browser = await pw.chromium.launch(
                headless=True,
                args=[
                    "--disable-blink-features=AutomationControlled",
                    "--no-sandbox",
                    "--disable-dev-shm-usage",
                ],
            )
        except PlaywrightError as exc:
            log_event("spawn", error=f"chromium launch: {exc}")
            return []

        try:
            context = await new_stealth_context(browser)
            for url in targets:
                page = await context.new_page()
                try:
                    log_event("navigate", url=url)
                    await page.goto(url, timeout=30_000, wait_until="domcontentloaded")
                    found = await extract_listing(page, url, max_jobs=args.max_jobs)
                except PlaywrightTimeoutError as exc:
                    log_event("navigate", url=url, error=f"timeout: {exc}")
                    continue
                except PlaywrightError as exc:
                    log_event("navigate", url=url, error=str(exc))
                    continue
                finally:
                    try:
                        await page.close()
                    except PlaywrightError:
                        pass

                # Carry niche / task ids through to ingest
                for j in found:
                    j.nicheId = args.niche_id or None
                    j.taskId = args.task_id or None

                if args.detail_pages and found:
                    sem = asyncio.Semaphore(3)

                    async def _enrich(job: JobRecord) -> None:
                        async with sem:
                            await enrich_with_detail(context, job)

                    await asyncio.gather(*(_enrich(j) for j in found))

                all_jobs.extend(found)
        finally:
            await browser.close()

    return all_jobs


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        prog="scraper",
        description="JobScout live Playwright scraper",
    )
    p.add_argument("--url", default="", help="Direct URL to scrape")
    p.add_argument("--seed-keywords", default="", help="Comma-separated search terms")
    p.add_argument("--niche-id", default="", help="Niche UUID forwarded to backend")
    p.add_argument("--task-id", default="", help="Task UUID forwarded to backend")
    p.add_argument("--dry-run", action="store_true", help="Print JSON only; no POST")
    p.add_argument(
        "--detail-pages",
        action="store_true",
        help="Visit each apply URL to fetch full body text",
    )
    p.add_argument("--max-jobs", type=int, default=25, help="Hard cap per target")
    return p.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    load_dotenv()
    args = parse_args(argv)
    if not args.url and not args.seed_keywords:
        log_event("spawn", error="must supply --url or --seed-keywords")
        sys.stdout.write("[]\n")
        return 2

    try:
        jobs = asyncio.run(run_pipeline(args))
    except KeyboardInterrupt:
        log_event("spawn", error="interrupted")
        sys.stdout.write("[]\n")
        return 130
    except Exception as exc:
        # Anything we did not anticipate. Emit structured stderr line
        # plus an empty array so callers downstream still get parseable
        # stdout.
        log_event("spawn", error=f"unhandled: {exc!r}")
        sys.stdout.write("[]\n")
        return 1

    payload = [
        {k: v for k, v in asdict(j).items() if v not in (None, "", [], ())}
        for j in jobs
    ]
    sys.stdout.write(json.dumps(payload, ensure_ascii=False) + "\n")
    sys.stdout.flush()

    if args.dry_run:
        log_event("done", jobs=len(jobs), mode="dry-run")
        return 0

    api_base = os.environ.get("JOBSCOUT_API", "http://localhost:8000").rstrip("/")
    token = os.environ.get("INTERNAL_API_TOKEN", "")
    if not token:
        log_event("commit", error="INTERNAL_API_TOKEN not set; skipping POST")
        return 0
    accepted, rejected = post_jobs(api_base, token, jobs)
    log_event(
        "done",
        jobs=len(jobs),
        accepted=accepted,
        rejected=rejected,
        mode="live",
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())

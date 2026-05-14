"""End-to-end smoke test for the rewritten scraper.

Run from the repo root:

    python scrapers\\test_pipeline.py

The test executes scraper.py against the public Real Python "fake-jobs"
fixture, parses the strict JSON array on stdout, and asserts at least
one job came back with the three mandatory fields. It does NOT touch
the Go backend — `--dry-run` keeps everything in-process and prints to
stdout only.

Exit codes:
    0  — pass
    1  — at least one assertion failed
    2  — Python could not even spawn the scraper
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "scraper.py"
FIXTURE_URL = "https://realpython.github.io/fake-jobs/"
NICHE_ID = "00000000-0000-0000-0000-000000000001"


def main() -> int:
    print(f"[smoke] invoking {SCRIPT.name} against {FIXTURE_URL}")
    proc = subprocess.run(
        [
            sys.executable,
            str(SCRIPT),
            "--url",
            FIXTURE_URL,
            "--niche-id",
            NICHE_ID,
            "--dry-run",
            "--max-jobs",
            "5",
        ],
        capture_output=True,
        text=True,
        timeout=180,
    )
    print(f"[smoke] exit={proc.returncode}")
    if proc.stderr.strip():
        print("[smoke] --- stderr (structured events) ---")
        print(proc.stderr.strip())
        print("[smoke] -----------------------------------")

    if proc.returncode != 0:
        print(f"[smoke] FAIL: scraper returned exit {proc.returncode}")
        return 2

    stdout_trim = proc.stdout.strip()
    if not stdout_trim:
        print("[smoke] FAIL: empty stdout")
        return 1

    # scraper.py emits one JSON array per run on a single line.  We prefer
    # whole-line parsing because slicing on `[` / `]` would land inside a
    # nested `sources` array.  Iterate from the bottom so any debug
    # `print()` lines from third-party libs don't trip us up.
    jobs = None
    for line in reversed(stdout_trim.splitlines()):
        line = line.strip()
        if line.startswith("[") and line.endswith("]"):
            try:
                jobs = json.loads(line)
                break
            except json.JSONDecodeError:
                continue
    if jobs is None:
        print("[smoke] FAIL: no JSON array on stdout")
        print(f"        got: {stdout_trim[:300]}")
        return 1

    if not isinstance(jobs, list):
        print(f"[smoke] FAIL: expected list, got {type(jobs).__name__}")
        return 1
    if len(jobs) == 0:
        print("[smoke] FAIL: zero jobs extracted from a fixture that has 100")
        return 1

    sample = jobs[0]
    required = ("title", "company", "url")
    missing = [k for k in required if not sample.get(k)]
    if missing:
        print(f"[smoke] FAIL: first job missing required fields: {missing}")
        print(f"        sample: {sample}")
        return 1

    print(f"[smoke] PASS: extracted {len(jobs)} jobs (sample: {sample['title']!r} @ {sample['company']!r})")
    return 0


if __name__ == "__main__":
    sys.exit(main())

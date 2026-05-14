# JobScout (TechPulse)

JobScout is a full-stack, automated job scraping and aggregation platform. It orchestrates headless browser scrapers to extract job listings from various ATS systems (Lever, Greenhouse, Workable) and job boards, validates them against user-defined "Niche Profiles" (keyword rules), and presents them in a unified UI.

## Architecture

The project is split into three main components:

- **`backend/`**: A Go backend built with Fiber v2, GORM, and PostgreSQL. It manages user-defined scrape targets, spawns the Python scraper subprocesses, validates extracted jobs against niche profiles, and handles telemetry via WebSockets.
- **`frontend/`**: A React + Vite frontend dashboard. It provides the Targeting Dashboard for managing job sources, a Scraper Health Matrix for monitoring scraping pipelines, and a live feed of ingested jobs.
- **`scrapers/`**: An asynchronous Python scraping engine powered by Playwright. It spins up stealth-configured headless Chromium instances to execute dynamic searches and extract job listings from complex career sites.
- **`scripts/`**: Setup scripts for installing services and configuring local environments on Windows.

## Quick Start

### 1. Database (PostgreSQL)
Ensure you have PostgreSQL running. The backend defaults to an in-memory mode if the database isn't reachable, but for persistence, set the `DATABASE_URL` in `backend/.env`.

### 2. Scraper Environment
```powershell
cd scrapers
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
python -m playwright install chromium
```

### 3. Backend
```powershell
cd backend
go mod tidy
go run .
```
*(Runs on http://localhost:8000)*

### 4. Frontend
```powershell
cd frontend
npm install
npm run dev
```
*(Runs on http://localhost:5173)*

## Features

- **Dynamic Extraction**: Uses heuristic CSS selectors and dedicated ATS extraction rules to pull structured data from almost any job board.
- **Context Validation**: Niche profiles ensure that only jobs meeting specific context keywords (e.g., minimum keyword match count) make it to the main feed.
- **Deduplication**: SHA256-based fingerprinting of job titles and companies to merge duplicate listings across multiple job boards.
- **Live Telemetry**: The backend broadcasts real-time scraping progress and log outputs to the frontend via WebSockets.

## Component Documentation

For more detailed setup and architecture notes, see the internal component READMEs:
- [Backend README](backend/README.md)
- [Scrapers README](scrapers/README.md)

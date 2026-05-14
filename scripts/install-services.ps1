# Installs PostgreSQL + Redis via Chocolatey for the JobScout dev stack.
# Run from an elevated PowerShell (the launcher in dev-up.ps1 handles UAC).
#
# Safe to re-run: choco install is idempotent for the latest package.

$ErrorActionPreference = "Stop"

function Step($msg) {
    Write-Host "`n=== $msg ===" -ForegroundColor Cyan
}

# 1. Sanity-check choco is on PATH.
if (-not (Get-Command choco -ErrorAction SilentlyContinue)) {
    Write-Error "Chocolatey is not installed or not on PATH."
    exit 1
}

# 2. Install PostgreSQL with a known dev password so the .env matches.
#    The Chocolatey package installs as a Windows service that auto-starts.
Step "Installing PostgreSQL (password: postgres)"
choco install postgresql -y --params "/Password:postgres" --no-progress

# 3. Install Redis (the redis-64 package ships the Microsoft port that
#    also registers a Windows service).
Step "Installing Redis"
choco install redis-64 -y --no-progress

# 4. Refresh PATH for this elevated session so psql works below.
Step "Refreshing PATH"
$env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
            [Environment]::GetEnvironmentVariable("Path", "User")

# 5. Make sure both services are running. Chocolatey starts them by
#    default but we re-assert in case the user installed previously and
#    stopped them.
Step "Starting services"
foreach ($svcGlob in "postgresql-x64-*", "Redis", "redis-*") {
    Get-Service -Name $svcGlob -ErrorAction SilentlyContinue | ForEach-Object {
        if ($_.Status -ne 'Running') {
            Write-Host "  starting $($_.Name)..."
            Start-Service $_.Name
        } else {
            Write-Host "  $($_.Name) already running"
        }
    }
}

# 6. Create the jobscout database (idempotent — ignore "already exists").
Step "Creating jobscout database"
$env:PGPASSWORD = "postgres"
$psql = Get-Command psql -ErrorAction SilentlyContinue
if (-not $psql) {
    Write-Warning "psql not on PATH yet — open a new terminal and run:"
    Write-Warning '  psql -U postgres -h localhost -c "CREATE DATABASE jobscout;"'
} else {
    & psql -U postgres -h localhost -c "CREATE DATABASE jobscout;" 2>&1 |
        ForEach-Object {
            if ($_ -match "already exists") {
                Write-Host "  jobscout DB already exists — skipping"
            } else {
                Write-Host "  $_"
            }
        }
}

Step "Done"
Write-Host "PostgreSQL: localhost:5432 user=postgres password=postgres db=jobscout"
Write-Host "Redis:      localhost:6379 (no password)"
Write-Host ""
Write-Host "Press any key to close this window..."
[void][System.Console]::ReadKey($true)

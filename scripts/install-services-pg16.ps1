# Cleans up the stuck PostgreSQL 18 install and replaces it with the
# battle-tested PostgreSQL 16 + Redis-64 chocolatey packages.
#
# Must be run elevated. Designed to be idempotent.

$ErrorActionPreference = "Continue"

function Step($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }

# 1. Kill the stuck installer + the parent choco process. We match by
#    name rather than PID so the script works even if PIDs have rotated.
Step "Killing stuck PG18 installer and choco"
foreach ($name in 'postgresql-18.3-1-windows-x64','choco','msiexec') {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        Write-Host "  killing $($_.ProcessName) [PID $($_.Id)]"
        try { Stop-Process -Id $_.Id -Force -ErrorAction Stop } catch { Write-Warning $_ }
    }
}

# Give Windows a moment to release file handles before we try to delete.
Start-Sleep -Seconds 2

# 2. Wipe the half-installed PG 18 tree and the choco lockfile.
Step "Removing partial PG 18 install"
if (Test-Path 'C:\Program Files\PostgreSQL\18') {
    Remove-Item -Recurse -Force 'C:\Program Files\PostgreSQL\18' -ErrorAction SilentlyContinue
    if (Test-Path 'C:\Program Files\PostgreSQL\18') {
        Write-Warning "Could not fully delete C:\Program Files\PostgreSQL\18 — reboot may be needed."
    } else {
        Write-Host "  Removed C:\Program Files\PostgreSQL\18"
    }
}
Get-ChildItem 'C:\ProgramData\chocolatey\lib-bad' -Filter 'postgresql*' -ErrorAction SilentlyContinue | ForEach-Object {
    Remove-Item -Recurse -Force $_.FullName -ErrorAction SilentlyContinue
}
Get-ChildItem 'C:\ProgramData\chocolatey\lib' -Filter 'postgresql18*' -ErrorAction SilentlyContinue | ForEach-Object {
    Remove-Item -Recurse -Force $_.FullName -ErrorAction SilentlyContinue
}

# 3. Install PostgreSQL 16 and Redis. PG 16 is the most recent version
#    with a mature chocolatey package (5+ years of releases, very few
#    silent-install issues reported in 2025/2026).
Step "Installing PostgreSQL 16 + Redis (this takes 2-3 minutes)"
choco install postgresql16 -y --params "/Password:postgres" --no-progress
choco install redis-64 -y --no-progress

# 4. Refresh PATH in this elevated session.
$env:Path = [Environment]::GetEnvironmentVariable("Path","Machine") + ";" +
            [Environment]::GetEnvironmentVariable("Path","User")

# 5. Start services if not already running.
Step "Starting services"
foreach ($svcGlob in 'postgresql-x64-16','postgresql-x64-*','Redis','redis-*') {
    Get-Service -Name $svcGlob -ErrorAction SilentlyContinue | ForEach-Object {
        if ($_.Status -ne 'Running') {
            Write-Host "  starting $($_.Name)..."
            try { Start-Service $_.Name -ErrorAction Stop } catch { Write-Warning $_ }
        } else {
            Write-Host "  $($_.Name) already running"
        }
    }
}

# 6. Create jobscout DB.
Step "Creating jobscout database"
$env:PGPASSWORD = "postgres"
$psql = Get-Command psql -ErrorAction SilentlyContinue
if (-not $psql) {
    # Fall back to the bin folder created by the choco package.
    $candidate = Get-ChildItem 'C:\Program Files\PostgreSQL' -ErrorAction SilentlyContinue |
                 Sort-Object Name -Descending | Select-Object -First 1
    if ($candidate) {
        $psql = Join-Path $candidate.FullName 'bin\psql.exe'
    }
}
if ($psql) {
    & $psql -U postgres -h localhost -c "CREATE DATABASE jobscout;" 2>&1 |
        ForEach-Object { if ($_ -match 'already exists') { "  jobscout DB already exists" } else { "  $_" } }
} else {
    Write-Warning "Could not locate psql.exe. Run manually after install:"
    Write-Warning '  psql -U postgres -h localhost -c "CREATE DATABASE jobscout;"'
}

Step "Done"
Get-Service | Where-Object { $_.Name -like '*postgres*' -or $_.Name -like '*redis*' } |
    Format-Table Name, Status, StartType -AutoSize

Write-Host "`nPress any key to close..." -ForegroundColor Yellow
[void][System.Console]::ReadKey($true)

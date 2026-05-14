# Recovers a PostgreSQL 18 install where the postgres password is unknown,
# then installs Redis-64. Idempotent.

$ErrorActionPreference = "Continue"

function Step($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }

# --- Locate PG 18 install ---
$pgRoot = (Get-ChildItem 'C:\Program Files\PostgreSQL' -Directory |
           Sort-Object Name -Descending |
           Select-Object -First 1).FullName
if (-not $pgRoot) { Write-Error "PostgreSQL not installed"; exit 1 }

$dataDir = Join-Path $pgRoot 'data'
$hba     = Join-Path $dataDir 'pg_hba.conf'
$psql    = Join-Path $pgRoot 'bin\psql.exe'
$svcName = (Get-Service | Where-Object Name -like 'postgresql-x64-*').Name

Write-Host "PG root:    $pgRoot"
Write-Host "Data dir:   $dataDir"
Write-Host "pg_hba:     $hba"
Write-Host "psql:       $psql"
Write-Host "Service:    $svcName"

if (-not (Test-Path $hba)) { Write-Error "pg_hba.conf not found at $hba"; exit 1 }

# --- Switch to trust auth temporarily ---
Step "Backing up pg_hba.conf and switching to trust auth"
Copy-Item $hba "$hba.backup" -Force
$content = Get-Content $hba -Raw
# Replace scram-sha-256 / md5 only on local + host lines for IPv4/IPv6
# loopback; leave everything else untouched.
$patched = $content `
    -replace '(?m)^(host\s+all\s+all\s+127\.0\.0\.1/32\s+)(scram-sha-256|md5|password)', '$1trust' `
    -replace '(?m)^(host\s+all\s+all\s+::1/128\s+)(scram-sha-256|md5|password)', '$1trust' `
    -replace '(?m)^(local\s+all\s+all\s+)(scram-sha-256|md5|password)', '$1trust'
Set-Content $hba $patched -Encoding ascii

Step "Restarting $svcName"
Restart-Service $svcName -Force
Start-Sleep -Seconds 3

# --- Reset password to 'postgres' ---
Step "Setting postgres user password to 'postgres'"
& $psql -U postgres -h localhost -c "ALTER USER postgres WITH PASSWORD 'postgres';" 2>&1 |
    ForEach-Object { "  $_" }

# --- Restore secure pg_hba.conf ---
Step "Restoring secure pg_hba.conf"
Copy-Item "$hba.backup" $hba -Force
Restart-Service $svcName -Force
Start-Sleep -Seconds 3

# --- Verify auth works ---
Step "Verifying password auth"
$env:PGPASSWORD = "postgres"
& $psql -U postgres -h localhost -c "SELECT version();" 2>&1 |
    Select-Object -First 3 | ForEach-Object { "  $_" }

# --- Create jobscout DB ---
Step "Creating jobscout database"
$out = & $psql -U postgres -h localhost -c "CREATE DATABASE jobscout;" 2>&1
$out | ForEach-Object { if ($_ -match 'already exists') { "  jobscout DB already exists" } else { "  $_" } }

# --- Install Redis ---
Step "Installing Redis"
if (Get-Service -Name 'Redis' -ErrorAction SilentlyContinue) {
    Write-Host "  Redis service already exists"
} else {
    choco install redis-64 -y --no-progress
}

Step "Starting Redis"
Get-Service -Name 'Redis','redis-*' -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_.Status -ne 'Running') {
        Write-Host "  starting $($_.Name)..."
        try { Start-Service $_.Name -ErrorAction Stop } catch { Write-Warning $_ }
    } else {
        Write-Host "  $($_.Name) already running"
    }
}

Step "Final status"
Get-Service | Where-Object { $_.Name -like '*postgres*' -or $_.Name -like '*redis*' } |
    Format-Table Name, Status, StartType -AutoSize
Get-NetTCPConnection -LocalPort 5432,6379 -ErrorAction SilentlyContinue |
    Select-Object LocalAddress, LocalPort, State | Format-Table -AutoSize

Write-Host "`nPress any key to close..." -ForegroundColor Yellow
[void][System.Console]::ReadKey($true)

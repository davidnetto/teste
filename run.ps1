# run.ps1 - Florentina Server
# Usage: .\run.ps1

# The system now loads variables automatically from the .env file.
# Ensure the .env file exists in the root.

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $here

Write-Host "--- Florentina API ---" -ForegroundColor Green
Write-Host "Configured via .env"
Write-Host ""

# Ensure dependencies are downloaded
if (-not (Test-Path ".\go.sum") -or ((Get-Item ".\go.mod").LastWriteTime -gt (Get-Item ".\go.sum").LastWriteTime)) {
    Write-Host "Downloading dependencies (go mod tidy)..." -ForegroundColor Yellow
    go mod tidy
    if ($LASTEXITCODE -ne 0) { Write-Host "Error: go mod tidy failed" -ForegroundColor Red; exit 1 }
}

# Build if necessary
$rebuild = $true
if (Test-Path ".\userapi.exe") {
    $exeTime = (Get-Item ".\userapi.exe").LastWriteTime
    $newest  = (Get-ChildItem -Recurse -Filter "*.go" | Sort-Object LastWriteTime -Descending | Select-Object -First 1).LastWriteTime
    if ($exeTime -gt $newest) { $rebuild = $false }
}
if ($rebuild) {
    Write-Host "Compiling..." -ForegroundColor Yellow
    go build -o userapi.exe .
    if ($LASTEXITCODE -ne 0) { Write-Host "Error: build failed" -ForegroundColor Red; exit 1 }
    Write-Host "Compiled: userapi.exe" -ForegroundColor Green
}

Write-Host ""
Write-Host "Starting server..." -ForegroundColor Green
Write-Host ""
.\userapi.exe

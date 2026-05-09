# Job Scout — start all services
# Run from the project root: .\start.ps1

Set-StrictMode -Version Latest

function Write-Step($msg) { Write-Host "`n>> $msg" -ForegroundColor Cyan }

Write-Host "`n========================================" -ForegroundColor Magenta
Write-Host "  Job Scout — Starting" -ForegroundColor Magenta
Write-Host "========================================`n" -ForegroundColor Magenta

# ── 1. Start Ollama (if not already running) ─────────────────────────────────
Write-Step "Starting Ollama..."
$ollamaRunning = Get-Process -Name "ollama" -ErrorAction SilentlyContinue
if ($ollamaRunning) {
    Write-Host "   Ollama already running" -ForegroundColor Green
} else {
    $env:OLLAMA_NUM_PARALLEL = "2"
    Start-Process -FilePath "ollama" -ArgumentList "serve" -WindowStyle Minimized
    Start-Sleep -Seconds 2
    Write-Host "   Ollama started (2 parallel slots)" -ForegroundColor Green
}

# ── 2. Build frontend (if dist is missing or stale) ──────────────────────────
Write-Step "Checking frontend build..."
$distIndex = "$PSScriptRoot\client\dist\index.html"
if (-not (Test-Path $distIndex)) {
    Write-Host "   Building frontend..." -ForegroundColor Yellow
    Push-Location "$PSScriptRoot\client"
    npm run build --silent
    Pop-Location
    Write-Host "   Frontend built" -ForegroundColor Green
} else {
    Write-Host "   Frontend already built" -ForegroundColor Green
}

# ── 3. Start Go server ───────────────────────────────────────────────────────
Write-Step "Starting Job Scout server..."
Write-Host "   Open http://localhost:8080 in your browser" -ForegroundColor Green
Write-Host "   Press Ctrl+C to stop`n" -ForegroundColor Yellow

Push-Location "$PSScriptRoot\server"
go run ./cmd/server
Pop-Location

# Job Scout — automated installer for Windows
# Run once from the project root: .\setup.ps1

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step($msg) { Write-Host "`n>> $msg" -ForegroundColor Cyan }
function Write-OK($msg)   { Write-Host "   OK: $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "   !! $msg" -ForegroundColor Yellow }
function Write-Fail($msg) { Write-Host "   FAIL: $msg" -ForegroundColor Red; exit 1 }

function Install-WithWinget($id, $name) {
    Write-Host "   Installing $name via winget..." -ForegroundColor Yellow
    winget install --id $id --silent --accept-package-agreements --accept-source-agreements
    # Refresh PATH so the new binary is available immediately
    $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH","Machine") + ";" +
                [System.Environment]::GetEnvironmentVariable("PATH","User")
}

Write-Host "`n========================================" -ForegroundColor Magenta
Write-Host "  Job Scout — Installer (Windows)" -ForegroundColor Magenta
Write-Host "========================================`n" -ForegroundColor Magenta

# ── Winget check ──────────────────────────────────────────────────────────────
if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    Write-Warn "winget not found. Install 'App Installer' from the Microsoft Store, then re-run."
    exit 1
}

# ── 1. Go ─────────────────────────────────────────────────────────────────────
Write-Step "Checking Go..."
if (Get-Command go -ErrorAction SilentlyContinue) {
    Write-OK (go version)
} else {
    Install-WithWinget "GoLang.Go" "Go"
    Write-OK (go version)
}

# ── 2. Node.js ────────────────────────────────────────────────────────────────
Write-Step "Checking Node.js..."
if (Get-Command node -ErrorAction SilentlyContinue) {
    Write-OK "Node.js $(node --version)"
} else {
    Install-WithWinget "OpenJS.NodeJS.LTS" "Node.js LTS"
    Write-OK "Node.js $(node --version)"
}

# ── 3. Ollama ─────────────────────────────────────────────────────────────────
Write-Step "Checking Ollama..."
if (Get-Command ollama -ErrorAction SilentlyContinue) {
    Write-OK (ollama --version)
} else {
    Install-WithWinget "Ollama.Ollama" "Ollama"
    Write-OK (ollama --version)
}

# ── 4. Pull LLM model ─────────────────────────────────────────────────────────
Write-Step "Pulling llama3.1:8b model (~5 GB, only needed once)..."
$models = (ollama list 2>&1)
if ($models -match "llama3.1:8b") {
    Write-OK "llama3.1:8b already installed"
} else {
    Write-Host "   Downloading... this may take several minutes" -ForegroundColor Yellow
    # Start ollama serve temporarily so we can pull
    $ollamaProcess = Start-Process "ollama" -ArgumentList "serve" -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 3
    ollama pull llama3.1:8b
    Stop-Process -Id $ollamaProcess.Id -ErrorAction SilentlyContinue
    Write-OK "llama3.1:8b downloaded"
}

# ── 5. Frontend dependencies ──────────────────────────────────────────────────
Write-Step "Installing frontend dependencies..."
Push-Location "$PSScriptRoot\client"
npm install --silent
Write-OK "npm packages installed"
Pop-Location

# ── 6. Build frontend ─────────────────────────────────────────────────────────
Write-Step "Building frontend..."
Push-Location "$PSScriptRoot\client"
npm run build --silent
Write-OK "Frontend built"
Pop-Location

# ── 7. .env file ──────────────────────────────────────────────────────────────
Write-Step "Setting up .env..."
$envFile = "$PSScriptRoot\.env"
if (Test-Path $envFile) {
    Write-OK ".env already exists"
} else {
    Copy-Item "$PSScriptRoot\.env.example" $envFile
    Write-Warn ".env created — open it and fill in your Adzuna API keys"
}

# ── 8. Go dependencies ────────────────────────────────────────────────────────
Write-Step "Downloading Go dependencies..."
Push-Location "$PSScriptRoot\server"
go mod download
Write-OK "Go modules ready"
Pop-Location

# ── 9. profile.md ─────────────────────────────────────────────────────────────
Write-Step "Checking profile.md..."
if (Test-Path "$PSScriptRoot\profile.md") {
    Write-OK "profile.md found"
} else {
    Write-Warn "No profile.md — add your CV/profile to profile.md in the project root"
}

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "  Setup complete!" -ForegroundColor Green
Write-Host "  Run .\start.ps1 to launch Job Scout." -ForegroundColor Green
Write-Host "========================================`n" -ForegroundColor Green

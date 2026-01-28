$ErrorActionPreference = "Stop"

# Configuration
$AppName = "FeatherWings"
$BinDir = "build"
$BinName = "wings.exe"
$ConfigDir = "C:\ProgramData\FeatherPanel"
$ConfigFile = Join-Path $ConfigDir "config.yml"

Write-Host "Starting $AppName on Windows..." -ForegroundColor Cyan

# Check for Go
if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Error "Go is not installed or not in PATH. Please install Go to build Wings."
    exit 1
}

# Create Build Directory
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir | Out-Null
}

# Build Wings
Write-Host "Building Wings..." -ForegroundColor Yellow
try {
    go build -tags windows -o (Join-Path $BinDir $BinName) .
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed"
    }
    Write-Host "Build successful." -ForegroundColor Green
} catch {
    Write-Error "Failed to build Wings: $_"
    exit 1
}

# Check Config
if (-not (Test-Path $ConfigFile)) {
    Write-Warning "Configuration file not found at $ConfigFile"
    Write-Host "Please ensure you have configured Wings properly."
}

# Run Wings
Write-Host "Running Wings..." -ForegroundColor Cyan
$ExePath = Join-Path (Join-Path $PWD $BinDir) $BinName
Start-Process -FilePath $ExePath -ArgumentList "--debug" -NoNewWindow -Wait

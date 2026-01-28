$ErrorActionPreference = "Stop"

Write-Host "Building frontend for production..." -ForegroundColor Cyan
cd panel
npm run build
if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed."
    exit 1
}

Write-Host "Starting production server..." -ForegroundColor Green
npm start

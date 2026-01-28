$ErrorActionPreference = "Continue"

Write-Host "Starting PropelPanel Environment..." -ForegroundColor Cyan

# Start Wings in a new window
Write-Host "Starting Wings..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd wings; ./propel.exe --debug"

# Start Panel in current window (or new one)
Write-Host "Starting Panel (Development Mode)..." -ForegroundColor Yellow
cd panel
npm run dev

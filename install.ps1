# PropelWings (FeatherWings) Windows Installer
# Version: 1.0.0
# Copyright (C) 2026 PriyxStudio

$ErrorActionPreference = "Continue"

# Check for admin privileges
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!" -ForegroundColor Red
    Write-Host "   ERROR: ADMINISTRATOR PRIVILEGES REQUIRED" -ForegroundColor Red
    Write-Host "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!" -ForegroundColor Red
    Write-Host "This script must be run as Administrator to register services."
    Write-Host "Please restart your terminal as Administrator."
    Pause
    exit 1
}

Clear-Host
Write-Host " ___                   _ ____                  _" -ForegroundColor Cyan
Write-Host "| _ \_ _ ___ _ __  ___| |  _ \ __ _ _ __   ___| |" -ForegroundColor Cyan
Write-Host "|  _/ '_/ _ \ '_ \/ -_) |  __/ _' | '_ \ / _ \ |" -ForegroundColor Cyan
Write-Host "|_| |_| \___/ .__/\___|_|_|  \__,_|_| |_|\___|_|" -ForegroundColor Cyan
Write-Host "            |_|                                  "
Write-Host "==================================================" -ForegroundColor DarkGray
Write-Host "       Welcome to PropelWings Installer" -ForegroundColor White
Write-Host "==================================================" -ForegroundColor DarkGray
Write-Host ""

$InstallDir = $PSScriptRoot
$WingsExe = Join-Path $InstallDir "propel.exe"
$ScriptsDir = Join-Path $InstallDir "bin"

# 1. Prerequisite Check
Write-Host "[*] Checking prerequisites..." -ForegroundColor Yellow

function Check-Command {
    param([string]$cmd, [string]$name)
    if (Get-Command $cmd -ErrorAction SilentlyContinue) {
        Write-Host "[√] $name found." -ForegroundColor Green
        return $true
    } else {
        Write-Host "[X] $name NOT found." -ForegroundColor Red
        return $false
    }
}

$prereqs = $true
if (-not (Check-Command "go" "Go (Golang)")) { $prereqs = $false }
if (-not (Check-Command "git" "Git")) { $prereqs = $false }

if (-not $prereqs) {
    Write-Host "Please install missing prerequisites and try again." -ForegroundColor Red
    exit 1
}

# 2. Directory Setup
if (-not (Test-Path $ScriptsDir)) {
    New-Item -ItemType Directory -Path $ScriptsDir | Out-Null
}

# 3. Build Propel Wings
Write-Host "`n[*] Building Propel Wings..." -ForegroundColor Cyan
if (Test-Path "wings.go") {
    go mod tidy
    go build -trimpath -ldflags="-s -w" -o propel.exe wings.go
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[-] Build failed!" -ForegroundColor Red
        exit 1
    }
    Write-Host "[+] Build successful: $WingsExe" -ForegroundColor Green
} else {
    Write-Host "[!] wings.go not found in current directory. Searching..." -ForegroundColor Yellow
    # Fallback to copy existing propel.exe if it exists
    if (-not (Test-Path "propel.exe")) {
        Write-Host "[-] Could not find wings.go or propel.exe" -ForegroundColor Red
        exit 1
    }
}

# 4. Service Registration
Write-Host "`n[*] Registering Windows Service..." -ForegroundColor Cyan

# Remove old if exists
sc.exe stop PropelWings 2>$null | Out-Null
sc.exe delete PropelWings 2>$null | Out-Null

sc.exe create PropelWings binPath= "`"$WingsExe`"" start= auto
sc.exe description PropelWings "PropelWings Daemon Service"

# 5. CLI Setup
Write-Host "`n[*] Setting up CLI Command..." -ForegroundColor Cyan

$PropelBat = @"
@echo off
set ACTION=%1
if "%ACTION%"=="" set ACTION=-status
if "%ACTION%"=="status" sc query PropelWings
if "%ACTION%"=="-status" sc query PropelWings
if "%ACTION%"=="start" sc start PropelWings
if "%ACTION%"=="-start" sc start PropelWings
if "%ACTION%"=="stop" sc stop PropelWings
if "%ACTION%"=="-stop" sc stop PropelWings
if "%ACTION%"=="restart" goto restart_propel
if "%ACTION%"=="-restart" goto restart_propel
goto end

:restart_propel
sc stop PropelWings && sc start PropelWings
goto end

:end
"@

$PropelBat | Out-File (Join-Path $ScriptsDir "propel.bat") -Encoding ASCII

# Add to PATH
$currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($currentPath -notlike "*$ScriptsDir*") {
    $newPath = "$currentPath;$ScriptsDir"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "Machine")
    Write-Host "[+] Added $ScriptsDir to System PATH." -ForegroundColor Green
    $env:Path += ";$ScriptsDir"
}

Write-Host "`n==================================================" -ForegroundColor Green
Write-Host "   PropelWings Installation Complete!" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Green
Write-Host "Command available:"
Write-Host " - propel status"
Write-Host ""
Write-Host "Service registered: PropelWings"
Write-Host "=================================================="
Write-Host "Please RESTART your terminal to use 'propel' command."
Pause

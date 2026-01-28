$ErrorActionPreference = "Stop"

Write-Host " ___                   _ ____                  _" -ForegroundColor Cyan
Write-Host "| _ \_ _ ___ _ __  ___| |  _ \ __ _ _ __   ___| |" -ForegroundColor Cyan
Write-Host "|  _/ '_/ _ \ '_ \/ -_) |  __/ _' | '_ \ / -_) |" -ForegroundColor Cyan
Write-Host "|_| |_| \___/ .__/\___|_|_|  \__,_|_| |_|\___|_|" -ForegroundColor Cyan
Write-Host "            |_|                                  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor DarkGray
Write-Host "       Welcome to PropelPanel Installer" -ForegroundColor White
Write-Host "==================================================" -ForegroundColor DarkGray
Write-Host ""

# Check prerequisites
Write-Host "[*] Checking prerequisites..." -ForegroundColor Yellow

if (-not (Get-Command "npm" -ErrorAction SilentlyContinue)) {
    Write-Error "Node.js (npm) is not installed. Please install Node.js (v18+) first."
    exit 1
}

if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Error "Go (Golang) is not installed. Please install Go (v1.21+) first."
    exit 1
}

Write-Host "[+] Prerequisites found." -ForegroundColor Green
Write-Host ""

# Web Server Setup Logic
$setupWebServer = Read-Host "Do you want to setup a web server proxy (Nginx/Apache)? (y/n)"
if ($setupWebServer -eq 'y') {
    $serverType = Read-Host "Which web server are you using? (nginx/apache)"
    if ($serverType -eq 'nginx') {
        Write-Host "Generating Nginx configuration snippet..." -ForegroundColor Cyan
        $domain = Read-Host "Enter your domain name (e.g. panel.example.com)"
        $port = Read-Host "Enter backend port [3000]"
        if ($port -eq "") { $port = "3000" }

        $nginxConfig = @"
server {
    listen 80;
    server_name $domain;

    location / {
        proxy_pass http://localhost:$port;
        proxy_http_version 1.1;
        proxy_set_header Upgrade `$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host `$host;
        proxy_cache_bypass `$http_upgrade;
    }
}
"@
        $nginxPath = "nginx_$domain.conf"
        Set-Content -Path $nginxPath -Value $nginxConfig
        Write-Host "[+] Nginx configuration saved to $nginxPath" -ForegroundColor Green
        Write-Host "(!) Please copy this file to your Nginx sites-available/conf.d folder and reload Nginx." -ForegroundColor Yellow
    }
    elseif ($serverType -eq 'apache') {
        Write-Host "Apache configuration generation is not yet fully automatic." -ForegroundColor Yellow
        Write-Host "Please ensure you have mod_proxy and mod_proxy_http enabled." -ForegroundColor Gray
    }
}

Write-Host ""
Write-Host "[*] Installing Panel dependencies..." -ForegroundColor Cyan
Push-Location panel
npm install
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to install panel dependencies."
    Pop-Location
    exit 1
}
Pop-Location
Write-Host "[+] Panel dependencies installed." -ForegroundColor Green

Write-Host ""
Write-Host "[*] Building Wings (Daemon)..." -ForegroundColor Cyan
Push-Location wings
go mod tidy
go build -trimpath -ldflags="-s -w" -o propel.exe wings.go
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build Wings."
    Pop-Location
    exit 1
}
Pop-Location
Write-Host "[+] Wings built successfully." -ForegroundColor Green

Write-Host ""
Write-Host "==================================================" -ForegroundColor DarkGray
Write-Host "   Installation Complete!" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor DarkGray
Write-Host "1. Configure your database in panel/.env (if needed)"
Write-Host "2. Run 'start-panel-dev.ps1' to start the panel in dev mode"
Write-Host "3. Run 'start.ps1' to start both wings and panel"
Write-Host "==================================================" -ForegroundColor DarkGray

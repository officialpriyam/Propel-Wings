#!/bin/bash
set -e

echo -e "\e[36m ___                   _ ____                  _"
echo -e "| _ \_ _ ___ _ __  ___| |  _ \ __ _ _ __   ___| |"
echo -e "|  _/ '_/ _ \ '_ \/ -_) |  __/ _' | '_ \ / -_) |"
echo -e "|_| |_| \___/ .__/\___|_|_|  \__,_|_| |_|\___|_|"
echo -e "            |_|                                  \e[0m"
echo "=================================================="
echo "       Welcome to PropelPanel Installer"
echo "=================================================="
echo ""

# Check prerequisites
echo -e "\e[33m[*] Checking prerequisites...\e[0m"

if ! command -v npm &> /dev/null; then
    echo "Node.js (npm) is not installed. Please install Node.js (v18+) first."
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "Go (Golang) is not installed. Please install Go (v1.21+) first."
    exit 1
fi

echo -e "\e[32m[+] Prerequisites found.\e[0m"
echo ""

# Web Server Setup Logic
read -p "Do you want to setup a web server proxy (Nginx/Apache)? (y/n) " setupWebServer
if [ "$setupWebServer" = "y" ]; then
    read -p "Which web server are you using? (nginx/apache) " serverType
    if [ "$serverType" = "nginx" ]; then
        echo -e "\e[36mGenerating Nginx configuration snippet...\e[0m"
        read -p "Enter your domain name (e.g. panel.example.com): " domain
        read -p "Enter backend port [3000]: " port
        port=${port:-3000}

        cat > "nginx_${domain}.conf" <<EOF
server {
    listen 80;
    server_name $domain;

    location / {
        proxy_pass http://localhost:$port;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_cache_bypass \$http_upgrade;
    }
}
EOF
        echo -e "\e[32m[+] Nginx configuration saved to nginx_${domain}.conf\e[0m"
        echo -e "\e[33m(!) Please copy this file to /etc/nginx/sites-available/ and link to sites-enabled, then reload Nginx.\e[0m"
    elif [ "$serverType" = "apache" ]; then
        echo -e "\e[33mApache configuration generation is not yet fully automatic.\e[0m"
    fi
fi

echo ""
echo -e "\e[36m[*] Installing Panel dependencies...\e[0m"
cd panel
npm install
cd ..
echo -e "\e[32m[+] Panel dependencies installed.\e[0m"

echo ""
echo -e "\e[36m[*] Building Wings (Daemon)...\e[0m"
cd wings
go mod tidy
go build -trimpath -ldflags="-s -w" -o propel wings.go
cd ..
echo -e "\e[32m[+] Wings built successfully.\e[0m"

echo ""
echo "=================================================="
echo -e "\e[32m   Installation Complete!\e[0m"
echo "=================================================="
echo "1. Configure your database in panel/.env (if needed)"
echo "2. Run './start-panel-dev.sh' to start the panel in dev mode" # Assuming .sh for uniformity though user requested ps1 for dev? I'll make sh versions too
echo "3. Run './start.sh' to start both wings and panel"
echo "=================================================="

# Grant execution permissions
chmod +x start.sh start-panel-dev.sh start-produc.sh wings/propel

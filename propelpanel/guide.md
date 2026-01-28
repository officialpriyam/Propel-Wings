# PropelPanel Installation & Usage Guide

Welcome to the PropelPanel distribution package. This folder contains everything you need to set up your own game server management panel.

## 📂 Directory Structure

- **panel/**: Source code for the Frontend/Backend (Next.js)
- **wings/**: Source code for the Daemon (Go)
- **install.ps1 / install.sh**: Scripts to install dependencies and build binaries.
- **start.ps1 / start.sh**: Scripts to launch the complete environment (Wings + Panel).

## 🚀 Installation

### Windows
1. Open PowerShell as Administrator (recommended).
2. Run the installer script:
   ```powershell
   ./install.ps1
   ```
3. Follow the prompts. It will checks for Node.js/Go, install dependencies, and build the Wings binary.
4. You can optionally choose to generate an Nginx configuration snippet if you plan to use a reverse proxy.

### Linux
1. Open your terminal.
2. Make sure scripts are executable:
   ```bash
   chmod +x install.sh start.sh start-panel-dev.sh start-produc.sh
   ```
3. Run the installer:
   ```bash
   ./install.sh
   ```

## ▶️ Running the Panel

### Development Mode
Use this for testing or modifying the code.
- **Windows**: Run `./start-panel-dev.ps1` (just Panel) or `./start.ps1` (Panel + Wings).
- **Linux**: Run `./start-panel-dev.sh` or `./start.sh`.

### Production Mode
Use this for a live server.
- **Windows**: Run `./start-produc.ps1`. This will build the frontend and start the optimized server.
- **Linux**: Run `./start-produc.sh`.

## ⚙️ Configuration

- **Panel**: Edit `panel/.env` to configure your database connection and other settings.
- **Wings**: Edit `wings/config.json` (created after first run) to configure ports and SSL.

---
*Powered by PriyxStudio*

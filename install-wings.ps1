$ErrorActionPreference = "Stop"

$ProgramData = $env:ProgramData
if (-not $ProgramData) {
    $ProgramData = "C:\ProgramData"
}

$BaseDir = Join-Path $ProgramData "OrbitPanel"
$LogDir = Join-Path $BaseDir "logs"
$DataDir = Join-Path $BaseDir "volumes"
$ArchiveDir = Join-Path $BaseDir "archives"
$BackupDir = Join-Path $BaseDir "backups"
$TmpDir = Join-Path $env:TEMP "FeatherWings"

Write-Host "Installing FeatherWings directory structure..." -ForegroundColor Cyan

$Directories = @($BaseDir, $LogDir, $DataDir, $ArchiveDir, $BackupDir, $TmpDir)

foreach ($Dir in $Directories) {
    if (-not (Test-Path $Dir)) {
        New-Item -ItemType Directory -Path $Dir | Out-Null
        Write-Host "Created: $Dir" -ForegroundColor Green
    } else {
        Write-Host "Exists: $Dir" -ForegroundColor Gray
    }
}

Write-Host "Installation structure created successfully." -ForegroundColor Green
Write-Host "Config file should be placed at: $(Join-Path $BaseDir 'config.yml')"

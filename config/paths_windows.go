//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// Platform-specific path defaults for Windows

// GetDefaultConfigLocation returns the default configuration file path for Windows.
func GetDefaultConfigLocation() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "config.yml")
}

// GetDefaultRootDirectory returns the default root directory for Windows.
func GetDefaultRootDirectory() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel")
}

// GetDefaultLogDirectory returns the default log directory for Windows.
func GetDefaultLogDirectory() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "logs")
}

// GetDefaultDataDirectory returns the default data directory for Windows.
func GetDefaultDataDirectory() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "volumes")
}

// GetDefaultArchiveDirectory returns the default archive directory for Windows.
func GetDefaultArchiveDirectory() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "archives")
}

// GetDefaultBackupDirectory returns the default backup directory for Windows.
func GetDefaultBackupDirectory() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "backups")
}

// GetDefaultTmpDirectory returns the default temporary directory for Windows.
func GetDefaultTmpDirectory() string {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.Getenv("TMP")
	}
	if tempDir == "" {
		tempDir = "C:\\Windows\\Temp"
	}
	return filepath.Join(tempDir, "Propel")
}

// GetDefaultPasswdFile returns the default passwd file path for Windows.
// On Windows, this is not used in the same way as Linux, but we still need a path.
func GetDefaultPasswdFile() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "passwd")
}

// GetDefaultMachineIDDirectory returns the default machine-id directory for Windows.
func GetDefaultMachineIDDirectory() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}
	return filepath.Join(programData, "Propel", "machine-id")
}

// GetDefaultHostShell returns the default shell for host terminal on Windows.
func GetDefaultHostShell() string {
	return "powershell.exe"
}

// GetDefaultNginxConfigPath returns the default nginx config path for Windows.
// Nginx on Windows typically uses a different path structure.
func GetDefaultNginxConfigPath() string {
	return "C:\\nginx\\conf\\propel-fastdl.conf"
}



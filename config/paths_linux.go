//go:build linux

package config

// Platform-specific path defaults for Linux

// GetDefaultConfigLocation returns the default configuration file path for Linux.
func GetDefaultConfigLocation() string {
	return "/etc/propel/config.yml"
}

// GetDefaultRootDirectory returns the default root directory for Linux.
func GetDefaultRootDirectory() string {
	return "/var/lib/propel"
}

// GetDefaultLogDirectory returns the default log directory for Linux.
func GetDefaultLogDirectory() string {
	return "/var/log/propel"
}

// GetDefaultDataDirectory returns the default data directory for Linux.
func GetDefaultDataDirectory() string {
	return "/var/lib/propel/volumes"
}

// GetDefaultArchiveDirectory returns the default archive directory for Linux.
func GetDefaultArchiveDirectory() string {
	return "/var/lib/propel/archives"
}

// GetDefaultBackupDirectory returns the default backup directory for Linux.
func GetDefaultBackupDirectory() string {
	return "/var/lib/propel/backups"
}

// GetDefaultTmpDirectory returns the default temporary directory for Linux.
func GetDefaultTmpDirectory() string {
	return "/tmp/propel"
}

// GetDefaultPasswdFile returns the default passwd file path for Linux.
func GetDefaultPasswdFile() string {
	return "/etc/propel/passwd"
}

// GetDefaultMachineIDDirectory returns the default machine-id directory for Linux.
func GetDefaultMachineIDDirectory() string {
	return "/run/wings/machine-id"
}

// GetDefaultHostShell returns the default shell for host terminal on Linux.
func GetDefaultHostShell() string {
	return "/bin/bash"
}

// GetDefaultNginxConfigPath returns the default nginx config path for Linux.
func GetDefaultNginxConfigPath() string {
	return "/etc/nginx/sites-available/propel-fastdl"
}



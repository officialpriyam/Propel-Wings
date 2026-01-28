//go:build windows

package config

import (
	"runtime"
)

// applyPlatformDefaults applies Windows-specific default values to the configuration.
// This is called after the struct defaults are set to override Linux-specific defaults.
func applyPlatformDefaults(c *Configuration) {
	// Only apply if values are still at Linux defaults
	if c.System.RootDirectory == "/var/lib/propel" || c.System.RootDirectory == "" {
		c.System.RootDirectory = GetDefaultRootDirectory()
	}
	if c.System.LogDirectory == "/var/log/propel" || c.System.LogDirectory == "" {
		c.System.LogDirectory = GetDefaultLogDirectory()
	}
	if c.System.Data == "/var/lib/propel/volumes" || c.System.Data == "" {
		c.System.Data = GetDefaultDataDirectory()
	}
	if c.System.ArchiveDirectory == "/var/lib/propel/archives" || c.System.ArchiveDirectory == "" {
		c.System.ArchiveDirectory = GetDefaultArchiveDirectory()
	}
	if c.System.BackupDirectory == "/var/lib/propel/backups" || c.System.BackupDirectory == "" {
		c.System.BackupDirectory = GetDefaultBackupDirectory()
	}
	if c.System.TmpDirectory == "/tmp/propel" || c.System.TmpDirectory == "" {
		c.System.TmpDirectory = GetDefaultTmpDirectory()
	}
	if c.System.User.PasswdFile == "/etc/propel/passwd" || c.System.User.PasswdFile == "" {
		c.System.User.PasswdFile = GetDefaultPasswdFile()
	}
	if c.System.MachineID.Directory == "/run/wings/machine-id" || c.System.MachineID.Directory == "" {
		c.System.MachineID.Directory = GetDefaultMachineIDDirectory()
	}
	if c.System.HostTerminal.Shell == "/bin/bash" || c.System.HostTerminal.Shell == "" {
		c.System.HostTerminal.Shell = GetDefaultHostShell()
	}
	if c.System.FastDL.NginxConfigPath == "/etc/nginx/sites-available/propel-fastdl" || c.System.FastDL.NginxConfigPath == "" {
		c.System.FastDL.NginxConfigPath = GetDefaultNginxConfigPath()
	}
}

// IsWindows returns true if running on Windows
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsLinux returns true if running on Linux
func IsLinux() bool {
	return runtime.GOOS == "linux"
}



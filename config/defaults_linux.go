//go:build linux

package config

import (
	"runtime"
)

// applyPlatformDefaults applies Linux-specific default values to the configuration.
// On Linux, the struct tag defaults are already correct, so this is mostly a no-op.
func applyPlatformDefaults(c *Configuration) {
	// Linux defaults are already set via struct tags, but we apply them here for consistency
	if c.System.RootDirectory == "" {
		c.System.RootDirectory = GetDefaultRootDirectory()
	}
	if c.System.LogDirectory == "" {
		c.System.LogDirectory = GetDefaultLogDirectory()
	}
	if c.System.Data == "" {
		c.System.Data = GetDefaultDataDirectory()
	}
	if c.System.ArchiveDirectory == "" {
		c.System.ArchiveDirectory = GetDefaultArchiveDirectory()
	}
	if c.System.BackupDirectory == "" {
		c.System.BackupDirectory = GetDefaultBackupDirectory()
	}
	if c.System.TmpDirectory == "" {
		c.System.TmpDirectory = GetDefaultTmpDirectory()
	}
	if c.System.User.PasswdFile == "" {
		c.System.User.PasswdFile = GetDefaultPasswdFile()
	}
	if c.System.MachineID.Directory == "" {
		c.System.MachineID.Directory = GetDefaultMachineIDDirectory()
	}
	if c.System.HostTerminal.Shell == "" {
		c.System.HostTerminal.Shell = GetDefaultHostShell()
	}
	if c.System.FastDL.NginxConfigPath == "" {
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


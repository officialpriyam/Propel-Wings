//go:build windows

package config

import (
	"github.com/apex/log"
)

// EnsureFeatherUser on Windows is a simplified check.
// We don't create system users like on Linux. We rely on the process runner's identity.
func EnsureFeatherUser() error {
	log.Debug("user creation is skipped on Windows - running as current user")
	// Set default UID/GID to 0 or some value, but it's not really used for file permissions on NTFS in the same way.
	// For Docker mount mapping, Windows handles it differently.
	_config.System.User.Uid = 0
	_config.System.User.Gid = 0
	return nil
}


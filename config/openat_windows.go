//go:build windows

package config

// UseOpenat2 returns false on Windows as openat2 is a Linux-specific syscall.
func UseOpenat2() bool {
	return false
}


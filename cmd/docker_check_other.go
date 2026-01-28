//go:build !linux

package cmd

// isDockerSnap is a no-op on non-Linux platforms as Snap is Linux-specific.
func isDockerSnap() bool {
	return false
}


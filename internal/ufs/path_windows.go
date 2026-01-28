//go:build windows

package ufs

import "os"

// endsWithDot reports whether the final component of path is ".".
func endsWithDot(path string) bool {
	if path == "." {
		return true
	}
	if len(path) >= 2 && path[len(path)-1] == '.' && os.IsPathSeparator(path[len(path)-2]) {
		return true
	}
	return false
}

func basename(name string) string {
	// Not strictly used by my simple implementation but might be needed by others?
	// path_unix.go has it.
	// But let's only add if needed.
	return ""
}


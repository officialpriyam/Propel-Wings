//go:build windows

package ufs

import (
	"syscall"
)

// errnoToPathError converts an errno into a proper path error.
// On Windows, syscall.Errno matches Windows error codes.
func errnoToPathError(err syscall.Errno, op, path string) error {
	switch err {
	case syscall.ERROR_FILE_EXISTS, syscall.ERROR_ALREADY_EXISTS:
		return &PathError{Op: op, Path: path, Err: ErrExist}
	case syscall.ERROR_PATH_NOT_FOUND, syscall.ERROR_FILE_NOT_FOUND:
		return &PathError{Op: op, Path: path, Err: ErrNotExist}
	case syscall.ERROR_ACCESS_DENIED:
		return &PathError{Op: op, Path: path, Err: ErrPermission}
	default:
		return &PathError{Op: op, Path: path, Err: err}
	}
}


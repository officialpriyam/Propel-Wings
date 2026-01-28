//go:build windows

package ufs

import (
	"errors"
	"path/filepath"
)

type unixFS interface {
	Open(name string) (File, error)
	Remove(name string) error
	unlinkat(dirfd int, path string, flags int) error
}

func (fs *UnixFS) removeAll(path string) error {
	return removeAll(fs, path)
}

func (fs *UnixFS) removeContents(path string) error {
	return removeContents(fs, path)
}

func removeAll(fs unixFS, path string) error {
	if path == "" {
		return nil
	}
	if endsWithDot(path) {
		return &PathError{Op: "removeall", Path: path, Err: errors.New("invalid argument")}
	}

	// 1. Try Remove first (handles files and empty dirs)
	err := fs.Remove(path)
	if err == nil || errors.Is(err, ErrNotExist) {
		return nil
	}

	// 2. Likely a non-empty directory, recurse.
	if err := removeContents(fs, path); err != nil {
		return err
	}

	// 3. Remove the now-empty directory
	return fs.Remove(path)
}

func removeContents(fs unixFS, path string) error {
	f, err := fs.Open(path)
	if err != nil {
		if errors.Is(err, ErrNotExist) {
			return nil
		}
		return err
	}
	// Read names.
	// Note: We might want to use Readdir to get types to avoid Stat in Remove?
	// But fs.Remove calls RemoveStat anyway.
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return err
	}

	for _, name := range names {
		childPath := filepath.Join(path, name)
		if err := removeAll(fs, childPath); err != nil {
			return err
		}
	}
	return nil
}


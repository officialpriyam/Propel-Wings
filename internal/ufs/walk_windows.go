//go:build windows

package ufs

import (
)

// ReadDirMap reads the directory named by dirname and returns a list of
// directory entries transformed by the mapping function fn.
func ReadDirMap[T any](fs *UnixFS, path string, fn func(DirEntry) (T, error)) ([]T, error) {
	// On Windows, we can just use os.ReadDir via fs.ReadDir (which uses os.ReadDir).
	// But fs.ReadDir returns []DirEntry.
	entries, err := fs.ReadDir(path)
	if err != nil {
		return nil, err
	}

	out := make([]T, len(entries))
	for i, e := range entries {
		v, err := fn(e)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// WalkDirat is a stub for Windows since we don't have dirfd.
// We can implement it using standard WalkDir if needed, or just standard filepath walk.
// usage in filesystem.go: fs.unixFS.WalkDirat(dirfd, name, func(...) )
// We implemented WalkDirat on UnixFS in fs_windows.go?
// Let's check fs_windows.go. I added a stub there earlier?
// Re-reading fs_windows.go content from memory/history...
// In step 36 I saw:
// func (fs *UnixFS) WalkDirat(dirfd int, path string, fn func(dirfd int, path, _ string, d DirEntry, err error) error) error {
// ... implementation using filepath.WalkDir ...
// }
// So WalkDirat is already in fs_windows.go.


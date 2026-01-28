//go:build windows

package filesystem

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"emperror.dev/errors"
)

// DirectorySize calculates the size of a directory and its descendants.
// Windows implementation uses standard filepath.WalkDir.
func (fs *Filesystem) DirectorySize(root string) (int64, error) {
	// fs.Path() should give the absolute path to the server root
	fullPath := filepath.Join(fs.Path(), root)
	
	var size atomic.Int64

	err := filepath.WalkDir(fullPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Get file info to check size
		info, err := d.Info()
		if err != nil {
			return nil // Skip files we can't stat
		}

		// Add size
		size.Add(info.Size())
		return nil
	})

	return size.Load(), errors.WrapIf(err, "server/filesystem: directorysize: failed to walk directory")
}


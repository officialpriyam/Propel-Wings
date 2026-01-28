//go:build windows

package ufs

import (
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

)

type UnixFS struct {
	basePath string
}

func NewUnixFS(basePath string, _ bool) (*UnixFS, error) {
	basePath = strings.TrimSuffix(basePath, "/")
	cleanPath := filepath.Clean(basePath)
	return &UnixFS{
		basePath: cleanPath,
	}, nil
}

func (fs *UnixFS) BasePath() string {
	return fs.basePath
}

func (fs *UnixFS) Close() error {
	return nil
}

func (fs *UnixFS) Chmod(name string, mode FileMode) error {
	path, err := fs.safePath(name)
	if err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func (fs *UnixFS) Chown(name string, uid, gid int) error {
	// Windows doesn't support chown in the same way.
	return nil
}

func (fs *UnixFS) Lchown(name string, uid, gid int) error {
	// Windows doesn't support lchown.
	return nil
}

func (fs *UnixFS) Chtimes(name string, atime, mtime time.Time) error {
	path, err := fs.safePath(name)
	if err != nil {
		return err
	}
	return os.Chtimes(path, atime, mtime)
}

func (fs *UnixFS) Create(name string) (File, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.Create(path)
}

func (fs *UnixFS) Mkdir(name string, perm FileMode) error {
	path, err := fs.safePath(name)
	if err != nil {
		return err
	}
	return os.Mkdir(path, perm)
}

func (fs *UnixFS) MkdirAll(path string, perm FileMode) error {
	safePath, err := fs.safePath(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(safePath, perm)
}

func (fs *UnixFS) Open(name string) (File, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (fs *UnixFS) OpenFile(name string, flag int, perm FileMode) (File, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, perm)
}

func (fs *UnixFS) ReadDir(name string) ([]DirEntry, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(path)
}

func (fs *UnixFS) Remove(name string) error {
	path, err := fs.safePath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (fs *UnixFS) RemoveAll(path string) error {
	safePath, err := fs.safePath(path)
	if err != nil {
		return err
	}
	// Check against base path removal
	if safePath == fs.basePath {
		return &PathError{Op: "removeall", Path: path, Err: ErrBadPathResolution}
	}
	return os.RemoveAll(safePath)
}

func (fs *UnixFS) Rename(oldname, newname string) error {
	oldPath, err := fs.safePath(oldname)
	if err != nil {
		return err
	}
	newPath, err := fs.safePath(newname)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (fs *UnixFS) Stat(name string) (FileInfo, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (fs *UnixFS) Lstat(name string) (FileInfo, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.Lstat(path)
}

func (fs *UnixFS) Symlink(oldname, newname string) error {
	newPath, err := fs.safePath(newname)
	if err != nil {
		return err
	}
	return os.Symlink(oldname, newPath)
}

func (fs *UnixFS) WalkDir(root string, fn WalkDirFunc) error {
	safeRoot, err := fs.safePath(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(safeRoot, func(path string, d iofs.DirEntry, err error) error {
		// Convert absolute path back to relative for the callback if necessary,
		// or just pass as is. The original Unix implementation might pass relative paths?
		// WalkDirat in unix passes names.
		// Standard filepath.WalkDir passes absolute path if we start with absolute.
		// We'll pass whatever WalkDir gives.
		return fn(path, d, err)
	})
}

// safePath validates that the path is within the base path.
func (fs *UnixFS) safePath(path string) (string, error) {
	// If path is absolute, strip base path if it starts with it?
	// Or treat input as relative to base path.
	// ufs seems to treat input as relative usually.
	
	joined := filepath.Join(fs.basePath, path)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	// Verify it's inside base path
	if !strings.HasPrefix(strings.ToLower(abs), strings.ToLower(fs.basePath)) {
		return "", &PathError{Op: "safePath", Path: path, Err: ErrBadPathResolution}
	}

	return abs, nil
}

// Additional methods required by disk_space.go or other callers if they use specific methods.
// disk_space.go uses: SafePath (returning 3 args), WalkDirat, Lstatat.
// Since these are on *UnixFS struct, I should implement them to match the struct usage if disk_space.go is not updated.
// But I plan to update disk_space.go to be platform specific.
// However, UnixFS methods on Windows should probably match the signature if possible to minimize churn,
// but returning `dirfd` (int) is meaningless on Windows.
//
// So it is better to refactor safePath usage in disk_space.go.


// WalkDirat is specific to Unix with dirfd. Windows implementation will just use path.
func (fs *UnixFS) WalkDirat(dirfd int, path string, fn func(dirfd int, path, _ string, d DirEntry, err error) error) error {
	// Ignore dirfd, use path.
	// We need to resolve path. If path is relative and dirfd is 0, we assume it's relative to what?
	// If SafePath returns absolute path as string, we use that.
	return filepath.WalkDir(path, func(p string, d iofs.DirEntry, err error) error {
		// Callback expects dirfd.
		return fn(0, p, "", d, err)
	})
}

// Lstatat stub
func (fs *UnixFS) Lstatat(dirfd int, name string) (FileInfo, error) {
	return os.Lstat(name)
}

// Touch is needed?
func (fs *UnixFS) Touch(path string, flag int, mode FileMode) (File, error) {
	if flag&O_CREATE == 0 {
		flag |= O_CREATE
	}
	safePath, err := fs.safePath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(safePath, flag, mode)
	if err != nil && os.IsNotExist(err) {
		// Ensure dir exists
		if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
			return nil, err
		}
		return os.OpenFile(safePath, flag, mode)
	}
	return f, err
}

// RemoveContents implementation
func (fs *UnixFS) RemoveContents(name string) error {
	safePath, err := fs.safePath(name)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(safePath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(safePath, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Needed to satisfy interface/usage?
// Limit and Usage are not in Filesystem interface but used by disk_space.go.
// They seem to be added to Filesystem struct in disk_space.go via composition?
// No, `fs.unixFS` is a field in `filesystem.Filesystem` struct.
// `server/filesystem/filesystem.go` probably defines `Filesystem` struct which embeds `*ufs.UnixFS`.
// RemoveStat is a combination of Stat and Remove.
func (fs *UnixFS) RemoveStat(name string) (FileInfo, error) {
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	s, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	err = os.Remove(path)
	if err != nil {
		return s, err
	}
	return s, nil
}

func (fs *UnixFS) unsafePath(path string) (string, error) {
	// Simple cleaning for Windows, ensuring it is within base path.
	// Since safePath returns absolute path, we can use that but we might want relative?
	// fs_quota expectation of unsafePath:
	// "This will also trim the filesystem's base path... and join the base path back"
	
	r := filepath.Clean(filepath.Join(fs.basePath, strings.TrimPrefix(path, fs.basePath)))
	if fs.unsafeIsPathInsideOfBase(r) {
		r = strings.TrimPrefix(strings.TrimPrefix(r, fs.basePath), "\\")
		if r == "" {
			return ".", nil
		}
		return r, nil
	}
	return "", &PathError{Op: "unsafePath", Path: path, Err: ErrBadPathResolution}
}

func (fs *UnixFS) unsafeIsPathInsideOfBase(path string) bool {
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(fs.basePath))
}

func (fs *UnixFS) unlinkat(dirfd int, name string, flags int) error {
	path, err := fs.safePath(name)
	if err != nil {
		return err
	}
	// flags are ignored on Windows for now (AT_REMOVEDIR vs 0)
	// But standard os.Remove handles both usually?
	// Actually os.Remove handles files and empty dirs.
	return os.Remove(path)
}

func (fs *UnixFS) OpenFileat(dirfd int, name string, flag int, mode FileMode) (File, error) {
	// Ignore dirfd, use name.
	path, err := fs.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, mode)
}

func (fs *UnixFS) Lchownat(dirfd int, name string, uid, gid int) error {
	return nil // Not supported on Windows
}

func (fs *UnixFS) TouchPath(path string) (int, string, func(), error, bool) {
	// Stub implementation to satisfy interface if needed?
	// fs_unix.go TouchPath returns (dirfd, name, closeFd, err, existed)
	// We'll just return simplified values.
	
	// Ensure directories exist
	safePath, err := fs.safePath(path)
	if err != nil {
		return 0, "", func(){}, err, false
	}
	
	dir := filepath.Dir(safePath)
	existed := true
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		existed = false
		if err := os.MkdirAll(dir, 0755); err != nil {
			return 0, "", func(){}, err, false
		}
	}
	
	return 0, path, func(){}, nil, existed
}

func (fs *UnixFS) SafePath(path string) (int, string, func(), error) {
	p, err := fs.safePath(path)
	return 0, p, func(){}, err
}


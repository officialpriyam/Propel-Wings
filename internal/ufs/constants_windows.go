//go:build windows

package ufs

import "syscall"

// Re-using the same names as Go's official `unix` and `os` package do.
// Mapped to standard syscall constants on Windows where available.
const (
	// O_RDONLY opens the file read-only.
	O_RDONLY = syscall.O_RDONLY
	// O_WRONLY opens the file write-only.
	O_WRONLY = syscall.O_WRONLY
	// O_RDWR opens the file read-write.
	O_RDWR = syscall.O_RDWR
	// O_APPEND appends data to the file when writing.
	O_APPEND = syscall.O_APPEND
	// O_CREATE creates a new file if it doesn't exist.
	O_CREATE = syscall.O_CREAT
	// O_EXCL is used with O_CREATE, file must not exist.
	O_EXCL = syscall.O_EXCL
	// O_SYNC open for synchronous I/O.
	O_SYNC = syscall.O_SYNC
	// O_TRUNC truncates regular writable file when opened.
	O_TRUNC = syscall.O_TRUNC
	
	// Windows doesn't map these exactly the same way or they are unused by os package
	// but we define them to satisfy build.
	O_DIRECTORY = 0 // Not used in Windows OpenFile
	O_NOFOLLOW  = 0 // Not typically supported in basic Open
	O_CLOEXEC   = syscall.O_CLOEXEC
	// O_LARGEFILE is not needed on Windows (files are large by default/64bit)
	O_LARGEFILE = 0 
)

const (
	// These AT_* constants are Unix specific for *at syscalls.
	// We define them as 0 for Windows to allow compilation of code that might reference them,
	// though logic using them should likely be guarded.
	AT_SYMLINK_NOFOLLOW = 0
	AT_REMOVEDIR        = 0
	AT_EMPTY_PATH       = 0
)


//go:build linux

package system

import (
	"syscall"

	"github.com/shirou/gopsutil/v3/disk"
)

// getDiskForPath finds the mountpoint where the given path is stored using syscall.Statfs
func getDiskForPath(path string, partitions []disk.PartitionStat) (string, string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "", "", err
	}

	for _, part := range partitions {
		var pStat syscall.Statfs_t
		if err := syscall.Statfs(part.Mountpoint, &pStat); err != nil {
			continue
		}
		if stat.Fsid == pStat.Fsid {
			return part.Device, part.Mountpoint, nil
		}
	}

	return "", "", nil // No error, but couldn't find the disk
}


//go:build windows

package system

import (
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v3/disk"
)

// getDiskForPath finds the mountpoint where the given path is stored for Windows.
// On Windows, mountpoints are typically drive letters (e.g., "C:").
func getDiskForPath(path string, partitions []disk.PartitionStat) (string, string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}

	// Simple drive letter matching for Windows
	volumeName := filepath.VolumeName(absPath)
	if volumeName == "" {
		return "", "", nil
	}

	// Ensure volume name ends with backslash for comparison with mountpoints if needed,
	// though gopsutil usually returns "C:" or "C:\".
	// Let's normalize to uppercase for comparison.
	volumeNameUpper := strings.ToUpper(volumeName)

	for _, part := range partitions {
		// part.Mountpoint usually is "C:" or "C:\"
		mountPoint := part.Mountpoint
		// Strip trailing backslash for comparison with VolumeName which usually doesn't have it (e.g. "C:")
		mountPointTrimmed := strings.TrimRight(mountPoint, "\\")

		if strings.ToUpper(mountPointTrimmed) == volumeNameUpper {
			return part.Device, part.Mountpoint, nil
		}
	}

	return "", "", nil
}


//go:build !linux && !windows

package system

func getKernelVersion() (string, error) {
	return "Unknown", nil
}

func getOperatingSystemName() (string, error) {
	return "Unknown", nil
}

func getSystemName() (string, error) {
	return "unknown", nil
}


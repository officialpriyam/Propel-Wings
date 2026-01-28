//go:build windows

package system

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func getKernelVersion() (string, error) {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber), nil
}

func getOperatingSystemName() (string, error) {
	// Standard Windows report
	return "Windows", nil
}

func getSystemName() (string, error) {
	return "windows", nil
}


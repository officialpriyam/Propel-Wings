//go:build linux

package system

import (
	"github.com/acobaugh/osrelease"
	"github.com/docker/docker/pkg/parsers/kernel"
)

func getKernelVersion() (string, error) {
	k, err := kernel.GetKernelVersion()
	if err != nil {
		return "", err
	}
	return k.String(), nil
}

func getOperatingSystemName() (string, error) {
	release, err := osrelease.Read()
	if err != nil {
		return "Linux", nil
	}

	if release["PRETTY_NAME"] != "" {
		return release["PRETTY_NAME"], nil
	} else if release["NAME"] != "" {
		return release["NAME"], nil
	}
	return "Linux", nil
}

func getSystemName() (string, error) {
	release, err := osrelease.Read()
	if err != nil {
		return "", err
	}
	return release["ID"], nil
}


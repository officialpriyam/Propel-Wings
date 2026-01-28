//go:build linux

package cmd

import (
	"context"
	"strings"

	"github.com/apex/log"
	"github.com/docker/docker/client"
)

// isDockerSnap checks if Docker is installed via Snap on Linux.
func isDockerSnap() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Unable to initialize Docker client: %s", err)
	}

	defer cli.Close()

	info, err := cli.Info(context.Background())
	if err != nil {
		log.Fatalf("Unable to get Docker info: %s", err)
	}

	// Check if Docker root directory contains '/var/snap/docker'
	return strings.Contains(info.DockerRootDir, "/var/snap/docker")
}


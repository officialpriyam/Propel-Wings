//go:build !linux

package system

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
)

func GetDockerDiskUsage(ctx context.Context) (*DockerDiskUsage, error) {
	// Try to get info, but fallback to empty if it fails
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return &DockerDiskUsage{}, nil
	}
	defer c.Close()

	d, err := c.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return &DockerDiskUsage{}, nil
	}

	// Logic from linux implementation
	var bcs int64
	for _, bc := range d.BuildCache {
		if !bc.Shared {
			bcs += bc.Size
		}
	}
	var a int64
	for _, i := range d.Images {
		if i.Containers > 0 {
			a++
		}
	}
	var cs int64
	for _, b := range d.Containers {
		cs += b.SizeRootFs
	}

	return &DockerDiskUsage{
		ImagesTotal:    len(d.Images),
		ImagesActive:   a,
		ImagesSize:     int64(d.LayersSize),
		ContainersSize: int64(cs),
		BuildCacheSize: bcs,
	}, nil
}

func PruneDockerImages(ctx context.Context) (image.PruneReport, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return image.PruneReport{}, nil
	}
	defer c.Close()

	prune, err := c.ImagesPrune(ctx, filters.Args{})
	if err != nil {
		return image.PruneReport{}, nil
	}
	return prune, nil
}

func GetDockerInfo(ctx context.Context) (types.Version, system.Info, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return types.Version{}, system.Info{}, nil
	}
	defer c.Close()

	dockerVersion, err := c.ServerVersion(ctx)
	if err != nil {
		// Ignore error and return empty version
		return types.Version{}, system.Info{}, nil
	}

	dockerInfo, err := c.Info(ctx)
	if err != nil {
		// Ignore error and return empty info
		return types.Version{}, system.Info{}, nil
	}

	return dockerVersion, dockerInfo, nil
}


//go:build linux

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
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return &DockerDiskUsage{}, err
	}
	defer c.Close()

	d, err := c.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return &DockerDiskUsage{}, err
	}

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
		return image.PruneReport{}, err
	}
	defer c.Close()

	prune, err := c.ImagesPrune(ctx, filters.Args{})
	if err != nil {
		return image.PruneReport{}, err
	}
	return prune, nil
}

func GetDockerInfo(ctx context.Context) (types.Version, system.Info, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return types.Version{}, system.Info{}, err
	}
	defer c.Close()

	dockerVersion, err := c.ServerVersion(ctx)
	if err != nil {
		return types.Version{}, system.Info{}, err
	}

	dockerInfo, err := c.Info(ctx)
	if err != nil {
		return types.Version{}, system.Info{}, err
	}

	return dockerVersion, dockerInfo, nil
}


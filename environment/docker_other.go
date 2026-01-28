//go:build !linux

package environment

import (
	"context"

	"emperror.dev/errors"
	"github.com/apex/log"
	"github.com/docker/docker/client"
)

// Docker returns a docker client. On non-Linux platforms, we return a client
// that will likely fail if used, but we try to initialize it from environment.
func Docker() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "environment/docker: could not create client")
	}
	return cli, nil
}

// ConfigureDocker is a no-op on non-Linux platforms to allow the system to boot
// without a working Docker daemon.
func ConfigureDocker(ctx context.Context) error {
	log.Info("skipping docker network configuration on non-linux platform")
	return nil
}


//go:build !windows

package process

import (
    "context"
    "time"
    "github.com/priyxstudio/propel/environment"
    "github.com/priyxstudio/propel/events"
)

type Metadata struct {}

type Process struct {}

func New(id string, meta *Metadata, conf *environment.Configuration) (*Process, error) {
    return nil, nil
}
// Stub methods to satisfy interface if needed, but really this package shouldn't be used on linux



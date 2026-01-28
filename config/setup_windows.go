//go:build windows

package config

import (
	"fmt"
	"time"

	"emperror.dev/errors"
)

// ConfigureTimezone for Windows.
// We mostly rely on Go's time package to pick up system time.
func ConfigureTimezone() error {
	if _config.System.Timezone == "" {
		// Default to Local if not specified
		_config.System.Timezone = "Local"
	}
	
	// Validate the timezone location
	_, err := time.LoadLocation(_config.System.Timezone)
	return errors.WithMessage(err, fmt.Sprintf("the supplied timezone %s is invalid", _config.System.Timezone))
}

// EnableLogRotation for Windows.
// Logrotate is not standard on Windows, so we skip this.
// Users can use other tools for log rotation if needed.
func EnableLogRotation() error {
	return nil
}


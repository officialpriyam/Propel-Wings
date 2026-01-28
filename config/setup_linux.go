//go:build linux

package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"text/template"
	"time"

	"emperror.dev/errors"
	"github.com/apex/log"
)

// ConfigureTimezone sets the timezone data for the configuration if it is
// currently missing. If a value has been set, this functionality will only run
// to validate that the timezone being used is valid.
func ConfigureTimezone() error {
	tz := os.Getenv("TZ")
	if _config.System.Timezone == "" && tz != "" {
		_config.System.Timezone = tz
	}
	if _config.System.Timezone == "" {
		b, err := os.ReadFile("/etc/timezone")
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.WithMessage(err, "config: failed to open timezone file")
			}

			_config.System.Timezone = "UTC"
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			// Okay, file isn't found on this OS, we will try using timedatectl to handle this. If this
			// command fails, exit, but if it returns a value use that. If no value is returned we will
			// fall through to UTC to get Wings booted at least.
			out, err := exec.CommandContext(ctx, "timedatectl").Output()
			if err != nil {
				log.WithField("error", err).Warn("failed to execute \"timedatectl\" to determine system timezone, falling back to UTC")
				return nil
			}

			r := regexp.MustCompile(`Time zone: ([\w/]+)`)
			matches := r.FindSubmatch(out)
			if len(matches) != 2 || string(matches[1]) == "" {
				log.Warn("failed to parse timezone from \"timedatectl\" output, falling back to UTC")
				return nil
			}
			_config.System.Timezone = string(matches[1])
		} else {
			_config.System.Timezone = string(b)
		}
	}

	_config.System.Timezone = regexp.MustCompile(`(?i)[^a-z_/]+`).ReplaceAllString(_config.System.Timezone, "")
	_, err := time.LoadLocation(_config.System.Timezone)

	return errors.WithMessage(err, fmt.Sprintf("the supplied timezone %s is invalid", _config.System.Timezone))
}

// EnableLogRotation writes a logrotate file for wings to the system logrotate
// configuration directory if one exists and a logrotate file is not found.
func EnableLogRotation() error {
	if !_config.System.EnableLogRotate {
		log.Info("skipping log rotate configuration, disabled in wings config file")
		return nil
	}

	if st, err := os.Stat("/etc/logrotate.d"); err != nil && !os.IsNotExist(err) {
		return err
	} else if (err != nil && os.IsNotExist(err)) || !st.IsDir() {
		return nil
	}
	if _, err := os.Stat("/etc/logrotate.d/wings"); err == nil || !os.IsNotExist(err) {
		return err
	}

	log.Info("no log rotation configuration found: adding file now")
	f, err := os.Create("/etc/logrotate.d/wings")
	if err != nil {
		return err
	}
	defer f.Close()

	t, err := template.New("logrotate").Parse(`{{.LogDirectory}}/wings.log {
    size 10M
    compress
    delaycompress
    dateext
    maxage 7
    missingok
    notifempty
    postrotate
        /usr/bin/systemctl kill -s HUP wings.service >/dev/null 2>&1 || true
    endscript
}`)
	if err != nil {
		return err
	}

	return errors.Wrap(t.Execute(f, _config.System), "config: failed to write logrotate to disk")
}


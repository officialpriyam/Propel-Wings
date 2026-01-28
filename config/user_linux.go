//go:build linux

package config

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/acobaugh/osrelease"
	"github.com/apex/log"
	"github.com/priyxstudio/propel/system"
)

// EnsureFeatherUser ensures that the Propel core user exists on the
// system. This user will be the owner of all data in the root data directory
// and is used as the user within containers. If files are not owned by this
// user there will be issues with permissions on Docker mount points.
func EnsureFeatherUser() error {
	sysName, err := getSystemName()
	if err != nil {
		return err
	}

	// Our way of detecting if wings is running inside of Docker.
	if sysName == "distroless" {
		_config.System.Username = system.FirstNotEmpty(os.Getenv("WINGS_USERNAME"), "propel")
		_config.System.User.Uid = system.MustInt(system.FirstNotEmpty(os.Getenv("WINGS_UID"), "988"))
		_config.System.User.Gid = system.MustInt(system.FirstNotEmpty(os.Getenv("WINGS_GID"), "988"))
		return nil
	}

	if _config.System.User.Rootless.Enabled {
		log.Info("rootless mode is enabled, skipping user creation...")
		u, err := user.Current()
		if err != nil {
			return err
		}
		_config.System.Username = u.Username
		_config.System.User.Uid = system.MustInt(u.Uid)
		_config.System.User.Gid = system.MustInt(u.Gid)
		return nil
	}

	log.WithField("username", _config.System.Username).Info("checking for feather system user")
	u, err := user.Lookup(_config.System.Username)
	// If an error is returned but it isn't the unknown user error just abort
	// the process entirely. If we did find a user, return it immediately.
	if err != nil {
		if _, ok := err.(user.UnknownUserError); !ok {
			return err
		}
	} else {
		_config.System.User.Uid = system.MustInt(u.Uid)
		_config.System.User.Gid = system.MustInt(u.Gid)
		return nil
	}

	command := fmt.Sprintf("useradd --system --no-create-home --shell /usr/sbin/nologin %s", _config.System.Username)
	// Alpine Linux is the only OS we currently support that doesn't work with the useradd
	// command, so in those cases we just modify the command a bit to work as expected.
	if strings.HasPrefix(sysName, "alpine") {
		command = fmt.Sprintf("adduser -S -D -H -G %[1]s -s /sbin/nologin %[1]s", _config.System.Username)
		// We have to create the group first on Alpine, so do that here before continuing on
		// to the user creation process.
		if _, err := exec.Command("addgroup", "-S", _config.System.Username).Output(); err != nil {
			return err
		}
	}

	split := strings.Split(command, " ")
	if _, err := exec.Command(split[0], split[1:]...).Output(); err != nil {
		return err
	}
	u, err = user.Lookup(_config.System.Username)
	if err != nil {
		return err
	}
	_config.System.User.Uid = system.MustInt(u.Uid)
	_config.System.User.Gid = system.MustInt(u.Gid)
	return nil
}

// Gets the system release name.
func getSystemName() (string, error) {
	// use osrelease to get release version and ID
	release, err := osrelease.Read()
	if err != nil {
		return "", err
	}
	return release["ID"], nil
}



package server

import (
	"bufio"
	"context"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"runtime"
	"net/http"
	"fmt"
	"encoding/json"
	"os/exec"

	"emperror.dev/errors"
	"github.com/apex/log"
	"github.com/docker/docker/api/types/container"
	dockerImage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/parsers/kernel"

	"github.com/priyxstudio/propel/config"
	"github.com/priyxstudio/propel/environment"
	"github.com/priyxstudio/propel/remote"
	"github.com/priyxstudio/propel/system"
)

// Install executes the installation stack for a server process. Bubbles any
// errors up to the calling function which should handle contacting the panel to
// notify it of the server state.
//
// Pass true as the first argument in order to execute a server sync before the
// process to ensure the latest information is used.
func (s *Server) Install() error {
	return s.install(false)
}

func (s *Server) install(reinstall bool) error {
	var err error
	if !s.Config().SkipEggScripts {
		// Send the start event so the Panel can automatically update. We don't
		// send this unless the process is actually going to run, otherwise all
		// sorts of weird rapid UI behavior happens since there isn't an actual
		// install process being executed.
		s.Events().Publish(InstallStartedEvent, "")

		err = s.internalInstall()
	} else {
		s.Log().Info("server configured to skip running installation scripts for this egg, not executing process")
	}

	s.Log().WithField("was_successful", err == nil).Debug("notifying panel of server install state")
	if serr := s.SyncInstallState(err == nil, reinstall); serr != nil {
		l := s.Log().WithField("was_successful", err == nil)

		// If the request was successful but there was an error with this request,
		// attach the error to this log entry. Otherwise, ignore it in this log
		// since whatever is calling this function should handle the error and
		// will end up logging the same one.
		if err == nil {
			l.WithField("error", err)
		}

		l.Warn("failed to notify panel of server install state")
	}

	// Ensure that the server is marked as offline at this point, otherwise you
	// end up with a blank value which is a bit confusing.
	s.Environment.SetState(environment.ProcessOfflineState)

	// Push an event to the websocket, so we can auto-refresh the information in
	// the panel once the installation is completed.
	s.Events().Publish(InstallCompletedEvent, "")

	return errors.WithStackIf(err)
}

// Reinstall reinstalls a server's software by utilizing the installation script
// for the server egg. This does not touch any existing files for the server,
// other than what the script modifies.
func (s *Server) Reinstall() error {
	if s.Environment.State() != environment.ProcessOfflineState {
		s.Log().Debug("waiting for server instance to enter a stopped state")
		if err := s.Environment.WaitForStop(s.Context(), time.Second*10, true); err != nil {
			return errors.WrapIf(err, "install: failed to stop running environment")
		}
	}

	s.Log().Info("syncing server state with remote source before executing re-installation process")
	if err := s.Sync(); err != nil {
		return errors.WrapIf(err, "install: failed to sync server state with Panel")
	}

	return s.install(true)
}

// Internal installation function used to simplify reporting back to the Panel.
func (s *Server) internalInstall() error {
	script, err := s.client.GetInstallationScript(s.Context(), s.ID())
	if err != nil {
		return err
	}
	p, err := NewInstallationProcess(s, &script)
	if err != nil {
		return err
	}

	s.Log().Info("beginning installation process for server")
	if err := p.Run(); err != nil {
		return err
	}

	s.Log().Info("completed installation process for server")
	return nil
}

type InstallationProcess struct {
	Server *Server
	Script *remote.InstallationScript
	client *client.Client
}

// NewInstallationProcess returns a new installation process struct that will be
// used to create containers and otherwise perform installation commands for a
// server.
func NewInstallationProcess(s *Server, script *remote.InstallationScript) (*InstallationProcess, error) {
	proc := &InstallationProcess{
		Script: script,
		Server: s,
	}

	if runtime.GOOS == "windows" {
		return proc, nil
	}

	if c, err := environment.Docker(); err != nil {
		return nil, err
	} else {
		proc.client = c
	}

	return proc, nil
}

// IsInstalling returns if the server is actively running the installation
// process by checking the status of the installer lock.
func (s *Server) IsInstalling() bool {
	return s.installing.Load()
}

func (s *Server) IsTransferring() bool {
	return s.transferring.Load()
}

func (s *Server) SetTransferring(state bool) {
	s.transferring.Store(state)
}

func (s *Server) IsRestoring() bool {
	return s.restoring.Load()
}

func (s *Server) SetRestoring(state bool) {
	s.restoring.Store(state)
}

// RemoveContainer removes the installation container for the server.
func (ip *InstallationProcess) RemoveContainer() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	err := ip.client.ContainerRemove(ip.Server.Context(), ip.Server.ID()+"_installer", container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}
	return nil
}

// Run runs the installation process, this is done as in a background thread.
// This will configure the required environment, and then spin up the
// installation container. Once the container finishes installing the results
// are stored in an installation log in the server's configuration directory.
func (ip *InstallationProcess) Run() error {
	ip.Server.Log().Debug("acquiring installation process lock")
	if !ip.Server.installing.SwapIf(true) {
		return errors.New("install: cannot obtain installation lock")
	}

	// We now have an exclusive lock on this installation process. Ensure that whenever this
	// process is finished that the semaphore is released so that other processes and be executed
	// without encountering a wait timeout.
	defer func() {
		ip.Server.Log().Debug("releasing installation process lock")
		ip.Server.installing.Store(false)
	}()

	if err := ip.BeforeExecute(); err != nil {
		return err
	}

	cID, err := ip.Execute()
	if err != nil {
		_ = ip.RemoveContainer()
		return err
	}

	// If this step fails, log a warning but don't exit out of the process. This is completely
	// internal to the daemon's functionality, and does not affect the status of the server itself.
	if err := ip.AfterExecute(cID); err != nil {
		ip.Server.Log().WithField("error", err).Warn("failed to complete after-execute step of installation process")
	}

	return nil
}

// Returns the location of the temporary data for the installation process.
func (ip *InstallationProcess) tempDir() string {
	return filepath.Join(config.Get().System.TmpDirectory, ip.Server.ID())
}

// Writes the installation script to a temporary file on the host machine so that it
// can be properly mounted into the installation container and then executed.
func (ip *InstallationProcess) writeScriptToDisk() error {
	// Make sure the temp directory root exists before trying to make a directory within it. The
	// os.TempDir call expects this base to exist, it won't create it for you.
	if err := os.MkdirAll(ip.tempDir(), 0o700); err != nil {
		return errors.WithMessage(err, "could not create temporary directory for install process")
	}
	f, err := os.OpenFile(filepath.Join(ip.tempDir(), "install.sh"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return errors.WithMessage(err, "failed to write server installation script to disk before mount")
	}
	defer f.Close()
	if _, err := io.Copy(f, strings.NewReader(strings.ReplaceAll(ip.Script.Script, "\r\n", "\n"))); err != nil {
		return err
	}
	return nil
}

// Pulls the docker image to be used for the installation container.
func (ip *InstallationProcess) pullInstallationImage() error {
	// Get a registry auth configuration from the config.
	var registryAuth *config.RegistryConfiguration
	for registry, c := range config.Get().Docker.Registries {
		if !strings.HasPrefix(ip.Script.ContainerImage, registry) {
			continue
		}

		log.WithField("registry", registry).Debug("using authentication for registry")
		registryAuth = &c
		break
	}

	// Get the ImagePullOptions.
	imagePullOptions := dockerImage.PullOptions{All: false}
	if registryAuth != nil {
		b64, err := registryAuth.Base64()
		if err != nil {
			log.WithError(err).Error("failed to get registry auth credentials")
		}

		// b64 is a string so if there is an error it will just be empty, not nil.
		imagePullOptions.RegistryAuth = b64
	}

	r, err := ip.client.ImagePull(ip.Server.Context(), ip.Script.ContainerImage, imagePullOptions)
	if err != nil {
		images, ierr := ip.client.ImageList(ip.Server.Context(), dockerImage.ListOptions{})
		if ierr != nil {
			// Well damn, something has gone really wrong here, just go ahead and abort there
			// isn't much anything we can do to try and self-recover from this.
			return ierr
		}

		for _, img := range images {
			for _, t := range img.RepoTags {
				if t != ip.Script.ContainerImage {
					continue
				}

				log.WithFields(log.Fields{
					"image": ip.Script.ContainerImage,
					"err":   err.Error(),
				}).Warn("unable to pull requested image from remote source, however the image exists locally")

				// Okay, we found a matching container image, in that case just go ahead and return
				// from this function, since there is nothing else we need to do here.
				return nil
			}
		}

		return err
	}
	defer r.Close()

	log.WithField("image", ip.Script.ContainerImage).Debug("pulling docker image... this could take a bit of time")

	// Block continuation until the image has been pulled successfully.
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Debug(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// BeforeExecute runs before the container is executed. This pulls down the
// required docker container image as well as writes the installation script to
// the disk. This process is executed in an async manner, if either one fails
// the error is returned.
func (ip *InstallationProcess) BeforeExecute() error {
	if err := ip.writeScriptToDisk(); err != nil {
		return errors.WithMessage(err, "failed to write installation script to disk")
	}

	if runtime.GOOS == "windows" {
		return nil
	}

	if err := ip.pullInstallationImage(); err != nil {
		return errors.WithMessage(err, "failed to pull updated installation container image for server")
	}
	if err := ip.RemoveContainer(); err != nil {
		return errors.WithMessage(err, "failed to remove existing install container for server")
	}
	return nil
}

// GetLogPath returns the log path for the installation process.
func (ip *InstallationProcess) GetLogPath() string {
	return filepath.Join(config.Get().System.LogDirectory, "/install", ip.Server.ID()+".log")
}

// AfterExecute cleans up after the execution of the installation process.
// This grabs the logs from the process to store in the server configuration
// directory, and then destroys the associated installation container.
func (ip *InstallationProcess) AfterExecute(containerId string) error {
	defer ip.RemoveContainer()

	if runtime.GOOS == "windows" {
		return nil
	}


	ip.Server.Log().WithField("container_id", containerId).Debug("pulling installation logs for server")
	reader, err := ip.client.ContainerLogs(ip.Server.Context(), containerId, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
	})

	if err != nil && !client.IsErrNotFound(err) {
		return err
	}

	// Get kernel version using the kernel package
	v, err := kernel.GetKernelVersion()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(ip.GetLogPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	// We write the contents of the container output to a more "permanent" file so that they
	// can be referenced after this container is deleted. We'll also include the environment
	// variables passed into the container to make debugging things a little easier.
	ip.Server.Log().WithField("path", ip.GetLogPath()).Debug("writing most recent installation logs to disk")

	tmpl, err := template.New("header").Parse(`Propel Server Installation Log

|
| Details
| ------------------------------
  Server UUID:          {{.Server.ID}}
  Container Image:      {{.Script.ContainerImage}}
  Container Entrypoint: {{.Script.Entrypoint}}
  Kernel Version:       {{.KernelVersion}}

|
| Environment Variables
| ------------------------------
{{ range $key, $value := .Server.GetEnvironmentVariables }}  {{ $value }}
{{ end }}

|
| Script Output
| ------------------------------
`)
	if err != nil {
		return err
	}

	// Create a data structure that includes both the InstallationProcess and the kernel version
	data := struct {
		*InstallationProcess
		KernelVersion string
	}{
		InstallationProcess: ip,
		KernelVersion:       v.String(),
	}

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	if _, err := io.Copy(f, reader); err != nil {
		return err
	}

	return nil
}

// Execute executes the installation process inside a specially created docker
// container.
func (ip *InstallationProcess) Execute() (string, error) {
	// Create a child context that is canceled once this function is done running. This
	// will also be canceled if the parent context (from the Server struct) is canceled
	// which occurs if the server is deleted.
	ctx, cancel := context.WithCancel(ip.Server.Context())
	defer cancel()

	if runtime.GOOS == "windows" {
		ip.Server.Log().Info("Windows: Skipping Docker install container execution.")
		if err := ip.downloadServerFile(); err != nil {
			ip.Server.Log().WithField("error", err).Error("Windows: Failed to download server file.")
			return "", err
		}
		return "windows-dummy-id", nil
	}


	conf := &container.Config{
		Hostname:     "installer",
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		OpenStdin:    true,
		Tty:          true,
		Cmd:          []string{ip.Script.Entrypoint, "/mnt/install/install.sh"},
		Image:        ip.Script.ContainerImage,
		Env:          ip.Server.GetEnvironmentVariables(),
		Labels: map[string]string{
			"Service":       "Propel",
			"ContainerType": "server_installer",
		},
	}

	cfg := config.Get()
	tmpfsSize := strconv.Itoa(int(cfg.Docker.TmpfsSize))
	hostConf := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Target:   "/mnt/server",
				Source:   ip.Server.Filesystem().Path(),
				Type:     mount.TypeBind,
				ReadOnly: false,
			},
			{
				Target:   "/mnt/install",
				Source:   ip.tempDir(),
				Type:     mount.TypeBind,
				ReadOnly: false,
			},
		},
		Resources: ip.resourceLimits(),
		Tmpfs: map[string]string{
			"/tmp": "rw,exec,nosuid,size=" + tmpfsSize + "M",
		},
		DNS:         cfg.Docker.Network.Dns,
		LogConfig:   cfg.Docker.ContainerLogConfig(),
		NetworkMode: container.NetworkMode(cfg.Docker.Network.Mode),
		UsernsMode:  container.UsernsMode(cfg.Docker.UsernsMode),
	}

	// Ensure the root directory for the server exists properly before attempting
	// to trigger the reinstall of the server. It is possible the directory would
	// not exist when this runs if Wings boots with a missing directory and a user
	// triggers a reinstall before trying to start the server.
	if err := ip.Server.EnsureDataDirectoryExists(); err != nil {
		return "", err
	}

	ip.Server.Log().WithField("install_script", ip.tempDir()+"/install.sh").Info("creating install container for server process")
	// Remove the temporary directory when the installation process finishes for this server container.
	defer func() {
		if err := os.RemoveAll(ip.tempDir()); err != nil {
			if !os.IsNotExist(err) {
				ip.Server.Log().WithField("error", err).Warn("failed to remove temporary data directory after install process")
			}
		}
	}()

	var netConf *network.NetworkingConfig = nil //In case when no networking config is needed set nil
	var serverNetConfig = config.Get().Docker.Network
	if "macvlan" == serverNetConfig.Driver { //Generate networking config for macvlan driver
		var defaultMapping = ip.Server.Config().Allocations.DefaultMapping
		ip.Server.Log().Debug("Set macvlan " + serverNetConfig.Name + " IP to " + defaultMapping.Ip)
		netConf = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				serverNetConfig.Name: { //Get network name from wings config
					IPAMConfig: &network.EndpointIPAMConfig{
						IPv4Address: defaultMapping.Ip,
					},
					IPAddress: defaultMapping.Ip, //Use default mapping ip address (wings support only one network per server)
					Gateway:   serverNetConfig.Interfaces.V4.Gateway,
				},
			},
		}
	}
	// Pass the networkings configuration or nil if none required
	r, err := ip.client.ContainerCreate(ctx, conf, hostConf, netConf, nil, ip.Server.ID()+"_installer")
	
	if err != nil {
		return "", err
	}

	ip.Server.Log().WithField("container_id", r.ID).Info("running installation script for server in container")
	if err := ip.client.ContainerStart(ctx, r.ID, container.StartOptions{}); err != nil {
		return "", err
	}

	// Process the install event in the background by listening to the stream output until the
	// container has stopped, at which point we'll disconnect from it.
	//
	// If there is an error during the streaming output just report it and do nothing else, the
	// install can still run, the console just won't have any output.
	go func(id string) {
		ip.Server.Events().Publish(DaemonMessageEvent, "Starting installation process, this could take a few minutes...")
		if err := ip.StreamOutput(ctx, id); err != nil {
			ip.Server.Log().WithField("error", err).Warn("error connecting to server install stream output")
		}
	}(r.ID)

	sChan, eChan := ip.client.ContainerWait(ctx, r.ID, container.WaitConditionNotRunning)
	select {
	case err := <-eChan:
		// Once the container has stopped running we can mark the install process as being completed.
		if err == nil {
			ip.Server.Events().Publish(DaemonMessageEvent, "Installation process completed.")
		} else {
			return "", err
		}
	case <-sChan:
	}

	return r.ID, nil
}

// StreamOutput streams the output of the installation process to a log file in
// the server configuration directory, as well as to a websocket listener so
// that the process can be viewed in the panel by administrators.
func (ip *InstallationProcess) StreamOutput(ctx context.Context, id string) error {
	opts := container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true}
	reader, err := ip.client.ContainerLogs(ctx, id, opts)
	if err != nil {
		return err
	}
	defer reader.Close()

	err = system.ScanReader(reader, ip.Server.Sink(system.InstallSink).Push)
	if err != nil && !errors.Is(err, context.Canceled) {
		ip.Server.Log().WithFields(log.Fields{"container_id": id, "error": err}).Warn("error processing install output lines")
	}
	return nil
}

// resourceLimits returns resource limits for the installation container. This
// looks at the globally defined install container limits and attempts to use
// the higher of the two (defined limits & server limits). This allows for servers
// with super low limits (e.g. Discord bots with 128Mb of memory) to perform more
// intensive installation processes if needed.
//
// This also avoids a server with limits such as 4GB of memory from accidentally
// consuming 2-5x the defined limits during the install process and causing
// system instability.
func (ip *InstallationProcess) resourceLimits() container.Resources {
	limits := config.Get().Docker.InstallerLimits

	// Create a copy of the configuration, so we're not accidentally making
	// changes to the underlying server build data.
	c := *ip.Server.Config()
	cfg := c.Build
	if cfg.MemoryLimit < limits.Memory {
		cfg.MemoryLimit = limits.Memory
	}
	// Only apply the CPU limit if neither one is currently set to unlimited. If the
	// installer CPU limit is unlimited don't even waste time with the logic, just
	// set the config to unlimited for this.
	if limits.Cpu == 0 {
		cfg.CpuLimit = 0
	} else if cfg.CpuLimit != 0 && cfg.CpuLimit < limits.Cpu {
		cfg.CpuLimit = limits.Cpu
	}

	resources := cfg.AsContainerResources()
	// Explicitly remove the PID limits for the installation container. These scripts are
	// defined at an administrative level and users can't manually execute things like a
	// fork bomb during this process.
	resources.PidsLimit = nil

	return resources
}

// SyncInstallState makes an HTTP request to the Panel instance notifying it that
// the server has completed the installation process, and what the state of the
// server is.
func (s *Server) SyncInstallState(successful, reinstall bool) error {
	return s.client.SetInstallationStatus(s.Context(), s.ID(), remote.InstallStatusRequest{
		Successful: successful,
		Reinstall:  reinstall,
	})
}

func (ip *InstallationProcess) downloadServerFile() error {
	env := ip.Server.GetEnvironmentVariables()
	
	// Create map for variable expansion
	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

    // Try to detect project from script if not in env
    if envMap["PROJECT"] == "" {
        scriptLower := strings.ToLower(ip.Script.Script)
        if strings.Contains(scriptLower, "purpur") {
            envMap["PROJECT"] = "purpur"
        } else if strings.Contains(scriptLower, "paper") {
            envMap["PROJECT"] = "paper"
        } else if strings.Contains(scriptLower, "velocity") {
            envMap["PROJECT"] = "velocity"
        } else if strings.Contains(scriptLower, "waterfall") {
            envMap["PROJECT"] = "waterfall"
        } else if strings.Contains(scriptLower, "bungeecord") {
             envMap["PROJECT"] = "bungeecord"
        }
    }
    
	ip.Server.Log().WithField("env_vars", envMap).Info("Debug: Windows Installer environment variables")

	var url, jar string
	for _, e := range env {
		if strings.HasPrefix(e, "DOWNLOAD_URL=") {
			url = strings.TrimPrefix(e, "DOWNLOAD_URL=")
		}
		if strings.HasPrefix(e, "SERVER_JARFILE=") {
			jar = strings.TrimPrefix(e, "SERVER_JARFILE=")
		}
	}
    
    // Expand variables in URL (e.g. ${PROJECT}, ${VERSION})
    if url != "" {
        url = os.Expand(url, func(key string) string {
            return envMap[key]
        })
    }

	// --- PaperMC & Purpur API Integration ---
	// Check if known project type
	if url == "" || strings.Contains(url, "papermc.io") || strings.Contains(url, "purpurmc.org") {
		project := envMap["PROJECT"]
		version := envMap["MINECRAFT_VERSION"]
		if version == "" {
			version = envMap["VERSION"]
		}
		build := envMap["BUILD_NUMBER"]
		if build == "" {
			build = envMap["BUILD"]
		}

        // Special handling for Purpur (supports latest keyword in URL)
        // URL: https://api.purpurmc.org/v2/purpur/{version}/{build}/download
        if project == "purpur" && version != "" && version != "latest" { // Purpur API needs explicit version like 1.20.1, not 'latest'
             if build == "" || build == "latest" {
                 build = "latest" // Purpur supports literal 'latest'
             }
             url = fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/%s/download", version, build)
             ip.Server.Log().WithField("api_url", url).Info("Windows: Resolved Purpur download URL")
        } else if project != "" && version != "" && (project == "paper" || project == "waterfall" || project == "velocity" || project == "folia") {
             // PaperMC Handling
			ip.Server.Log().WithFields(log.Fields{
				"project": project,
				"version": version,
			}).Info("Windows: Detected PaperMC project, querying API...")

			apiURL, err := fetchPaperBuild(project, version, build)
			if err == nil && apiURL != "" {
				url = apiURL
				ip.Server.Log().WithField("api_url", url).Info("Windows: Resolved PaperMC download URL")
			} else {
				ip.Server.Log().WithError(err).Warn("Windows: Failed to resolve PaperMC API URL, falling back...")
			}
		}
	}
    
    // Fallback: Try to find a URL in the script if still empty
    if url == "" {
        script := ip.Script.Script
        lines := strings.Split(script, "\n")
        for _, line := range lines {
            if strings.Contains(line, "http") {
                parts := strings.Fields(line)
                for _, p := range parts {
                    if strings.HasPrefix(p, "http") {
                        clean := strings.Trim(p, "\"'")
                        clean = os.Expand(clean, func(key string) string {
                            return envMap[key]
                        })
                        if strings.Contains(clean, "://") {
                            url = clean
                            ip.Server.Log().WithField("fallback_url", url).Info("Windows: Found potential download URL in script")
                            break
                        }
                    }
                }
                if url != "" { break }
            }
        }
    }

	if url == "" {
		ip.Server.Log().Warn("Windows: No DOWNLOAD_URL found in environment or script. Assuming manual file upload or local install.")
        // Return nil to allow the install to "succeed" so the user can upload files manually.
		return nil
	}
	if jar == "" {
		jar = "server.jar"
	}

	ip.Server.Log().WithField("url", url).Info("Windows: Downloading server file (using curl)...")
	
	outPath := filepath.Join(ip.Server.Filesystem().Path(), jar)

    // Try using curl (recommended for Windows 10/11)
    // -L follows redirects, -o writes to file.
    // -A sets User-Agent to avoid some basic blocks
    cmd := exec.Command("curl", "-L", "-f", "-o", outPath, "-A", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", url)
    output, err := cmd.CombinedOutput()
    if err == nil {
        ip.Server.Log().Info("Windows: Download completed via curl.")
        return nil
    }
    
    ip.Server.Log().WithField("error", err).WithField("output", string(output)).Warn("Windows: curl failed, falling back to Go http.Get...")

    // Fallback to Go HTTP if curl fails
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %s", resp.Status)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// fetchPaperBuild queries the PaperMC API to get the download URL
func fetchPaperBuild(project, version, build string) (string, error) {
    // Resolve "latest" version
    if version == "latest" || version == "" {
        resp, err := http.Get(fmt.Sprintf("https://api.papermc.io/v2/projects/%s", project))
        // Try Purpur domain if project is purpur and paper domain fails (though Purpur supports paper API format usually)
        if (err != nil || resp.StatusCode != 200) && project == "purpur" {
             resp, err = http.Get(fmt.Sprintf("https://api.purpurmc.org/v2/%s", project))
        }

        if err != nil {
            return "", err
        }
        defer resp.Body.Close()

        if resp.StatusCode != 200 {
            return "", fmt.Errorf("metadata API returned %d", resp.StatusCode)
        }

        var res struct {
            Versions []string `json:"versions"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
            return "", err
        }
        if len(res.Versions) == 0 {
            return "", fmt.Errorf("no versions found for project %s", project)
        }
        version = res.Versions[len(res.Versions)-1]
    }

    if build == "" || build == "latest" {
        // Fetch latest build
        // Note: For Purpur, we might not need this if we use their /latest/download endpoint with resolved version
        // But for Paper, we must query builds.
        
        baseURL := "https://api.papermc.io"
        if project == "purpur" {
            baseURL = "https://api.purpurmc.org"
        }
        
        resp, err := http.Get(fmt.Sprintf("%s/v2/projects/%s/versions/%s", baseURL, project, version))
        if err != nil {
            return "", err
        }
        defer resp.Body.Close()
        
        if resp.StatusCode != 200 {
             return "", fmt.Errorf("version API returned %d", resp.StatusCode)
        }

        var res struct {
            Builds []int `json:"builds"`
        }
        // Paper uses builds: [1, 2], Purpur might use builds: {all:[], latest:str} or similar?
        // Actually Purpur V2 API for /versions/{version} returns { "builds": { "all": ["1", "2"], "latest": "2" } }
        // Paper V2 API returns { "builds": [1, 2, 3] }
        
        // Use a generic decoder or handle differentiation
        // Or simply use Purpur's /latest/download shortcut if it's Purpur!
        if project == "purpur" {
             // We resolved version, so just return the simplified URL
             // https://api.purpurmc.org/v2/purpur/{version}/latest/download
             return fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/latest/download", version), nil
        }

        if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
            return "", err
        }
        
        if len(res.Builds) == 0 {
            return "", fmt.Errorf("no builds found for version %s", version)
        }
        
        // Last build is the latest
        lastBuild := res.Builds[len(res.Builds)-1]
        build = strconv.Itoa(lastBuild)
    }
    
    // Construct download URL
    // filename format: project-version-build.jar
    filename := fmt.Sprintf("%s-%s-%s.jar", project, version, build)
    return fmt.Sprintf("https://api.papermc.io/v2/projects/%s/versions/%s/builds/%s/downloads/%s", project, version, build, filename), nil
}



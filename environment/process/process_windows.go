//go:build windows

package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
    "net/http"
	"os"
	"os/exec"
    "path/filepath"
	"strings"
	"sync"
    "syscall"
	"time"
    "unsafe"

	"github.com/apex/log"
	"github.com/priyxstudio/propel/environment"
	"github.com/priyxstudio/propel/events"
	"github.com/shirou/gopsutil/v3/process"
    "golang.org/x/sys/windows"
)

type Metadata struct {
	DataDir string
}

type Process struct {
	mu          sync.RWMutex
	id          string
	meta        *Metadata
	conf        *environment.Configuration
	state       string
	emitter     *events.Bus
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	startTime   time.Time
	logBuffer   []string
	logCallback func([]byte)
    lastPid     int  // Store PID separately for reliable kill
}

func New(id string, meta *Metadata, conf *environment.Configuration) (*Process, error) {
	return &Process{
		id:      id,
		meta:    meta,
		conf:    conf,
		state:   environment.ProcessOfflineState,
		emitter: events.NewBus(),
	}, nil
}

func (p *Process) Type() string {
	return "process"
}

func (p *Process) Config() *environment.Configuration {
	return p.conf
}

func (p *Process) Events() *events.Bus {
	return p.emitter
}

func (p *Process) Exists() (bool, error) {
	return true, nil
}

func (p *Process) IsRunning(ctx context.Context) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == environment.ProcessRunningState || p.state == environment.ProcessStartingState, nil
}

func (p *Process) InSituUpdate() error {
	return nil
}

func (p *Process) OnBeforeStart(ctx context.Context) error {
	return nil
}

func (p *Process) Start(ctx context.Context) error {
	p.SetState(environment.ProcessStartingState)

	// Get the STARTUP command from environment variables
	var startup string
	for _, e := range p.conf.EnvironmentVariables() {
		if strings.HasPrefix(e, "STARTUP=") {
			startup = strings.TrimPrefix(e, "STARTUP=")
			break
		}
	}

	if startup == "" {
		p.SetState(environment.ProcessOfflineState)
		return fmt.Errorf("no STARTUP command found in environment variables")
	}

	log.WithField("server", p.id).WithField("command", startup).Info("Starting server process")

	// Parse command - split on spaces but handle quotes
	args := strings.Fields(startup)
	if len(args) == 0 {
		p.SetState(environment.ProcessOfflineState)
		return fmt.Errorf("empty startup command")
	}

	// Clean quotes from executable path if present
    // This fixes issues where absolute paths wrapped in quotes are treated as relative files
    args[0] = strings.Trim(args[0], "\"")
    args[0] = strings.Trim(args[0], "'")

    // Filter out comments (anything starting with #)
    // The panel might append comments to the startup command which breaks execution
    for i, arg := range args {
        if strings.HasPrefix(arg, "#") {
            args = args[:i]
            break
        }
    }

    // Filter out deprecated JVM flags that cause crashes on modern Java (e.g. UseAOT removed in JDK 17)
    // Only apply this filtering if it looks like a Java command
    if strings.Contains(strings.ToLower(args[0]), "java") {
        filteredArgs := args[:0]
        for _, arg := range args {
            // Skip UseAOT flags (both enabled and disabled) as they crash modern Java
            if strings.Contains(arg, "UseAOT") {
                log.WithField("server", p.id).WithField("flag", arg).Info("Stripping deprecated/removed JVM flag to prevent crash")
                continue
            }
            
            // Fix broken AOTCache flag (user reported it comes in as just -XX:AOTCache sometimes)
            if arg == "-XX:AOTCache" {
                log.WithField("server", p.id).Info("Fixing broken AOTCache flag to -XX:AOTCache=HytaleServer.aot")
                arg = "-XX:AOTCache=HytaleServer.aot"
            }
            
            filteredArgs = append(filteredArgs, arg)
        }
        args = filteredArgs
    }

	// For Minecraft/Java servers, ensure --nogui is present
    // Hytale does NOT support nogui and will fail if present
    isHytale := isHytaleServer(args, p.conf.EnvironmentVariables())

	isJavaJar := false
	for _, arg := range args {
		if strings.HasSuffix(strings.ToLower(arg), ".jar") {
			isJavaJar = true
			break
		}
	}
	if isJavaJar && !isHytale {
		hasNogui := false
		for _, arg := range args {
			if strings.ToLower(arg) == "nogui" || strings.ToLower(arg) == "--nogui" {
				hasNogui = true
				break
			}
		}
		if !hasNogui {
			args = append(args, "nogui")
			log.WithField("server", p.id).Info("Added 'nogui' flag for Minecraft server")
		}
	}
    
    // USE LOCAL JAVA: Replace system Java with local Java from server directory
    // This allows running as restricted user without needing system-wide Java access
    if isJavaCommand(args[0]) {
        envVars := p.conf.EnvironmentVariables()
        
        // Check if this is a Hytale server (requires Java 25 for AOTCache)
        if isHytaleServer(args, envVars) {
            // Check if user has specified an absolute path to use specific Java
            // If so, and it exists, respect it and don't force local Java 25
            if filepath.IsAbs(args[0]) {
                 log.WithField("server", p.id).WithField("path", args[0]).Info("Using absolute path for Hytale Java (Overriding local Java 25)")
                 // Ensure we still have AOTCache if needed? 
                 // We assume user knows what they are doing if they provide absolute path
            } else {
                java25Path := filepath.Join(p.meta.DataDir, "java25", "bin", "java.exe")
            
                // Check if Java 25 exists, if not download it
                if _, err := os.Stat(java25Path); os.IsNotExist(err) {
                log.WithField("server", p.id).Info("Hytale detected: Downloading Java 25 for AOTCache support...")
                p.publishConsole("[Propel Daemon]: Hytale server detected! Downloading Java 25 (required for AOTCache)...")
                
                if err := downloadJava25ForHytale(p.meta.DataDir); err != nil {
                    log.WithField("server", p.id).WithField("error", err).Error("Failed to download Java 25 for Hytale")
                    p.publishConsole(fmt.Sprintf("[Propel Daemon]: ERROR: Could not download Java 25: %v", err))
                    p.publishConsole("[Propel Daemon]: Hytale requires Java 25 for AOTCache. Please install Java 25 manually.")
                    p.SetState(environment.ProcessOfflineState)
                    return fmt.Errorf("hytale requires Java 25 for AOTCache support: %w", err)
                } else {
                    log.WithField("server", p.id).Info("Java 25 for Hytale installed successfully")
                    p.publishConsole("[Propel Daemon]: Java 25 installed successfully!")
                }
            }
            
            // Use Java 25 for Hytale
            if _, err := os.Stat(java25Path); err == nil {
                // Hide java25 folder from Panel file browser
                java25Dir := filepath.Join(p.meta.DataDir, "java25")
                exec.Command("attrib", "+h", "+s", java25Dir).Run()
            }

            // Hytale Port Configuration (First Start Only)
            // Ensure the server uses the panel-assigned port by creating server.properties if it doesn't exist.
            serverPropsPath := filepath.Join(p.meta.DataDir, "server.properties")
            if _, err := os.Stat(serverPropsPath); os.IsNotExist(err) {
                // p.conf.Allocations is a method, not a field
                port := p.conf.Allocations().DefaultMapping.Port
                log.WithField("server", p.id).WithField("port", port).Info("First start detected: Generating server.properties for Hytale")
                
                // Write minimal server.properties with the correct port
                content := fmt.Sprintf("server-port=%d\nserver-ip=0.0.0.0\n", port)
                if err := os.WriteFile(serverPropsPath, []byte(content), 0644); err != nil {
                     log.WithField("server", p.id).WithField("error", err).Warn("Failed to generate server.properties")
                } else {
                     p.publishConsole(fmt.Sprintf("[Propel Daemon]: First start detected. Configured Hytale to use port %d.", port))
                }
            }
        }
    } else {
        // Regular Java server - use Java 21
            localJavaPath := filepath.Join(p.meta.DataDir, "java", "bin", "java.exe")
            
            // Check if local Java exists, if not download it
            if _, err := os.Stat(localJavaPath); os.IsNotExist(err) {
                log.WithField("server", p.id).Info("Local Java not found, downloading portable Java...")
                p.publishConsole("[Propel Daemon]: Downloading portable Java runtime...")
                
                if err := downloadPortableJava(p.meta.DataDir); err != nil {
                    log.WithField("server", p.id).WithField("error", err).Warn("Failed to download portable Java, using system Java")
                    p.publishConsole(fmt.Sprintf("[Propel Daemon]: Warning: Could not download Java: %v", err))
                } else {
                    log.WithField("server", p.id).Info("Portable Java downloaded successfully")
                    p.publishConsole("[Propel Daemon]: Portable Java installed successfully!")
                }
            }
            
            // Use local Java if it exists now
            if _, err := os.Stat(localJavaPath); err == nil {
                log.WithField("server", p.id).WithField("path", localJavaPath).Info("Using local Java")
                args[0] = localJavaPath
                
                // Ensure java folder is hidden from Panel file browser
                javaDir := filepath.Join(p.meta.DataDir, "java")
                exec.Command("attrib", "+h", "+s", javaDir).Run()
            }
        }
    }

	// Create the command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = p.meta.DataDir
	cmd.Env = append(os.Environ(), p.conf.EnvironmentVariables()...)
    
    // NOTE: User impersonation disabled - requires SE_ASSIGNPRIMARYTOKEN privilege
    // which Wings doesn't have when run normally. The local Java per-server 
    // already provides isolation. The "running as admin" warning is informational only.
    /*
    // Try to run as restricted user to avoid the "running as admin" warning
    username := "Srv_" + p.id[:8]
    password := "Pw" + p.id[:8] + "!"
    
    userToken, err := logonUser(username, password)
    if err != nil {
        log.WithField("server", p.id).WithField("user", username).WithField("error", err).Warn("Failed to logon as user, running as current user")
    } else {
        log.WithField("server", p.id).WithField("user", username).Info("Running server as restricted user")
        cmd.SysProcAttr = &syscall.SysProcAttr{
            Token: syscall.Token(userToken),
        }
        defer windows.CloseHandle(userToken)
    }
    
    // Grant the user access to the server directory
    if userToken != 0 {
        grantDirAccess(p.meta.DataDir, username)
    }
    */

	// Set up stdin pipe for sending commands
	stdin, err := cmd.StdinPipe()
	if err != nil {
		p.SetState(environment.ProcessOfflineState)
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	p.stdin = stdin

	// Set up stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.SetState(environment.ProcessOfflineState)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Set up stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.SetState(environment.ProcessOfflineState)
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		p.SetState(environment.ProcessOfflineState)
		return fmt.Errorf("failed to start process: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.startTime = time.Now()
    p.lastPid = cmd.Process.Pid  // Store PID for reliable kill later
	p.mu.Unlock()
    
    // ALSO persist PID to file so we can kill even after Wings restarts
    pidFile := filepath.Join(p.meta.DataDir, ".pid")
    if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
        log.WithField("server", p.id).WithField("error", err).Warn("Failed to write PID file")
    } else {
        log.WithField("server", p.id).WithField("pidFile", pidFile).Debug("Wrote PID file")
    }
    
    // Apply resource limits using Windows Job Objects
    cpuLimit := p.conf.Limits().CpuLimit  // 100 = 1 core, 200 = 2 cores
    if cpuLimit > 0 {
        if err := applyJobObjectLimits(cmd.Process.Pid, cpuLimit); err != nil {
            log.WithField("server", p.id).WithField("error", err).Warn("Failed to apply CPU limit via Job Object")
        } else {
            log.WithField("server", p.id).WithField("cpuLimit", cpuLimit).Info("Applied hard CPU limit via Job Object")
        }
    }
    
    log.WithField("server", p.id).WithField("pid", cmd.Process.Pid).Info("Server process started")

	p.SetState(environment.ProcessRunningState)

	// Stream stdout to console events
	go p.streamOutput(stdout)
	go p.streamOutput(stderr)

	// Wait for process to exit in background
	go func() {
		err := cmd.Wait()
		exitCode := uint32(0)
		if cmd.ProcessState != nil {
			exitCode = uint32(cmd.ProcessState.ExitCode())
		}
		
		log.WithField("server", p.id).WithField("exit_code", exitCode).Info("Server process exited")
		
		// Only warn about error if we weren't trying to stop the server
		isStopping := false
		p.mu.RLock()
		if p.state == environment.ProcessStoppingState || p.state == environment.ProcessOfflineState {
			isStopping = true
		}
		p.mu.RUnlock()

		if err != nil && !isStopping {
			// Don't warn if exit code 1 or -1 if we initiated stop, but here exitCode is checked.
			// Standard warning:
			log.WithField("server", p.id).WithField("error", err).Warn("Process exited with error")
		}
		
		p.SetState(environment.ProcessOfflineState)
	}()

	// Start resource stats collection
	go p.collectStats(cmd)

	return nil
}

func (p *Process) streamOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Add to log buffer
		p.mu.Lock()
		p.logBuffer = append(p.logBuffer, line)
		if len(p.logBuffer) > 1000 {
			p.logBuffer = p.logBuffer[1:]
		}
		p.mu.Unlock()

		// Call the log callback if set
		p.mu.RLock()
		cb := p.logCallback
		p.mu.RUnlock()
		if cb != nil {
			cb([]byte(line))
		}
	}
}

func (p *Process) collectStats(cmd *exec.Cmd) {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	var proc *process.Process
	var err error

	for {
		select {
		case <-ticker.C:
			// Check if process is still running
			if p.State() == environment.ProcessOfflineState {
				return
			}

			if cmd == nil || cmd.Process == nil {
				return
			}

			// Initialize proc instance once to allows correct CPU usage calculation over time
			if proc == nil {
				pid := int32(cmd.Process.Pid)
				proc, err = process.NewProcess(pid)
				if err != nil {
					// Only log error periodically
					continue
				}
			}

            // Aggregate stats from parent + all children (more accurate for Java which spawns threads)
            var totalMemory uint64 = 0
            var totalCPU float64 = 0
            
            // Get parent process stats
			memInfo, err := proc.MemoryInfo()
			if err != nil {
				// Retry getting proc if it failed
				proc = nil
				continue
			}
			totalMemory += memInfo.RSS

			// CPU - Percent(0) returns cpu usage since last call
			cpuPercent, err := proc.Percent(0)
			if err == nil {
                totalCPU += cpuPercent
            }
            
            // Also aggregate child processes
            children, err := proc.Children()
            if err == nil {
                for _, child := range children {
                    if childMem, err := child.MemoryInfo(); err == nil {
                        totalMemory += childMem.RSS
                    }
                    if childCPU, err := child.Percent(0); err == nil {
                        totalCPU += childCPU
                    }
                }
            }

			// Get uptime
			uptime, _ := p.Uptime(context.Background())

			// Get memory limit from config
			memLimit := p.conf.Limits().MemoryLimit * 1024 * 1024 // Convert MB to bytes
            
            // Get CPU limit and normalize the CPU percentage
            // gopsutil returns CPU as % of ALL system cores
            // We want to show it as % of the ALLOCATED cores
            // CpuLimit is in format: 100 = 1 core, 200 = 2 cores, etc.
            cpuLimit := p.conf.Limits().CpuLimit  
            
            // If no limit set (0 or unlimited), just use the raw percentage
            // Otherwise, normalize: if limit is 100 (1 core) and totalCPU is 25% (on 4 core system),
            // that's actually 100% of the allocated 1 core (25 * 4 / 1 = 100%)
            var normalizedCPU float64
            if cpuLimit > 0 {
                // Convert cpuLimit from percentage to cores (100 = 1 core)
                allocatedCores := float64(cpuLimit) / 100.0
                // totalCPU is % of all system cores, normalize to % of allocated cores
                normalizedCPU = totalCPU / allocatedCores
            } else {
                normalizedCPU = totalCPU
            }

			// Build stats
			stats := environment.Stats{
				Memory:      totalMemory,
				MemoryLimit: uint64(memLimit),
				CpuAbsolute: normalizedCPU,
				Uptime:      uptime,
				Network: environment.NetworkStats{
					RxBytes: 0,
					TxBytes: 0,
				},
			}

			// Publish stats event
			p.Events().Publish(environment.ResourceEvent, stats)
		}
	}
}

func (p *Process) Stop(ctx context.Context) error {
	p.mu.RLock()
	cmd := p.cmd
	stdin := p.stdin
	p.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		p.SetState(environment.ProcessOfflineState)
		return nil
	}
    
    // Notify user in console
    p.publishConsole("[Propel Daemon]: Stopping server... (saving data)")

	p.SetState(environment.ProcessStoppingState)

	// Try graceful shutdown
    stopType := p.conf.Settings().StopType
    stopVal := p.conf.Settings().StopValue
    
    // Default to "stop" command if unspecified (common for MC)
    if stopType == "" {
        stopType = "command" 
        stopVal = "stop"
    }

	if stdin != nil && stopType == "command" {
        cmdStr := stopVal
        if cmdStr == "" { cmdStr = "stop" }
        
        // Use standard SendCommand to match user interaction exactly
        if err := p.SendCommand(cmdStr); err != nil {
             log.WithField("server", p.id).WithField("error", err).Warn("Failed to send stop command")
        }
        p.publishConsole(fmt.Sprintf("[Propel Daemon]: Sent stop command: '%s'", cmdStr))
	} else if stopType == "signal" {
        // Windows doesn't handle signals well (SIGINT/SIGTERM often ignored or instant kill)
        // But if it's ^C, we might need a way to send CTRL_C_EVENT?
        // For now, we'll log it and rely on graceful timeout or eventual kill.
        p.publishConsole(fmt.Sprintf("[Propel Daemon]: Sending signal %s (Not fully supported on Windows Native, waiting for graceful exit...)", stopVal))
    }

	// Wait up to 30 seconds (increased from 10) for graceful shutdown
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			p.SetState(environment.ProcessOfflineState)
            p.publishConsole("[Propel Daemon]: Server stopped gracefully.")
			return nil
		}
	}
    
    p.publishConsole("[Propel Daemon]: Server did not stop in time, forcing kill...")

	// Force kill if still running
	return p.Terminate(ctx, "SIGKILL")
}

func (p *Process) WaitForStop(ctx context.Context, duration time.Duration, terminate bool) error {
    // First, send the stop command
    if err := p.Stop(ctx); err != nil {
        // If stop fails but terminate is true, we'll kill it below anyway
        log.WithField("server", p.id).WithField("error", err).Warn("Failed to send stop command during WaitForStop")
    }
    
    // Now wait for the process to actually stop
	start := time.Now()
	for {
		if p.State() == environment.ProcessOfflineState {
			return nil
		}
		if time.Since(start) > duration {
			if terminate {
				return p.Terminate(ctx, "SIGKILL")
			}
			return fmt.Errorf("timeout waiting for process to stop")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (p *Process) Terminate(ctx context.Context, signal string) error {
    // Check if already offline
    if p.State() == environment.ProcessOfflineState {
        p.publishConsole("[Propel Daemon]: Server is already stopped.")
        return nil
    }
    
    log.WithField("server", p.id).Info("Terminating server process...")
    p.publishConsole("[Propel Daemon]: Killing server process!")

	p.SetState(environment.ProcessStoppingState)
    
    // Get the server's data directory path
    serverPath := p.meta.DataDir
    log.WithField("server", p.id).WithField("path", serverPath).Info("Looking for Java processes in server directory")
    
    // Write PowerShell script to temp file to avoid escaping issues
    psScript := fmt.Sprintf(`$serverPath = "%s"
$procs = Get-CimInstance Win32_Process | Where-Object {($_.Name -eq 'java.exe' -or $_.Name -eq 'javaw.exe') -and $_.CommandLine -like "*$serverPath*"}
if ($procs) {
    foreach ($proc in $procs) {
        Write-Output "Killing PID: $($proc.ProcessId)"
        Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
    }
} else {
    Write-Output "NO_JAVA_FOUND"
}
`, serverPath)
    
    // Create temp script file
    tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kill_%s.ps1", p.id[:8]))
    if err := os.WriteFile(tmpFile, []byte(psScript), 0644); err != nil {
        log.WithField("server", p.id).WithField("error", err).Warn("Failed to write temp PowerShell script")
        p.publishConsole("[Propel Daemon]: Failed to create kill script")
        p.SetState(environment.ProcessOfflineState)
        return err
    }
    defer os.Remove(tmpFile)
    
    log.WithField("server", p.id).WithField("script", tmpFile).Debug("Running PowerShell kill script")
    p.publishConsole("[Propel Daemon]: Searching for Java processes...")
    
    psCmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", tmpFile)
    out, err := psCmd.CombinedOutput()
    output := strings.TrimSpace(string(out))
    
    if output != "" && output != "NO_JAVA_FOUND" {
        log.WithField("server", p.id).WithField("output", output).Info("PowerShell killed process(es)")
        p.publishConsole(fmt.Sprintf("[Propel Daemon]: %s", output))
        p.publishConsole("[Propel Daemon]: Server process killed successfully!")
    } else {
        log.WithField("server", p.id).Info("PowerShell found no matching Java processes, trying PID file fallback...")
        p.publishConsole("[Propel Daemon]: Trying PID file fallback...")
        
        // Try reading PID from .pid file
        pidFile := filepath.Join(p.meta.DataDir, ".pid")
        var pid int
        if pidData, err := os.ReadFile(pidFile); err == nil {
            if _, err := fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &pid); err == nil && pid > 0 {
                log.WithField("server", p.id).WithField("pid", pid).Info("Found PID from file, using taskkill")
                p.publishConsole(fmt.Sprintf("[Propel Daemon]: Found PID %d from file, killing...", pid))
                
                killOut, killErr := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).CombinedOutput()
                killOutput := strings.TrimSpace(string(killOut))
                
                if killErr == nil || strings.Contains(killOutput, "SUCCESS") {
                    p.publishConsole("[Propel Daemon]: Process killed successfully!")
                    os.Remove(pidFile) // Clean up PID file
                } else if strings.Contains(killOutput, "not found") {
                    p.publishConsole("[Propel Daemon]: Process was already stopped.")
                    os.Remove(pidFile)
                } else {
                    log.WithField("server", p.id).WithField("output", killOutput).Warn("taskkill failed")
                    p.publishConsole(fmt.Sprintf("[Propel Daemon]: taskkill error: %s", killOutput))
                }
            }
        } else {
            p.publishConsole("[Propel Daemon]: No PID file found. Server may already be stopped.")
        }
        
        // Also try Go's process kill as backup if we have a reference
        p.mu.RLock()
        cmd := p.cmd
        p.mu.RUnlock()
        
        if cmd != nil && cmd.Process != nil {
            log.WithField("server", p.id).WithField("pid", cmd.Process.Pid).Info("Trying Go Process.Kill() as backup")
            if err := cmd.Process.Kill(); err == nil {
                p.publishConsole("[Propel Daemon]: Killed via process handle.")
            }
        }
    }
    
    if err != nil {
        log.WithField("server", p.id).WithField("error", err).Debug("PowerShell returned error (may be normal)")
    }
	
	p.SetState(environment.ProcessOfflineState)
	return nil
}

// Helper to push messages to console
func (p *Process) publishConsole(msg string) {
    p.mu.RLock()
    cb := p.logCallback
    p.mu.RUnlock()
    if cb != nil {
        cb([]byte(msg + "\n"))
    }
}

func (p *Process) Destroy() error {
    username := "Srv_" + p.id[:8]
    exec.Command("net", "user", username, "/delete").Run()
	return nil
}

func (p *Process) ExitState() (uint32, bool, error) {
	return 0, false, nil
}

func (p *Process) Create() error {
	username := "Srv_" + p.id[:8]
    password := "Pw" + p.id[:8] + "!"

    check := exec.Command("net", "user", username)
    if err := check.Run(); err != nil {
        log.WithField("user", username).Info("Creating Windows user for server")
        cmd := exec.Command("net", "user", username, password, "/ADD", "/PASSWORDCHG:NO", "/EXPIRES:NEVER")
        if out, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("failed to create user: %s: %s", err, string(out))
        }
    }
    return nil
}

func (p *Process) Attach(ctx context.Context) error {
	return nil
}

func (p *Process) SendCommand(cmd string) error {
	p.mu.RLock()
	stdin := p.stdin
	p.mu.RUnlock()

	if stdin == nil {
		return fmt.Errorf("process is not running")
	}

	_, err := stdin.Write([]byte(cmd + "\n"))
	return err
}

func (p *Process) Readlog(lines int) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.logBuffer) == 0 {
		return []string{}, nil
	}

	start := len(p.logBuffer) - lines
	if start < 0 {
		start = 0
	}
	return p.logBuffer[start:], nil
}

func (p *Process) State() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *Process) SetState(s string) {
	p.mu.Lock()
	p.state = s
	p.mu.Unlock()
	p.Events().Publish(environment.StateChangeEvent, s)
}

func (p *Process) Uptime(ctx context.Context) (int64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.startTime.IsZero() || p.state == environment.ProcessOfflineState {
		return 0, nil
	}
	return time.Since(p.startTime).Milliseconds(), nil
}

func (p *Process) SetLogCallback(f func([]byte)) {
	p.mu.Lock()
	p.logCallback = f
	p.mu.Unlock()
}

// logonUser gets a Windows user token for impersonation
func logonUser(username, password string) (windows.Handle, error) {
    const (
        LOGON32_LOGON_BATCH       = 4
        LOGON32_LOGON_INTERACTIVE = 2
        LOGON32_PROVIDER_DEFAULT  = 0
    )
    
    // Load advapi32.dll and get LogonUserW function
    advapi32 := syscall.NewLazyDLL("advapi32.dll")
    procLogonUser := advapi32.NewProc("LogonUserW")
    
    usernamePtr, err := syscall.UTF16PtrFromString(username)
    if err != nil {
        return 0, fmt.Errorf("invalid username: %w", err)
    }
    
    passwordPtr, err := syscall.UTF16PtrFromString(password)
    if err != nil {
        return 0, fmt.Errorf("invalid password: %w", err)
    }
    
    // Use "." for local machine
    domainPtr, err := syscall.UTF16PtrFromString(".")
    if err != nil {
        return 0, fmt.Errorf("invalid domain: %w", err)
    }
    
    var token windows.Handle
    
    // Try batch logon first (doesn't require interactive privileges)
    ret, _, errno := procLogonUser.Call(
        uintptr(unsafe.Pointer(usernamePtr)),
        uintptr(unsafe.Pointer(domainPtr)),
        uintptr(unsafe.Pointer(passwordPtr)),
        uintptr(LOGON32_LOGON_BATCH),
        uintptr(LOGON32_PROVIDER_DEFAULT),
        uintptr(unsafe.Pointer(&token)),
    )
    
    if ret == 0 {
        // Fallback to interactive logon
        ret, _, errno = procLogonUser.Call(
            uintptr(unsafe.Pointer(usernamePtr)),
            uintptr(unsafe.Pointer(domainPtr)),
            uintptr(unsafe.Pointer(passwordPtr)),
            uintptr(LOGON32_LOGON_INTERACTIVE),
            uintptr(LOGON32_PROVIDER_DEFAULT),
            uintptr(unsafe.Pointer(&token)),
        )
        if ret == 0 {
            return 0, fmt.Errorf("LogonUser failed: %w", errno)
        }
    }
    
    return token, nil
}

// grantDirAccess grants the user full access to a directory
func grantDirAccess(dirPath, username string) {
    // Use icacls to grant access
    cmd := exec.Command("icacls", dirPath, "/grant", fmt.Sprintf("%s:(OI)(CI)F", username), "/T", "/C", "/Q")
    if out, err := cmd.CombinedOutput(); err != nil {
        log.WithField("path", dirPath).WithField("user", username).WithField("error", err).WithField("output", string(out)).Warn("Failed to grant directory access")
    } else {
        log.WithField("path", dirPath).WithField("user", username).Debug("Granted directory access")
    }
}

// isJavaCommand checks if the command is a Java executable
func isJavaCommand(cmd string) bool {
    cmd = strings.ToLower(cmd)
    return cmd == "java" || 
           cmd == "java.exe" || 
           strings.HasSuffix(cmd, "\\java.exe") ||
           strings.HasSuffix(cmd, "/java.exe") ||
           strings.Contains(cmd, "\\java\\") ||
           strings.Contains(cmd, "/java/")
}

// downloadPortableJava downloads and extracts Adoptium Temurin JRE to the server directory
func downloadPortableJava(serverDir string) error {
    // Adoptium Temurin JRE 21 for Windows x64 (current LTS)
    // Using the official Adoptium API to get the latest release
    javaURL := "https://api.adoptium.net/v3/binary/latest/21/ga/windows/x64/jre/hotspot/normal/eclipse?project=jdk"
    
    javaDir := filepath.Join(serverDir, "java")
    zipPath := filepath.Join(serverDir, "java-download.zip")
    
    log.WithField("url", javaURL).WithField("dest", javaDir).Info("Downloading portable Java")
    
    // Create java directory
    if err := os.MkdirAll(javaDir, 0755); err != nil {
        return fmt.Errorf("failed to create java directory: %w", err)
    }
    
    // Download the ZIP file
    resp, err := http.Get(javaURL)
    if err != nil {
        return fmt.Errorf("failed to download Java: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("failed to download Java: HTTP %d", resp.StatusCode)
    }
    
    // Save to file
    outFile, err := os.Create(zipPath)
    if err != nil {
        return fmt.Errorf("failed to create zip file: %w", err)
    }
    
    _, err = io.Copy(outFile, resp.Body)
    outFile.Close()
    if err != nil {
        os.Remove(zipPath)
        return fmt.Errorf("failed to save zip file: %w", err)
    }
    
    log.Info("Java downloaded, extracting...")
    
    // Extract using PowerShell (most reliable on Windows)
    psCmd := exec.Command("powershell", "-NoProfile", "-Command", 
        fmt.Sprintf(`Expand-Archive -Path "%s" -DestinationPath "%s" -Force`, zipPath, javaDir))
    if out, err := psCmd.CombinedOutput(); err != nil {
        os.Remove(zipPath)
        return fmt.Errorf("failed to extract Java: %w: %s", err, string(out))
    }
    
    // Clean up zip
    os.Remove(zipPath)
    
    // Find the extracted directory (it's named like jdk-21.0.X+Y-jre)
    // and move java.exe to java/bin/
    entries, err := os.ReadDir(javaDir)
    if err != nil {
        return fmt.Errorf("failed to read java directory: %w", err)
    }
    
    for _, entry := range entries {
        if entry.IsDir() && (strings.HasPrefix(entry.Name(), "jdk-") || strings.HasPrefix(entry.Name(), "jre-")) {
            extractedDir := filepath.Join(javaDir, entry.Name())
            
            // Move contents up one level
            subEntries, _ := os.ReadDir(extractedDir)
            for _, subEntry := range subEntries {
                src := filepath.Join(extractedDir, subEntry.Name())
                dst := filepath.Join(javaDir, subEntry.Name())
                os.Rename(src, dst)
            }
            os.RemoveAll(extractedDir)
            break
        }
    }
    
    // Verify java.exe exists
    javaExe := filepath.Join(javaDir, "bin", "java.exe")
    if _, err := os.Stat(javaExe); os.IsNotExist(err) {
        return fmt.Errorf("java.exe not found after extraction")
    }
    
    // Hide the java folder from Panel file browser
    hideCmd := exec.Command("attrib", "+h", "+s", javaDir)
    if out, err := hideCmd.CombinedOutput(); err != nil {
        log.WithField("error", err).WithField("output", string(out)).Warn("Failed to hide java folder")
    } else {
        log.WithField("path", javaDir).Debug("Java folder hidden from file browser")
    }
    
    log.WithField("path", javaExe).Info("Portable Java installed successfully")
    return nil
}

// isHytaleServer checks if this server is running Hytale
// Hytale uses Java 25 features like AOTCache which aren't available in older Java versions
func isHytaleServer(args []string, envVars []string) bool {
    // Check args for Hytale indicators
    for _, arg := range args {
        argLower := strings.ToLower(arg)
        if strings.Contains(argLower, "hytale") ||
           strings.Contains(argLower, "hypixel") {
            return true
        }
    }
    
    // Check environment variables
    for _, env := range envVars {
        envLower := strings.ToLower(env)
        if strings.Contains(envLower, "hytale") ||
           strings.Contains(envLower, "hypixel") {
            return true
        }
    }
    
    // Check for AOTCache JVM option (only Java 25+ supports this)
    for _, arg := range args {
        if strings.Contains(arg, "AOTCache") {
            return true
        }
    }
    
    return false
}

// downloadJava25ForHytale downloads Java 25 (required for Hytale's AOTCache feature)
func downloadJava25ForHytale(serverDir string) error {
    // Adoptium OpenJDK 25 EA (Early Access) - required for AOTCache
    // Note: Using GitHub releases for Java 25 since it's newer
    javaURL := "https://api.adoptium.net/v3/binary/latest/25/ea/windows/x64/jdk/hotspot/normal/eclipse?project=jdk"
    
    java25Dir := filepath.Join(serverDir, "java25")
    zipPath := filepath.Join(serverDir, "java25-download.zip")
    
    log.WithField("url", javaURL).WithField("dest", java25Dir).Info("Downloading Java 25 for Hytale (AOTCache support)")
    
    // Create java25 directory
    if err := os.MkdirAll(java25Dir, 0755); err != nil {
        return fmt.Errorf("failed to create java25 directory: %w", err)
    }
    
    // Create HTTP client with longer timeout for large download
    client := &http.Client{
        Timeout: 10 * time.Minute,
    }
    
    // Download the ZIP file
    resp, err := client.Get(javaURL)
    if err != nil {
        return fmt.Errorf("failed to download Java 25: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("failed to download Java 25: HTTP %d", resp.StatusCode)
    }
    
    // Save to file
    outFile, err := os.Create(zipPath)
    if err != nil {
        return fmt.Errorf("failed to create zip file: %w", err)
    }
    
    _, err = io.Copy(outFile, resp.Body)
    outFile.Close()
    if err != nil {
        os.Remove(zipPath)
        return fmt.Errorf("failed to save zip file: %w", err)
    }
    
    log.Info("Java 25 downloaded, extracting...")
    
    // Extract using PowerShell (most reliable on Windows)
    psCmd := exec.Command("powershell", "-NoProfile", "-Command", 
        fmt.Sprintf(`Expand-Archive -Path "%s" -DestinationPath "%s" -Force`, zipPath, java25Dir))
    if out, err := psCmd.CombinedOutput(); err != nil {
        os.Remove(zipPath)
        return fmt.Errorf("failed to extract Java 25: %w: %s", err, string(out))
    }
    
    // Clean up zip
    os.Remove(zipPath)
    
    // Find the extracted directory and move contents up one level
    entries, err := os.ReadDir(java25Dir)
    if err != nil {
        return fmt.Errorf("failed to read java25 directory: %w", err)
    }
    
    for _, entry := range entries {
        if entry.IsDir() && (strings.HasPrefix(entry.Name(), "jdk-") || strings.HasPrefix(entry.Name(), "jre-")) {
            extractedDir := filepath.Join(java25Dir, entry.Name())
            
            // Move contents up one level
            subEntries, _ := os.ReadDir(extractedDir)
            for _, subEntry := range subEntries {
                src := filepath.Join(extractedDir, subEntry.Name())
                dst := filepath.Join(java25Dir, subEntry.Name())
                os.Rename(src, dst)
            }
            os.RemoveAll(extractedDir)
            break
        }
    }
    
    // Verify java.exe exists
    javaExe := filepath.Join(java25Dir, "bin", "java.exe")
    if _, err := os.Stat(javaExe); os.IsNotExist(err) {
        return fmt.Errorf("java.exe not found after Java 25 extraction")
    }
    
    // Hide the java25 folder from Panel file browser
    hideCmd := exec.Command("attrib", "+h", "+s", java25Dir)
    if out, err := hideCmd.CombinedOutput(); err != nil {
        log.WithField("error", err).WithField("output", string(out)).Warn("Failed to hide java25 folder")
    } else {
        log.WithField("path", java25Dir).Debug("Java 25 folder hidden from file browser")
    }
    
    log.WithField("path", javaExe).Info("Java 25 for Hytale installed successfully")
    return nil
}

// Windows Job Object constants
const (
    JOB_OBJECT_CPU_RATE_CONTROL_ENABLE     = 0x1
    JOB_OBJECT_CPU_RATE_CONTROL_HARD_CAP   = 0x4
    JobObjectCpuRateControlInformation     = 15
)

// JOBOBJECT_CPU_RATE_CONTROL_INFORMATION structure
type cpuRateControlInfo struct {
    ControlFlags uint32
    CpuRate      uint32
}

// applyJobObjectLimits creates a Job Object and applies hard CPU limits
func applyJobObjectLimits(pid int, cpuLimit int64) error {
    kernel32 := syscall.NewLazyDLL("kernel32.dll")
    procCreateJobObject := kernel32.NewProc("CreateJobObjectW")
    procAssignProcess := kernel32.NewProc("AssignProcessToJobObject")
    procSetInformationJobObject := kernel32.NewProc("SetInformationJobObject")
    
    // Create a Job Object
    jobHandle, _, err := procCreateJobObject.Call(0, 0)
    if jobHandle == 0 {
        return fmt.Errorf("CreateJobObject failed: %v", err)
    }
    
    // Open the process
    processHandle, err := windows.OpenProcess(
        windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_ALL_ACCESS,
        false,
        uint32(pid),
    )
    if err != nil {
        windows.CloseHandle(windows.Handle(jobHandle))
        return fmt.Errorf("OpenProcess failed: %v", err)
    }
    defer windows.CloseHandle(processHandle)
    
    // Assign process to job
    ret, _, err := procAssignProcess.Call(jobHandle, uintptr(processHandle))
    if ret == 0 {
        windows.CloseHandle(windows.Handle(jobHandle))
        return fmt.Errorf("AssignProcessToJobObject failed: %v", err)
    }
    
    // Set CPU rate control
    // cpuLimit is in format: 100 = 1 core, so CpuRate = cpuLimit * 100
    // CpuRate is percentage * 100, so 10000 = 100% of one core
    cpuRate := uint32(cpuLimit * 100)
    if cpuRate < 100 {
        cpuRate = 100  // Minimum 1%
    }
    
    info := cpuRateControlInfo{
        ControlFlags: JOB_OBJECT_CPU_RATE_CONTROL_ENABLE | JOB_OBJECT_CPU_RATE_CONTROL_HARD_CAP,
        CpuRate:      cpuRate,
    }
    
    ret, _, err = procSetInformationJobObject.Call(
        jobHandle,
        uintptr(JobObjectCpuRateControlInformation),
        uintptr(unsafe.Pointer(&info)),
        uintptr(unsafe.Sizeof(info)),
    )
    
    if ret == 0 {
        log.WithField("error", err).Warn("SetInformationJobObject (CPU) failed")
    } else {
        log.WithField("cpuRate", cpuRate).Info("Applied hard CPU limit via Job Object")
    }
    
    // Don't close the job handle - it needs to stay open for limits to be enforced
    // The handle will be closed when the process exits
    
    return nil
}




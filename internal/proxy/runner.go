package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// State persisted alongside the PID file.
type State struct {
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	ListenAddr string    `json:"listen_addr"`
}

// StartOptions controls how Start launches the daemon.
type StartOptions struct {
	FreshLogs bool
}

// LoadState reads the PID file. Returns os.ErrNotExist if no daemon was started.
func LoadState() (*State, error) {
	path, err := PIDPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("malformed PID file %s: %w", path, err)
	}
	return &s, nil
}

func saveState(s *State) error {
	if err := EnsureDir(); err != nil {
		return err
	}
	path, err := PIDPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func clearState() error {
	path, err := PIDPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsAlive checks whether the given PID exists.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return windowsProcessExists(pid)
	}
	if err := p.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

// Start forks self as `aitoll proxy daemon`, detaches it, and records state.
// Errors if a daemon is already running.
func Start(opts StartOptions) (*State, error) {
	if existing, err := LoadState(); err == nil && IsAlive(existing.PID) {
		return nil, fmt.Errorf("aitoll proxy already running (PID %d, started %s)",
			existing.PID, existing.StartedAt.Format(time.RFC3339))
	}
	_ = clearState()

	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load proxy config: %w", err)
	}

	// Make sure we have a singbox.json before we fork — otherwise the daemon
	// just dies immediately and the user has no idea why.
	if err := EnsureCachedConfig(); err != nil {
		return nil, fmt.Errorf("prepare singbox config: %w", err)
	}

	if err := EnsureDir(); err != nil {
		return nil, err
	}
	logPath, err := LogPath()
	if err != nil {
		return nil, err
	}
	logFlags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if opts.FreshLogs {
		logFlags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	logFile, err := os.OpenFile(logPath, logFlags, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer logFile.Close()

	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate self: %w", err)
	}

	cmd := exec.Command(self, "proxy", "daemon")
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	applyDetachAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fork daemon: %w", err)
	}
	go func() { _ = cmd.Wait() }()

	state := &State{
		PID:        cmd.Process.Pid,
		StartedAt:  time.Now(),
		ListenAddr: cfg.ListenAddr,
	}
	if err := saveState(state); err != nil {
		_ = killProcess(cmd.Process.Pid)
		return nil, fmt.Errorf("started daemon (PID %d) but failed to save state: %w", cmd.Process.Pid, err)
	}

	// Give the daemon a moment to bind the listener before we report success.
	if err := waitForListener(cfg.ListenAddr, 5*time.Second); err != nil {
		// Don't kill it — sing-box may still be initializing rule-sets, GeoIP, etc.
		// Just warn through the returned state.
		return state, fmt.Errorf("daemon started (PID %d) but listener %s not ready yet: %w (check logs: aitoll proxy logs --follow)",
			state.PID, cfg.ListenAddr, err)
	}
	return state, nil
}

// Stop terminates the running daemon. Returns os.ErrNotExist if no daemon.
func Stop() (int, error) {
	state, err := LoadState()
	if err != nil {
		return 0, err
	}
	if !IsAlive(state.PID) {
		_ = clearState()
		return state.PID, fmt.Errorf("PID %d not alive (stale state cleaned)", state.PID)
	}
	if err := killProcess(state.PID); err != nil {
		return state.PID, err
	}
	_ = clearState()
	return state.PID, nil
}

// EnsureRunning starts the daemon if it isn't already up. Used by the smart
// resolver so callers don't need to track lifecycle themselves.
func EnsureRunning() (*State, error) {
	if existing, err := LoadState(); err == nil && IsAlive(existing.PID) {
		return existing, nil
	}
	return Start(StartOptions{})
}

// ListenerURL returns the http://host:port URL to point HTTP_PROXY at.
func ListenerURL(addr string) string {
	if addr == "" {
		addr = DefaultListenAddr
	}
	return "http://" + addr
}

func waitForListener(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("timeout")
	}
	return lastErr
}

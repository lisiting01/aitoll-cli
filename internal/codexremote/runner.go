package codexremote

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/proxy"
)

// State persisted alongside the PID file.
type State struct {
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	Proxy       string    `json:"proxy"`
	ProxySource string    `json:"proxy_source,omitempty"`
}

// StartOptions controls how Start launches the daemon.
type StartOptions struct {
	Proxy     string // overrides config; empty means use config
	FreshLogs bool   // truncate log instead of append
	Debug     bool   // inject RUST_LOG=codex=debug,... for verbose output
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

// IsAlive checks whether the given PID exists. Uses signal 0 (probe) on Unix,
// and Go's runtime emulation on Windows.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		// On Windows, FindProcess always succeeds; verify via a separate call.
		return windowsProcessExists(pid)
	}
	if err := p.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

// FindCodex returns the absolute path to the codex executable.
// Tries PATH first, then npm global install location on Windows.
func FindCodex() (string, error) {
	candidates := []string{"codex"}
	if runtime.GOOS == "windows" {
		candidates = []string{"codex.cmd", "codex.exe", "codex"}
	}
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata != "" {
			for _, name := range []string{"codex.cmd", "codex.exe"} {
				p := filepath.Join(appdata, "npm", name)
				if _, err := os.Stat(p); err == nil {
					return p, nil
				}
			}
		}
	}
	return "", errors.New("codex executable not found in PATH (try: npm install -g @openai/codex@0.130.0)")
}

// CodexVersion runs `codex --version` and returns the version string.
func CodexVersion(codexPath string) (string, error) {
	out, err := exec.Command(codexPath, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckProxy verifies the proxy can reach chatgpt.com.
// Returns the latency on success.
func CheckProxy(proxyURL string) (time.Duration, error) {
	if proxyURL == "" {
		return 0, errors.New("empty proxy URL")
	}
	tr := &http.Transport{
		Proxy: http.ProxyURL(parseProxy(proxyURL)),
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	start := time.Now()
	req, err := http.NewRequest(http.MethodHead, "https://chatgpt.com/", nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return time.Since(start), nil
}

// Start launches `codex remote-control --enable remote_control` in the background.
// Returns the new state on success. Errors if a daemon is already running.
func Start(opts StartOptions) (*State, error) {
	if existing, err := LoadState(); err == nil && IsAlive(existing.PID) {
		return nil, fmt.Errorf("codex-remote already running (PID %d, started %s)",
			existing.PID, existing.StartedAt.Format(time.RFC3339))
	}
	// Stale state file — clean up before proceeding.
	_ = clearState()

	codexPath, err := FindCodex()
	if err != nil {
		return nil, err
	}

	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Smart proxy resolution: explicit --proxy flag > config value > smart fallback
	// (probe local proxy software; if none, start the aitoll proxy daemon).
	override := opts.Proxy
	if override == "" {
		override = cfg.Proxy
	}
	resolved, rErr := proxy.Resolve(proxy.ResolveOptions{
		Override:        override,
		AutoStartDaemon: true,
	})
	if rErr != nil {
		return nil, fmt.Errorf("resolve proxy: %w", rErr)
	}
	proxyURL := resolved.URL
	proxySource := string(resolved.Source)

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
		return nil, fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.Command(codexPath, "remote-control", "--enable", "remote_control")
	cmd.Env = append(os.Environ(),
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"ALL_PROXY="+proxyURL,
	)
	if opts.Debug {
		cmd.Env = append(cmd.Env, "RUST_LOG=codex=debug,codex_app_server=debug,codex_app_server_transport=debug,info")
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	applyDetachAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start codex: %w", err)
	}

	// Detach: don't Wait. Let the OS reap when it exits.
	go func() { _ = cmd.Wait() }()

	state := &State{
		PID:         cmd.Process.Pid,
		StartedAt:   time.Now(),
		Proxy:       proxyURL,
		ProxySource: proxySource,
	}
	if err := saveState(state); err != nil {
		// We started the process but failed to record state. Try to kill it
		// to keep things consistent.
		_ = killProcess(cmd.Process.Pid)
		return nil, fmt.Errorf("started codex (PID %d) but failed to save state: %w", cmd.Process.Pid, err)
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

func parseProxy(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return nil
	}
	return u
}

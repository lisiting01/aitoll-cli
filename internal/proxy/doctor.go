package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/auth"
)

// CheckLevel mirrors codex-remote's doctor levels for consistent output.
type CheckLevel int

const (
	LevelOK CheckLevel = iota
	LevelWarn
	LevelInfo
	LevelError
)

func (l CheckLevel) String() string {
	switch l {
	case LevelOK:
		return "OK"
	case LevelWarn:
		return "WARN"
	case LevelInfo:
		return "INFO"
	case LevelError:
		return "ERROR"
	default:
		return "?"
	}
}

// CheckResult is one row of doctor output.
type CheckResult struct {
	Level   CheckLevel
	Message string
}

// Doctor runs the 5 proxy preflight checks and returns results in order.
func Doctor() []CheckResult {
	results := make([]CheckResult, 0, 5)

	// 1. Logged in?
	store := auth.NewTokenStore()
	creds, err := store.Load()
	switch {
	case err != nil:
		results = append(results, CheckResult{LevelError, "read credentials: " + err.Error()})
	case creds == nil || creds.Token == "":
		results = append(results, CheckResult{LevelError, "not logged in — run `aitoll login`"})
	default:
		ok, _ := store.IsTokenValid()
		if !ok {
			results = append(results, CheckResult{LevelError, "stored token is expired — run `aitoll login` again"})
		} else {
			results = append(results, CheckResult{LevelOK, "logged in (base " + creds.BaseURL + ")"})
		}
	}

	// 2. Cached singbox config present?
	results = append(results, checkSingboxCache())

	// 3. Daemon running?
	results = append(results, checkDaemonState())

	// 4. Listener reachable on TCP?
	results = append(results, checkListener())

	// 5. End-to-end reach to chatgpt.com via the listener (only if running).
	results = append(results, checkChatgpt())

	return results
}

func checkSingboxCache() CheckResult {
	path, err := SingboxConfigPath()
	if err != nil {
		return CheckResult{LevelError, err.Error()}
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{LevelWarn, path + " missing — run `aitoll proxy sync` (or `aitoll proxy start` will sync on demand)"}
		}
		return CheckResult{LevelError, "stat " + path + ": " + err.Error()}
	}
	meta, _ := LoadSyncMeta()
	age := "unknown"
	if meta != nil {
		age = time.Since(meta.SyncedAt).Round(time.Second).String() + " ago"
	}
	return CheckResult{LevelOK, fmt.Sprintf("singbox config present (%d bytes, synced %s)", st.Size(), age)}
}

func checkDaemonState() CheckResult {
	state, err := LoadState()
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{LevelInfo, "aitoll proxy daemon not running (use `aitoll proxy start`)"}
		}
		return CheckResult{LevelWarn, "read state: " + err.Error()}
	}
	if !IsAlive(state.PID) {
		return CheckResult{LevelWarn, fmt.Sprintf("PID %d in state file is not alive (stale)", state.PID)}
	}
	uptime := time.Since(state.StartedAt).Round(time.Second)
	return CheckResult{LevelOK, fmt.Sprintf("daemon running (PID %d, uptime %s, listener %s)", state.PID, uptime, state.ListenAddr)}
}

func checkListener() CheckResult {
	state, err := LoadState()
	if err != nil || !IsAlive(state.PID) {
		return CheckResult{LevelInfo, "skipping listener probe (daemon not running)"}
	}
	conn, dErr := net.DialTimeout("tcp", state.ListenAddr, 1*time.Second)
	if dErr != nil {
		return CheckResult{LevelError, fmt.Sprintf("listener %s not bound: %s", state.ListenAddr, dErr.Error())}
	}
	_ = conn.Close()
	return CheckResult{LevelOK, "listener " + state.ListenAddr + " accepting connections"}
}

func checkChatgpt() CheckResult {
	state, err := LoadState()
	if err != nil || !IsAlive(state.PID) {
		return CheckResult{LevelInfo, "skipping chatgpt.com reach test (daemon not running)"}
	}
	proxyURL, _ := url.Parse(ListenerURL(state.ListenAddr))
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	client := &http.Client{Transport: tr, Timeout: 8 * time.Second}
	start := time.Now()
	req, _ := http.NewRequest(http.MethodHead, "https://chatgpt.com/", nil)
	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{LevelError, "chatgpt.com via aitoll proxy: " + err.Error()}
	}
	defer resp.Body.Close()
	return CheckResult{LevelOK, fmt.Sprintf("chatgpt.com HEAD via aitoll proxy: %d in %dms", resp.StatusCode, time.Since(start).Milliseconds())}
}

package codexremote

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/proxy"
)

// CheckLevel mirrors the OK/WARN/INFO/ERROR labels printed to the user.
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

// Doctor runs all five checks and returns their results in order.
func Doctor() []CheckResult {
	results := make([]CheckResult, 0, 5)

	// 1. codex executable
	codexPath, err := FindCodex()
	if err != nil {
		results = append(results, CheckResult{LevelError, err.Error()})
		// Skip version check, but continue with the rest.
	} else {
		results = append(results, CheckResult{LevelOK, "codex executable: " + codexPath})

		// 2. codex version
		ver, vErr := CodexVersion(codexPath)
		switch {
		case vErr != nil:
			results = append(results, CheckResult{LevelWarn, "could not read codex version: " + vErr.Error()})
		case ver == "":
			results = append(results, CheckResult{LevelWarn, "codex --version returned empty"})
		default:
			msg := "codex version: " + ver
			if !isRecommendedVersion(ver) {
				msg += " (recommended: 0.130.0; 0.131.0+ remote-control may be gated on Windows)"
				results = append(results, CheckResult{LevelWarn, msg})
			} else {
				results = append(results, CheckResult{LevelOK, msg})
			}
		}
	}

	// 3. proxy reachability — uses the smart resolver to mirror what `start`
	//    will actually do. AutoStartDaemon is false: doctor only reports the
	//    state, it doesn't spin anything up as a side effect.
	cfg, cfgErr := LoadConfig()
	override := ""
	if cfgErr == nil {
		override = cfg.Proxy
	}
	resolved, rErr := proxy.Resolve(proxy.ResolveOptions{
		Override:        override,
		AutoStartDaemon: false,
	})
	if rErr != nil {
		results = append(results, CheckResult{LevelError, "proxy resolve: " + rErr.Error()})
	} else if resolved.URL == "" {
		results = append(results, CheckResult{LevelError, "no proxy resolved (source=" + string(resolved.Source) + ")"})
	} else {
		latency, pErr := CheckProxy(resolved.URL)
		if pErr != nil {
			results = append(results, CheckResult{LevelError, fmt.Sprintf("proxy %s (%s) unreachable: %s", resolved.URL, resolved.Source, pErr.Error())})
		} else {
			results = append(results, CheckResult{LevelOK, fmt.Sprintf("proxy %s [source=%s] reachable (chatgpt.com HEAD %dms)", resolved.URL, resolved.Source, latency.Milliseconds())})
		}
	}

	// 4. ~/.codex/auth.json
	results = append(results, checkAuthFile())

	// 5. running state
	results = append(results, checkRunningState())

	return results
}

func checkAuthFile() CheckResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return CheckResult{LevelError, "could not resolve home directory: " + err.Error()}
	}
	path := filepath.Join(home, ".codex", "auth.json")
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{LevelError, path + " missing — run `codex login` (with proxy env)"}
		}
		return CheckResult{LevelError, "stat " + path + ": " + err.Error()}
	}
	return CheckResult{LevelOK, fmt.Sprintf("%s present (%d bytes)", path, st.Size())}
}

func checkRunningState() CheckResult {
	state, err := LoadState()
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{LevelInfo, "no codex-remote process tracked (use `aitoll codex-remote start`)"}
		}
		return CheckResult{LevelWarn, "could not read state: " + err.Error()}
	}
	if !IsAlive(state.PID) {
		return CheckResult{LevelWarn, fmt.Sprintf("PID %d in state file is not alive (stale)", state.PID)}
	}
	uptime := time.Since(state.StartedAt).Round(time.Second)
	return CheckResult{LevelOK, fmt.Sprintf("codex-remote running (PID %d, uptime %s)", state.PID, uptime)}
}

func isRecommendedVersion(ver string) bool {
	// codex --version output is like "codex 0.130.0" or just "0.130.0"; match
	// against the substring 0.130.0 to be lenient.
	return contains(ver, "0.130.0")
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package proxy

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Source identifies where a resolved proxy URL came from. Surfaced in `status`
// and `doctor` so users can see which path the smart resolver took.
type Source string

const (
	SourceExplicit Source = "explicit"      // user-provided URL
	SourceLocal    Source = "local"         // detected a running local proxy
	SourceAitoll   Source = "aitoll"        // started the embedded daemon
	SourceNone     Source = "none"          // nothing worked
)

// ResolveOptions controls resolution.
type ResolveOptions struct {
	// Override, if set to a non-special value, short-circuits resolution.
	// Special values:
	//   ""       — full smart fallback
	//   "auto"   — same as ""
	//   "aitoll" — skip local detection, force the aitoll proxy daemon
	// Anything else is treated as a literal proxy URL.
	Override string

	// AutoStartDaemon controls whether Resolve will start the aitoll daemon
	// when no local proxy is found. codex-remote start passes true; doctor
	// passes false (just report the state).
	AutoStartDaemon bool
}

// Result describes a resolved proxy.
type Result struct {
	URL    string
	Source Source
	// Detail is a short human-readable note (e.g. "Clash @ 127.0.0.1:7890",
	// "started aitoll proxy daemon PID 1234"). Surfaced by status/doctor.
	Detail string
}

// commonLocalProxies lists ports we probe in order. Ordering matters:
// 7890 = Clash classic, 7897 = Clash Verge default, 10809 = V2RayN HTTP,
// 1080 = generic SOCKS/HTTP fallback.
var commonLocalProxies = []string{
	"127.0.0.1:7890",
	"127.0.0.1:7897",
	"127.0.0.1:10809",
	"127.0.0.1:1080",
}

// Resolve picks a proxy URL according to the smart fallback policy. See
// ResolveOptions.Override for the precedence rules.
func Resolve(opts ResolveOptions) (*Result, error) {
	switch strings.ToLower(strings.TrimSpace(opts.Override)) {
	case "":
		// fall through to smart fallback
	case "auto":
		// same: smart fallback
	case "aitoll":
		return resolveAitoll(opts.AutoStartDaemon)
	default:
		return &Result{URL: opts.Override, Source: SourceExplicit, Detail: "user-provided"}, nil
	}

	// Smart fallback: probe locals first.
	if r := probeLocal(); r != nil {
		return r, nil
	}

	// No local proxy — fall back to aitoll daemon.
	return resolveAitoll(opts.AutoStartDaemon)
}

func probeLocal() *Result {
	for _, addr := range commonLocalProxies {
		if dialOK(addr, 300*time.Millisecond) {
			return &Result{
				URL:    "http://" + addr,
				Source: SourceLocal,
				Detail: fmt.Sprintf("local proxy detected at %s", addr),
			}
		}
	}
	return nil
}

func resolveAitoll(autoStart bool) (*Result, error) {
	if state, err := LoadState(); err == nil && IsAlive(state.PID) {
		return &Result{
			URL:    ListenerURL(state.ListenAddr),
			Source: SourceAitoll,
			Detail: fmt.Sprintf("aitoll proxy daemon already running (PID %d)", state.PID),
		}, nil
	}
	if !autoStart {
		return &Result{Source: SourceNone}, errors.New("aitoll proxy daemon is not running; start it with `aitoll proxy start`")
	}
	state, err := Start(StartOptions{})
	if err != nil {
		return &Result{Source: SourceNone}, fmt.Errorf("start aitoll proxy daemon: %w", err)
	}
	return &Result{
		URL:    ListenerURL(state.ListenAddr),
		Source: SourceAitoll,
		Detail: fmt.Sprintf("started aitoll proxy daemon (PID %d)", state.PID),
	}, nil
}

func dialOK(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

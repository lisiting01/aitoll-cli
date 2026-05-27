package codexremote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LastInvocation records the user-supplied flags from the most recent
// successful Start, so `aitoll codex-remote restart` can replay them after a
// stop (which removes the PID state file).
//
// Only original user intent is stored — not the resolved proxy URL or source.
// Restart re-runs the proxy resolver in the current network environment.
type LastInvocation struct {
	Debug         bool      `json:"debug"`
	ProxyOverride string    `json:"proxy_override"`
	SavedAt       time.Time `json:"saved_at"`
}

func lastInvocationPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "last-invocation.json"), nil
}

// LoadLastInvocation reads last-invocation.json. Returns (nil, nil) if the
// file does not exist; an error is returned only for IO/parse failures.
func LoadLastInvocation() (*LastInvocation, error) {
	path, err := lastInvocationPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var li LastInvocation
	if err := json.Unmarshal(data, &li); err != nil {
		return nil, err
	}
	return &li, nil
}

// SaveLastInvocation writes last-invocation.json.
func SaveLastInvocation(li *LastInvocation) error {
	if err := EnsureDir(); err != nil {
		return err
	}
	path, err := lastInvocationPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(li, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

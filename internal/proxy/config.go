package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lisiting01/aitoll-cli/internal/config"
)

// DefaultListenAddr is where the embedded sing-box mixed inbound listens.
// 7891 (one above Clash's 7890 default) avoids colliding with a running Clash.
const DefaultListenAddr = "127.0.0.1:7891"

// Config holds aitoll proxy local settings (the listener; not the upstream).
type Config struct {
	ListenAddr string `json:"listen_addr"`
}

// Dir returns ~/.config/aitoll/proxy/ (or platform equivalent).
func Dir() (string, error) {
	base, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "proxy"), nil
}

// EnsureDir creates the proxy subdirectory.
func EnsureDir() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

func configPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SingboxConfigPath is the cached sing-box JSON pulled from the aitoll API.
func SingboxConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "singbox.json"), nil
}

// SyncMetaPath stores synced_at and source URL alongside singbox.json.
func SyncMetaPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sync.json"), nil
}

// PIDPath returns the PID/state file path for the proxy daemon.
func PIDPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "proxy.pid"), nil
}

// LogPath returns the proxy daemon log file path.
func LogPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "proxy.log"), nil
}

// LoadConfig reads config.json, returning defaults if missing.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{ListenAddr: DefaultListenAddr}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &Config{ListenAddr: DefaultListenAddr}, nil
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	return &cfg, nil
}

// SaveConfig writes config.json.
func SaveConfig(cfg *Config) error {
	if err := EnsureDir(); err != nil {
		return err
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

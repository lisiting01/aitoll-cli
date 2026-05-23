package codexremote

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lisiting01/aitoll-cli/internal/config"
)

const DefaultProxy = "http://127.0.0.1:7890"

// Config holds codex-remote specific settings.
type Config struct {
	Proxy string `json:"proxy"`
}

// Dir returns ~/.config/aitoll/codex-remote/ (or platform equivalent).
func Dir() (string, error) {
	base, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "codex-remote"), nil
}

// EnsureDir creates the codex-remote subdirectory.
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

// LoadConfig reads config.json, returning defaults if missing.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Proxy: DefaultProxy}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &Config{Proxy: DefaultProxy}, nil
	}
	if cfg.Proxy == "" {
		cfg.Proxy = DefaultProxy
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

// PIDPath returns the PID file path.
func PIDPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-remote.pid"), nil
}

// LogPath returns the log file path.
func LogPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-remote.log"), nil
}

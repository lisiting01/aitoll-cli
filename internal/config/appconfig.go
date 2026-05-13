package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AppConfig holds persistent user preferences (separate from credentials).
type AppConfig struct {
	AutoUpdate bool `json:"auto_update"`
}

func appConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadAppConfig reads config.json, returning defaults if the file doesn't exist.
func LoadAppConfig() (*AppConfig, error) {
	path, err := appConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AppConfig{AutoUpdate: true}, nil
		}
		return nil, err
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &AppConfig{AutoUpdate: true}, nil
	}
	return &cfg, nil
}

// SaveAppConfig writes config.json.
func SaveAppConfig(cfg *AppConfig) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	path, err := appConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

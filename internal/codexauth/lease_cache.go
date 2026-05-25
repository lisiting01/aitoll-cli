package codexauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/config"
)

// LeaseCache mirrors what the platform returned at lease time. It lives at
// {ConfigDir}/aitoll/codex-auth/lease.json so `status` / `doctor` /
// `codex-remote start`'s expiry warning can answer "are we leased?"
// without round-tripping to the backend.
//
// The cache is the source of truth for *local intent* — if it disagrees
// with the platform (e.g. user revoked from another machine), the platform
// wins and the next online call (status, refresh) will overwrite or clear
// the cache.
//
// Token / password are NOT cached. Tokens go to ~/.codex/auth.json (where
// codex looks for them); the lease password is shown to the user once at
// lease time and never persisted.
type LeaseCache struct {
	LeaseID       string    `json:"leaseId"`
	OpenAIEmail   string    `json:"openaiEmail"`
	AccountType   string    `json:"accountType,omitempty"` // Free/Plus/Pro/Team
	LeasedAt      time.Time `json:"leasedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	RefreshAt     time.Time `json:"refreshAt,omitempty"` // when CLI should auto-refresh
	BackupPath    string    `json:"backupPath,omitempty"`
	PlatformBase  string    `json:"platformBase"`
	Version       int       `json:"version"` // schema version, currently 1
}

// LeaseCachePath is {ConfigDir}/aitoll/codex-auth/lease.json.
func LeaseCachePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-auth", "lease.json"), nil
}

// SaveLeaseCache writes the cache to disk, creating its directory if
// needed. Best-effort: a failure here doesn't break the lease itself
// (the auth.json is already written), it just disables local status
// reads.
func SaveLeaseCache(c *LeaseCache) error {
	path, err := LeaseCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}
	c.Version = 1
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lease cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// LoadLeaseCache returns the cached lease, or (nil, nil) if none is
// stored. Stale or expired entries are returned as-is — caller decides
// whether expiry matters.
func LoadLeaseCache() (*LeaseCache, error) {
	path, err := LeaseCachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c LeaseCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

// ClearLeaseCache removes the cache file. Called on `release` and when
// the platform tells us a lease no longer exists.
func ClearLeaseCache() error {
	path, err := LeaseCachePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// IsExpired reports whether the cached lease's ExpiresAt is in the past.
func (c *LeaseCache) IsExpired() bool {
	if c == nil {
		return false
	}
	return !c.ExpiresAt.IsZero() && time.Now().After(c.ExpiresAt)
}

// Remaining returns how long until the cached lease expires. Negative
// duration means already expired.
func (c *LeaseCache) Remaining() time.Duration {
	if c == nil || c.ExpiresAt.IsZero() {
		return 0
	}
	return time.Until(c.ExpiresAt)
}

package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/api"
	"github.com/lisiting01/aitoll-cli/internal/auth"
)

// SyncMeta tracks when the singbox config was last refreshed.
type SyncMeta struct {
	SyncedAt time.Time `json:"synced_at"`
	BaseURL  string    `json:"base_url"`
}

// ErrNotLoggedIn is returned when the proxy needs a fresh config from the
// platform but the user has no valid token.
var ErrNotLoggedIn = errors.New("not logged in — run `aitoll login` (or set up a local proxy via `aitoll codex-remote config set proxy <url>`)")

// ConfigCacheTTL is how long a cached singbox.json is considered fresh.
// After this, background sync is attempted on next daemon start.
const ConfigCacheTTL = 24 * time.Hour

// Sync fetches a fresh singbox config from the aitoll backend and caches it
// locally. The token must be valid; baseURL is read from credentials.
func Sync() error {
	store := auth.NewTokenStore()
	creds, err := store.Load()
	if err != nil {
		return fmt.Errorf("read credentials: %w", err)
	}
	if creds == nil || creds.Token == "" {
		return ErrNotLoggedIn
	}
	ok, _ := store.IsTokenValid()
	if !ok {
		return fmt.Errorf("token expired: %w", ErrNotLoggedIn)
	}

	client := api.NewClient(creds.BaseURL)
	body, err := client.GetProxyConfig(creds.Token)
	if err != nil {
		return fmt.Errorf("fetch proxy config: %w", err)
	}

	// Validate it's JSON before we write it.
	var sniff map[string]any
	if err := json.Unmarshal(body, &sniff); err != nil {
		return fmt.Errorf("backend returned non-JSON proxy config: %w", err)
	}

	if err := EnsureDir(); err != nil {
		return err
	}
	path, err := SingboxConfigPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	meta := SyncMeta{SyncedAt: time.Now(), BaseURL: creds.BaseURL}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	metaPath, _ := SyncMetaPath()
	_ = os.WriteFile(metaPath, metaBytes, 0600)
	return nil
}

// LoadSyncMeta returns the last sync metadata, or nil if never synced.
func LoadSyncMeta() (*SyncMeta, error) {
	path, err := SyncMetaPath()
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
	var meta SyncMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// CacheIsFresh reports whether the cached singbox.json is younger than TTL.
func CacheIsFresh() bool {
	meta, err := LoadSyncMeta()
	if err != nil || meta == nil {
		return false
	}
	return time.Since(meta.SyncedAt) < ConfigCacheTTL
}

// EnsureCachedConfig makes sure singbox.json exists, syncing on demand if
// missing. If the cache exists but is stale, sync is scheduled in the
// background so we don't block startup.
func EnsureCachedConfig() error {
	path, err := SingboxConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// Cache missing — must block-sync.
		return Sync()
	}
	if !CacheIsFresh() {
		go func() { _ = Sync() }()
	}
	return nil
}

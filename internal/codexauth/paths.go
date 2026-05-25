// Package codexauth provides local file management for ~/.codex/auth.json,
// the OAuth credentials file Codex CLI / app-server reads to authenticate
// with chatgpt.com. It exists to support `aitoll codex-auth lease/release/
// status/import/whoami/refresh/doctor` —— the user-facing commands that
// let aitoll-platform users borrow an OpenAI account from a managed pool
// (see plan: aitoll-codex-auth-auth-sharded-wirth.md).
//
// Design constraints:
//   - Atomic writes (write .tmp, rename) so a crashed lease never leaves
//     half-written auth.json that breaks codex.
//   - Backup before overwriting (auth.backup.<unix>.json), keep last 5.
//   - File lock (~/.codex/auth.lock) so concurrent `aitoll codex-auth lease`
//     invocations on the same machine don't race.
//   - Zero coupling to internal/codexremote — codexremote can optionally
//     import codexauth.LeaseInfo() to enrich its doctor output, but
//     codexauth never imports codexremote.
package codexauth

import (
	"fmt"
	"os"
	"path/filepath"
)

// CodexDir returns the user's ~/.codex directory (where Codex CLI stores
// auth.json, sessions, etc). Cross-platform: uses os.UserHomeDir which
// resolves to %USERPROFILE% on Windows and $HOME on Unix.
func CodexDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// AuthJSONPath returns the absolute path to ~/.codex/auth.json.
func AuthJSONPath() (string, error) {
	dir, err := CodexDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// LockPath returns the path to the cross-process lock file.
func LockPath() (string, error) {
	dir, err := CodexDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.lock"), nil
}

// BackupGlob returns a glob pattern matching all timestamped backups
// `auth.backup.<unix>.json`. Use filepath.Glob to enumerate.
func BackupGlob() (string, error) {
	dir, err := CodexDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.backup.*.json"), nil
}

// EnsureCodexDir creates ~/.codex with mode 0700 if missing. Codex CLI
// itself creates this on first login, but we may be the first writer.
func EnsureCodexDir() error {
	dir, err := CodexDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

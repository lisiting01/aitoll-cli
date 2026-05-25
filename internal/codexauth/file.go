package codexauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MaxBackupsKept caps how many auth.backup.*.json files we retain after
// a backup-before-overwrite. Older backups are pruned silently.
const MaxBackupsKept = 5

// ReadAuthJSON reads ~/.codex/auth.json and decodes it into a generic map.
// Returns (nil, nil) if the file does not exist —— callers can check for
// this case explicitly.
func ReadAuthJSON() (map[string]any, error) {
	path, err := AuthJSONPath()
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
	var blob map[string]any
	if err := json.Unmarshal(data, &blob); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return blob, nil
}

// ReadAuthJSONRaw returns the raw bytes of ~/.codex/auth.json without
// parsing. Used when we want to back it up byte-for-byte.
func ReadAuthJSONRaw() ([]byte, error) {
	path, err := AuthJSONPath()
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
	return data, nil
}

// WriteAuthJSONAtomic serializes blob and atomically replaces ~/.codex/auth.json.
// Implementation: write to a sibling tempfile, fsync, then rename. POSIX
// rename is atomic; Windows rename within the same volume is also atomic
// per MSDN MoveFileEx with MOVEFILE_REPLACE_EXISTING (which os.Rename uses).
//
// File mode is forced to 0600 even on Windows (Go honors the readable bit).
func WriteAuthJSONAtomic(blob map[string]any) error {
	if err := EnsureCodexDir(); err != nil {
		return err
	}
	path, err := AuthJSONPath()
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(blob, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth.json: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("fsync tempfile: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close tempfile: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// WriteAuthJSONRawAtomic is the byte-level variant — useful when the caller
// already has authoritative bytes (e.g. from a backup file we're restoring)
// and shouldn't re-marshal.
func WriteAuthJSONRawAtomic(body []byte) error {
	if err := EnsureCodexDir(); err != nil {
		return err
	}
	path, err := AuthJSONPath()
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// Backup copies the current auth.json to auth.backup.<unix>.json (sibling
// directory). Returns the backup path on success, or empty string + nil
// if there was nothing to back up. Old backups beyond MaxBackupsKept are
// pruned automatically.
func Backup() (string, error) {
	raw, err := ReadAuthJSONRaw()
	if err != nil {
		return "", err
	}
	if raw == nil {
		return "", nil
	}
	dir, err := CodexDir()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("auth.backup.%d.json", time.Now().Unix())
	target := filepath.Join(dir, name)
	if err := os.WriteFile(target, raw, 0600); err != nil {
		return "", fmt.Errorf("write backup %s: %w", target, err)
	}
	if err := pruneBackups(MaxBackupsKept); err != nil {
		// pruning failure is non-fatal — backup itself succeeded
		return target, nil
	}
	return target, nil
}

// ListBackups returns absolute paths of all auth.backup.*.json files,
// sorted newest first (by embedded unix timestamp, falling back to mtime).
func ListBackups() ([]string, error) {
	pattern, err := BackupGlob()
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool {
		return backupTimestamp(matches[i]) > backupTimestamp(matches[j])
	})
	return matches, nil
}

// LatestBackup returns the most recent backup path, or empty string if none.
func LatestBackup() (string, error) {
	all, err := ListBackups()
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", nil
	}
	return all[0], nil
}

// RestoreLatest replaces auth.json with the contents of the latest backup.
// Returns the path of the backup used. If no backup exists, returns ("", nil)
// without modifying anything — caller decides whether that's an error.
func RestoreLatest() (string, error) {
	latest, err := LatestBackup()
	if err != nil {
		return "", err
	}
	if latest == "" {
		return "", nil
	}
	body, err := os.ReadFile(latest)
	if err != nil {
		return "", fmt.Errorf("read backup %s: %w", latest, err)
	}
	if err := WriteAuthJSONRawAtomic(body); err != nil {
		return "", err
	}
	return latest, nil
}

// DeleteAuthJSON removes ~/.codex/auth.json. Used by `release --keep-auth=false`
// when there's no backup to restore. Missing file is not an error.
func DeleteAuthJSON() error {
	path, err := AuthJSONPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// pruneBackups deletes oldest backups until at most maxKeep remain.
func pruneBackups(maxKeep int) error {
	all, err := ListBackups()
	if err != nil {
		return err
	}
	if len(all) <= maxKeep {
		return nil
	}
	for _, p := range all[maxKeep:] {
		_ = os.Remove(p)
	}
	return nil
}

// backupTimestamp extracts the embedded unix timestamp from a backup
// filename. Returns 0 if the name doesn't match.
func backupTimestamp(path string) int64 {
	base := filepath.Base(path)
	const prefix = "auth.backup."
	const suffix = ".json"
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
		return 0
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(base, prefix), suffix)
	n, err := strconv.ParseInt(mid, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

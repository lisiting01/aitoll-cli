package codexauth

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// ErrLocked is returned when the lock file is already held by another
// process. Callers should treat this as "another aitoll codex-auth
// invocation is in progress".
var ErrLocked = errors.New("auth.lock held by another process")

// Lock represents an acquired lock on ~/.codex/auth.lock. Callers must
// call Release exactly once.
type Lock struct {
	path string
}

// Acquire creates ~/.codex/auth.lock with O_CREATE|O_EXCL — the syscall
// fails atomically if the file exists, which is the cross-platform way
// to coordinate without flock(2). Lock contents are the holder PID
// (informational).
//
// Stale locks (process gone) are NOT auto-cleaned to keep this simple.
// If a previous run crashed without releasing, the user must remove
// auth.lock manually. We document this in `aitoll codex-auth doctor`.
//
// timeout > 0 enables a poll-and-retry loop with 50ms intervals.
func Acquire(timeout time.Duration) (*Lock, error) {
	if err := EnsureCodexDir(); err != nil {
		return nil, err
	}
	path, err := LockPath()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return &Lock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire lock %s: %w", path, err)
		}
		if timeout <= 0 || time.Now().After(deadline) {
			return nil, ErrLocked
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Release deletes the lock file. Idempotent.
func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("release lock %s: %w", l.path, err)
	}
	l.path = ""
	return nil
}

// CurrentLockHolder reads the PID stored in auth.lock, if any. Returns
// 0 if the file is missing or unparseable. Used by `doctor` to surface
// stale locks.
func CurrentLockHolder() int {
	path, err := LockPath()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(string(bytesTrim(data)))
	if err != nil {
		return 0
	}
	return pid
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

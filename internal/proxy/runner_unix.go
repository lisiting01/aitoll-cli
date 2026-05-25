//go:build !windows

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func applyDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := p.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("SIGKILL after SIGTERM timeout: %w", err)
	}
	return nil
}

func windowsProcessExists(pid int) bool {
	return false
}

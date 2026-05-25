//go:build windows

package proxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

func applyDetachAttrs(cmd *exec.Cmd) {
	// Do NOT use DETACHED_PROCESS — it severs stdio inheritance, so
	// cmd.Stderr/Stdout file handles never reach the daemon process.
	// CREATE_NEW_PROCESS_GROUP keeps the child in its own group so a Ctrl+C
	// in the parent shell doesn't kill it; CREATE_NO_WINDOW + HideWindow
	// suppress the console.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNewProcessGroup | createNoWindow,
	}
}

func killProcess(pid int) error {
	out, err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill failed: %w (output: %s)", err, string(out))
	}
	return nil
}

func windowsProcessExists(pid int) bool {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH", "/FO", "CSV").Output()
	if err != nil {
		return false
	}
	return len(out) > 0 && containsPID(string(out), pid)
}

func containsPID(output string, pid int) bool {
	target := `"` + strconv.Itoa(pid) + `"`
	for i := 0; i+len(target) <= len(output); i++ {
		if output[i:i+len(target)] == target {
			return true
		}
	}
	return false
}

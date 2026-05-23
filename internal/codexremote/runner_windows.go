//go:build windows

package codexremote

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
	// Note: we deliberately do NOT use DETACHED_PROCESS — it severs stdio
	// inheritance, so our cmd.Stderr/Stdout file handle never reaches the
	// grand-child (codex.exe). CREATE_NEW_PROCESS_GROUP keeps the child in
	// its own process group so Ctrl+C in the parent shell doesn't kill it,
	// and CREATE_NO_WINDOW + HideWindow keeps the console invisible.
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
	// Use tasklist with a CSV filter; absence of the PID returns "INFO: No tasks..."
	// on stderr/stdout. We just check whether the PID appears in output.
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

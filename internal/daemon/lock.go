package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func lockPath(loomRoot string) string {
	return filepath.Join(loomRoot, "loom.lock")
}

func AcquireLock(loomRoot string) error {
	return tryCreateLock(loomRoot, true)
}

func tryCreateLock(loomRoot string, canRetry bool) error {
	f, err := os.OpenFile(lockPath(loomRoot), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		_, werr := f.WriteString(strconv.Itoa(os.Getpid()))
		f.Close()
		return werr
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	pid, alive := CheckLock(loomRoot)
	if alive {
		return fmt.Errorf("loom already running (pid %d)", pid)
	}
	if !canRetry {
		return err
	}
	os.Remove(lockPath(loomRoot))
	return tryCreateLock(loomRoot, false)
}

func ReleaseLock(loomRoot string) error {
	return os.Remove(lockPath(loomRoot))
}

func CheckLock(loomRoot string) (int, bool) {
	data, err := os.ReadFile(lockPath(loomRoot))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	if err := p.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	// Verify the process is actually a loom daemon, not a recycled PID.
	if !isLoomProcess(pid) {
		return pid, false
	}
	return pid, true
}

// isLoomProcess checks whether the given PID belongs to a loom process
// by inspecting its command line. Prevents false positives from recycled PIDs.
func isLoomProcess(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return false
	}
	cmd := strings.TrimSpace(string(out))
	return strings.Contains(cmd, "loom")
}



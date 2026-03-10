package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func lockPath(loomRoot string) string {
	return filepath.Join(loomRoot, "loom.lock")
}

func AcquireLock(loomRoot string) error {
	pid, alive := CheckLock(loomRoot)
	if alive {
		return fmt.Errorf("loom already running (pid %d)", pid)
	}
	// Stale lock — remove it
	if pid != 0 {
		os.Remove(lockPath(loomRoot))
	}
	return os.WriteFile(lockPath(loomRoot), []byte(strconv.Itoa(os.Getpid())), 0644)
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
	return pid, true
}

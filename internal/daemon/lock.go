package daemon

import (
	"fmt"
	"log"
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

// KillStaleDaemons finds and kills any orphaned "loom start" daemon processes
// other than the current process. This prevents dual-daemon races where two
// daemons fight over agent YAML files, each marking the other's agents dead.
func KillStaleDaemons() {
	self := os.Getpid()
	out, err := exec.Command("pgrep", "-f", "loom start").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid == self || pid == os.Getppid() {
			continue
		}
		// Verify it's actually a loom process before killing
		if !isLoomProcess(pid) {
			continue
		}
		log.Printf("[daemon] killing stale daemon process pid=%d", pid)
		syscall.Kill(pid, syscall.SIGTERM)
	}
}

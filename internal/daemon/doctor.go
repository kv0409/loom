package daemon

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// DoctorReport summarizes what Doctor found and fixed.
type DoctorReport struct {
	StaleProcesses int
	StaleLock       bool
	StaleSocket     bool
	Fixed           int
	Messages        []string
}

// Doctor inspects the loom environment for orphaned processes, stale lock
// files, and stale sockets. When dryRun is false it cleans them up.
func Doctor(loomRoot string, dryRun bool) (DoctorReport, error) {
	var r DoctorReport
	self := os.Getpid()

	// 1. Kill orphaned "loom start" and "loom mcp-server" processes.
	for _, pattern := range []string{"loom start", "loom mcp-server"} {
		out, err := exec.Command("pgrep", "-f", pattern).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil || pid == self || pid == os.Getppid() {
				continue
			}
			if !isLoomProcess(pid) {
				continue
			}
			r.StaleProcesses++
			if dryRun {
				r.Messages = append(r.Messages, fmt.Sprintf("[dry-run] Would kill stale %s process pid=%d", pattern, pid))
			} else {
				log.Printf("[doctor] killing stale %s process pid=%d", pattern, pid)
				syscall.Kill(pid, syscall.SIGTERM)
				r.Fixed++
				r.Messages = append(r.Messages, fmt.Sprintf("Killed stale %s process pid=%d", pattern, pid))
			}
		}
	}

	// 2. Remove stale loom.lock when the PID it references is dead.
	lp := lockPath(loomRoot)
	if data, err := os.ReadFile(lp); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 && pid != self {
			if !processAlive(pid) || !isLoomProcess(pid) {
				r.StaleLock = true
				if dryRun {
					r.Messages = append(r.Messages, fmt.Sprintf("[dry-run] Would remove stale loom.lock (pid %d dead)", pid))
				} else {
					os.Remove(lp)
					r.Fixed++
					r.Messages = append(r.Messages, fmt.Sprintf("Removed stale loom.lock (pid %d dead)", pid))
				}
			}
		}
	}

	// 3. Remove stale daemon.sock when no daemon is alive.
	sp := SockPath(loomRoot)
	if _, err := os.Stat(sp); err == nil {
		_, alive := CheckLock(loomRoot)
		if !alive {
			r.StaleSocket = true
			if dryRun {
				r.Messages = append(r.Messages, "[dry-run] Would remove stale daemon.sock")
			} else {
				os.Remove(sp)
				r.Fixed++
				r.Messages = append(r.Messages, "Removed stale daemon.sock")
			}
		}
	}

	return r, nil
}

// processAlive checks whether a PID is still running.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Package daemon: log rotation.
//
// The daemon's stdout/stderr are wired to logs/daemon.log at process start
// (re-exec with LOOM_DAEMON=1 redirects child.Stdout/Stderr to the file).
// To rotate without losing writes from the Go log package, fmt.Printf, or
// panic traces, we take ownership of fds 1 and 2 via dup2 and call
// log.SetOutput on the same *os.File. Rotation then means: close the fd,
// rename the file, open a fresh file, and dup2 it back over 1/2.
package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const rotatedSuffixFormat = "2006-01-02T15-04-05.000"

// Rotator owns the daemon log file and handles size-based rotation plus
// retention sweeps of previously rotated files.
type Rotator struct {
	path       string // logs/daemon.log
	dir        string
	mu         sync.Mutex
	f          *os.File
	maxBytes   int64
	maxBackups int
	retention  time.Duration
}

// NewRotator returns a Rotator for loomRoot/logs/daemon.log. Call Install
// before starting daemon goroutines.
func NewRotator(loomRoot string, maxSizeMB, maxBackups, retentionDays int) *Rotator {
	dir := filepath.Join(loomRoot, "logs")
	return &Rotator{
		path:       filepath.Join(dir, "daemon.log"),
		dir:        dir,
		maxBytes:   int64(maxSizeMB) * 1024 * 1024,
		maxBackups: maxBackups,
		retention:  time.Duration(retentionDays) * 24 * time.Hour,
	}
}

// Install opens the log file, redirects os.Stdout/os.Stderr (fds 1/2) to it,
// and points the standard log package at it. Idempotent on top of an
// already-redirected process (runStart pre-wires child Stdout/Stderr to the
// same file before exec). Safe to call once per process.
func (r *Rotator) Install() error {
	if err := os.MkdirAll(r.dir, 0755); err != nil {
		return fmt.Errorf("creating logs dir: %w", err)
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening daemon log: %w", err)
	}
	log.SetOutput(f)
	if err := unix.Dup2(int(f.Fd()), 1); err != nil {
		f.Close()
		return fmt.Errorf("dup2 stdout: %w", err)
	}
	if err := unix.Dup2(int(f.Fd()), 2); err != nil {
		f.Close()
		return fmt.Errorf("dup2 stderr: %w", err)
	}
	r.mu.Lock()
	r.f = f
	r.mu.Unlock()
	return nil
}

// UpdateThresholds swaps the rotator's size/count/retention limits. Called
// from Daemon.Reload so SIGHUP config reloads take effect without restart.
func (r *Rotator) UpdateThresholds(maxSizeMB, maxBackups, retentionDays int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxBytes = int64(maxSizeMB) * 1024 * 1024
	r.maxBackups = maxBackups
	r.retention = time.Duration(retentionDays) * 24 * time.Hour
}

// MaybeRotate rotates if the current log exceeds the configured max size.
func (r *Rotator) MaybeRotate() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	info, err := r.f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < r.maxBytes {
		return nil
	}
	return r.rotateLocked()
}

// rotateLocked renames the current log with a timestamp suffix, reopens a
// fresh one, and redirects fds 1/2 + the log package to it. log.SetOutput
// is called before dup2 so log.Printf writes immediately go to the new file
// (fds 1/2 still briefly point at the renamed old file until dup2 lands —
// those writes land in the rotated file, which is acceptable).
func (r *Rotator) rotateLocked() error {
	rotated := r.path + "." + time.Now().UTC().Format(rotatedSuffixFormat)
	if err := os.Rename(r.path, rotated); err != nil {
		return fmt.Errorf("rename log: %w", err)
	}
	nf, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("reopen log: %w", err)
	}
	log.SetOutput(nf)
	if err := unix.Dup2(int(nf.Fd()), 1); err != nil {
		nf.Close()
		return fmt.Errorf("dup2 stdout: %w", err)
	}
	if err := unix.Dup2(int(nf.Fd()), 2); err != nil {
		nf.Close()
		return fmt.Errorf("dup2 stderr: %w", err)
	}
	old := r.f
	r.f = nf
	if old != nil {
		old.Close()
	}
	return nil
}

// Sweep deletes rotated log files older than the retention window or beyond
// the max-backup count, whichever prunes more. Returns the number of files
// removed.
func (r *Rotator) Sweep() (int, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	prefix := "daemon.log."
	cutoff := time.Now().Add(-r.retention)
	type rotFile struct {
		name    string
		modTime time.Time
	}
	var rotated []rotFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		rotated = append(rotated, rotFile{e.Name(), info.ModTime()})
	}
	// Newest first so we can trim by count from the tail.
	sort.Slice(rotated, func(i, j int) bool {
		return rotated[i].modTime.After(rotated[j].modTime)
	})
	removed := 0
	for i, rf := range rotated {
		overCount := r.maxBackups > 0 && i >= r.maxBackups
		overAge := rf.modTime.Before(cutoff)
		if !overCount && !overAge {
			continue
		}
		if err := os.Remove(filepath.Join(r.dir, rf.name)); err != nil {
			continue
		}
		removed++
	}
	return removed, nil
}

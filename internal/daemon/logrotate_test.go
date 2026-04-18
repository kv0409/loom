package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openNoDup opens the log file and attaches it to the rotator WITHOUT
// replacing fds 1/2. Used by tests — Install cannot run in the test process.
func (r *Rotator) openNoDup(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(r.dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	r.f = f
}

func TestMaybeRotate_NoopBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	r := NewRotator(dir, 1, 5, 7) // 1 MB
	r.openNoDup(t)
	defer r.f.Close()

	if _, err := r.f.WriteString("small payload\n"); err != nil {
		t.Fatal(err)
	}
	if err := r.MaybeRotate(); err != nil {
		t.Fatalf("MaybeRotate: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "logs"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "daemon.log.") {
			t.Errorf("unexpected rotated file: %s", e.Name())
		}
	}
}

func TestMaybeRotate_RotatesWhenOversized(t *testing.T) {
	dir := t.TempDir()
	r := NewRotator(dir, 1, 5, 7) // 1 MB threshold
	r.openNoDup(t)
	defer func() {
		if r.f != nil {
			r.f.Close()
		}
	}()

	// Write > 1 MB.
	payload := strings.Repeat("x", 1024)
	for i := 0; i < 1100; i++ {
		if _, err := r.f.WriteString(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.MaybeRotate(); err != nil {
		t.Fatalf("MaybeRotate: %v", err)
	}

	// Current log should be empty (just created).
	info, err := os.Stat(filepath.Join(dir, "logs", "daemon.log"))
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("current log size = %d, want 0", info.Size())
	}
	// Exactly one rotated file should exist.
	entries, _ := os.ReadDir(filepath.Join(dir, "logs"))
	rotated := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "daemon.log.") {
			rotated++
		}
	}
	if rotated != 1 {
		t.Errorf("rotated count = %d, want 1", rotated)
	}
}

func TestSweep_RemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	r := NewRotator(dir, 1, 10, 7) // 7-day retention
	if err := os.MkdirAll(r.dir, 0755); err != nil {
		t.Fatal(err)
	}

	old := filepath.Join(r.dir, "daemon.log.2020-01-01T00-00-00.000")
	fresh := filepath.Join(r.dir, "daemon.log.2099-01-01T00-00-00.000")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Backdate the old file.
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := r.Sweep()
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file still exists")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file removed: %v", err)
	}
}

func TestSweep_EnforcesMaxBackups(t *testing.T) {
	dir := t.TempDir()
	r := NewRotator(dir, 1, 2, 365) // keep only 2
	if err := os.MkdirAll(r.dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create 5 rotated files, all fresh (retention not a factor).
	base := time.Now()
	names := []string{
		"daemon.log.2024-01-01T00-00-00.000",
		"daemon.log.2024-01-02T00-00-00.000",
		"daemon.log.2024-01-03T00-00-00.000",
		"daemon.log.2024-01-04T00-00-00.000",
		"daemon.log.2024-01-05T00-00-00.000",
	}
	for i, n := range names {
		p := filepath.Join(r.dir, n)
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		// Distinct mtimes so sort is deterministic: oldest first in the list,
		// newest last — matches the name order.
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := r.Sweep()
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if removed != 3 {
		t.Errorf("removed = %d, want 3", removed)
	}
	// Two newest should survive.
	for _, n := range names[:3] {
		if _, err := os.Stat(filepath.Join(r.dir, n)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", n)
		}
	}
	for _, n := range names[3:] {
		if _, err := os.Stat(filepath.Join(r.dir, n)); err != nil {
			t.Errorf("%s should remain: %v", n, err)
		}
	}
}

func TestSweep_EmptyDirNoError(t *testing.T) {
	dir := t.TempDir()
	r := NewRotator(dir, 1, 5, 7)
	// Don't create the logs dir.
	removed, err := r.Sweep()
	if err != nil {
		t.Fatalf("Sweep on missing dir: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

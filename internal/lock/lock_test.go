package lock

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func setupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "locks"), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAcquireRelease(t *testing.T) {
	root := setupRoot(t)
	opts := AcquireOpts{File: "src/main.go", Agent: "builder-001", Issue: "LOOM-001"}

	if err := Acquire(root, opts); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Lock file should exist on disk.
	encoded := lockPath(root, "src/main.go")
	if _, err := os.Stat(encoded); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	if err := Release(root, "src/main.go"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Lock file should be gone.
	if _, err := os.Stat(encoded); !os.IsNotExist(err) {
		t.Fatal("lock file still exists after release")
	}
}

func TestAcquireDuplicate(t *testing.T) {
	root := setupRoot(t)
	opts := AcquireOpts{File: "pkg/util.go", Agent: "builder-001", Issue: "LOOM-001"}

	if err := Acquire(root, opts); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	err := Acquire(root, AcquireOpts{File: "pkg/util.go", Agent: "builder-002", Issue: "LOOM-002"})
	if err == nil {
		t.Fatal("expected error on duplicate acquire")
	}
	if !strings.Contains(err.Error(), "LOCKED by builder-001") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheck(t *testing.T) {
	root := setupRoot(t)

	// Check unlocked file returns nil.
	l, err := Check(root, "nofile.go")
	if err != nil {
		t.Fatalf("Check unlocked: %v", err)
	}
	if l != nil {
		t.Fatal("expected nil for unlocked file")
	}

	// Acquire then check.
	if err := Acquire(root, AcquireOpts{File: "nofile.go", Agent: "builder-003", Issue: "LOOM-003"}); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	l, err = Check(root, "nofile.go")
	if err != nil {
		t.Fatalf("Check locked: %v", err)
	}
	if l == nil {
		t.Fatal("expected lock, got nil")
	}
	if l.Agent != "builder-003" {
		t.Errorf("Agent: got %q, want %q", l.Agent, "builder-003")
	}
	if l.Issue != "LOOM-003" {
		t.Errorf("Issue: got %q, want %q", l.Issue, "LOOM-003")
	}
}

func TestPathEncoding(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"src/main.go", "src__main.go.lock.yaml"},
		{"a/b/c/d.ts", "a__b__c__d.ts.lock.yaml"},
		{"root.go", "root.go.lock.yaml"},
	}
	for _, tt := range tests {
		got := filepath.Base(lockPath("/fake", tt.file))
		if got != tt.want {
			t.Errorf("lockPath(%q) base = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func TestReleaseUnlocked(t *testing.T) {
	root := setupRoot(t)
	err := Release(root, "never-locked.go")
	if err == nil {
		t.Fatal("expected error releasing unlocked file")
	}
	if !strings.Contains(err.Error(), "is not locked") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListLocks(t *testing.T) {
	root := setupRoot(t)

	// Empty list.
	locks, err := ListLocks(root)
	if err != nil {
		t.Fatalf("ListLocks empty: %v", err)
	}
	if len(locks) != 0 {
		t.Fatalf("expected 0 locks, got %d", len(locks))
	}

	// Add two locks.
	for _, f := range []string{"a.go", "b.go"} {
		if err := Acquire(root, AcquireOpts{File: f, Agent: "builder-001", Issue: "LOOM-001"}); err != nil {
			t.Fatalf("Acquire %s: %v", f, err)
		}
	}

	locks, err = ListLocks(root)
	if err != nil {
		t.Fatalf("ListLocks: %v", err)
	}
	if len(locks) != 2 {
		t.Fatalf("expected 2 locks, got %d", len(locks))
	}
}

func TestListLocksNoDir(t *testing.T) {
	root := t.TempDir() // no locks/ subdir
	locks, err := ListLocks(root)
	if err != nil {
		t.Fatalf("ListLocks missing dir: %v", err)
	}
	if len(locks) != 0 {
		t.Fatalf("expected 0 locks, got %d", len(locks))
	}
}

func TestConcurrentAcquire(t *testing.T) {
	root := setupRoot(t)
	const n = 20
	var (
		mu       sync.Mutex
		wins     int
		failures int
		wg       sync.WaitGroup
	)
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			err := Acquire(root, AcquireOpts{
				File:  "contested.go",
				Agent: "builder-" + strings.Repeat("x", i),
				Issue: "LOOM-999",
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else {
				failures++
			}
		}(i)
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", wins)
	}
	if failures != n-1 {
		t.Fatalf("expected %d failures, got %d", n-1, failures)
	}
}

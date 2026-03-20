package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initRepoForWorktreeGCTest(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	var err error
	repo, err = filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", args, out)
		}
	}

	loomRoot := filepath.Join(repo, ".loom")
	for _, dir := range []string{"agents", "issues", "worktrees"} {
		if err := os.MkdirAll(filepath.Join(loomRoot, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return repo
}

func TestRunWorktreeGCPreservesCleanUnmergedBranchWhenIssueMissing(t *testing.T) {
	repo := initRepoForWorktreeGCTest(t)
	loomRoot := filepath.Join(repo, ".loom")
	wtName := "LOOM-001-task"
	wtRelPath := filepath.Join(".loom", "worktrees", wtName)
	wtPath := filepath.Join(repo, wtRelPath)

	for _, args := range [][]string{
		{"git", "worktree", "add", wtRelPath, "-b", wtName},
		{"git", "-C", wtPath, "commit", "--allow-empty", "-m", "diverge"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", args, out)
		}
	}

	d := &Daemon{LoomRoot: loomRoot}
	d.runWorktreeGC()

	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("expected worktree to be preserved, stat error: %v", err)
	}

	cmd := exec.Command("git", "-C", repo, "rev-parse", "--verify", wtName)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected branch %s to be preserved: %s", wtName, out)
	}
}

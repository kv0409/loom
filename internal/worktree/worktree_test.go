package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/store"
)

// initBareRepo creates a minimal git repo in a temp dir and returns the repo path
// and a cleanup function. The repo has one initial commit on "main".
func initBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Resolve symlinks so paths match git's resolved output (e.g. macOS /var → /private/var).
	dir, err := filepath.EvalSymlinks(dir)
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
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", args, out)
		}
	}
	return dir
}

// --- ExtractIssueID ---

func TestExtractIssueID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"LOOM-001-some-slug", "LOOM-001"},
		{"LOOM-001-01-slug", "LOOM-001-01"},
		{"LOOM-123-45-feature-work", "LOOM-123-45"},
		{"LOOM-001-02-03-slug", "LOOM-001-02-03"},
		{"random-branch", ""},
		{"", ""},
		{"LOOM-007", "LOOM-007"},
	}
	for _, tt := range tests {
		if got := ExtractIssueID(tt.name); got != tt.want {
			t.Errorf("ExtractIssueID(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- parseDiffStat ---

func TestParseDiffStat(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   DiffStats
	}{
		{
			name:  "full stats",
			input: " file.go | 10 ++++------\n 1 file changed, 4 insertions(+), 6 deletions(-)\n",
			want:  DiffStats{FilesChanged: 1, Insertions: 4, Deletions: 6},
		},
		{
			name:  "insertions only",
			input: " a.go | 5 +++++\n 1 file changed, 5 insertions(+)\n",
			want:  DiffStats{FilesChanged: 1, Insertions: 5, Deletions: 0},
		},
		{
			name:  "deletions only",
			input: " a.go | 3 ---\n 1 file changed, 3 deletions(-)\n",
			want:  DiffStats{FilesChanged: 1, Insertions: 0, Deletions: 3},
		},
		{
			name:  "multiple files",
			input: " a.go | 2 ++\n b.go | 3 ---\n 2 files changed, 2 insertions(+), 3 deletions(-)\n",
			want:  DiffStats{FilesChanged: 2, Insertions: 2, Deletions: 3},
		},
		{
			name:  "empty output",
			input: "",
			want:  DiffStats{},
		},
		{
			name:  "no match",
			input: "nothing useful here\n",
			want:  DiffStats{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiffStat(tt.input)
			if *got != tt.want {
				t.Errorf("parseDiffStat(%q) = %+v, want %+v", tt.input, *got, tt.want)
			}
		})
	}
}

// --- HasDirtyFiles ---

func TestHasDirtyFiles_Clean(t *testing.T) {
	repo := initBareRepo(t)
	if HasDirtyFiles(repo) {
		t.Error("expected clean repo to have no dirty files")
	}
}

func TestHasDirtyFiles_Untracked(t *testing.T) {
	repo := initBareRepo(t)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("hello"), 0644)
	if !HasDirtyFiles(repo) {
		t.Error("expected untracked file to be detected as dirty")
	}
}

func TestHasDirtyFiles_Staged(t *testing.T) {
	repo := initBareRepo(t)
	os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("data"), 0644)
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = repo
	cmd.CombinedOutput()
	if !HasDirtyFiles(repo) {
		t.Error("expected staged file to be detected as dirty")
	}
}

func TestHasDirtyFiles_InvalidPath(t *testing.T) {
	// Non-existent path should return false (git status fails).
	if HasDirtyFiles("/nonexistent/path") {
		t.Error("expected false for invalid path")
	}
}

// --- Create reuse ---

func TestCreate_ReusesExistingWorktree(t *testing.T) {
	// Create a fake loom root with the worktree directory already present.
	dir := t.TempDir()
	loomRoot := filepath.Join(dir, ".loom")
	wtDir := filepath.Join(loomRoot, "worktrees", "LOOM-001-slug")
	os.MkdirAll(wtDir, 0755)

	wt, err := Create(loomRoot, "LOOM-001", "slug", "builder-001")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Name != "LOOM-001-slug" {
		t.Errorf("Name = %q, want %q", wt.Name, "LOOM-001-slug")
	}
	if wt.Issue != "LOOM-001" {
		t.Errorf("Issue = %q, want %q", wt.Issue, "LOOM-001")
	}
	if wt.Agent != "builder-001" {
		t.Errorf("Agent = %q, want %q", wt.Agent, "builder-001")
	}
	if wt.Branch != "LOOM-001-slug" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "LOOM-001-slug")
	}
}

// --- IsMerged ---

func TestIsMerged_AncestorBranch(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(loomRoot, 0755)

	// Create a branch at the current commit — it's an ancestor of HEAD.
	cmd := exec.Command("git", "branch", "LOOM-001-feature")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %s", out)
	}

	if !IsMerged(loomRoot, "LOOM-001-feature") {
		t.Error("expected branch at HEAD to be considered merged")
	}
}

func TestIsMerged_UnmergedBranch(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(loomRoot, 0755)

	// Create a branch, then add a commit only on that branch.
	for _, args := range [][]string{
		{"git", "branch", "LOOM-002-feature"},
		{"git", "checkout", "LOOM-002-feature"},
		{"git", "commit", "--allow-empty", "-m", "diverge"},
		{"git", "checkout", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	if IsMerged(loomRoot, "LOOM-002-feature") {
		t.Error("expected diverged branch to NOT be considered merged")
	}
}

func TestIsMerged_SquashFallback(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "issues"), 0755)

	// Write a config with squash strategy.
	cfg := struct {
		Merge struct {
			Strategy string `yaml:"strategy"`
		} `yaml:"merge"`
		Kiro struct {
			DefaultMode string `yaml:"default_mode"`
		} `yaml:"kiro"`
	}{}
	cfg.Merge.Strategy = "squash"
	cfg.Kiro.DefaultMode = "acp"
	store.WriteYAML(filepath.Join(loomRoot, "config.yaml"), &cfg)

	// Write an issue with MergedAt set.
	now := time.Now()
	iss := &issue.Issue{
		ID:       "LOOM-003",
		Status:   "done",
		MergedAt: &now,
	}
	store.WriteYAML(filepath.Join(loomRoot, "issues", "LOOM-003.yaml"), iss)

	// Create a diverged branch so merge-base --is-ancestor fails.
	for _, args := range [][]string{
		{"git", "branch", "LOOM-003-feature"},
		{"git", "checkout", "LOOM-003-feature"},
		{"git", "commit", "--allow-empty", "-m", "diverge"},
		{"git", "checkout", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	if !IsMerged(loomRoot, "LOOM-003-feature") {
		t.Error("expected squash-merged issue to be considered merged via fallback")
	}
}

func TestIsMerged_SquashFallback_NoMergedAt(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "issues"), 0755)

	// Squash config but issue has no MergedAt.
	cfg := struct {
		Merge struct {
			Strategy string `yaml:"strategy"`
		} `yaml:"merge"`
		Kiro struct {
			DefaultMode string `yaml:"default_mode"`
		} `yaml:"kiro"`
	}{}
	cfg.Merge.Strategy = "squash"
	cfg.Kiro.DefaultMode = "acp"
	store.WriteYAML(filepath.Join(loomRoot, "config.yaml"), &cfg)

	iss := &issue.Issue{ID: "LOOM-004", Status: "in-progress"}
	store.WriteYAML(filepath.Join(loomRoot, "issues", "LOOM-004.yaml"), iss)

	// Diverged branch.
	for _, args := range [][]string{
		{"git", "branch", "LOOM-004-feature"},
		{"git", "checkout", "LOOM-004-feature"},
		{"git", "commit", "--allow-empty", "-m", "diverge"},
		{"git", "checkout", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	if IsMerged(loomRoot, "LOOM-004-feature") {
		t.Error("expected unmerged issue to NOT be considered merged even with squash strategy")
	}
}

// --- agentOwnerMap ---

func TestAgentOwnerMap(t *testing.T) {
	dir := t.TempDir()
	loomRoot := filepath.Join(dir, ".loom")
	agentsDir := filepath.Join(loomRoot, "agents")
	os.MkdirAll(agentsDir, 0755)

	// Write two agent files with worktree fields.
	store.WriteYAML(filepath.Join(agentsDir, "builder-001.yaml"), map[string]string{
		"id": "builder-001", "worktree": "LOOM-001-feat",
	})
	store.WriteYAML(filepath.Join(agentsDir, "builder-002.yaml"), map[string]string{
		"id": "builder-002", "worktree": "LOOM-002-fix",
	})
	// Agent without worktree — should be skipped.
	store.WriteYAML(filepath.Join(agentsDir, "lead-001.yaml"), map[string]string{
		"id": "lead-001",
	})

	m := agentOwnerMap(loomRoot)
	if m["LOOM-001-feat"] != "builder-001" {
		t.Errorf("expected builder-001 for LOOM-001-feat, got %q", m["LOOM-001-feat"])
	}
	if m["LOOM-002-fix"] != "builder-002" {
		t.Errorf("expected builder-002 for LOOM-002-fix, got %q", m["LOOM-002-fix"])
	}
	if len(m) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m))
	}
}

func TestAgentOwnerMap_MissingDir(t *testing.T) {
	m := agentOwnerMap("/nonexistent/path")
	if m != nil {
		t.Errorf("expected nil for missing agents dir, got %v", m)
	}
}

// --- List with agent ownership ---

func TestList_PopulatesAgentFromRegistry(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "agents"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "worktrees"), 0755)

	// Create a worktree via git.
	cmd := exec.Command("git", "worktree", "add",
		filepath.Join(".loom", "worktrees", "LOOM-010-slug"), "-b", "LOOM-010-slug")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}

	// Register an agent that owns this worktree.
	store.WriteYAML(filepath.Join(loomRoot, "agents", "builder-010.yaml"), map[string]string{
		"id": "builder-010", "worktree": "LOOM-010-slug",
	})

	wts, err := List(loomRoot)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Agent != "builder-010" {
		t.Errorf("Agent = %q, want %q", wts[0].Agent, "builder-010")
	}
	if wts[0].Issue != "LOOM-010" {
		t.Errorf("Issue = %q, want %q", wts[0].Issue, "LOOM-010")
	}
}

func TestList_NoAgentWhenUnowned(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "worktrees"), 0755)

	cmd := exec.Command("git", "worktree", "add",
		filepath.Join(".loom", "worktrees", "LOOM-011-orphan"), "-b", "LOOM-011-orphan")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}

	// No agent files — Agent should remain empty.
	wts, err := List(loomRoot)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Agent != "" {
		t.Errorf("Agent = %q, want empty", wts[0].Agent)
	}
}

// --- Cleanup with reconstructed metadata ---

func TestCleanup_OrphanedByDeadAgent(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "agents"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "issues"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "worktrees"), 0755)

	// Create a worktree.
	cmd := exec.Command("git", "worktree", "add",
		filepath.Join(".loom", "worktrees", "LOOM-020-task"), "-b", "LOOM-020-task")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}

	// Register an agent that owns this worktree.
	store.WriteYAML(filepath.Join(loomRoot, "agents", "builder-020.yaml"), map[string]string{
		"id": "builder-020", "worktree": "LOOM-020-task",
	})

	// Write an active issue so the issue-based path would NOT flag it as orphaned.
	store.WriteYAML(filepath.Join(loomRoot, "issues", "LOOM-020.yaml"), &issue.Issue{
		ID: "LOOM-020", Status: "in-progress",
	})

	// Agent is registered → not orphaned.
	orphaned, err := Cleanup(loomRoot)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(orphaned) != 0 {
		t.Errorf("expected 0 orphans with registered agent, got %v", orphaned)
	}

	// Now remove the agent file (simulating deregistration).
	os.Remove(filepath.Join(loomRoot, "agents", "builder-020.yaml"))

	// With no agent, falls through to issue check — issue is active, so still not orphaned.
	orphaned, err = Cleanup(loomRoot)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(orphaned) != 0 {
		t.Errorf("expected 0 orphans with active issue, got %v", orphaned)
	}
}

func TestCleanup_OrphanedByDeadAgentAndDoneIssue(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "agents"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "issues"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "worktrees"), 0755)

	cmd := exec.Command("git", "worktree", "add",
		filepath.Join(".loom", "worktrees", "LOOM-021-task"), "-b", "LOOM-021-task")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}

	// Agent registered but pointing to a different worktree (not this one).
	store.WriteYAML(filepath.Join(loomRoot, "agents", "builder-021.yaml"), map[string]string{
		"id": "builder-021", "worktree": "LOOM-099-other",
	})

	// Issue is done — should be orphaned.
	store.WriteYAML(filepath.Join(loomRoot, "issues", "LOOM-021.yaml"), &issue.Issue{
		ID: "LOOM-021", Status: "done",
	})

	orphaned, err := Cleanup(loomRoot)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(orphaned) != 1 || orphaned[0] != "LOOM-021-task" {
		t.Errorf("expected [LOOM-021-task] orphaned, got %v", orphaned)
	}
}

func TestCleanup_AgentRegisteredNotOrphaned(t *testing.T) {
	repo := initBareRepo(t)
	loomRoot := filepath.Join(repo, ".loom")
	os.MkdirAll(filepath.Join(loomRoot, "agents"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "issues"), 0755)
	os.MkdirAll(filepath.Join(loomRoot, "worktrees"), 0755)

	cmd := exec.Command("git", "worktree", "add",
		filepath.Join(".loom", "worktrees", "LOOM-022-task"), "-b", "LOOM-022-task")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}

	// Agent owns this worktree and is registered.
	store.WriteYAML(filepath.Join(loomRoot, "agents", "builder-022.yaml"), map[string]string{
		"id": "builder-022", "worktree": "LOOM-022-task",
	})

	// Issue is done, but agent is registered — agent check takes priority.
	store.WriteYAML(filepath.Join(loomRoot, "issues", "LOOM-022.yaml"), &issue.Issue{
		ID: "LOOM-022", Status: "done",
	})

	orphaned, err := Cleanup(loomRoot)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(orphaned) != 0 {
		t.Errorf("expected 0 orphans with registered agent, got %v", orphaned)
	}
}
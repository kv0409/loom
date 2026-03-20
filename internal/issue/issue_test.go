package issue

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// setupTestRoot creates a temporary loom root with the issues directory and counter file.
func setupTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "issues")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "counter.txt"), []byte("0"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestIsReady_NoDeps(t *testing.T) {
	root := setupTestRoot(t)
	iss, err := Create(root, "no deps", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !iss.IsReady(root) {
		t.Error("issue with no dependencies should be ready")
	}
}

func TestIsReady_DepDone(t *testing.T) {
	root := setupTestRoot(t)
	dep, err := Create(root, "dependency", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Status = "done"
	if err := Save(root, dep); err != nil {
		t.Fatal(err)
	}

	iss, err := Create(root, "depends on done", CreateOpts{DependsOn: []string{dep.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if !iss.IsReady(root) {
		t.Error("issue depending on done issue should be ready")
	}
}

func TestIsReady_DepCancelled(t *testing.T) {
	root := setupTestRoot(t)
	dep, err := Create(root, "dependency", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Status = "cancelled"
	if err := Save(root, dep); err != nil {
		t.Fatal(err)
	}

	iss, err := Create(root, "depends on cancelled", CreateOpts{DependsOn: []string{dep.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if !iss.IsReady(root) {
		t.Error("issue depending on cancelled issue should be ready")
	}
}

func TestIsReady_DepOpen(t *testing.T) {
	root := setupTestRoot(t)
	dep, err := Create(root, "dependency", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	iss, err := Create(root, "depends on open", CreateOpts{DependsOn: []string{dep.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if iss.IsReady(root) {
		t.Error("issue depending on open issue should NOT be ready")
	}
}

func TestIsReady_DepInProgress(t *testing.T) {
	root := setupTestRoot(t)
	dep, err := Create(root, "dependency", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Status = "in-progress"
	if err := Save(root, dep); err != nil {
		t.Fatal(err)
	}

	iss, err := Create(root, "depends on in-progress", CreateOpts{DependsOn: []string{dep.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if iss.IsReady(root) {
		t.Error("issue depending on in-progress issue should NOT be ready")
	}
}

func TestIsReady_DepMissing(t *testing.T) {
	root := setupTestRoot(t)
	iss, err := Create(root, "depends on missing", CreateOpts{DependsOn: []string{"LOOM-999"}})
	if err != nil {
		t.Fatal(err)
	}
	if iss.IsReady(root) {
		t.Error("issue depending on missing issue should NOT be ready")
	}
}

func TestIsReady_MultipleDeps_AllResolved(t *testing.T) {
	root := setupTestRoot(t)
	dep1, _ := Create(root, "dep1", CreateOpts{})
	dep1.Status = "done"
	Save(root, dep1)
	dep2, _ := Create(root, "dep2", CreateOpts{})
	dep2.Status = "cancelled"
	Save(root, dep2)

	iss, _ := Create(root, "multi-dep", CreateOpts{DependsOn: []string{dep1.ID, dep2.ID}})
	if !iss.IsReady(root) {
		t.Error("issue with all deps resolved should be ready")
	}
}

func TestIsReady_MultipleDeps_OneUnresolved(t *testing.T) {
	root := setupTestRoot(t)
	dep1, _ := Create(root, "dep1", CreateOpts{})
	dep1.Status = "done"
	Save(root, dep1)
	dep2, _ := Create(root, "dep2", CreateOpts{})
	// dep2 stays open

	iss, _ := Create(root, "multi-dep partial", CreateOpts{DependsOn: []string{dep1.ID, dep2.ID}})
	if iss.IsReady(root) {
		t.Error("issue with one unresolved dep should NOT be ready")
	}
}

func TestIsReady_UnblocksWhenDepResolves(t *testing.T) {
	root := setupTestRoot(t)
	dep, _ := Create(root, "blocker", CreateOpts{})
	iss, _ := Create(root, "blocked", CreateOpts{DependsOn: []string{dep.ID}})

	if iss.IsReady(root) {
		t.Fatal("should be blocked initially")
	}

	dep.Status = "done"
	Save(root, dep)

	if !iss.IsReady(root) {
		t.Error("should become ready after dep resolves")
	}
}

func TestListReady_FiltersBlockedIssues(t *testing.T) {
	root := setupTestRoot(t)
	dep, _ := Create(root, "blocker", CreateOpts{})
	Create(root, "blocked", CreateOpts{DependsOn: []string{dep.ID}})
	Create(root, "free", CreateOpts{})

	ready, err := ListReady(root)
	if err != nil {
		t.Fatal(err)
	}

	// Only "free" and "blocker" should be ready (both open, no unresolved deps).
	ids := make(map[string]bool)
	for _, r := range ready {
		ids[r.ID] = true
	}
	if len(ready) != 2 {
		t.Errorf("expected 2 ready issues, got %d: %v", len(ready), ids)
	}
	// The blocked issue should not appear.
	for _, r := range ready {
		if r.Title == "blocked" {
			t.Error("blocked issue should not be in ready list")
		}
	}
}

func TestListReady_ExcludesAssigned(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "assigned issue", CreateOpts{})
	iss.Assignee = "builder-001"
	Save(root, iss)

	ready, err := ListReady(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range ready {
		if r.ID == iss.ID {
			t.Error("assigned issue should not be in ready list")
		}
	}
}

func TestListReady_IncludesUnblockedAfterResolve(t *testing.T) {
	root := setupTestRoot(t)
	dep, _ := Create(root, "blocker", CreateOpts{})
	Create(root, "was-blocked", CreateOpts{DependsOn: []string{dep.ID}})

	ready, _ := ListReady(root)
	for _, r := range ready {
		if r.Title == "was-blocked" {
			t.Fatal("should be blocked before dep resolves")
		}
	}

	dep.Status = "done"
	Save(root, dep)

	ready, _ = ListReady(root)
	found := false
	for _, r := range ready {
		if r.Title == "was-blocked" {
			found = true
		}
	}
	if !found {
		t.Error("issue should appear in ready list after dep resolves")
	}
}

func TestCreateSubIssuesConcurrentlyKeepsUniqueIDsAndParentLinks(t *testing.T) {
	root := setupTestRoot(t)
	parent, err := Create(root, "parent", CreateOpts{})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	const n = 12
	start := make(chan struct{})
	errs := make(chan error, n)
	ids := make(chan string, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			child, err := Create(root, "child", CreateOpts{Parent: parent.ID})
			if err != nil {
				errs <- err
				return
			}
			ids <- child.ID
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)
	close(ids)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent child create failed: %v", err)
		}
	}

	seen := make(map[string]bool, n)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate child ID created: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique child IDs, got %d", n, len(seen))
	}

	reloadedParent, err := Load(root, parent.ID)
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if len(reloadedParent.Children) != n {
		t.Fatalf("expected %d parent children, got %d: %v", n, len(reloadedParent.Children), reloadedParent.Children)
	}
	for _, childID := range reloadedParent.Children {
		if !seen[childID] {
			t.Fatalf("parent children contains unexpected child ID %s", childID)
		}
	}
}

func TestListReturnsErrorOnCorruptedIssueFile(t *testing.T) {
	root := setupTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "issues", "LOOM-999.yaml"), []byte(":\n- bad"), 0644); err != nil {
		t.Fatalf("write corrupt issue: %v", err)
	}

	if _, err := List(root, ListOpts{All: true}); err == nil {
		t.Fatal("expected corrupted issue file to make List fail")
	}
}

func TestUpdate_AssignBlockedByDeps(t *testing.T) {
	root := setupTestRoot(t)
	dep, _ := Create(root, "blocker", CreateOpts{})
	blocked, _ := Create(root, "blocked", CreateOpts{DependsOn: []string{dep.ID}})

	_, err := Update(root, blocked.ID, UpdateOpts{Assignee: "builder-001"})
	if err == nil {
		t.Fatal("assigning a dependency-blocked issue should fail")
	}
}

func TestUpdate_AssignAllowedWhenDepsResolved(t *testing.T) {
	root := setupTestRoot(t)
	dep, _ := Create(root, "blocker", CreateOpts{})
	blocked, _ := Create(root, "needs-dep", CreateOpts{DependsOn: []string{dep.ID}})

	dep.Status = "done"
	Save(root, dep)

	updated, err := Update(root, blocked.ID, UpdateOpts{Assignee: "builder-001"})
	if err != nil {
		t.Fatalf("assigning should succeed after deps resolve: %v", err)
	}
	if updated.Assignee != "builder-001" {
		t.Error("assignee should be set")
	}
}

func TestUpdate_AssignNoDepsAlwaysAllowed(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "no deps", CreateOpts{})

	updated, err := Update(root, iss.ID, UpdateOpts{Assignee: "builder-001"})
	if err != nil {
		t.Fatalf("assigning issue with no deps should succeed: %v", err)
	}
	if updated.Assignee != "builder-001" {
		t.Error("assignee should be set")
	}
}

func TestValidateTransition_ValidTransitions(t *testing.T) {
	valid := []struct{ from, to string }{
		{"open", "assigned"},
		{"assigned", "in-progress"},
		{"in-progress", "review"},
		{"in-progress", "blocked"},
		{"in-progress", "done"},
		{"blocked", "in-progress"},
		{"review", "done"},
		{"review", "in-progress"},
	}
	for _, tc := range valid {
		if err := validateTransition(tc.from, tc.to); err != nil {
			t.Errorf("expected %s → %s to be valid, got error: %v", tc.from, tc.to, err)
		}
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	invalid := []struct{ from, to string }{
		{"open", "done"},
		{"open", "in-progress"},
		{"open", "review"},
		{"open", "blocked"},
		{"assigned", "done"},
		{"assigned", "review"},
		{"assigned", "blocked"},
		{"in-progress", "assigned"},
		{"blocked", "done"},
		{"blocked", "open"},
		{"blocked", "review"},
		{"review", "open"},
		{"review", "blocked"},
		{"done", "open"},
		{"done", "in-progress"},
	}
	for _, tc := range invalid {
		if err := validateTransition(tc.from, tc.to); err == nil {
			t.Errorf("expected %s → %s to be invalid, but got no error", tc.from, tc.to)
		}
	}
}

func TestValidateTransition_CancelledRejected(t *testing.T) {
	statuses := []string{"open", "assigned", "in-progress", "blocked", "review", "done"}
	for _, from := range statuses {
		if err := validateTransition(from, "cancelled"); err == nil {
			t.Errorf("expected %s → cancelled to be rejected (must use Cancel())", from)
		}
	}
}

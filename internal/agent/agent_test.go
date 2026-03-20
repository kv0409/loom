package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/issue"
)

// setupRoot creates a temp .loom root with the agents/ and issues/ directories.
func setupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"agents", "issues"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Issue counter.
	os.WriteFile(filepath.Join(root, "issues", "counter.txt"), []byte("0"), 0644)
	return root
}

func makeAgent(id, role string) *Agent {
	return &Agent{
		ID:        id,
		Role:      role,
		Status:    "active",
		SpawnedAt: time.Now(),
		Heartbeat: time.Now(),
	}
}

func replaceAgentFileWithDirectory(t *testing.T, root, id string) {
	t.Helper()
	path := agentPath(root, id)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove agent file %s: %v", id, err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatalf("replace agent file %s with directory: %v", id, err)
	}
}

// --- Register / Load / Save ---

func TestRegisterAndLoad(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	a.AssignedIssues = []string{"LOOM-001"}

	if err := Register(root, a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// YAML file should exist.
	if _, err := os.Stat(agentPath(root, "builder-001")); err != nil {
		t.Fatalf("agent file not created: %v", err)
	}

	// Mailbox dir should exist.
	if _, err := os.Stat(mailboxDir(root, "builder-001")); err != nil {
		t.Fatalf("mailbox dir not created: %v", err)
	}

	loaded, err := Load(root, "builder-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != "builder-001" {
		t.Errorf("ID: got %q, want %q", loaded.ID, "builder-001")
	}
	if loaded.Role != "builder" {
		t.Errorf("Role: got %q, want %q", loaded.Role, "builder")
	}
	if len(loaded.AssignedIssues) != 1 || loaded.AssignedIssues[0] != "LOOM-001" {
		t.Errorf("AssignedIssues: got %v, want [LOOM-001]", loaded.AssignedIssues)
	}
}

func TestLoadNonexistent(t *testing.T) {
	root := setupRoot(t)
	_, err := Load(root, "no-such-agent")
	if err == nil {
		t.Fatal("expected error loading nonexistent agent")
	}
}

func TestSave(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("lead-001", "lead")
	if err := Register(root, a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	a.Status = "idle"
	if err := Save(root, a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(root, "lead-001")
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.Status != "idle" {
		t.Errorf("Status: got %q, want %q", loaded.Status, "idle")
	}
}

// --- List ---

func TestListEmpty(t *testing.T) {
	root := setupRoot(t)
	agents, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestListNoDir(t *testing.T) {
	// agents/ directory doesn't exist at all.
	root := t.TempDir()
	agents, err := List(root)
	if err != nil {
		t.Fatalf("List on missing dir: %v", err)
	}
	if agents != nil {
		t.Errorf("expected nil, got %v", agents)
	}
}

func TestListMultiple(t *testing.T) {
	root := setupRoot(t)
	now := time.Now()

	// Register agents with staggered spawn times.
	for i, id := range []string{"builder-001", "builder-002", "lead-001"} {
		a := makeAgent(id, "builder")
		if id == "lead-001" {
			a.Role = "lead"
		}
		a.SpawnedAt = now.Add(time.Duration(i) * time.Second)
		if err := Register(root, a); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	agents, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	// List sorts by SpawnedAt descending (newest first).
	if agents[0].ID != "lead-001" {
		t.Errorf("first agent: got %q, want %q", agents[0].ID, "lead-001")
	}
	if agents[2].ID != "builder-001" {
		t.Errorf("last agent: got %q, want %q", agents[2].ID, "builder-001")
	}
}

func TestListReturnsErrorOnCorruptedAgentFile(t *testing.T) {
	root := setupRoot(t)
	if err := os.WriteFile(filepath.Join(root, "agents", "broken.yaml"), []byte(":\n- bad"), 0644); err != nil {
		t.Fatalf("write corrupt agent: %v", err)
	}

	if _, err := List(root); err == nil {
		t.Fatal("expected corrupted agent file to make List fail")
	}
}

// --- Deregister ---

func TestDeregister(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	if err := Register(root, a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := Deregister(root, "builder-001"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	if _, err := os.Stat(agentPath(root, "builder-001")); !os.IsNotExist(err) {
		t.Error("agent file should be removed after Deregister")
	}

	// Load should fail.
	if _, err := Load(root, "builder-001"); err == nil {
		t.Error("expected error loading deregistered agent")
	}
}

func TestDeregisterNonexistent(t *testing.T) {
	root := setupRoot(t)
	err := Deregister(root, "ghost")
	if err == nil {
		t.Fatal("expected error deregistering nonexistent agent")
	}
}

// --- NextID ---

func TestNextIDOrchestrator(t *testing.T) {
	root := setupRoot(t)
	id := NextID(root, "orchestrator")
	if id != "orchestrator" {
		t.Errorf("got %q, want %q", id, "orchestrator")
	}
}

func TestNextIDFirstOfRole(t *testing.T) {
	root := setupRoot(t)
	id := NextID(root, "builder")
	if id != "builder-001" {
		t.Errorf("got %q, want %q", id, "builder-001")
	}
}

func TestNextIDIncrementsExisting(t *testing.T) {
	root := setupRoot(t)

	// Register builder-001 and builder-003 (gap).
	for _, id := range []string{"builder-001", "builder-003"} {
		if err := Register(root, makeAgent(id, "builder")); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	id := NextID(root, "builder")
	if id != "builder-004" {
		t.Errorf("got %q, want %q", id, "builder-004")
	}
}

func TestNextIDIgnoresOtherRoles(t *testing.T) {
	root := setupRoot(t)

	// Register a lead — should not affect builder numbering.
	if err := Register(root, makeAgent("lead-005", "lead")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	id := NextID(root, "builder")
	if id != "builder-001" {
		t.Errorf("got %q, want %q", id, "builder-001")
	}
}

func TestNextIDConcurrentReservationsAreUnique(t *testing.T) {
	root := setupRoot(t)

	const n = 8
	start := make(chan struct{})
	ids := make(chan string, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ids <- NextID(root, "builder")
		}()
	}

	close(start)
	wg.Wait()
	close(ids)

	seen := make(map[string]bool, n)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID allocated: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique IDs, got %d", n, len(seen))
	}
}

// --- AssignIssue / UnassignIssue ---

func createTestIssue(t *testing.T, root, title string) *issue.Issue {
	t.Helper()
	iss, err := issue.Create(root, title, issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}
	return iss
}

func TestAssignIssue(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "test task")

	if err := AssignIssue(root, "builder-001", iss.ID); err != nil {
		t.Fatalf("AssignIssue: %v", err)
	}

	// Agent should have the issue in AssignedIssues.
	loaded, _ := Load(root, "builder-001")
	if len(loaded.AssignedIssues) != 1 || loaded.AssignedIssues[0] != iss.ID {
		t.Errorf("agent AssignedIssues: got %v, want [%s]", loaded.AssignedIssues, iss.ID)
	}

	// Issue should have the agent as assignee and status assigned.
	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Assignee != "builder-001" {
		t.Errorf("issue Assignee: got %q, want %q", loadedIss.Assignee, "builder-001")
	}
	if loadedIss.Status != "assigned" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "assigned")
	}
}

func TestAssignIssueMissingAgentDoesNotMutateIssue(t *testing.T) {
	root := setupRoot(t)
	iss := createTestIssue(t, root, "missing assignee")

	if err := AssignIssue(root, "builder-404", iss.ID); err == nil {
		t.Fatal("expected missing agent assignment to fail")
	}

	loadedIss, err := issue.Load(root, iss.ID)
	if err != nil {
		t.Fatalf("issue.Load: %v", err)
	}
	if loadedIss.Assignee != "" {
		t.Fatalf("issue assignee mutated on failed assignment: %q", loadedIss.Assignee)
	}
	if loadedIss.Status != "open" {
		t.Fatalf("issue status mutated on failed assignment: %q", loadedIss.Status)
	}
}

func TestReassignIssue(t *testing.T) {
	root := setupRoot(t)
	a1 := makeAgent("builder-001", "builder")
	a2 := makeAgent("builder-002", "builder")
	Register(root, a1)
	Register(root, a2)
	iss := createTestIssue(t, root, "reassign task")

	// Assign to builder-001 first.
	AssignIssue(root, "builder-001", iss.ID)

	// Reassign to builder-002.
	if err := AssignIssue(root, "builder-002", iss.ID); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	// Old agent should no longer have the issue.
	old, _ := Load(root, "builder-001")
	if len(old.AssignedIssues) != 0 {
		t.Errorf("old agent still has issues: %v", old.AssignedIssues)
	}

	// New agent should have it.
	new, _ := Load(root, "builder-002")
	if len(new.AssignedIssues) != 1 || new.AssignedIssues[0] != iss.ID {
		t.Errorf("new agent AssignedIssues: got %v, want [%s]", new.AssignedIssues, iss.ID)
	}

	// Issue should point to new agent.
	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Assignee != "builder-002" {
		t.Errorf("issue Assignee: got %q, want %q", loadedIss.Assignee, "builder-002")
	}
}

func TestUnassignIssue(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "unassign task")

	AssignIssue(root, "builder-001", iss.ID)

	if err := UnassignIssue(root, iss.ID); err != nil {
		t.Fatalf("UnassignIssue: %v", err)
	}

	// Agent should have no assigned issues.
	loaded, _ := Load(root, "builder-001")
	if len(loaded.AssignedIssues) != 0 {
		t.Errorf("agent still has issues: %v", loaded.AssignedIssues)
	}

	// Issue should have no assignee and be open.
	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Assignee != "" {
		t.Errorf("issue Assignee: got %q, want empty", loadedIss.Assignee)
	}
	if loadedIss.Status != "open" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "open")
	}
}

func TestUnassignIssueNoop(t *testing.T) {
	root := setupRoot(t)
	iss := createTestIssue(t, root, "unassigned task")

	// Unassigning an already-unassigned issue should be a no-op.
	if err := UnassignIssue(root, iss.ID); err != nil {
		t.Fatalf("UnassignIssue on unassigned: %v", err)
	}
}

func TestUnassignInProgressReopens(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "in-progress unassign")

	AssignIssue(root, "builder-001", iss.ID)
	issue.Update(root, iss.ID, issue.UpdateOpts{Status: "in-progress"})

	if err := UnassignIssue(root, iss.ID); err != nil {
		t.Fatalf("UnassignIssue: %v", err)
	}

	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Status != "open" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "open")
	}
	if loadedIss.Assignee != "" {
		t.Errorf("issue Assignee: got %q, want empty", loadedIss.Assignee)
	}
}

func TestUnassignIssueAgentLoadFailureRollsBackIssue(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "rollback unassign")

	AssignIssue(root, "builder-001", iss.ID)
	replaceAgentFileWithDirectory(t, root, "builder-001")

	if err := UnassignIssue(root, iss.ID); err == nil {
		t.Fatal("expected UnassignIssue to fail when agent reconciliation fails")
	}

	loadedIss, err := issue.Load(root, iss.ID)
	if err != nil {
		t.Fatalf("issue.Load: %v", err)
	}
	if loadedIss.Status != "assigned" {
		t.Fatalf("issue status should roll back to assigned, got %q", loadedIss.Status)
	}
	if loadedIss.Assignee != "builder-001" {
		t.Fatalf("issue assignee should roll back to builder-001, got %q", loadedIss.Assignee)
	}
}

func TestAssignIssueIdempotent(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "idempotent task")

	AssignIssue(root, "builder-001", iss.ID)
	AssignIssue(root, "builder-001", iss.ID) // second call

	loaded, _ := Load(root, "builder-001")
	if len(loaded.AssignedIssues) != 1 {
		t.Errorf("expected 1 issue, got %d: %v", len(loaded.AssignedIssues), loaded.AssignedIssues)
	}
}

func TestReassignInProgressIssue(t *testing.T) {
	root := setupRoot(t)
	a1 := makeAgent("builder-001", "builder")
	a2 := makeAgent("builder-002", "builder")
	Register(root, a1)
	Register(root, a2)
	iss := createTestIssue(t, root, "in-progress reassign")

	// Assign and move to in-progress.
	AssignIssue(root, "builder-001", iss.ID)
	issue.Update(root, iss.ID, issue.UpdateOpts{Status: "in-progress"})

	// Reassign while in-progress — should not fail.
	if err := AssignIssue(root, "builder-002", iss.ID); err != nil {
		t.Fatalf("reassign in-progress: %v", err)
	}

	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Assignee != "builder-002" {
		t.Errorf("issue Assignee: got %q, want %q", loadedIss.Assignee, "builder-002")
	}
	// Status should remain in-progress (not regress to assigned).
	if loadedIss.Status != "in-progress" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "in-progress")
	}
}

// --- CancelIssue / CloseIssue ---

func TestCancelIssue_AssignedClearsAgent(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "cancel assigned")
	AssignIssue(root, "builder-001", iss.ID)

	cancelled, err := CancelIssue(root, iss.ID)
	if err != nil {
		t.Fatalf("CancelIssue: %v", err)
	}
	if len(cancelled) != 1 {
		t.Fatalf("expected 1 cancelled, got %d", len(cancelled))
	}
	if cancelled[0].PreviousAssignee != "builder-001" {
		t.Errorf("PreviousAssignee: got %q, want %q", cancelled[0].PreviousAssignee, "builder-001")
	}

	// Agent should have no assigned issues.
	loaded, _ := Load(root, "builder-001")
	if len(loaded.AssignedIssues) != 0 {
		t.Errorf("agent still has issues: %v", loaded.AssignedIssues)
	}

	// Issue should be cancelled with no assignee.
	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Status != "cancelled" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "cancelled")
	}
	if loadedIss.Assignee != "" {
		t.Errorf("issue Assignee: got %q, want empty", loadedIss.Assignee)
	}
}

func TestCancelIssue_InProgressClearsAgent(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "cancel in-progress")
	AssignIssue(root, "builder-001", iss.ID)
	issue.Update(root, iss.ID, issue.UpdateOpts{Status: "in-progress"})

	cancelled, err := CancelIssue(root, iss.ID)
	if err != nil {
		t.Fatalf("CancelIssue: %v", err)
	}
	if len(cancelled) != 1 || cancelled[0].PreviousAssignee != "builder-001" {
		t.Fatalf("unexpected cancelled result: %v", cancelled)
	}

	loaded, _ := Load(root, "builder-001")
	if len(loaded.AssignedIssues) != 0 {
		t.Errorf("agent still has issues: %v", loaded.AssignedIssues)
	}

	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Status != "cancelled" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "cancelled")
	}
}

func TestCancelIssue_UnassignedNoError(t *testing.T) {
	root := setupRoot(t)
	iss := createTestIssue(t, root, "cancel unassigned")

	cancelled, err := CancelIssue(root, iss.ID)
	if err != nil {
		t.Fatalf("CancelIssue: %v", err)
	}
	if len(cancelled) != 1 {
		t.Fatalf("expected 1 cancelled, got %d", len(cancelled))
	}
	if cancelled[0].PreviousAssignee != "" {
		t.Errorf("PreviousAssignee: got %q, want empty", cancelled[0].PreviousAssignee)
	}
}

func TestCancelIssue_CascadesChildren(t *testing.T) {
	root := setupRoot(t)
	a1 := makeAgent("builder-001", "builder")
	a2 := makeAgent("builder-002", "builder")
	Register(root, a1)
	Register(root, a2)

	parent, _ := issue.Create(root, "parent", issue.CreateOpts{})
	child, _ := issue.Create(root, "child", issue.CreateOpts{Parent: parent.ID})
	AssignIssue(root, "builder-001", parent.ID)
	AssignIssue(root, "builder-002", child.ID)

	cancelled, err := CancelIssue(root, parent.ID)
	if err != nil {
		t.Fatalf("CancelIssue: %v", err)
	}
	if len(cancelled) != 2 {
		t.Fatalf("expected 2 cancelled, got %d", len(cancelled))
	}

	// Both agents should have empty AssignedIssues.
	for _, id := range []string{"builder-001", "builder-002"} {
		loaded, _ := Load(root, id)
		if len(loaded.AssignedIssues) != 0 {
			t.Errorf("agent %s still has issues: %v", id, loaded.AssignedIssues)
		}
	}
}

func TestCancelIssueAgentSyncFailureRollsBackIssues(t *testing.T) {
	root := setupRoot(t)
	a1 := makeAgent("builder-001", "builder")
	a2 := makeAgent("builder-002", "builder")
	Register(root, a1)
	Register(root, a2)

	parent, _ := issue.Create(root, "parent", issue.CreateOpts{})
	child, _ := issue.Create(root, "child", issue.CreateOpts{Parent: parent.ID})
	AssignIssue(root, "builder-001", parent.ID)
	AssignIssue(root, "builder-002", child.ID)

	replaceAgentFileWithDirectory(t, root, "builder-002")

	if _, err := CancelIssue(root, parent.ID); err == nil {
		t.Fatal("expected CancelIssue to fail when agent reconciliation fails")
	}

	loadedParent, err := issue.Load(root, parent.ID)
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if loadedParent.Status != "assigned" {
		t.Fatalf("parent status should roll back to assigned, got %q", loadedParent.Status)
	}
	if loadedParent.Assignee != "builder-001" {
		t.Fatalf("parent assignee should roll back to builder-001, got %q", loadedParent.Assignee)
	}

	loadedChild, err := issue.Load(root, child.ID)
	if err != nil {
		t.Fatalf("load child: %v", err)
	}
	if loadedChild.Status != "assigned" {
		t.Fatalf("child status should roll back to assigned, got %q", loadedChild.Status)
	}
	if loadedChild.Assignee != "builder-002" {
		t.Fatalf("child assignee should roll back to builder-002, got %q", loadedChild.Assignee)
	}

	loadedAgent, err := Load(root, "builder-001")
	if err != nil {
		t.Fatalf("load builder-001: %v", err)
	}
	if len(loadedAgent.AssignedIssues) != 1 || loadedAgent.AssignedIssues[0] != parent.ID {
		t.Fatalf("builder-001 assignments should roll back, got %v", loadedAgent.AssignedIssues)
	}
}

func TestCloseIssue_AssignedClearsAgent(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "close assigned")
	AssignIssue(root, "builder-001", iss.ID)
	issue.Update(root, iss.ID, issue.UpdateOpts{Status: "in-progress"})

	info, err := CloseIssue(root, iss.ID, "completed")
	if err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	if info.PreviousAssignee != "builder-001" {
		t.Errorf("PreviousAssignee: got %q, want %q", info.PreviousAssignee, "builder-001")
	}

	// Agent should have no assigned issues.
	loaded, _ := Load(root, "builder-001")
	if len(loaded.AssignedIssues) != 0 {
		t.Errorf("agent still has issues: %v", loaded.AssignedIssues)
	}

	// Issue should be done with no assignee.
	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Status != "done" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "done")
	}
	if loadedIss.Assignee != "" {
		t.Errorf("issue Assignee: got %q, want empty", loadedIss.Assignee)
	}
	if loadedIss.CloseReason != "completed" {
		t.Errorf("issue CloseReason: got %q, want %q", loadedIss.CloseReason, "completed")
	}
}

func TestCloseIssue_UnassignedNoError(t *testing.T) {
	root := setupRoot(t)
	iss := createTestIssue(t, root, "close unassigned")

	info, err := CloseIssue(root, iss.ID, "done")
	if err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	if info.PreviousAssignee != "" {
		t.Errorf("PreviousAssignee: got %q, want empty", info.PreviousAssignee)
	}

	loadedIss, _ := issue.Load(root, iss.ID)
	if loadedIss.Status != "done" {
		t.Errorf("issue Status: got %q, want %q", loadedIss.Status, "done")
	}
}

func TestCloseIssue_BlockedByOpenChildren(t *testing.T) {
	root := setupRoot(t)
	parent, _ := issue.Create(root, "parent", issue.CreateOpts{})
	issue.Create(root, "child", issue.CreateOpts{Parent: parent.ID})

	_, err := CloseIssue(root, parent.ID, "try close")
	if err == nil {
		t.Fatal("expected error closing parent with open children")
	}
}

func TestCloseIssueAgentLoadFailureRollsBackIssue(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	Register(root, a)
	iss := createTestIssue(t, root, "close rollback")
	AssignIssue(root, "builder-001", iss.ID)
	issue.Update(root, iss.ID, issue.UpdateOpts{Status: "in-progress"})

	replaceAgentFileWithDirectory(t, root, "builder-001")

	if _, err := CloseIssue(root, iss.ID, "completed"); err == nil {
		t.Fatal("expected CloseIssue to fail when agent reconciliation fails")
	}

	loadedIss, err := issue.Load(root, iss.ID)
	if err != nil {
		t.Fatalf("issue.Load: %v", err)
	}
	if loadedIss.Status != "in-progress" {
		t.Fatalf("issue status should roll back to in-progress, got %q", loadedIss.Status)
	}
	if loadedIss.Assignee != "builder-001" {
		t.Fatalf("issue assignee should roll back to builder-001, got %q", loadedIss.Assignee)
	}
	if loadedIss.CloseReason != "" {
		t.Fatalf("issue close reason should roll back, got %q", loadedIss.CloseReason)
	}
}

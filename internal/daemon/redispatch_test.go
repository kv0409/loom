package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
)

// setupRedispatchRoot creates a temp loom root with issues and agents dirs.
func setupRedispatchRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"issues", "agents"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "issues", "counter.txt"), []byte("0"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestRedispatch_ReopenedIssueAfterAgentDeath verifies that when an agent
// dies and its issue is reopened (unassigned back to open), the daemon's
// watchIssues loop will re-notify the orchestrator about it.
func TestRedispatch_ReopenedIssueAfterAgentDeath(t *testing.T) {
	root := setupRedispatchRoot(t)

	// Create an issue and simulate it being assigned to a builder.
	iss, err := issue.Create(root, "build login", issue.CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	iss.Status = "assigned"
	iss.Assignee = "builder-001"
	if err := issue.Save(root, iss); err != nil {
		t.Fatal(err)
	}

	// Create the builder agent with the issue assigned.
	a := &agent.Agent{
		ID:             "builder-001",
		Role:           "builder",
		Status:         "active",
		AssignedIssues: []string{iss.ID},
	}
	if err := agent.Save(root, a); err != nil {
		t.Fatal(err)
	}

	// Simulate daemon startup: seed notifiedAt from existing issues.
	// The assigned issue should be seeded (it's not open+unassigned).
	notifiedAt := make(map[string]time.Time)
	existing, _ := issue.List(root, issue.ListOpts{All: true})
	for _, e := range existing {
		if e.Status != "open" || e.Assignee != "" {
			notifiedAt[e.ID] = e.UpdatedAt
			continue
		}
		if e.IsReady(root) {
			notifiedAt[e.ID] = e.UpdatedAt
		}
	}

	// Issue should be seeded.
	if _, ok := notifiedAt[iss.ID]; !ok {
		t.Fatal("expected issue to be seeded in notifiedAt")
	}

	// Simulate agent death: unassign issues (reopens to open).
	if err := agent.UnassignAllIssues(root, a); err != nil {
		t.Fatalf("UnassignAllIssues: %v", err)
	}

	// Reload the issue to get the updated state.
	iss, err = issue.Load(root, iss.ID)
	if err != nil {
		t.Fatal(err)
	}
	if iss.Status != "open" {
		t.Fatalf("expected status open after unassign, got %s", iss.Status)
	}
	if iss.Assignee != "" {
		t.Fatalf("expected empty assignee after unassign, got %s", iss.Assignee)
	}

	// Now simulate the watchIssues tick: list ready issues and check notifiedAt.
	ready, err := issue.ListReady(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready issue, got %d", len(ready))
	}

	// The reopened issue should have a newer UpdatedAt than what was seeded,
	// making it eligible for re-notification.
	prev := notifiedAt[ready[0].ID]
	if !ready[0].UpdatedAt.After(prev) {
		t.Fatal("reopened issue UpdatedAt should be after the seeded timestamp; redispatch would fail")
	}
}

// TestRedispatch_StableIssueNotRenotified verifies that an issue that hasn't
// changed does not trigger duplicate notifications.
func TestRedispatch_StableIssueNotRenotified(t *testing.T) {
	root := setupRedispatchRoot(t)

	iss, err := issue.Create(root, "stable task", issue.CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Seed: open+unassigned+ready → seeded with current UpdatedAt.
	notifiedAt := make(map[string]time.Time)
	notifiedAt[iss.ID] = iss.UpdatedAt

	// Simulate tick: issue hasn't changed.
	ready, err := issue.ListReady(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range ready {
		prev, ok := notifiedAt[r.ID]
		if ok && !r.UpdatedAt.After(prev) {
			// This is the expected path: skip, no re-notification.
			continue
		}
		t.Fatalf("stable issue %s should NOT be re-notified", r.ID)
	}
}

// TestRedispatch_KillPathReopensIssue verifies that agent.Kill → UnassignAllIssues
// reopens in-progress issues, making them eligible for redispatch.
func TestRedispatch_KillPathReopensIssue(t *testing.T) {
	root := setupRedispatchRoot(t)

	iss, err := issue.Create(root, "in-progress task", issue.CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	iss.Status = "in-progress"
	iss.Assignee = "builder-002"
	if err := issue.Save(root, iss); err != nil {
		t.Fatal(err)
	}
	beforeReopen := iss.UpdatedAt

	a := &agent.Agent{
		ID:             "builder-002",
		Role:           "builder",
		Status:         "active",
		AssignedIssues: []string{iss.ID},
	}
	if err := agent.Save(root, a); err != nil {
		t.Fatal(err)
	}

	// Simulate kill path: unassign issues.
	if err := agent.UnassignAllIssues(root, a); err != nil {
		t.Fatalf("UnassignAllIssues: %v", err)
	}

	iss, err = issue.Load(root, iss.ID)
	if err != nil {
		t.Fatal(err)
	}
	if iss.Status != "open" {
		t.Fatalf("expected open, got %s", iss.Status)
	}
	if !iss.UpdatedAt.After(beforeReopen) {
		t.Fatal("UpdatedAt should advance after reopen")
	}

	ready, err := issue.ListReady(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != iss.ID {
		t.Fatalf("expected reopened issue in ready list, got %v", ready)
	}
}

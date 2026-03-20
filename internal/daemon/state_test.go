package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
)

func setupStateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"issues", "agents", "mail/inbox", "mail/archive"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "issues", "counter.txt"), []byte("0"), 0644); err != nil {
		t.Fatalf("write issues counter: %v", err)
	}
	return root
}

func registerStateAgent(t *testing.T, root, id, role string) {
	t.Helper()
	a := &agent.Agent{ID: id, Role: role, Status: "active"}
	if err := agent.Register(root, a); err != nil {
		t.Fatalf("register agent %s: %v", id, err)
	}
}

func TestDaemonStateSyncIssuesCachesUnchangedFiles(t *testing.T) {
	root := setupStateRoot(t)
	iss, err := issue.Create(root, "cache me", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	state := newDaemonState(root)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues first: %v", err)
	}

	first := state.issues[iss.ID]
	if first == nil {
		t.Fatalf("missing issue %s after first sync", iss.ID)
	}

	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues second: %v", err)
	}
	if state.issues[iss.ID] != first {
		t.Fatal("unchanged issue should not be reparsed into a new object")
	}

	if _, err := issue.Update(root, iss.ID, issue.UpdateOpts{Priority: "high"}); err != nil {
		t.Fatalf("issue.Update: %v", err)
	}
	state.invalidate(stateTargetIssues)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues after update: %v", err)
	}
	if state.issues[iss.ID] == first {
		t.Fatal("changed issue file should be reparsed")
	}
	if got := state.issues[iss.ID].Priority; got != "high" {
		t.Fatalf("expected updated priority, got %q", got)
	}
}

func TestDaemonStateReadyIssuesUsesCachedDependencyState(t *testing.T) {
	root := setupStateRoot(t)
	dep, err := issue.Create(root, "dependency", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("create dependency: %v", err)
	}
	iss, err := issue.Create(root, "blocked", issue.CreateOpts{DependsOn: []string{dep.ID}})
	if err != nil {
		t.Fatalf("create dependent issue: %v", err)
	}

	state := newDaemonState(root)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues first: %v", err)
	}

	if ready := state.readyIssues(); len(ready) != 1 || ready[0].ID != dep.ID {
		t.Fatalf("expected only dependency issue to be ready initially, got %v", ready)
	}

	if _, err := agent.CloseIssue(root, dep.ID, "done"); err != nil {
		t.Fatalf("close dependency: %v", err)
	}
	state.invalidate(stateTargetIssues)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues second: %v", err)
	}

	ready := state.readyIssues()
	if len(ready) != 1 || ready[0].ID != iss.ID {
		t.Fatalf("expected dependent issue to become ready, got %v", ready)
	}
}

func TestDaemonStateSyncAgentsAndMailCachesUnreadMessages(t *testing.T) {
	root := setupStateRoot(t)
	registerStateAgent(t, root, "lead-001", "lead")
	registerStateAgent(t, root, "builder-001", "builder")

	if err := mail.Send(root, mail.SendOpts{
		From:    "lead-001",
		To:      "builder-001",
		Subject: "cache mail",
		Type:    "task",
	}); err != nil {
		t.Fatalf("mail.Send: %v", err)
	}

	state := newDaemonState(root)
	if err := state.syncAgents(); err != nil {
		t.Fatalf("syncAgents first: %v", err)
	}
	if err := state.syncMail(); err != nil {
		t.Fatalf("syncMail first: %v", err)
	}

	firstAgent := state.agents["builder-001"]
	if firstAgent == nil {
		t.Fatal("missing cached agent after sync")
	}
	unread := state.unreadMessages("builder-001")
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(unread))
	}
	firstMail := state.mailByAgent["builder-001"][unread[0].ID]

	if err := state.syncAgents(); err != nil {
		t.Fatalf("syncAgents second: %v", err)
	}
	if err := state.syncMail(); err != nil {
		t.Fatalf("syncMail second: %v", err)
	}
	if state.agents["builder-001"] != firstAgent {
		t.Fatal("unchanged agent should not be reparsed into a new object")
	}
	if state.mailByAgent["builder-001"][unread[0].ID] != firstMail {
		t.Fatal("unchanged mail file should not be reparsed into a new object")
	}

	if err := mail.MarkRead(root, "builder-001", unread[0].ID); err != nil {
		t.Fatalf("mail.MarkRead: %v", err)
	}
	state.invalidate(stateTargetMail)
	if err := state.syncMail(); err != nil {
		t.Fatalf("syncMail after mark-read: %v", err)
	}
	if got := state.unreadMessages("builder-001"); len(got) != 0 {
		t.Fatalf("expected no unread messages after mark-read, got %d", len(got))
	}
	if state.mailByAgent["builder-001"][unread[0].ID] == firstMail {
		t.Fatal("changed mail file should be reparsed")
	}
}

func TestDaemonStateSkipsIssueRescanUntilInvalidated(t *testing.T) {
	root := setupStateRoot(t)
	iss, err := issue.Create(root, "cache me", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	state := newDaemonState(root)
	state.now = func() time.Time { return now }
	state.reconcileEvery = time.Hour
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues first: %v", err)
	}

	if got := state.issueByID(iss.ID).Priority; got != "normal" {
		t.Fatalf("expected initial priority normal, got %q", got)
	}

	if _, err := issue.Update(root, iss.ID, issue.UpdateOpts{Priority: "high"}); err != nil {
		t.Fatalf("issue.Update: %v", err)
	}

	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues without invalidation: %v", err)
	}
	if got := state.issueByID(iss.ID).Priority; got != "normal" {
		t.Fatalf("expected cached priority to remain normal before invalidation, got %q", got)
	}

	state.invalidate(stateTargetIssues)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues after invalidation: %v", err)
	}
	if got := state.issueByID(iss.ID).Priority; got != "high" {
		t.Fatalf("expected invalidated cache to reload high priority, got %q", got)
	}
}

func TestDaemonStateReconcilesDirtyIssueAfterInterval(t *testing.T) {
	root := setupStateRoot(t)
	iss, err := issue.Create(root, "cache me later", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	state := newDaemonState(root)
	state.now = func() time.Time { return now }
	state.reconcileEvery = time.Minute
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues first: %v", err)
	}

	if _, err := issue.Update(root, iss.ID, issue.UpdateOpts{Priority: "high"}); err != nil {
		t.Fatalf("issue.Update: %v", err)
	}

	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues before reconcile interval: %v", err)
	}
	if got := state.issueByID(iss.ID).Priority; got != "normal" {
		t.Fatalf("expected cached priority to remain normal before reconcile interval, got %q", got)
	}

	now = now.Add(2 * time.Minute)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues after reconcile interval: %v", err)
	}
	if got := state.issueByID(iss.ID).Priority; got != "high" {
		t.Fatalf("expected reconcile interval to refresh high priority, got %q", got)
	}
}

func TestDaemonStateCachesDerivedIssueIndexes(t *testing.T) {
	root := setupStateRoot(t)
	parent, err := issue.Create(root, "parent", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := issue.Create(root, "child", issue.CreateOpts{Parent: parent.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	registerStateAgent(t, root, "builder-001", "builder")
	dep, err := issue.Create(root, "dependency", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("create dependency: %v", err)
	}
	blocked, err := issue.Create(root, "blocked", issue.CreateOpts{DependsOn: []string{dep.ID}})
	if err != nil {
		t.Fatalf("create blocked issue: %v", err)
	}
	if err := agent.AssignIssue(root, "builder-001", child.ID); err != nil {
		t.Fatalf("assign child: %v", err)
	}
	if _, err := agent.CloseIssue(root, dep.ID, "done"); err != nil {
		t.Fatalf("close dependency: %v", err)
	}

	state := newDaemonState(root)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues: %v", err)
	}

	issues := state.allIssues()
	if len(issues) != 4 {
		t.Fatalf("expected 4 issues, got %d", len(issues))
	}
	if issues[0].ID != dep.ID {
		t.Fatalf("expected most recently updated issue first, got %s", issues[0].ID)
	}

	ready := state.readyIssues()
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready issues, got %d", len(ready))
	}
	if ready[0].ID != blocked.ID || ready[1].ID != parent.ID {
		t.Fatalf("unexpected ready issue order: got %s then %s", ready[0].ID, ready[1].ID)
	}

	resolved := state.resolvedIssueSet()
	if !resolved[dep.ID] {
		t.Fatalf("expected resolved set to include %s", dep.ID)
	}
	if resolved[parent.ID] {
		t.Fatalf("did not expect resolved set to include %s", parent.ID)
	}

	if state.allDescendantsResolved(parent.ID) {
		t.Fatalf("expected unresolved descendants while child %s is assigned", child.ID)
	}

	if _, err := agent.CloseIssue(root, child.ID, "done"); err != nil {
		t.Fatalf("close child: %v", err)
	}
	state.invalidate(stateTargetIssues)
	if err := state.syncIssues(); err != nil {
		t.Fatalf("syncIssues after child close: %v", err)
	}
	if !state.allDescendantsResolved(parent.ID) {
		t.Fatalf("expected parent descendants to be resolved after closing %s", child.ID)
	}
}

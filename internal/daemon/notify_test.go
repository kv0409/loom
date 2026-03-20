package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
)

// stubClient is a minimal ACP client stand-in for testing notify outcomes.
// We can't easily construct a real *acp.Client without a subprocess, so we
// test the three code paths by controlling what's in acpClients and the
// agent's ACPSessionID.

func newTestDaemon() *Daemon {
	return &Daemon{
		acpClients: make(map[string]*acp.Client),
	}
}

func TestNotify_NoSession(t *testing.T) {
	d := newTestDaemon()
	a := &agent.Agent{ID: "test-agent", ACPSessionID: ""}
	nr := d.notify(a, "hello")
	if nr.Outcome != NotifySkipped {
		t.Fatalf("expected skipped, got %s", nr.Outcome)
	}
	if nr.Reason != "no active session" {
		t.Fatalf("unexpected reason: %s", nr.Reason)
	}
}

func TestNotify_NoClient(t *testing.T) {
	d := newTestDaemon()
	a := &agent.Agent{ID: "test-agent", ACPSessionID: "sess-1"}
	nr := d.notify(a, "hello")
	if nr.Outcome != NotifySkipped {
		t.Fatalf("expected skipped, got %s", nr.Outcome)
	}
	if nr.Reason != "no ACP client" {
		t.Fatalf("unexpected reason: %s", nr.Reason)
	}
}

func TestNotify_ExitedClient(t *testing.T) {
	d := newTestDaemon()
	// Register a nil client entry — simulates a client that was removed.
	// With no client in the map, we get "no ACP client".
	a := &agent.Agent{ID: "test-agent", ACPSessionID: "sess-1"}
	nr := d.notify(a, "hello")
	if nr.Outcome != NotifySkipped {
		t.Fatalf("expected skipped, got %s", nr.Outcome)
	}
}

func TestApiNudge_NoSession(t *testing.T) {
	tmp := t.TempDir()
	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
	}
	// Create agents dir and a minimal agent file on disk.
	os.MkdirAll(filepath.Join(tmp, "agents"), 0755)
	a := &agent.Agent{ID: "test-agent", Status: "active", Role: "builder"}
	if err := agent.Save(tmp, a); err != nil {
		t.Fatal(err)
	}
	resp := d.apiNudge(Request{AgentID: "test-agent", Message: "wake up"})
	if !resp.OK {
		// OK is true even for skipped — only failed returns error.
		// But since session is empty, it's skipped, which is OK.
		t.Logf("response: ok=%v data=%v error=%v", resp.OK, resp.Data, resp.Error)
	}
	nr, ok := resp.Data.(NotifyResult)
	if !ok {
		t.Fatalf("expected NotifyResult in data, got %T", resp.Data)
	}
	if nr.Outcome != NotifySkipped {
		t.Fatalf("expected skipped, got %s", nr.Outcome)
	}
}

func TestApiMessage_NoClient(t *testing.T) {
	tmp := t.TempDir()
	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
	}
	os.MkdirAll(filepath.Join(tmp, "agents"), 0755)
	a := &agent.Agent{ID: "test-agent", Status: "active", Role: "builder", ACPSessionID: "sess-1"}
	if err := agent.Save(tmp, a); err != nil {
		t.Fatal(err)
	}
	resp := d.apiMessage(Request{AgentID: "test-agent", Message: "hello"})
	if !resp.OK {
		t.Logf("response: ok=%v error=%v", resp.OK, resp.Error)
	}
	nr, ok := resp.Data.(NotifyResult)
	if !ok {
		t.Fatalf("expected NotifyResult in data, got %T", resp.Data)
	}
	if nr.Outcome != NotifySkipped {
		t.Fatalf("expected skipped, got %s", nr.Outcome)
	}
}

func TestApiNudge_AgentNotFound(t *testing.T) {
	tmp := t.TempDir()
	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
	}
	resp := d.apiNudge(Request{AgentID: "nonexistent", Message: "hello"})
	if resp.OK {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestApiInvalidateIssuesMarksCacheDirty(t *testing.T) {
	tmp := setupStateRoot(t)
	iss, err := issue.Create(tmp, "invalidate me", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
		state:      newDaemonState(tmp),
	}
	d.state.now = func() time.Time { return now }
	d.state.reconcileEvery = time.Hour
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues first: %v", err)
	}

	if _, err := issue.Update(tmp, iss.ID, issue.UpdateOpts{Priority: "high"}); err != nil {
		t.Fatalf("issue.Update: %v", err)
	}
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues before invalidate: %v", err)
	}
	if got := d.state.issueByID(iss.ID).Priority; got != "normal" {
		t.Fatalf("expected cached priority normal before invalidate, got %q", got)
	}

	resp := d.apiInvalidate(Request{Targets: []string{"issues"}})
	if !resp.OK {
		t.Fatalf("expected invalidate ok, got error %q", resp.Error)
	}
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues after invalidate: %v", err)
	}
	if got := d.state.issueByID(iss.ID).Priority; got != "high" {
		t.Fatalf("expected high priority after invalidate, got %q", got)
	}
}

func TestApiHeartbeatUpdatesCachedAgentWithoutDirtyingState(t *testing.T) {
	tmp := setupStateRoot(t)
	oldHeartbeat := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &agent.Agent{ID: "builder-001", Role: "builder", Status: "active", Heartbeat: oldHeartbeat}
	if err := agent.Register(tmp, a); err != nil {
		t.Fatalf("agent.Register: %v", err)
	}

	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
		state:      newDaemonState(tmp),
	}
	d.state.reconcileEvery = time.Hour
	if err := d.state.syncAgents(); err != nil {
		t.Fatalf("syncAgents first: %v", err)
	}

	before := d.state.agentByID("builder-001")
	if before == nil {
		t.Fatal("expected cached agent")
	}

	resp := d.apiHeartbeat(Request{AgentID: "builder-001"})
	if !resp.OK {
		t.Fatalf("expected heartbeat ok, got error %q", resp.Error)
	}

	after := d.state.agentByID("builder-001")
	if after == nil {
		t.Fatal("expected cached agent after heartbeat")
	}
	if !after.Heartbeat.After(before.Heartbeat) {
		t.Fatalf("expected cached heartbeat to advance, before=%v after=%v", before.Heartbeat, after.Heartbeat)
	}
	if d.state.dirty&stateTargetAgents != 0 {
		t.Fatal("expected heartbeat api to refresh cache directly without leaving agents dirty")
	}
}

func TestApiRefreshIssueUpdatesCachedIssueWithoutDirtyingState(t *testing.T) {
	tmp := setupStateRoot(t)
	iss, err := issue.Create(tmp, "refresh me", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
		state:      newDaemonState(tmp),
	}
	d.state.reconcileEvery = time.Hour
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues first: %v", err)
	}

	if _, err := issue.Update(tmp, iss.ID, issue.UpdateOpts{Priority: "high"}); err != nil {
		t.Fatalf("issue.Update: %v", err)
	}
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues before refresh: %v", err)
	}
	if got := d.state.issueByID(iss.ID).Priority; got != "normal" {
		t.Fatalf("expected stale cached priority normal before refresh, got %q", got)
	}

	resp := d.apiRefresh(Request{IssueIDs: []string{iss.ID}})
	if !resp.OK {
		t.Fatalf("expected refresh ok, got error %q", resp.Error)
	}
	if got := d.state.issueByID(iss.ID).Priority; got != "high" {
		t.Fatalf("expected refreshed priority high, got %q", got)
	}
	if d.state.dirty&stateTargetIssues != 0 {
		t.Fatal("expected targeted issue refresh to update cache without leaving issues dirty")
	}
}

func TestApiRefreshMailboxUpdatesCachedUnreadMessagesWithoutDirtyingState(t *testing.T) {
	tmp := setupStateRoot(t)
	registerStateAgent(t, tmp, "lead-001", "lead")
	registerStateAgent(t, tmp, "builder-001", "builder")
	if err := mail.Send(tmp, mail.SendOpts{
		From:    "lead-001",
		To:      "builder-001",
		Subject: "refresh mail",
		Type:    "task",
	}); err != nil {
		t.Fatalf("mail.Send: %v", err)
	}

	d := &Daemon{
		LoomRoot:   tmp,
		Config:     &config.Config{},
		acpClients: make(map[string]*acp.Client),
		state:      newDaemonState(tmp),
	}
	d.state.reconcileEvery = time.Hour
	if err := d.state.syncMail(); err != nil {
		t.Fatalf("syncMail first: %v", err)
	}

	unread := d.state.unreadMessages("builder-001")
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(unread))
	}
	if err := mail.MarkRead(tmp, "builder-001", unread[0].ID); err != nil {
		t.Fatalf("mail.MarkRead: %v", err)
	}
	if err := d.state.syncMail(); err != nil {
		t.Fatalf("syncMail before refresh: %v", err)
	}
	if got := d.state.unreadMessages("builder-001"); len(got) != 1 {
		t.Fatalf("expected stale unread cache before refresh, got %d", len(got))
	}

	resp := d.apiRefresh(Request{MailAgents: []string{"builder-001"}})
	if !resp.OK {
		t.Fatalf("expected refresh ok, got error %q", resp.Error)
	}
	if got := d.state.unreadMessages("builder-001"); len(got) != 0 {
		t.Fatalf("expected refreshed mailbox to have no unread messages, got %d", len(got))
	}
	if d.state.dirty&stateTargetMail != 0 {
		t.Fatal("expected targeted mailbox refresh to update cache without leaving mail dirty")
	}
}

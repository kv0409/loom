package daemon

import (
	"testing"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
)

func TestAPIRefreshIssueSignalsWakeup(t *testing.T) {
	root := setupStateRoot(t)
	iss, err := issue.Create(root, "wake me", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	d := New(root, &config.Config{})
	d.state.reconcileEvery = time.Hour
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues: %v", err)
	}

	if _, err := issue.Update(root, iss.ID, issue.UpdateOpts{Priority: "high"}); err != nil {
		t.Fatalf("issue.Update: %v", err)
	}

	resp := d.apiRefresh(Request{IssueIDs: []string{iss.ID}})
	if !resp.OK {
		t.Fatalf("expected refresh ok, got error %q", resp.Error)
	}

	select {
	case <-d.issueWake:
	default:
		t.Fatal("expected issue refresh to wake issue watchers")
	}

	select {
	case <-d.agentWake:
		t.Fatal("did not expect issue refresh to wake agent watchers")
	default:
	}

	select {
	case <-d.mailWake:
		t.Fatal("did not expect issue refresh to wake mail watchers")
	default:
	}
}

func TestKillRefreshOptsTracksAffectedAgentsIssuesAndMail(t *testing.T) {
	root := setupStateRoot(t)
	registerStateAgent(t, root, "lead-001", "lead")
	child := &agent.Agent{ID: "builder-001", Role: "builder", Status: "active", SpawnedBy: "lead-001"}
	if err := agent.Register(root, child); err != nil {
		t.Fatalf("agent.Register child: %v", err)
	}
	iss, err := issue.Create(root, "kill target", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}
	if err := agent.AssignIssue(root, "builder-001", iss.ID); err != nil {
		t.Fatalf("agent.AssignIssue: %v", err)
	}
	if err := mail.Send(root, mail.SendOpts{
		From:    "lead-001",
		To:      "builder-001",
		Subject: "cleanup mail",
		Type:    "task",
	}); err != nil {
		t.Fatalf("mail.Send: %v", err)
	}

	d := New(root, &config.Config{})
	if err := d.state.syncAgents(); err != nil {
		t.Fatalf("syncAgents: %v", err)
	}
	if err := d.state.syncIssues(); err != nil {
		t.Fatalf("syncIssues: %v", err)
	}
	if err := d.state.syncMail(); err != nil {
		t.Fatalf("syncMail: %v", err)
	}

	opts := d.killRefreshOpts("lead-001", nil)
	if len(opts.AgentIDs) != 2 {
		t.Fatalf("expected 2 agent refresh targets, got %v", opts.AgentIDs)
	}
	if len(opts.IssueIDs) != 1 || opts.IssueIDs[0] != iss.ID {
		t.Fatalf("expected issue refresh target %s, got %v", iss.ID, opts.IssueIDs)
	}
	if len(opts.MailAgents) != 2 {
		t.Fatalf("expected 2 mailbox refresh targets, got %v", opts.MailAgents)
	}
}

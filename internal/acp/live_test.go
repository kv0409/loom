//go:build live

// Live integration test for the ACP client against a real kiro-cli process.
//
// Skipped by default. Run with:
//
//	go test -tags=live -run TestLive -v ./internal/acp/...
//
// Requires `kiro-cli` on PATH. Uses a temporary working directory so it does
// not touch any loom state (no daemon, no .loom/, no agent/issue files).
package acp

import (
	"os/exec"
	"testing"
	"time"
)

func TestLive_Handshake(t *testing.T) {
	bin, err := exec.LookPath("kiro-cli")
	if err != nil {
		t.Skip("kiro-cli not on PATH")
	}
	workDir := t.TempDir()

	c, err := NewClient(bin, workDir, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	res, err := c.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Logf("server: name=%q version=%q", res.ServerInfo.Name, res.ServerInfo.Version)

	sessionID, err := c.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sessionID == "" {
		t.Fatal("NewSession returned empty session id")
	}
	t.Logf("session: %s", sessionID)

	// Fire-and-forget prompt: SendPrompt must return immediately without error
	// even though the agent will still be processing the turn.
	if err := c.SendPrompt(sessionID, "Reply with exactly the word 'pong' and nothing else."); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	// Wait up to 60s for any streamed output so we know the session/update
	// notification path works end-to-end. We don't assert on content — just
	// that the channel is alive and SessionUpdate callbacks fire.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if len(c.RecentOutput(1)) > 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("no session updates received within 60s")
}

func TestLive_CancelSession(t *testing.T) {
	bin, err := exec.LookPath("kiro-cli")
	if err != nil {
		t.Skip("kiro-cli not on PATH")
	}
	c, err := NewClient(bin, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sessionID, err := c.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := c.SendPrompt(sessionID, "Count slowly to 1000."); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	// Give it a moment to start.
	time.Sleep(500 * time.Millisecond)
	if err := c.CancelSession(sessionID); err != nil {
		t.Fatalf("CancelSession: %v", err)
	}
}

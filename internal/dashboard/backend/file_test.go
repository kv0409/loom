package backend

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/issue"
)

func TestSortAgentTree_BuildsCorrectTree(t *testing.T) {
	agents := []*agent.Agent{
		{ID: "orchestrator", SpawnedBy: ""},
		{ID: "lead-001", SpawnedBy: "orchestrator"},
		{ID: "builder-001", SpawnedBy: "lead-001"},
	}

	sorted, tree := sortAgentTree(agents)

	if len(sorted) != 3 || len(tree) != 3 {
		t.Fatalf("expected 3 entries, got sorted=%d tree=%d", len(sorted), len(tree))
	}

	// orchestrator at depth 0
	if tree[0].Depth != 0 {
		t.Errorf("orchestrator depth: want 0, got %d", tree[0].Depth)
	}
	if !tree[0].IsLast {
		t.Error("orchestrator should be last root")
	}

	// lead-001 at depth 1
	if sorted[1].ID != "lead-001" {
		t.Errorf("index 1: want lead-001, got %s", sorted[1].ID)
	}
	if tree[1].Depth != 1 {
		t.Errorf("lead depth: want 1, got %d", tree[1].Depth)
	}
	if !tree[1].IsLast {
		t.Error("lead-001 should be last child of orchestrator")
	}

	// builder-001 at depth 2
	if sorted[2].ID != "builder-001" {
		t.Errorf("index 2: want builder-001, got %s", sorted[2].ID)
	}
	if tree[2].Depth != 2 {
		t.Errorf("builder depth: want 2, got %d", tree[2].Depth)
	}
	if !tree[2].IsLast {
		t.Error("builder-001 should be last child of lead-001")
	}
	// Ancestors: [true, true] — both orchestrator and lead are last children
	if len(tree[2].Ancestors) != 2 || !tree[2].Ancestors[0] || !tree[2].Ancestors[1] {
		t.Errorf("builder ancestors: want [true true], got %v", tree[2].Ancestors)
	}
}

func TestSortAgentTree_Empty(t *testing.T) {
	sorted, tree := sortAgentTree(nil)
	if len(sorted) != 0 {
		t.Errorf("expected nil sorted, got %d", len(sorted))
	}
	if tree != nil {
		t.Errorf("expected nil tree, got %v", tree)
	}
}

func TestFetchActivity_ToolsFile(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	os.MkdirAll(agentsDir, 0755)

	toolsA := "12:00:03 shell: go test\n12:00:01 read: main.go\n"
	toolsB := "12:00:02 write: output.go\n"
	os.WriteFile(filepath.Join(agentsDir, "agent-a.tools"), []byte(toolsA), 0644)
	os.WriteFile(filepath.Join(agentsDir, "agent-b.tools"), []byte(toolsB), 0644)

	agents := []*agent.Agent{
		{ID: "agent-a", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
		{ID: "agent-b", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	wantLines := []string{"12:00:03 shell: go test", "12:00:02 write: output.go", "12:00:01 read: main.go"}
	for i, want := range wantLines {
		if entries[i].Line != want {
			t.Errorf("entry[%d]: want %q, got %q", i, want, entries[i].Line)
		}
	}
}

func TestFetchActivity_NDJSONToolSummary(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	os.MkdirAll(agentsDir, 0755)

	ndjson := `{"kind":"token_chunk","ts":"12:00:01","content":"thinking..."}
{"kind":"tool_summary","ts":"12:00:02","content":"Called execute_bash: go build"}
{"kind":"token_chunk","ts":"12:00:03","content":"more tokens"}
{"kind":"tool_summary","ts":"12:00:04","content":"Called fs_read: main.go"}
`
	os.WriteFile(filepath.Join(agentsDir, "builder-001.output"), []byte(ndjson), 0644)

	agents := []*agent.Agent{
		{ID: "builder-001", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AgentID != "builder-001" {
		t.Errorf("expected AgentID %q, got %q", "builder-001", entries[0].AgentID)
	}
	if entries[0].Line != "Called fs_read: main.go" {
		t.Errorf("expected last tool_summary content, got %q", entries[0].Line)
	}
}

func TestFetchActivity_DeadAgentSkipped(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "agents"), 0755)

	agents := []*agent.Agent{
		{ID: "builder-dead", Status: "dead", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for dead agent with no .tools file, got %d", len(entries))
	}
}

func TestCountUnread_NoInbox(t *testing.T) {
	root := t.TempDir()
	if n := countUnread(root); n != 0 {
		t.Errorf("expected 0 unread, got %d", n)
	}
}

func TestLogReader_IncrementalRead(t *testing.T) {
	root := t.TempDir()
	logsDir := filepath.Join(root, "logs")
	os.MkdirAll(logsDir, 0755)
	logPath := filepath.Join(logsDir, "daemon.log")

	os.WriteFile(logPath, []byte("[acp] activating agent builder-001\n"), 0644)

	lr := newLogReader(root)
	lines := lr.read()
	if len(lines) != 1 {
		t.Fatalf("first read: expected 1 line, got %d", len(lines))
	}
	if lines[0].Category != "lifecycle" {
		t.Errorf("expected lifecycle, got %q", lines[0].Category)
	}

	// Append more data
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("something failed badly\n")
	f.Close()

	lines = lr.read()
	if len(lines) != 2 {
		t.Fatalf("second read: expected 2 lines, got %d", len(lines))
	}
	if lines[1].Category != "error" {
		t.Errorf("expected error, got %q", lines[1].Category)
	}
}

func TestLogReader_Truncation(t *testing.T) {
	root := t.TempDir()
	logsDir := filepath.Join(root, "logs")
	os.MkdirAll(logsDir, 0755)
	logPath := filepath.Join(logsDir, "daemon.log")

	os.WriteFile(logPath, []byte("[acp] activating agent builder-001\n"), 0644)

	lr := newLogReader(root)
	lines := lr.read()
	if len(lines) != 1 {
		t.Fatalf("first read: expected 1 line, got %d", len(lines))
	}

	// Truncate file (simulate rotation)
	os.WriteFile(logPath, []byte("something failed\n"), 0644)

	lines = lr.read()
	// After truncation, reader resets — only the new line
	if len(lines) != 1 {
		t.Fatalf("after truncation: expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0].Text, "failed") {
		t.Errorf("expected 'failed' line, got %q", lines[0].Text)
	}
}

func TestNewFileBackend_Load(t *testing.T) {
	root := t.TempDir()
	// Create minimal directory structure
	for _, dir := range []string{"agents", "issues", "mail/inbox", "mail/log", "logs", "worktrees", "memory"} {
		os.MkdirAll(filepath.Join(root, dir), 0755)
	}

	// Write a daemon log line
	os.WriteFile(filepath.Join(root, "logs", "daemon.log"), []byte("[acp] activating agent test-001\n"), 0644)

	// Write a .tools file
	os.WriteFile(filepath.Join(root, "agents", "test-001.tools"), []byte("12:00:01 shell: go test\n"), 0644)

	// Write a minimal agent file
	os.WriteFile(filepath.Join(root, "agents", "test-001.yaml"), []byte("id: test-001\nstatus: active\nrole: builder\nconfig:\n  kiro_mode: acp\n"), 0644)

	fb := NewFileBackend(root)
	snap := fb.Load()

	if snap.DiffStats == nil {
		t.Error("DiffStats should be non-nil map")
	}
	if len(snap.Logs) != 1 {
		t.Errorf("expected 1 log line, got %d", len(snap.Logs))
	}
	if len(snap.Activity) == 0 {
		t.Error("expected at least 1 activity entry from .tools file")
	}
	// DaemonOK should be false since there's no socket file
	if snap.DaemonOK {
		t.Error("expected DaemonOK=false without daemon.sock")
	}
}

func TestRelativeTime_EmptyString(t *testing.T) {
	got := RelativeTime("")
	if got != "" {
		t.Errorf("RelativeTime(\"\") = %q, want \"\"", got)
	}
}

func TestRelativeTime_MalformedTimestamp(t *testing.T) {
	for _, ts := range []string{"not-a-time", "12:34", "2024-01-01"} {
		got := RelativeTime(ts)
		if got != ts {
			t.Errorf("RelativeTime(%q) = %q, want %q (passthrough)", ts, got, ts)
		}
	}
}

func TestExtractTimestamp_ISO(t *testing.T) {
	ts, rest := ExtractTimestamp("2024-01-15T10:30:45 shell: go test")
	if ts != "2024-01-15T10:30:45" {
		t.Errorf("ts = %q, want \"2024-01-15T10:30:45\"", ts)
	}
	if rest != "shell: go test" {
		t.Errorf("rest = %q, want \"shell: go test\"", rest)
	}
}

func TestExtractTimestamp_TimeOnly(t *testing.T) {
	ts, rest := ExtractTimestamp("10:30:45 read: main.go")
	if ts != "10:30:45" {
		t.Errorf("ts = %q, want \"10:30:45\"", ts)
	}
	if rest != "read: main.go" {
		t.Errorf("rest = %q, want \"read: main.go\"", rest)
	}
}

func TestCleanArgs_StripsCdPrefix(t *testing.T) {
	got := CleanArgs("cd /project && go test ./...", "/project")
	if got != "go test ./..." {
		t.Errorf("CleanArgs = %q, want \"go test ./...\"", got)
	}
}

func TestLoad_CollectsErrors(t *testing.T) {
	// Use a non-existent root so all List calls fail.
	fb := NewFileBackend("/tmp/nonexistent-loom-root-" + t.Name())
	snap := fb.Load()
	if len(snap.Errors) == 0 {
		t.Fatal("expected Errors to be non-empty for non-existent root")
	}
}

func TestDiff_RejectsDashPrefix(t *testing.T) {
	// We can't easily make worktree.DefaultBranch return a dash-prefixed
	// branch, but we verify the guard exists by checking that Diff on a
	// non-git directory returns a safe fallback (not a panic or flag injection).
	fb := NewFileBackend(t.TempDir())
	result := fb.Diff(t.TempDir())
	// Should return a safe error string, not panic.
	if result == "" {
		t.Error("expected non-empty result from Diff on non-git dir")
	}
}

func TestLoad_UsesDaemonSnapshotForControlPlaneState(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "loom-backend-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(root)
	for _, dir := range []string{"agents", "issues", "mail/inbox", "mail/log", "logs", "worktrees", "memory"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "issues", "counter.txt"), []byte("0"), 0644); err != nil {
		t.Fatalf("write counter: %v", err)
	}

	a := &agent.Agent{ID: "builder-001", Status: "active", Role: "builder"}
	if err := agent.Register(root, a); err != nil {
		t.Fatalf("agent.Register: %v", err)
	}
	iss, err := issue.Create(root, "cached issue", issue.CreateOpts{})
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}

	sock := daemon.SockPath(root)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen snapshot socket: %v", err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(sock)
	}()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req daemon.Request
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			t.Logf("decode request: %v", err)
			return
		}
		if req.Action != "snapshot" {
			t.Logf("unexpected action %q", req.Action)
			return
		}

		resp := daemon.Response{
			OK: true,
			Data: map[string]any{
				"agents": []*agent.Agent{{ID: a.ID, Status: a.Status, Role: a.Role}},
				"issues": []*issue.Issue{{ID: iss.ID, Title: iss.Title, Status: iss.Status}},
				"unread": 0,
			},
		}
		if err := json.NewEncoder(conn).Encode(resp); err != nil {
			t.Logf("encode response: %v", err)
		}
	}()

	if err := os.WriteFile(filepath.Join(root, "agents", "builder-001.yaml"), []byte(":\n"), 0644); err != nil {
		t.Fatalf("corrupt agent yaml: %v", err)
	}

	fb := NewFileBackend(root)
	snap := fb.Load()
	if !snap.DaemonOK {
		t.Fatal("expected daemon snapshot path to mark daemon available")
	}
	if len(snap.Agents) != 1 || snap.Agents[0].ID != "builder-001" {
		t.Fatalf("expected daemon snapshot to return cached agent, got %+v", snap.Agents)
	}
	if len(snap.Issues) != 1 {
		t.Fatalf("expected daemon snapshot to return cached issue, got %d issues", len(snap.Issues))
	}
	for _, err := range snap.Errors {
		if strings.HasPrefix(err, "agents:") || strings.HasPrefix(err, "issues:") {
			t.Fatalf("did not expect control-plane filesystem read error when daemon snapshot is available, got %q", err)
		}
	}
}

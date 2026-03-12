package dashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/karanagi/loom/internal/agent"
)

func TestFetchActivity_NDJSONToolSummaryFiltered(t *testing.T) {
	// Set up a temp loom root with an agent .output file containing NDJSON.
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ndjson := `{"kind":"token_chunk","ts":"12:00:01","content":"thinking..."}
{"kind":"tool_summary","ts":"12:00:02","content":"Called execute_bash: go build"}
{"kind":"token_chunk","ts":"12:00:03","content":"more tokens"}
{"kind":"tool_summary","ts":"12:00:04","content":"Called fs_read: main.go"}
`
	outPath := filepath.Join(agentsDir, "builder-001.output")
	if err := os.WriteFile(outPath, []byte(ndjson), 0644); err != nil {
		t.Fatal(err)
	}

	agents := []*agent.Agent{
		{
			ID:     "builder-001",
			Status: "active",
			Config: agent.AgentConfig{KiroMode: "acp"},
		},
	}

	entries := fetchActivity(root, agents)

	if len(entries) != 1 {
		t.Fatalf("expected 1 activity entry, got %d", len(entries))
	}
	if entries[0].AgentID != "builder-001" {
		t.Errorf("expected AgentID %q, got %q", "builder-001", entries[0].AgentID)
	}
	// Should show the last tool_summary, not a token_chunk.
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
		t.Errorf("expected 0 entries for dead agent, got %d", len(entries))
	}
}

func TestFetchActivity_NoOutputFile_Skipped(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "agents"), 0755)

	agents := []*agent.Agent{
		{ID: "builder-nofile", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries when output file missing, got %d", len(entries))
	}
}

package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/agent"
)

func TestFetchActivity_ToolsFileAllLinesSortedChronologically(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Two agents with .tools files; lines interleaved by timestamp.
	toolsA := "12:00:03 shell: go test\n12:00:01 read: main.go\n"
	toolsB := "12:00:02 write: output.go\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agent-a.tools"), []byte(toolsA), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "agent-b.tools"), []byte(toolsB), 0644); err != nil {
		t.Fatal(err)
	}

	agents := []*agent.Agent{
		{ID: "agent-a", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
		{ID: "agent-b", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Sorted by timestamp: 12:00:01, 12:00:02, 12:00:03
	wantLines := []string{"12:00:01 read: main.go", "12:00:02 write: output.go", "12:00:03 shell: go test"}
	for i, want := range wantLines {
		if entries[i].Line != want {
			t.Errorf("entry[%d]: want %q, got %q", i, want, entries[i].Line)
		}
	}
}

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

func TestFetchActivity_TokenChunkFallbackConcatenates(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// No tool_summary events — only token_chunks that should be concatenated.
	ndjson := `{"kind":"token_chunk","ts":"12:00:01","content":"Hello, "}
{"kind":"token_chunk","ts":"12:00:02","content":"world"}
{"kind":"token_chunk","ts":"12:00:03","content":"!"}
`
	if err := os.WriteFile(filepath.Join(agentsDir, "builder-002.output"), []byte(ndjson), 0644); err != nil {
		t.Fatal(err)
	}

	agents := []*agent.Agent{
		{ID: "builder-002", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Line != "Hello, world!" {
		t.Errorf("expected concatenated token chunks %q, got %q", "Hello, world!", entries[0].Line)
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

func TestFetchActivity_UTF8ByteSlicing(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Build a string of multi-byte chars that exceeds maxLen (200).
	// Each '日' is 3 bytes; 210 runes = 630 bytes.
	longUTF8 := strings.Repeat("日", 210)
	ndjson := `{"kind":"tool_summary","ts":"12:00:01","content":"` + longUTF8 + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "utf8-agent.output"), []byte(ndjson), 0644); err != nil {
		t.Fatal(err)
	}

	agents := []*agent.Agent{
		{ID: "utf8-agent", Status: "active", Config: agent.AgentConfig{KiroMode: "acp"}},
	}

	entries := fetchActivity(root, agents)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	line := entries[0].Line
	// Must be valid UTF-8 — no mid-rune slicing.
	for i, r := range line {
		if r == 0xFFFD {
			t.Fatalf("replacement char U+FFFD at byte %d — invalid UTF-8 from truncation", i)
		}
	}
	// Rune count must not exceed maxLen (200).
	runes := []rune(line)
	if len(runes) > 200 {
		t.Errorf("expected ≤200 runes, got %d", len(runes))
	}
}

func TestRenderActivity_ColumnWidthOffByOne(t *testing.T) {
	m := Model{width: 80, height: 30}
	// Populate with a few entries so the table renders.
	m.data.activity = []activityEntry{
		{AgentID: "agent-1", Line: "Called execute_bash: go test"},
		{AgentID: "agent-2", Line: "Called fs_read: main.go"},
	}

	rendered := m.renderActivity()
	// Each rendered line must fit within the panel width (width-2 for borders).
	panelW := m.width - 2
	for i, line := range strings.Split(rendered, "\n") {
		w := lipgloss.Width(line)
		if w > panelW {
			t.Errorf("line %d width %d exceeds panel inner width %d: %q", i, w, panelW, line)
		}
	}
}
package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
)

// --- renderMail ---

func TestRenderMail_Empty(t *testing.T) {
	m := testModel(viewMail)
	out := m.renderMail()
	if out == "" {
		t.Error("renderMail with empty data returned empty string")
	}
}

func TestRenderMail_WithData(t *testing.T) {
	m := testModel(viewMail)
	m.data.messages = []*mail.Message{
		{From: "alice", To: "bob", Type: "task", Subject: "fix bug", Timestamp: time.Now()},
		{From: "bob", To: "alice", Type: "reply", Subject: "done", Timestamp: time.Now()},
	}
	out := m.renderMail()
	if !strings.Contains(out, "MAIL") {
		t.Error("renderMail missing MAIL title")
	}
}

// --- renderMemory ---

func TestRenderMemory_Empty(t *testing.T) {
	m := testModel(viewMemory)
	out := m.renderMemory()
	if out == "" {
		t.Error("renderMemory with empty data returned empty string")
	}
}

func TestRenderMemory_WithData(t *testing.T) {
	m := testModel(viewMemory)
	m.data.memories = []*memory.Entry{
		{ID: "M1", Type: "decision", Title: "Use Go"},
		{ID: "M2", Type: "discovery", Title: "Found bug"},
	}
	out := m.renderMemory()
	if !strings.Contains(out, "MEMORY") {
		t.Error("renderMemory missing MEMORY title")
	}
}

// --- renderWorktrees ---

func TestRenderWorktrees_Empty(t *testing.T) {
	m := testModel(viewWorktrees)
	out := m.renderWorktrees()
	if out == "" {
		t.Error("renderWorktrees with empty data returned empty string")
	}
}

func TestRenderWorktrees_WithData(t *testing.T) {
	m := testModel(viewWorktrees)
	m.data.worktrees = []*worktree.Worktree{
		{Name: "LOOM-1-1-fix", Branch: "fix-branch", Agent: "builder-1", Issue: "I1"},
		{Name: "LOOM-2-1-feat", Branch: "feat-branch", Agent: "builder-2", Issue: "I2"},
	}
	m.data.diffStats = map[string]*worktree.DiffStats{
		"LOOM-1-1-fix": {FilesChanged: 3, Insertions: 10, Deletions: 5},
	}
	out := m.renderWorktrees()
	if !strings.Contains(out, "WORKTREES") {
		t.Error("renderWorktrees missing WORKTREES title")
	}
}

// --- renderActivity ---

func TestRenderActivity_Empty(t *testing.T) {
	m := testModel(viewActivity)
	out := m.renderActivity()
	if out == "" {
		t.Error("renderActivity with empty data returned empty string")
	}
}

func TestRenderActivity_WithData(t *testing.T) {
	m := testModel(viewActivity)
	m.data.activity = []activityEntry{
		{AgentID: "builder-1", Line: "Called execute_bash: go build"},
		{AgentID: "builder-2", Line: "Called fs_read: main.go"},
	}
	out := m.renderActivity()
	if !strings.Contains(out, "ACTIVITY") {
		t.Error("renderActivity missing ACTIVITY title")
	}
}

// --- renderAgents column budget regression ---

// TestRenderAgents_NarrowTerminalNoColumnOverflow verifies that at 80-col
// width — a common terminal width where the old code overflowed by 7 chars —
// no rendered line exceeds m.width display columns. Overflow caused the
// rightmost columns (ISSUES, HEARTBEAT) to be silently clipped.
func TestRenderAgents_NarrowTerminalNoColumnOverflow(t *testing.T) {
	for _, width := range []int{60, 70, 80, 90, 100} {
		m := testModel(viewAgents)
		m.width = width
		m.data.agents = []*agent.Agent{
			{ID: "orchestrator", Role: "orchestrator", Status: "active"},
			{ID: "builder-001", Role: "builder", Status: "in-progress",
				AssignedIssues: []string{"LOOM-001"}, WorktreeName: "LOOM-001-1-fix-crash"},
			{ID: "reviewer-001", Role: "reviewer", Status: "review"},
		}
		m.data.agentTree = []agentTreeNode{{}, {depth: 1, isLast: false}, {depth: 1, isLast: true}}

		out := m.renderAgents()
		for _, line := range strings.Split(out, "\n") {
			w := lipgloss.Width(line)
			if w > width {
				t.Errorf("width=%d: rendered line is %d cols wide (overflow %d): %q",
					width, w, w-width, line)
			}
		}
	}
}

// --- renderAgentDetail UTF-8 byte-slice regression ---

// TestRenderAgentDetail_ToolSummaryUTF8 verifies that ToolSummary lines
// containing multi-byte Unicode are truncated without producing U+FFFD
// replacement characters. The old code used line[:maxW] (byte slice) which
// would corrupt codepoints at the truncation boundary.
func TestRenderAgentDetail_ToolSummaryUTF8(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write an output file with a ToolSummary line that is deliberately wide
	// (100 multi-byte runes) so truncation is triggered.
	content := strings.Repeat("日", 100) // 3-byte runes; old [:maxW] would cut mid-rune
	ev := struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}{"tool_summary", content}
	line, _ := json.Marshal(ev)
	outputPath := filepath.Join(agentsDir, "builder-001.output")
	if err := os.WriteFile(outputPath, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(dir)
	m.width = 80
	m.height = 40
	m.view = viewAgentDetail
	m.data.agents = []*agent.Agent{
		{ID: "builder-001", Role: "builder", Status: "active",
			Config: agent.AgentConfig{KiroMode: "acp"}},
	}
	m.data.agentTree = []agentTreeNode{{}}

	out := m.renderAgentDetail()
	for i, r := range out {
		if r == utf8.RuneError {
			// RuneError can appear legitimately at byte index 0 as U+FFFD U+0000,
			// but in practice any occurrence here means we corrupted a codepoint.
			raw := []byte(out)
			if i+3 <= len(raw) && raw[i] == 0xEF && raw[i+1] == 0xBF && raw[i+2] == 0xBD {
				t.Errorf("renderAgentDetail produced U+FFFD replacement char at byte %d — UTF-8 was corrupted", i)
				break
			}
		}
	}
	if strings.ContainsRune(out, '\uFFFD') {
		t.Error("renderAgentDetail produced U+FFFD replacement chars — UTF-8 was corrupted by byte-slicing")
	}
}

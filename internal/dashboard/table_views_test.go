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
	"github.com/karanagi/loom/internal/dashboard/backend"
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
	m.data.Messages = []*backend.Message{
		{From: "alice", To: "bob", Type: "task", Subject: "fix bug", Timestamp: time.Now()},
		{From: "bob", To: "alice", Type: "reply", Subject: "done", Timestamp: time.Now()},
	}
	out := m.renderMail()
	if !strings.Contains(out, "MAIL") {
		t.Error("renderMail missing MAIL title")
	}
}

func TestRenderMail_ShowsInboxPressureAndNextUp(t *testing.T) {
	m := testModel(viewMail)
	m.data.Unread = 3
	m.data.Messages = []*backend.Message{
		{From: "lead", To: "builder", Type: "blocker", Priority: "critical", Subject: "Auth blocked", Ref: "LOOM-001", Timestamp: time.Now(), Read: false},
		{From: "reviewer", To: "lead", Type: "question", Priority: "normal", Subject: "Need clarification", Ref: "LOOM-002", Timestamp: time.Now().Add(-1 * time.Minute), Read: false},
		{From: "builder", To: "lead", Type: "status", Priority: "low", Subject: "Working", Ref: "LOOM-003", Timestamp: time.Now().Add(-2 * time.Minute), Read: true},
	}
	out := m.renderMail()
	for _, expected := range []string{"INBOX PRESSURE", "NEXT UP", "3 unread", "critical", "LOOM-001"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("renderMail missing %q in output:\n%s", expected, out)
		}
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
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "M1", Type: "decision", Title: "Use Go"},
		{ID: "M2", Type: "discovery", Title: "Found bug"},
	}
	out := m.renderMemory()
	if !strings.Contains(out, "MEMORY") {
		t.Error("renderMemory missing MEMORY title")
	}
}

func TestRenderMemory_ShowsOperationalSummary(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "DEC-001", Type: "decision", Title: "Use JWT", Decision: "Use JWT cookies", Affects: []string{"LOOM-001"}},
		{ID: "DISC-001", Type: "discovery", Title: "Exporter location", Finding: "CSV export lives in internal/exporter", Affects: []string{"LOOM-002"}},
	}
	out := m.renderMemory()
	for _, expected := range []string{"MEMORY MAP", "RECENT DECISIONS", "DEC-001", "LOOM-001", "JWT cookies"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("renderMemory missing %q in output:\n%s", expected, out)
		}
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
	m.data.Worktrees = []*backend.Worktree{
		{Name: "LOOM-1-1-fix", Branch: "fix-branch", Agent: "builder-1", Issue: "I1"},
		{Name: "LOOM-2-1-feat", Branch: "feat-branch", Agent: "builder-2", Issue: "I2"},
	}
	m.data.DiffStats = map[string]*backend.DiffStats{
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
	m.data.Activity = []backend.ActivityEntry{
		{AgentID: "builder-1", Line: "Called execute_bash: go build"},
		{AgentID: "builder-2", Line: "Called fs_read: main.go"},
	}
	out := m.renderActivity()
	if !strings.Contains(out, "ACTIVITY") {
		t.Error("renderActivity missing ACTIVITY title")
	}
}

func TestRenderLogs_ShowsInvestigationSummary(t *testing.T) {
	m := testModel(viewLogs)
	m.data.Logs = []backend.LogLine{
		{Category: "error", Agent: "builder-001", Text: "build failed on auth middleware"},
		{Category: "warn", Agent: "reviewer-001", Text: "warning about stale review"},
		{Category: "lifecycle", Agent: "builder-001", Text: "activating agent builder-001"},
	}
	out := m.renderLogs()
	for _, expected := range []string{"INVESTIGATION", "1 errors", "HOT AGENTS", "builder-001", "build failed on auth middleware"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("renderLogs missing %q in output:\n%s", expected, out)
		}
	}
}

func TestRenderLogs_SearchFiltersMessages(t *testing.T) {
	m := testModel(viewLogs)
	m.searchTI.SetValue("auth")
	m.data.Logs = []backend.LogLine{
		{Category: "error", Agent: "builder-001", Text: "build failed on auth middleware"},
		{Category: "warn", Agent: "reviewer-001", Text: "billing warning"},
	}
	out := m.renderLogs()
	if !strings.Contains(out, "auth middleware") {
		t.Fatalf("expected filtered logs to contain auth line:\n%s", out)
	}
	if strings.Contains(out, "billing warning") {
		t.Fatalf("expected filtered logs to exclude billing warning:\n%s", out)
	}
}

func TestRenderOverview_ShowsAttentionSections(t *testing.T) {
	m := testModel(viewOverview)
	m.data.Issues = []*backend.Issue{
		{ID: "LOOM-001", Title: "Blocked auth work", Status: "blocked", UpdatedAt: time.Now()},
		{ID: "LOOM-002", Title: "Waiting for review", Status: "review", UpdatedAt: time.Now()},
		{ID: "LOOM-003", Title: "Active work", Status: "in-progress", UpdatedAt: time.Now()},
	}
	m.data.Agents = []*backend.Agent{
		{ID: "builder-001", Status: "dead", AssignedIssues: []string{"LOOM-001"}},
		{ID: "reviewer-001", Status: "active", AssignedIssues: []string{"LOOM-002"}},
	}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}}
	m.data.Messages = []*backend.Message{{From: "lead-001", To: "builder-001", Subject: "Need status", Timestamp: time.Now()}}
	m.data.Unread = 2
	m.data.Activity = []backend.ActivityEntry{{AgentID: "reviewer-001", Tool: "READ", Detail: "reviewing auth middleware", Time: "1m ago"}}

	out := m.renderOverview()
	for _, expected := range []string{"NEEDS ATTENTION", "IN FLIGHT", "LATEST SIGNAL", "blocked", "review", "2 unread"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("renderOverview missing %q in output:\n%s", expected, out)
		}
	}
}

func TestRenderIssueDetail_ShowsRelatedContext(t *testing.T) {
	m := testModel(viewIssueDetail)
	m.data.Issues = []*backend.Issue{{
		ID:          "LOOM-001",
		Title:       "Build auth system",
		Status:      "blocked",
		Priority:    "high",
		Description: "Implement authentication",
		UpdatedAt:   time.Now(),
	}}
	m.data.Memories = []*backend.MemoryEntry{{
		ID:        "DEC-001",
		Type:      "decision",
		Title:     "Use JWT tokens",
		Decision:  "Use JWT for auth",
		Affects:   []string{"LOOM-001"},
		Timestamp: time.Now(),
	}}
	m.data.Messages = []*backend.Message{{
		From:      "lead-001",
		To:        "builder-001",
		Subject:   "Blocker on auth",
		Ref:       "LOOM-001",
		Timestamp: time.Now(),
	}}
	m.data.Worktrees = []*backend.Worktree{{
		Name:   "LOOM-001-01-auth-ui",
		Branch: "LOOM-001-auth-ui",
		Issue:  "LOOM-001",
		Agent:  "builder-001",
	}}

	out := m.renderIssueDetail()
	for _, expected := range []string{"NEXT ACTION", "RELATED MEMORY", "RELATED MAIL", "WORKTREE", "DEC-001", "lead-001"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("renderIssueDetail missing %q in output:\n%s", expected, out)
		}
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
		m.data.Agents = []*backend.Agent{
			{ID: "orchestrator", Role: "orchestrator", Status: "active"},
			{ID: "builder-001", Role: "builder", Status: "in-progress",
				AssignedIssues: []string{"LOOM-001"}, WorktreeName: "LOOM-001-1-fix-crash"},
			{ID: "reviewer-001", Role: "reviewer", Status: "review"},
		}
		m.data.AgentTree = []backend.AgentTreeNode{{}, {Depth: 1, IsLast: false}, {Depth: 1, IsLast: true}}

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
	m.data.Agents = []*backend.Agent{
		{ID: "builder-001", Role: "builder", Status: "active",
			Config: backend.AgentConfig{KiroMode: "acp"}},
	}
	m.data.AgentTree = []backend.AgentTreeNode{{}}

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

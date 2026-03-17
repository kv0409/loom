package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/dashboard/backend"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
)

func testModel(v view) Model {
	m := New("/tmp/test-loom")
	m.width = 120
	m.height = 40
	m.view = v
	return m
}

func TestListLen_FilteredMailDetail(t *testing.T) {
	m := testModel(viewMailDetail)
	m.data.Messages = []*mail.Message{
		{From: "a", To: "b", Subject: "hello"},
		{From: "c", To: "d", Subject: "world"},
		{From: "e", To: "f", Subject: "test"},
	}
	m.searchTI.SetValue("hello")

	got := m.listLen()
	want := len(m.filteredMessages())
	if got != want {
		t.Errorf("listLen(viewMailDetail) with filter: got %d, want %d (filtered count)", got, want)
	}
}

func TestListLen_AllViews(t *testing.T) {
	m := testModel(viewAgents)
	m.data.Agents = []*agent.Agent{{ID: "a1"}, {ID: "a2"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}}
	if m.listLen() != 2 {
		t.Errorf("viewAgents: expected 2, got %d", m.listLen())
	}

	m.view = viewIssues
	m.data.Issues = []*issue.Issue{{ID: "I1", Status: "open"}}
	if m.listLen() != 1 {
		t.Errorf("viewIssues: expected 1, got %d", m.listLen())
	}

	m.view = viewMemory
	m.data.Memories = []*memory.Entry{{ID: "M1"}}
	if m.listLen() != 1 {
		t.Errorf("viewMemory: expected 1, got %d", m.listLen())
	}
}

func TestClampCursor_EmptyList(t *testing.T) {
	m := testModel(viewAgents)
	m.cursor = 5
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("expected cursor clamped to 0 for empty list, got %d", m.cursor)
	}
}

func TestClampCursor_WithData(t *testing.T) {
	m := testModel(viewAgents)
	m.data.Agents = []*agent.Agent{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}, {}}
	m.cursor = 10
	m.clampCursor()
	if m.cursor != 2 {
		t.Errorf("expected cursor clamped to 2, got %d", m.cursor)
	}
}

func TestSwitchView_SavesAndRestoresCursor(t *testing.T) {
	m := testModel(viewAgents)
	m.data.Agents = []*agent.Agent{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}, {}}
	m.cursor = 2

	m.switchView(viewIssues)
	if m.cursor != 0 {
		t.Errorf("expected cursor 0 for fresh view, got %d", m.cursor)
	}

	m.data.Issues = []*issue.Issue{{ID: "I1", Status: "open"}, {ID: "I2", Status: "open"}}
	m.cursor = 1

	m.switchView(viewAgents)
	if m.cursor != 2 {
		t.Errorf("expected restored cursor 2, got %d", m.cursor)
	}
}

func TestSwitchView_ClearsSearch(t *testing.T) {
	m := testModel(viewAgents)
	m.searchTI.SetValue("test")
	m.searchMode = true
	m.switchView(viewIssues)
	if m.searchTI.Value() != "" {
		t.Errorf("expected search cleared, got %q", m.searchTI.Value())
	}
	if m.searchMode {
		t.Error("expected searchMode cleared")
	}
}

func TestLogsView_EntersSearchMode(t *testing.T) {
	m := testModel(viewLogs)
	updated, _ := m.handleKey(keyMsg("/"))
	got := updated.(Model)
	if !got.searchMode {
		t.Fatal("expected logs view to enter search mode on /")
	}
}

func TestHelpBar_LogsShowsSearchAndFilters(t *testing.T) {
	m := testModel(viewLogs)
	bar := m.helpBar()
	for _, expected := range []string{"[/]search", "[f]ilter", "[F]agent"} {
		if !strings.Contains(bar, expected) {
			t.Fatalf("helpBar for logs missing %q: %s", expected, bar)
		}
	}
}

func TestFilteredIssues_SearchesDescriptionsAndDependencies(t *testing.T) {
	m := testModel(viewIssues)
	m.data.Issues = []*issue.Issue{
		{ID: "LOOM-001", Title: "Auth", Description: "JWT middleware for admin routes", Status: "open", DependsOn: []string{"LOOM-099"}},
		{ID: "LOOM-002", Title: "Billing", Description: "Invoice export", Status: "open"},
	}

	m.searchTI.SetValue("middleware")
	if got := m.filteredIssues(); len(got) != 1 || got[0].ID != "LOOM-001" {
		t.Fatalf("expected description search to return LOOM-001, got %+v", got)
	}

	m.searchTI.SetValue("LOOM-099")
	if got := m.filteredIssues(); len(got) != 1 || got[0].ID != "LOOM-001" {
		t.Fatalf("expected dependency search to return LOOM-001, got %+v", got)
	}
}

func TestFilteredMessages_SearchesBodyAndRef(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*mail.Message{
		{From: "lead", To: "builder", Subject: "Status", Body: "Please inspect auth middleware failure", Ref: "LOOM-001"},
		{From: "lead", To: "reviewer", Subject: "Review", Body: "Check billing output", Ref: "LOOM-002"},
	}

	m.searchTI.SetValue("middleware")
	if got := m.filteredMessages(); len(got) != 1 || got[0].Ref != "LOOM-001" {
		t.Fatalf("expected body search to return LOOM-001 mail, got %+v", got)
	}

	m.searchTI.SetValue("LOOM-002")
	if got := m.filteredMessages(); len(got) != 1 || got[0].Ref != "LOOM-002" {
		t.Fatalf("expected ref search to return LOOM-002 mail, got %+v", got)
	}
}

func TestFilteredMemories_SearchesBodyFields(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*memory.Entry{
		{ID: "DEC-001", Type: "decision", Title: "Auth tokens", Decision: "Use JWT cookies", Affects: []string{"LOOM-001"}},
		{ID: "DISC-001", Type: "discovery", Title: "Billing export", Finding: "CSV code lives in internal/exporter", Affects: []string{"LOOM-002"}},
	}

	m.searchTI.SetValue("JWT")
	if got := m.filteredMemories(); len(got) != 1 || got[0].ID != "DEC-001" {
		t.Fatalf("expected decision search to return DEC-001, got %+v", got)
	}

	m.searchTI.SetValue("LOOM-002")
	if got := m.filteredMemories(); len(got) != 1 || got[0].ID != "DISC-001" {
		t.Fatalf("expected affects search to return DISC-001, got %+v", got)
	}
}

func TestAdjustIssuesIndex_BeforeSeparator(t *testing.T) {
	m := testModel(viewIssues)
	m.data.Issues = []*issue.Issue{
		{ID: "I1", Status: "open"},
		{ID: "I2", Status: "open"},
		{ID: "I3", Status: "done"},
	}
	m.cursor = 0
	// Screen idx=0 with viewport starting at 0 → item 0.
	idx := m.adjustIssuesIndex(0)
	if idx != 0 {
		t.Errorf("expected 0, got %d", idx)
	}
	// Screen idx=1 → item 1.
	idx = m.adjustIssuesIndex(1)
	if idx != 1 {
		t.Errorf("expected 1, got %d", idx)
	}
}

func TestAdjustIssuesIndex_OnSeparator(t *testing.T) {
	m := testModel(viewIssues)
	m.data.Issues = []*issue.Issue{
		{ID: "I1", Status: "open"},
		{ID: "I2", Status: "open"},
		{ID: "I3", Status: "done"},
	}
	m.cursor = 0
	// activeCount=2, separator starts at screen row 2. Rows 2,3,4 are separator.
	for screenIdx := 2; screenIdx < 2+issuesSectionGap; screenIdx++ {
		idx := m.adjustIssuesIndex(screenIdx)
		if idx != -1 {
			t.Errorf("screen idx %d: expected -1 for separator row, got %d", screenIdx, idx)
		}
	}
}

func TestAdjustIssuesIndex_AfterSeparator(t *testing.T) {
	m := testModel(viewIssues)
	m.data.Issues = []*issue.Issue{
		{ID: "I1", Status: "open"},
		{ID: "I2", Status: "open"},
		{ID: "I3", Status: "done"},
	}
	m.cursor = 0
	// activeCount=2, separator at screen rows 2..4, first done at screen row 5 → item 2.
	idx := m.adjustIssuesIndex(2 + issuesSectionGap)
	if idx != 2 {
		t.Errorf("expected 2, got %d", idx)
	}
}

func TestIsListView(t *testing.T) {
	listViews := []view{viewAgents, viewIssues, viewMail, viewMemory, viewWorktrees, viewActivity}
	for _, v := range listViews {
		if !isListView(v) {
			t.Errorf("expected isListView(%d)=true", v)
		}
	}
	nonListViews := []view{viewOverview, viewAgentDetail, viewIssueDetail, viewMailDetail, viewDiff, viewLogs, viewKanban}
	for _, v := range nonListViews {
		if isListView(v) {
			t.Errorf("expected isListView(%d)=false", v)
		}
	}
}

func TestHelpBar_SingleLine(t *testing.T) {
	views := []view{
		viewOverview, viewAgents, viewAgentDetail, viewIssues, viewIssueDetail,
		viewMail, viewMailDetail, viewMemory, viewMemoryDetail, viewActivity,
		viewLogs, viewWorktrees, viewDiff, viewKanban,
	}
	for _, v := range views {
		m := testModel(v)
		bar := m.helpBar()
		if strings.Contains(bar, "\n") {
			t.Errorf("helpBar() for view %d contains newline", v)
		}
	}
}

func TestKeyMap_ShortHelp(t *testing.T) {
	km := defaultKeyMap()
	bindings := km.ShortHelp()
	if len(bindings) == 0 {
		t.Error("ShortHelp() returned empty slice")
	}
}

func TestTitleBarWidth(t *testing.T) {
	m := testModel(viewOverview)
	m.data.Agents = nil
	m.data.Unread = 0
	for _, w := range []int{80, 120, 200} {
		m.width = w
		// Extract title bar (first line of View output)
		output := m.View()
		firstLine := strings.SplitN(output, "\n", 2)[0]
		got := lipgloss.Width(firstLine)
		if got != w {
			t.Errorf("width=%d: title bar width=%d, want %d", w, got, w)
		}
	}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Alt: false}
}

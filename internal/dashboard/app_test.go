package dashboard

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

func testModel(v view) Model {
	m := New("/tmp/test-loom", 300)
	m.width = 120
	m.height = 40
	m.view = v
	return m
}

func TestRenderMail_DoesNotMutateSnapshotOrder(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*backend.Message{
		{From: "a", Priority: "low", Subject: "oldest", Timestamp: time.Now().Add(-3 * time.Hour), Read: true},
		{From: "b", Priority: "critical", Subject: "middle", Timestamp: time.Now().Add(-2 * time.Hour), Read: false},
		{From: "c", Priority: "normal", Subject: "newest", Timestamp: time.Now().Add(-1 * time.Hour), Read: false},
	}

	orig := make([]string, len(m.data.Messages))
	for i, msg := range m.data.Messages {
		orig[i] = msg.Subject
	}

	m.renderMail()

	for i, msg := range m.data.Messages {
		if msg.Subject != orig[i] {
			t.Fatalf("renderMail mutated m.data.Messages[%d]: got %q, want %q", i, msg.Subject, orig[i])
		}
	}
}

func TestListLen_FilteredMailDetail(t *testing.T) {
	m := testModel(viewMailDetail)
	m.data.Messages = []*backend.Message{
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
	m.data.Agents = []*backend.Agent{{ID: "a1"}, {ID: "a2"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}}
	if m.listLen() != 2 {
		t.Errorf("viewAgents: expected 2, got %d", m.listLen())
	}

	m.view = viewIssues
	m.data.Issues = []*backend.Issue{{ID: "I1", Status: "open"}}
	if m.listLen() != 1 {
		t.Errorf("viewIssues: expected 1, got %d", m.listLen())
	}

	m.view = viewMemory
	m.data.Memories = []*backend.MemoryEntry{{ID: "M1"}}
	if m.listLen() != 1 {
		t.Errorf("viewMemory: expected 1, got %d", m.listLen())
	}

	m.view = viewLogs
	m.data.Logs = []backend.LogLine{{Category: "error", Agent: "a", Text: "x"}, {Category: "warn", Agent: "b", Text: "y"}}
	if m.listLen() != 2 {
		t.Errorf("viewLogs: expected 2, got %d", m.listLen())
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
	m.data.Agents = []*backend.Agent{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}, {}}
	m.cursor = 10
	m.clampCursor()
	if m.cursor != 2 {
		t.Errorf("expected cursor clamped to 2, got %d", m.cursor)
	}
}

func TestSwitchView_SavesAndRestoresCursor(t *testing.T) {
	m := testModel(viewAgents)
	m.data.Agents = []*backend.Agent{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}, {}}
	m.cursor = 2

	m.switchView(viewIssues)
	if m.cursor != 0 {
		t.Errorf("expected cursor 0 for fresh view, got %d", m.cursor)
	}

	m.data.Issues = []*backend.Issue{{ID: "I1", Status: "open"}, {ID: "I2", Status: "open"}}
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
	m.data.Issues = []*backend.Issue{
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
	m.data.Messages = []*backend.Message{
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
	m.data.Memories = []*backend.MemoryEntry{
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
	m.data.Issues = []*backend.Issue{
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
	m.data.Issues = []*backend.Issue{
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
	m.data.Issues = []*backend.Issue{
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

func TestListLen_ViewLogs(t *testing.T) {
	m := testModel(viewLogs)
	m.data.Logs = []backend.LogLine{
		{Category: "error", Agent: "builder", Text: "fail"},
		{Category: "lifecycle", Agent: "planner", Text: "start"},
		{Category: "warn", Agent: "builder", Text: "slow"},
	}
	if got := m.listLen(); got != 3 {
		t.Errorf("viewLogs no filter: got %d, want 3", got)
	}
}

func TestListLen_ViewLogs_WithCategoryFilter(t *testing.T) {
	m := testModel(viewLogs)
	m.data.Logs = []backend.LogLine{
		{Category: "error", Agent: "builder", Text: "fail"},
		{Category: "lifecycle", Agent: "planner", Text: "start"},
		{Category: "error", Agent: "planner", Text: "crash"},
	}
	m.logFilter = 2 // "error"
	if got := m.listLen(); got != 2 {
		t.Errorf("viewLogs category=error: got %d, want 2", got)
	}
}

func TestListLen_ViewLogs_WithAgentFilter(t *testing.T) {
	m := testModel(viewLogs)
	m.data.Logs = []backend.LogLine{
		{Category: "error", Agent: "builder", Text: "fail"},
		{Category: "lifecycle", Agent: "planner", Text: "start"},
		{Category: "warn", Agent: "builder", Text: "slow"},
	}
	// agents sorted: ["builder", "planner"], index 1 = "builder"
	m.logAgentFilter = 1
	if got := m.listLen(); got != 2 {
		t.Errorf("viewLogs agent=builder: got %d, want 2", got)
	}
}

func TestListLen_ViewLogs_AllFilters(t *testing.T) {
	m := testModel(viewLogs)
	m.data.Logs = []backend.LogLine{
		{Category: "error", Agent: "builder", Text: "fail"},
		{Category: "error", Agent: "planner", Text: "crash"},
		{Category: "lifecycle", Agent: "builder", Text: "start"},
		{Category: "warn", Agent: "builder", Text: "slow query"},
	}
	m.logFilter = 2      // "error"
	m.logAgentFilter = 1 // "builder" (sorted: builder, planner)
	m.searchTI.SetValue("fail")
	if got := m.listLen(); got != 1 {
		t.Errorf("viewLogs all filters: got %d, want 1", got)
	}
}

func TestHandleEnter_WorktreeReturnsCmd(t *testing.T) {
	m := testModel(viewWorktrees)
	m.data.Worktrees = []*backend.Worktree{{Name: "wt-1", Path: "/tmp/wt-1", Branch: "main"}}
	m.cursor = 0

	result, cmd := m.handleEnter()
	got := result.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil cmd for async diff fetch")
	}
	if !got.diffLoading {
		t.Error("expected diffLoading=true")
	}
	if got.diffContent != "" {
		t.Error("expected diffContent to be empty while loading")
	}
	if got.view != viewDiff {
		t.Errorf("expected viewDiff, got %d", got.view)
	}
}

func TestHandleEnter_FilteredWorktreeStoresName(t *testing.T) {
	m := testModel(viewWorktrees)
	m.data.Worktrees = []*backend.Worktree{
		{Name: "wt-alpha", Path: "/tmp/wt-alpha", Branch: "alpha"},
		{Name: "wt-beta", Path: "/tmp/wt-beta", Branch: "beta"},
		{Name: "wt-gamma", Path: "/tmp/wt-gamma", Branch: "gamma"},
	}
	// Filter to show only "beta"
	m.searchTI.SetValue("beta")
	m.cursor = 0

	result, _ := m.handleEnter()
	got := result.(Model)

	if got.selectedWorktreeName != "wt-beta" {
		t.Errorf("expected selectedWorktreeName='wt-beta', got %q", got.selectedWorktreeName)
	}
}

func TestRenderDiff_FilteredWorktreeShowsCorrectTitle(t *testing.T) {
	m := testModel(viewDiff)
	m.data.Worktrees = []*backend.Worktree{
		{Name: "wt-alpha", Path: "/tmp/wt-alpha"},
		{Name: "wt-beta", Path: "/tmp/wt-beta"},
	}
	m.selectedWorktreeName = "wt-beta"
	m.diffContent = "+added line"

	output := m.renderDiff()
	if !strings.Contains(output, "wt-beta") {
		t.Error("expected diff title to contain 'wt-beta'")
	}
	if strings.Contains(output, "wt-alpha") {
		t.Error("diff title should not contain 'wt-alpha'")
	}
}

func TestEscFromDiff_RestoresCursorByName(t *testing.T) {
	m := testModel(viewWorktrees)
	m.data.Worktrees = []*backend.Worktree{
		{Name: "wt-alpha", Path: "/tmp/wt-alpha"},
		{Name: "wt-beta", Path: "/tmp/wt-beta"},
		{Name: "wt-gamma", Path: "/tmp/wt-gamma"},
	}
	// Simulate entering diff from filtered list where beta was cursor=0
	m.searchTI.SetValue("beta")
	m.cursor = 0
	result, _ := m.handleEnter()
	m = result.(Model)

	// Now press Esc — search is cleared by switchView, so cursor should find beta at index 1
	m.searchTI.SetValue("") // search cleared on view switch
	result, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	got := result.(Model)

	if got.view != viewWorktrees {
		t.Fatalf("expected viewWorktrees, got %d", got.view)
	}
	if got.cursor != 1 {
		t.Errorf("expected cursor=1 (wt-beta in unfiltered list), got %d", got.cursor)
	}
}

func TestDiffResultMsg_SetsDiffContent(t *testing.T) {
	m := testModel(viewDiff)
	m.diffLoading = true

	result, _ := m.Update(diffResultMsg{content: "diff --git a/foo"})
	got := result.(Model)

	if got.diffContent != "diff --git a/foo" {
		t.Errorf("expected diffContent set, got %q", got.diffContent)
	}
	if got.diffLoading {
		t.Error("expected diffLoading=false after receiving result")
	}
}

func TestComposeSend_ValidData_ReturnsCmd(t *testing.T) {
	m := testModel(viewMail)
	m.composeMode = true
	m.composeData = &composeData{To: "builder-001", Subject: "test", Body: "hello"}

	result, cmd := m.composeSend()
	got := result.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil cmd for async send")
	}
	if got.composeMode {
		t.Error("expected composeMode=false after send")
	}
}

func TestAgentOutputMsg_UpdatesCache(t *testing.T) {
	m := testModel(viewAgentDetail)
	m.agentOutputID = "builder-001"
	events := []backend.ACPEvent{{Kind: backend.TokenChunk, Content: "hello"}}

	result, _ := m.Update(agentOutputMsg{agentID: "builder-001", events: events})
	got := result.(Model)

	if len(got.agentOutputCache) != 1 {
		t.Fatalf("expected 1 cached event, got %d", len(got.agentOutputCache))
	}
	if got.agentOutputCache[0].Content != "hello" {
		t.Errorf("expected cached content 'hello', got %q", got.agentOutputCache[0].Content)
	}
}

func TestAgentOutputMsg_IgnoredWhenWrongView(t *testing.T) {
	m := testModel(viewAgents) // not viewAgentDetail
	m.agentOutputID = "builder-001"

	result, _ := m.Update(agentOutputMsg{agentID: "builder-001", events: []backend.ACPEvent{{Content: "x"}}})
	got := result.(Model)

	if got.agentOutputCache != nil {
		t.Error("expected cache to remain nil when not on agent detail view")
	}
}

func TestHandleEnter_AgentDetail_FiresOutputCmd(t *testing.T) {
	m := testModel(viewAgents)
	m.data.Agents = []*backend.Agent{
		{ID: "builder-001", Config: backend.AgentConfig{KiroMode: "acp"}},
	}
	m.data.AgentTree = []backend.AgentTreeNode{{}}
	m.cursor = 0

	result, cmd := m.handleEnter()
	got := result.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil cmd for async agent output fetch")
	}
	if got.agentOutputID != "builder-001" {
		t.Errorf("expected agentOutputID='builder-001', got %q", got.agentOutputID)
	}
	if got.view != viewAgentDetail {
		t.Errorf("expected viewAgentDetail, got %d", got.view)
	}
}

func TestDaemonResultMsg_SetsFlash(t *testing.T) {
	m := testModel(viewAgents)

	result, cmd := m.Update(daemonResultMsg{flash: "Nudged builder", isErr: false})
	got := result.(Model)

	if got.flashMsg != "Nudged builder" {
		t.Errorf("expected flash 'Nudged builder', got %q", got.flashMsg)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd (clearFlashAfter)")
	}
}

func TestSendMailResultMsg_SetsFlash(t *testing.T) {
	m := testModel(viewMail)

	result, cmd := m.Update(sendMailResultMsg{flash: "Sent to builder", isErr: false})
	got := result.(Model)

	if got.flashMsg != "Sent to builder" {
		t.Errorf("expected flash 'Sent to builder', got %q", got.flashMsg)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd (clearFlashAfter)")
	}
}

func TestClampCursor_EnsuresNonNegativeScroll(t *testing.T) {
	m := testModel(viewAgentDetail)
	m.detailScroll = -5
	m.diffScroll = -3
	m.clampCursor()
	if m.detailScroll != 0 {
		t.Errorf("expected detailScroll clamped to 0, got %d", m.detailScroll)
	}
	if m.diffScroll != 0 {
		t.Errorf("expected diffScroll clamped to 0, got %d", m.diffScroll)
	}
}

func TestHandleEnter_MemoryDetail_ResetsScroll(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "DEC-001", Type: "decision", Title: "First"},
		{ID: "DEC-002", Type: "decision", Title: "Second"},
	}
	m.cursor = 0

	// Open first memory and simulate scrolling down.
	result, _ := m.handleEnter()
	got := result.(Model)
	if got.view != viewMemoryDetail {
		t.Fatalf("expected viewMemoryDetail, got %d", got.view)
	}
	if got.detailScroll != 0 {
		t.Fatalf("expected detailScroll=0 on first open, got %d", got.detailScroll)
	}
	got.detailScroll = 15 // simulate scrolling

	// Go back to memory list and open second entry.
	got.switchView(viewMemory)
	got.cursor = 1
	result2, _ := got.handleEnter()
	got2 := result2.(Model)
	if got2.view != viewMemoryDetail {
		t.Fatalf("expected viewMemoryDetail, got %d", got2.view)
	}
	if got2.detailScroll != 0 {
		t.Errorf("expected detailScroll reset to 0 on new memory entry, got %d", got2.detailScroll)
	}
}

func TestSnapshotRefresh_ResetsScrollForInactiveViews(t *testing.T) {
	tests := []struct {
		name             string
		view             view
		wantDetailReset  bool
		wantDiffReset    bool
	}{
		{"agents list resets both", viewAgents, true, true},
		{"overview resets both", viewOverview, true, true},
		{"agent detail keeps detailScroll", viewAgentDetail, false, true},
		{"issue detail keeps detailScroll", viewIssueDetail, false, true},
		{"memory detail keeps detailScroll", viewMemoryDetail, false, true},
		{"mail detail keeps detailScroll", viewMailDetail, false, true},
		{"diff view keeps diffScroll", viewDiff, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(tt.view)
			m.detailScroll = 50
			m.diffScroll = 50
			m.data.Agents = []*backend.Agent{{ID: "a1"}}
			m.data.AgentTree = []backend.AgentTreeNode{{}}

			result, _ := m.Update(backend.Snapshot{
				Agents:    []*backend.Agent{{ID: "a1"}},
				AgentTree: []backend.AgentTreeNode{{}},
			})
			got := result.(Model)

			if tt.wantDetailReset && got.detailScroll != 0 {
				t.Errorf("expected detailScroll reset to 0, got %d", got.detailScroll)
			}
			if !tt.wantDetailReset && got.detailScroll != 50 {
				t.Errorf("expected detailScroll preserved at 50, got %d", got.detailScroll)
			}
			if tt.wantDiffReset && got.diffScroll != 0 {
				t.Errorf("expected diffScroll reset to 0, got %d", got.diffScroll)
			}
			if !tt.wantDiffReset && got.diffScroll != 50 {
				t.Errorf("expected diffScroll preserved at 50, got %d", got.diffScroll)
			}
		})
	}
}

func TestSortedMessages_MatchesDisplayOrder(t *testing.T) {
	m := testModel(viewMail)
	now := time.Now()
	m.data.Messages = []*backend.Message{
		{From: "a", Priority: "low", Subject: "low-read", Timestamp: now.Add(-3 * time.Hour), Read: true},
		{From: "b", Priority: "critical", Subject: "critical-unread", Timestamp: now.Add(-2 * time.Hour), Read: false},
		{From: "c", Priority: "normal", Subject: "normal-unread", Timestamp: now.Add(-1 * time.Hour), Read: false},
	}

	sorted := m.sortedMessages()

	// Critical unread should be first.
	if sorted[0].Subject != "critical-unread" {
		t.Fatalf("expected critical-unread first, got %q", sorted[0].Subject)
	}
	// Normal unread second.
	if sorted[1].Subject != "normal-unread" {
		t.Fatalf("expected normal-unread second, got %q", sorted[1].Subject)
	}
	// Low read last.
	if sorted[2].Subject != "low-read" {
		t.Fatalf("expected low-read last, got %q", sorted[2].Subject)
	}
}

func TestHandleEnter_MailSelectsSortedMessage(t *testing.T) {
	m := testModel(viewMail)
	now := time.Now()
	// Snapshot order: low first, critical second.
	m.data.Messages = []*backend.Message{
		{From: "a", Priority: "low", Subject: "low-msg", Timestamp: now.Add(-2 * time.Hour), Read: true},
		{From: "b", Priority: "critical", Subject: "critical-msg", Timestamp: now.Add(-1 * time.Hour), Read: false},
	}
	m.cursor = 0 // First row in sorted order should be critical.

	result, _ := m.handleEnter()
	got := result.(Model)

	if got.view != viewMailDetail {
		t.Fatalf("expected viewMailDetail, got %d", got.view)
	}

	// Verify the detail view shows the critical message (sorted[0]).
	sorted := got.sortedMessages()
	if got.cursor >= len(sorted) || sorted[got.cursor].Subject != "critical-msg" {
		t.Fatalf("expected detail to show critical-msg at cursor %d, got %q", got.cursor, sorted[got.cursor].Subject)
	}
}

func TestComposeReply_TargetsSortedSender(t *testing.T) {
	m := testModel(viewMailDetail)
	now := time.Now()
	m.data.Messages = []*backend.Message{
		{From: "low-sender", Priority: "low", Subject: "low", Timestamp: now.Add(-2 * time.Hour), Read: true},
		{From: "critical-sender", Priority: "critical", Subject: "critical", Timestamp: now.Add(-1 * time.Hour), Read: false},
	}
	m.data.Agents = []*backend.Agent{{ID: "critical-sender"}, {ID: "low-sender"}}
	m.cursor = 0 // Sorted order: critical first.

	result, _ := m.handleKey(keyMsg("r"))
	got := result.(Model)

	if !got.composeMode {
		t.Fatal("expected composeMode=true after reply")
	}
	if got.composeData.To != "critical-sender" {
		t.Fatalf("expected reply to critical-sender, got %q", got.composeData.To)
	}
}
func TestHandleEnter_ActivityToAgentDetail_InitializesOutputState(t *testing.T) {
	m := testModel(viewActivity)
	m.data.Agents = []*backend.Agent{
		{ID: "builder-001"},
		{ID: "builder-002"},
	}
	m.data.AgentTree = []backend.AgentTreeNode{{}, {}}
	m.data.Activity = []backend.ActivityEntry{
		{AgentID: "builder-002", Line: "Called execute_bash"},
	}
	// Simulate stale cache from a previously viewed agent.
	m.agentOutputCache = []backend.ACPEvent{{Kind: backend.TokenChunk, Content: "stale"}}
	m.agentOutputID = "builder-001"
	m.cursor = 0

	result, cmd := m.handleEnter()
	got := result.(Model)

	if got.view != viewAgentDetail {
		t.Fatalf("expected viewAgentDetail, got %d", got.view)
	}
	if got.agentOutputID != "builder-002" {
		t.Errorf("expected agentOutputID='builder-002', got %q", got.agentOutputID)
	}
	if got.agentOutputCache != nil {
		t.Error("expected agentOutputCache cleared to nil")
	}
	if got.detailScroll != 0 {
		t.Errorf("expected detailScroll=0, got %d", got.detailScroll)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for immediate agent output fetch")
	}
}

// --- Regression tests: Mail cursor bounds and Enter behavior ---

func TestMailCursorBounds_ClampsToListLen(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*backend.Message{
		{From: "a", To: "b", Subject: "one", Priority: "normal"},
		{From: "c", To: "d", Subject: "two", Priority: "normal"},
	}
	m.cursor = 10
	m.clampCursor()
	if m.cursor != 1 {
		t.Errorf("expected cursor clamped to 1, got %d", m.cursor)
	}
}

func TestMailCursorBounds_EmptyList(t *testing.T) {
	m := testModel(viewMail)
	m.cursor = 5
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("expected cursor clamped to 0 for empty mail, got %d", m.cursor)
	}
}

func TestMailEnter_OpensDetailAtCorrectSortedIndex(t *testing.T) {
	m := testModel(viewMail)
	now := time.Now()
	m.data.Messages = []*backend.Message{
		{From: "a", Priority: "low", Subject: "low-msg", Timestamp: now.Add(-2 * time.Hour), Read: true},
		{From: "b", Priority: "critical", Subject: "critical-msg", Timestamp: now.Add(-1 * time.Hour), Read: false},
		{From: "c", Priority: "normal", Subject: "normal-msg", Timestamp: now.Add(-30 * time.Minute), Read: false},
	}
	// cursor=1 should be the second item in sorted order (normal-msg)
	m.cursor = 1
	result, _ := m.handleEnter()
	got := result.(Model)
	if got.view != viewMailDetail {
		t.Fatalf("expected viewMailDetail, got %d", got.view)
	}
	sorted := got.sortedMessages()
	if got.cursor >= len(sorted) || sorted[got.cursor].Subject != "normal-msg" {
		t.Errorf("expected detail for normal-msg at cursor %d, got %q", got.cursor, sorted[got.cursor].Subject)
	}
}

func TestMailEnter_OutOfBounds_NoTransition(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*backend.Message{
		{From: "a", To: "b", Subject: "only", Priority: "normal"},
	}
	m.cursor = 5 // out of bounds
	result, _ := m.handleEnter()
	got := result.(Model)
	if got.view != viewMail {
		t.Errorf("expected to stay on viewMail when cursor out of bounds, got %d", got.view)
	}
}

func TestMailCursorBounds_DownKeyStopsAtEnd(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*backend.Message{
		{From: "a", To: "b", Subject: "one", Priority: "normal"},
		{From: "c", To: "d", Subject: "two", Priority: "normal"},
	}
	m.cursor = 1
	// Press down — should clamp to 1 (last item)
	result, _ := m.handleKey(keyMsg("j"))
	got := result.(Model)
	if got.cursor != 1 {
		t.Errorf("expected cursor to stay at 1, got %d", got.cursor)
	}
}

func TestMailMouseClick_SelectsCorrectItem(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*backend.Message{
		{From: "a", To: "b", Subject: "first", Priority: "critical"},
		{From: "c", To: "d", Subject: "second", Priority: "normal"},
		{From: "e", To: "f", Subject: "third", Priority: "low"},
	}
	// Click on the second row (listHeaderRows + 1)
	result, _ := m.handleMouse(tea.MouseMsg{X: 5, Y: listHeaderRows + 1, Button: tea.MouseButtonLeft})
	got := result.(Model)
	if got.cursor != 1 {
		t.Errorf("expected cursor=1 after clicking second row, got %d", got.cursor)
	}
}

// --- Regression tests: Memory cursor bounds and Enter behavior ---

func TestMemoryCursorBounds_ClampsToListLen(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "M1", Type: "decision", Title: "First"},
		{ID: "M2", Type: "discovery", Title: "Second"},
		{ID: "M3", Type: "convention", Title: "Third"},
	}
	m.cursor = 10
	m.clampCursor()
	if m.cursor != 2 {
		t.Errorf("expected cursor clamped to 2, got %d", m.cursor)
	}
}

func TestMemoryCursorBounds_EmptyList(t *testing.T) {
	m := testModel(viewMemory)
	m.cursor = 5
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("expected cursor clamped to 0 for empty memory, got %d", m.cursor)
	}
}

func TestMemoryEnter_OpensDetailAtCorrectIndex(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "DEC-001", Type: "decision", Title: "First"},
		{ID: "DISC-001", Type: "discovery", Title: "Second"},
		{ID: "CONV-001", Type: "convention", Title: "Third"},
	}
	m.cursor = 2
	result, _ := m.handleEnter()
	got := result.(Model)
	if got.view != viewMemoryDetail {
		t.Fatalf("expected viewMemoryDetail, got %d", got.view)
	}
	memories := got.filteredMemories()
	if got.cursor >= len(memories) || memories[got.cursor].ID != "CONV-001" {
		t.Errorf("expected detail for CONV-001 at cursor %d", got.cursor)
	}
}

func TestMemoryEnter_OutOfBounds_NoTransition(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "M1", Type: "decision", Title: "Only"},
	}
	m.cursor = 5 // out of bounds
	result, _ := m.handleEnter()
	got := result.(Model)
	if got.view != viewMemory {
		t.Errorf("expected to stay on viewMemory when cursor out of bounds, got %d", got.view)
	}
}

func TestMemoryCursorBounds_DownKeyStopsAtEnd(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "M1", Type: "decision", Title: "First"},
		{ID: "M2", Type: "discovery", Title: "Second"},
	}
	m.cursor = 1
	result, _ := m.handleKey(keyMsg("j"))
	got := result.(Model)
	if got.cursor != 1 {
		t.Errorf("expected cursor to stay at 1, got %d", got.cursor)
	}
}

func TestMemoryMouseClick_SelectsCorrectItem(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "M1", Type: "decision", Title: "First"},
		{ID: "M2", Type: "discovery", Title: "Second"},
		{ID: "M3", Type: "convention", Title: "Third"},
	}
	// Click on the third row (listHeaderRows + 2)
	result, _ := m.handleMouse(tea.MouseMsg{X: 5, Y: listHeaderRows + 2, Button: tea.MouseButtonLeft})
	got := result.(Model)
	if got.cursor != 2 {
		t.Errorf("expected cursor=2 after clicking third row, got %d", got.cursor)
	}
}

func TestMailListLen_MatchesSortedMessages(t *testing.T) {
	m := testModel(viewMail)
	m.data.Messages = []*backend.Message{
		{From: "a", To: "b", Subject: "one", Priority: "normal"},
		{From: "c", To: "d", Subject: "two", Priority: "critical"},
		{From: "e", To: "f", Subject: "three", Priority: "low"},
	}
	if m.listLen() != len(m.sortedMessages()) {
		t.Errorf("listLen()=%d != len(sortedMessages())=%d", m.listLen(), len(m.sortedMessages()))
	}
}

func TestMemoryListLen_MatchesFilteredMemories(t *testing.T) {
	m := testModel(viewMemory)
	m.data.Memories = []*backend.MemoryEntry{
		{ID: "M1", Type: "decision", Title: "First"},
		{ID: "M2", Type: "discovery", Title: "Second"},
	}
	if m.listLen() != len(m.filteredMemories()) {
		t.Errorf("listLen()=%d != len(filteredMemories())=%d", m.listLen(), len(m.filteredMemories()))
	}
}

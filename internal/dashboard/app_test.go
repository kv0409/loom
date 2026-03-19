package dashboard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

func testModel(v view) Model {
	m := New("/tmp/test-loom", 300)
	m.width = 120
	m.height = 40
	m.view = v
	m.detailVP.SetWidth(panelWidth(m.width) - 2)
	m.detailVP.SetHeight(scrollViewport(m.height))
	m.diffVP.SetWidth(panelWidth(m.width) - 2)
	m.diffVP.SetHeight(scrollViewport(m.height))
	return m
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
	listViews := []view{viewAgents, viewIssues, viewMemory, viewWorktrees, viewActivity}
	for _, v := range listViews {
		if !isListView(v) {
			t.Errorf("expected isListView(%d)=true", v)
		}
	}
	nonListViews := []view{viewOverview, viewAgentDetail, viewIssueDetail, viewDiff}
	for _, v := range nonListViews {
		if isListView(v) {
			t.Errorf("expected isListView(%d)=false", v)
		}
	}
}

func TestHelpBar_SingleLine(t *testing.T) {
	views := []view{
		viewOverview, viewAgents, viewAgentDetail, viewIssues, viewIssueDetail,
		viewMemory, viewMemoryDetail, viewActivity,
		viewWorktrees, viewDiff,
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
		firstLine := strings.SplitN(output.Content, "\n", 2)[0]
		got := lipgloss.Width(firstLine)
		if got != w {
			t.Errorf("width=%d: title bar width=%d, want %d", w, got, w)
		}
	}
}

func keyMsg(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: s}
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
	result, _ = m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
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
	m := testModel(viewAgents)

	result, cmd := m.Update(sendMailResultMsg{flash: "Sent to builder", isErr: false})
	got := result.(Model)

	if got.flashMsg != "Sent to builder" {
		t.Errorf("expected flash 'Sent to builder', got %q", got.flashMsg)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd (clearFlashAfter)")
	}
}

func TestClampCursor_EnsuresNonNegativeCursor(t *testing.T) {
	m := testModel(viewAgentDetail)
	m.data.Agents = []*backend.Agent{{ID: "a1"}}
	m.data.AgentTree = []backend.AgentTreeNode{{}}
	m.cursor = 5
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("expected cursor clamped to 0, got %d", m.cursor)
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
	if got.detailYOff != 0 {
		t.Fatalf("expected detailYOff=0 on first open, got %d", got.detailYOff)
	}
	got.detailYOff = 15 // simulate scrolling

	// Go back to memory list and open second entry.
	got.switchView(viewMemory)
	got.cursor = 1
	result2, _ := got.handleEnter()
	got2 := result2.(Model)
	if got2.view != viewMemoryDetail {
		t.Fatalf("expected viewMemoryDetail, got %d", got2.view)
	}
	if got2.detailYOff != 0 {
		t.Errorf("expected detailYOff reset to 0 on new memory entry, got %d", got2.detailYOff)
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
		{"diff view keeps diffScroll", viewDiff, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(tt.view)
			m.detailYOff = 50
			m.diffYOff = 50
			m.data.Agents = []*backend.Agent{{ID: "a1"}}
			m.data.AgentTree = []backend.AgentTreeNode{{}}

			result, _ := m.Update(backend.Snapshot{
				Agents:    []*backend.Agent{{ID: "a1"}},
				AgentTree: []backend.AgentTreeNode{{}},
			})
			got := result.(Model)

			if tt.wantDetailReset && got.detailYOff != 0 {
				t.Errorf("expected detailYOff reset to 0, got %d", got.detailYOff)
			}
			if !tt.wantDetailReset && got.detailYOff != 50 {
				t.Errorf("expected detailYOff preserved at 50, got %d", got.detailYOff)
			}
			if tt.wantDiffReset && got.diffYOff != 0 {
				t.Errorf("expected diffYOff reset to 0, got %d", got.diffYOff)
			}
			if !tt.wantDiffReset && got.diffYOff != 50 {
				t.Errorf("expected diffYOff preserved at 50, got %d", got.diffYOff)
			}
		})
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
	if got.detailYOff != 0 {
		t.Errorf("expected detailYOff=0, got %d", got.detailYOff)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for immediate agent output fetch")
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
	result, _ := m.handleKey(keyMsg("down"))
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
	result, _ := m.handleMouseClick(tea.MouseClickMsg{X: 5, Y: listHeaderRows + 2, Button: tea.MouseLeft})
	got := result.(Model)
	if got.cursor != 2 {
		t.Errorf("expected cursor=2 after clicking third row, got %d", got.cursor)
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

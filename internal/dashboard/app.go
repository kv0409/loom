package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/nudge"
	"github.com/karanagi/loom/internal/worktree"
)

type view int

const (
	viewOverview view = iota
	viewAgents
	viewAgentDetail
	viewIssues
	viewIssueDetail
	viewMail
	viewMailDetail
	viewMemory
	viewMemoryDetail
	viewActivity
	viewLogs
	viewWorktrees
	viewDiff
	viewKanban
)

var viewOrder = []view{viewOverview, viewAgents, viewIssues, viewMail, viewMemory, viewActivity, viewLogs, viewWorktrees, viewKanban}

const (
	// listHeaderRows is the number of fixed rows above list items in the screen layout:
	// row 0 = dashboard title, row 1 = panel border, row 2 = column header, row 3 = separator, row 4 = blank line.
	listHeaderRows = 5
	// issuesSectionGap is the number of extra lines inserted between active and done
	// sections in the issues view (blank + "RECENTLY DONE" + separator).
	issuesSectionGap = 3

	minTermWidth  = 60
	minTermHeight = 15
)

type agentTreeNode struct {
	depth  int
	isLast bool
	// ancestors[i] is true if the ancestor at depth i is the last child of its parent
	ancestors []bool
}

type data struct {
	agents    []*agent.Agent
	agentTree []agentTreeNode
	issues    []*issue.Issue
	worktrees []*worktree.Worktree
	diffStats map[string]*worktree.DiffStats
	messages  []*mail.Message
	memories  []*memory.Entry
	unread    int
	activity  []activityEntry
	logs      []logLine
}

type Model struct {
	loomRoot         string
	view             view
	data             data
	cursor           int
	cursors          map[view]int // per-view cursor positions
	width            int
	height           int
	logFilter        int // 0=all, 1=lifecycle, 2=error, 3=stderr, 4=warn
	logAgentFilter   int // 0=all, 1..N = specific agent
	nudgeMode        bool
	nudgeCursor      int
	messageMode      bool
	messageInput     string
	killConfirm      bool
	selectedWorktree int
	diffContent      string
	kanbanCol        int // selected column in kanban view
	kanbanRow        int // selected row within column
	lr               *logReader
	lastClickTime    time.Time
	lastClickRow     int
	detailScroll     int // scroll offset for agent detail output
	diffScroll       int // scroll offset for diff view
	flashMsg         string
	flashIsErr       bool
	searchMode       bool
	searchQuery      string
	inputCursor      int // cursor position within the active input field
}

type tickMsg time.Time
type clearFlashMsg struct{}

func clearFlashAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearFlashMsg{} })
}

func New(loomRoot string) Model {
	return Model{loomRoot: loomRoot, width: 80, height: 24, lr: newLogReader(loomRoot), cursors: make(map[view]int)}
}

// switchView saves the current cursor position and switches to the target view,
// restoring its previously saved cursor position.
func (m *Model) switchView(target view) {
	m.cursors[m.view] = m.cursor
	m.view = target
	m.cursor = m.cursors[target]
	m.searchQuery = ""
	m.searchMode = false
}

func (m *Model) setFlash(msg string, isErr bool) tea.Cmd {
	m.flashMsg = msg
	m.flashIsErr = isErr
	return clearFlashAfter(3 * time.Second)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd())
}

// ProgramOptions returns the tea.ProgramOption set needed by the dashboard,
// including alt-screen and mouse support.
func ProgramOptions() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refresh() tea.Cmd {
	root := m.loomRoot
	lr := m.lr
	return func() tea.Msg {
		var d data
		d.agents, _ = agent.List(root)
		d.issues, _ = issue.List(root, issue.ListOpts{All: true})
		d.worktrees, _ = worktree.List(root)
		d.diffStats = make(map[string]*worktree.DiffStats)
		for _, wt := range d.worktrees {
			if ds, err := worktree.DiffStatsFor(wt.Path); err == nil {
				d.diffStats[wt.Name] = ds
			}
		}
		d.messages, _ = mail.Log(root, mail.LogOpts{})
		d.memories, _ = memory.List(root, memory.ListOpts{})
		d.unread = countUnread(root)
		d.agents, d.agentTree = sortAgentTree(d.agents)
		d.activity = fetchActivity(root, d.agents)
		d.logs = lr.read()
		return d
	}
}

func countUnread(loomRoot string) int {
	var count int
	inboxRoot := filepath.Join(loomRoot, "mail", "inbox")
	entries, err := os.ReadDir(inboxRoot)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		msgs, err := mail.Read(loomRoot, e.Name(), true)
		if err == nil {
			count += len(msgs)
		}
	}
	return count
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refresh(), tickCmd())
	case data:
		m.data = msg
		m.clampCursor()
		return m, nil
	case clearFlashMsg:
		m.flashMsg = ""
		return m, nil
	}
	return m, nil
}

// editInput handles key events for a text input field with cursor movement and paste support.
// Returns the updated string, cursor position, and whether the key was consumed.
func editInput(input string, cursor int, key tea.KeyMsg) (string, int, bool) {
	k := tea.Key(key)
	runes := []rune(input)
	if cursor > len(runes) {
		cursor = len(runes)
	}
	switch k.Type {
	case tea.KeyBackspace:
		if cursor > 0 {
			runes = append(runes[:cursor-1], runes[cursor:]...)
			cursor--
		}
		return string(runes), cursor, true
	case tea.KeyDelete:
		if cursor < len(runes) {
			runes = append(runes[:cursor], runes[cursor+1:]...)
		}
		return string(runes), cursor, true
	case tea.KeyLeft:
		if cursor > 0 {
			cursor--
		}
		return string(runes), cursor, true
	case tea.KeyRight:
		if cursor < len(runes) {
			cursor++
		}
		return string(runes), cursor, true
	case tea.KeyHome:
		return string(runes), 0, true
	case tea.KeyEnd:
		return string(runes), len(runes), true
	case tea.KeyRunes, tea.KeySpace:
		before := runes[:cursor]
		after := append([]rune{}, runes[cursor:]...)
		runes = append(append(before, k.Runes...), after...)
		cursor += len(k.Runes)
		return string(runes), cursor, true
	}
	return input, cursor, false
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Nudge mode: selection menu
	if m.nudgeMode {
		switch msg.String() {
		case "enter":
			agents := m.filteredAgents()
			if m.cursor < len(agents) && m.nudgeCursor < len(nudge.Types) {
				a := agents[m.cursor]
				nt := nudge.Types[m.nudgeCursor]
				err := daemon.Nudge(m.loomRoot, a.ID, nt.Message)
				m.nudgeMode = false
				if err != nil {
					return m, m.setFlash(fmt.Sprintf("Nudge failed: %s", err), true)
				}
				return m, m.setFlash(fmt.Sprintf("Nudged %s: %s", a.ID, nt.Label), false)
			}
			m.nudgeMode = false
		case "esc":
			m.nudgeMode = false
		case "j", "down":
			if m.nudgeCursor < len(nudge.Types)-1 {
				m.nudgeCursor++
			}
		case "k", "up":
			if m.nudgeCursor > 0 {
				m.nudgeCursor--
			}
		}
		return m, nil
	}

	// Kill confirm mode captures all input
	if m.killConfirm {
		switch msg.String() {
		case "y", "Y":
			agents := m.filteredAgents()
			if m.cursor < len(agents) {
				a := agents[m.cursor]
				err := daemon.Kill(m.loomRoot, a.ID, false)
				m.killConfirm = false
				if err != nil {
					return m, m.setFlash(fmt.Sprintf("Kill failed: %s", err), true)
				}
				return m, m.setFlash(fmt.Sprintf("Killed %s", a.ID), false)
			}
			m.killConfirm = false
		default:
			m.killConfirm = false
		}
		return m, nil
	}

	// Message mode captures all input
	if m.messageMode {
		switch msg.String() {
		case "enter":
			agents := m.filteredAgents()
			if m.cursor < len(agents) && m.messageInput != "" {
				a := agents[m.cursor]
				err := daemon.Message(m.loomRoot, a.ID, m.messageInput)
				m.messageMode = false
				m.messageInput = ""
				m.inputCursor = 0
				if err != nil {
					return m, m.setFlash(fmt.Sprintf("Message failed: %s", err), true)
				}
				return m, m.setFlash(fmt.Sprintf("Messaged %s", a.ID), false)
			}
			m.messageMode = false
			m.messageInput = ""
			m.inputCursor = 0
		case "esc":
			m.messageMode = false
			m.messageInput = ""
			m.inputCursor = 0
		default:
			s, c, _ := editInput(m.messageInput, m.inputCursor, msg)
			m.messageInput = s
			m.inputCursor = c
		}
		return m, nil
	}

	// Search mode captures all input
	if m.searchMode {
		switch msg.String() {
		case "enter":
			m.searchMode = false
			m.inputCursor = 0
			m.cursor = 0
			m.clampCursor()
		case "esc":
			m.searchMode = false
			m.searchQuery = ""
			m.inputCursor = 0
			m.cursor = 0
			m.clampCursor()
		default:
			s, c, _ := editInput(m.searchQuery, m.inputCursor, msg)
			m.searchQuery = s
			m.inputCursor = c
			m.cursor = 0
			m.clampCursor()
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		switch m.view {
		case viewAgentDetail:
			m.switchView(viewAgents)
		case viewIssueDetail:
			m.switchView(viewIssues)
case viewMailDetail:
			m.switchView(viewMail)
		case viewMemoryDetail:
			m.switchView(viewMemory)
		case viewDiff:
			m.view = viewWorktrees
			m.cursor = m.selectedWorktree
		default:
			if m.searchQuery != "" {
				m.searchQuery = ""
				m.cursor = 0
				m.clampCursor()
			} else {
				m.switchView(viewOverview)
			}
		}
		return m, nil
	case "a":
		m.switchView(viewAgents)
		return m, nil
	case "i":
		m.switchView(viewIssues)
		return m, nil
	case "m":
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.data.agents) > 0 {
			m.messageMode = true
			m.messageInput = ""
			m.inputCursor = 0
			return m, nil
		}
		m.switchView(viewMail)
		return m, nil
	case "d":
		m.switchView(viewMemory)
		return m, nil
	case "w":
		m.switchView(viewWorktrees)
		return m, nil
	case "t":
		m.switchView(viewActivity)
		return m, nil
	case "l":
		m.switchView(viewLogs)
		return m, nil
	case "f":
		if m.view == viewLogs {
			m.logFilter = (m.logFilter + 1) % 5 // all, lifecycle, error, stderr, warn
			return m, nil
		}
	case "F":
		if m.view == viewLogs {
			n := m.countLogAgents()
			m.logAgentFilter = (m.logAgentFilter + 1) % (n + 1) // 0=all, 1..n=agent
			return m, nil
		}
	case "n":
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.filteredAgents()) > 0 {
			m.nudgeMode = true
			m.nudgeCursor = 0
			return m, nil
		}
	case "x":
		if m.view == viewAgents && m.cursor < len(m.filteredAgents()) {
			m.killConfirm = true
			return m, nil
		}
	case "o":
		if m.view == viewAgents && m.cursor < len(m.filteredAgents()) {
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailScroll = 0
			return m, nil
		}
	case "/":
		if isListView(m.view) {
			m.searchMode = true
			m.searchQuery = ""
			m.inputCursor = 0
			return m, nil
		}
	case "tab":
		m.switchView(nextView(m.view))
		return m, nil
	case "j", "down":
		if m.view == viewKanban {
			m.kanbanRow++
			m.clampKanbanRow()
			return m, nil
		}
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail {
			m.detailScroll++
			return m, nil
		}
		if m.view == viewDiff {
			m.diffScroll++
			return m, nil
		}
		m.cursor++
		m.clampCursor()
		return m, nil
	case "b":
		m.switchView(viewKanban)
		return m, nil
	case "k", "up":
		if m.view == viewKanban {
			m.kanbanRow--
			if m.kanbanRow < 0 {
				m.kanbanRow = 0
			}
			return m, nil
		}
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail {
			m.detailScroll--
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		}
		if m.view == viewDiff {
			m.diffScroll--
			if m.diffScroll < 0 {
				m.diffScroll = 0
			}
			return m, nil
		}
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil
	case "h", "left":
		if m.view == viewKanban {
			m.kanbanCol--
			if m.kanbanCol < 0 {
				m.kanbanCol = 0
			}
			m.clampKanbanRow()
			return m, nil
		}
	case "right":
		if m.view == viewKanban {
			m.kanbanCol++
			if m.kanbanCol >= len(kanbanColumns) {
				m.kanbanCol = len(kanbanColumns) - 1
			}
			m.clampKanbanRow()
			return m, nil
		}
	case "enter":
		return m.handleEnter()
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewAgents:
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailScroll = 0
		}
	case viewAgentDetail:
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			a := agents[m.cursor]
			// ACP agents have no tmux pane — Enter is a no-op in detail view
			if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
				c := exec.Command("loom", "attach", a.ID)
				c.Stdin = os.Stdin
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return m, tea.ExecProcess(c, func(err error) tea.Msg { return nil })
			}
		}
	case viewIssues:
		if len(m.filteredIssues()) > 0 {
			m.cursors[m.view] = m.cursor
			m.view = viewIssueDetail
			m.detailScroll = 0
		}
	case viewMail:
		messages := m.filteredMessages()
		if m.cursor < len(messages) {
			m.cursors[m.view] = m.cursor
			m.view = viewMailDetail
			m.detailScroll = 0
		}
	case viewWorktrees:
		worktrees := m.filteredWorktrees()
		if m.cursor < len(worktrees) {
			m.cursors[m.view] = m.cursor
			m.selectedWorktree = m.cursor
			m.diffContent = fetchDiff(worktrees[m.cursor].Path)
			m.view = viewDiff
			m.diffScroll = 0
			m.cursor = 0
		}
	case viewMemory:
		memories := m.filteredMemories()
		if m.cursor < len(memories) {
			m.cursors[m.view] = m.cursor
			m.view = viewMemoryDetail
		}
	case viewActivity:
		activity := m.filteredActivity()
		if m.cursor < len(activity) {
			aid := activity[m.cursor].AgentID
			m.searchQuery = ""
			for i, a := range m.data.agents {
				if a.ID == aid {
					m.cursors[viewAgents] = i
					m.cursor = i
					m.view = viewAgentDetail
					m.detailScroll = 0
					break
				}
			}
		}
	case viewKanban:
		iss := m.kanbanSelectedIssue()
		if iss != nil {
			for i, is := range m.data.issues {
				if is.ID == iss.ID {
					m.cursors[viewIssues] = i
					m.cursor = i
					m.view = viewIssueDetail
					m.detailScroll = 0
					break
				}
			}
		}
	}
	return m, nil
}

func (m *Model) clampCursor() {
	max := m.listLen() - 1
	if max < 0 {
		max = 0
	}
	if m.cursor > max {
		m.cursor = max
	}
}

func (m Model) listLen() int {
	switch m.view {
	case viewAgents, viewAgentDetail:
		return len(m.filteredAgents())
	case viewIssues, viewIssueDetail:
		return len(m.filteredIssues())
	case viewMail:
		return len(m.filteredMessages())
	case viewMailDetail:
		return len(m.data.messages)
	case viewMemory, viewMemoryDetail:
		return len(m.filteredMemories())
	case viewWorktrees:
		return len(m.filteredWorktrees())
	case viewActivity:
		return len(m.filteredActivity())
	case viewDiff:
		return 0
	}
	return 0
}

func nextView(v view) view {
	for i, vv := range viewOrder {
		if vv == v {
			return viewOrder[(i+1)%len(viewOrder)]
		}
	}
	return viewOverview
}

func (m Model) View() string {
	// Minimum terminal size guard
	if m.width < minTermWidth || m.height < minTermHeight {
		msg := fmt.Sprintf("Terminal too small (%d×%d)\nNeed at least %d×%d", m.width, m.height, minTermWidth, minTermHeight)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
	}

	var content string
	switch m.view {
	case viewOverview:
		content = m.renderOverview()
	case viewAgents:
		content = m.renderAgents()
	case viewAgentDetail:
		content = m.renderAgentDetail()
	case viewIssues:
		content = m.renderIssues()
	case viewIssueDetail:
		content = m.renderIssueDetail()
	case viewMail:
		content = m.renderMail()
	case viewMailDetail:
		content = m.renderMailDetail()
	case viewMemory:
		content = m.renderMemory()
	case viewMemoryDetail:
		content = m.renderMemoryDetail()
	case viewActivity:
		content = m.renderActivity()
	case viewLogs:
		content = m.renderLogs()
	case viewWorktrees:
		content = m.renderWorktrees()
	case viewDiff:
		content = m.renderDiff()
	case viewKanban:
		content = m.renderKanban()
	}

	// Full-width title bar with status summary
	left := " ◈ LOOM DASHBOARD"
	right := fmt.Sprintf("%d agents  %d unread ", len(m.data.agents), m.data.unread)
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	titleBar := titleStyle.Width(m.width).Render(left + strings.Repeat(" ", padding) + right)

	help := m.helpBar()
	if m.searchMode {
		runes := []rune(m.searchQuery)
		before := string(runes[:m.inputCursor])
		after := string(runes[m.inputCursor:])
		searchBox := lipgloss.NewStyle().Background(colSelBg).Foreground(colFg).Padding(0, 1)
		help = searchBox.Render(fmt.Sprintf("/ %s█%s", before, after)) + helpStyle.Render("  [Enter]filter [Esc]cancel")
	}
	if m.nudgeMode {
		agentName := ""
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			agentName = agents[m.cursor].ID
		}
		var items []string
		for i, nt := range nudge.Types {
			prefix := "  "
			if i == m.nudgeCursor {
				prefix = "▸ "
			}
			items = append(items, prefix+nt.Label)
		}
		help = helpStyle.Render(fmt.Sprintf(" Nudge %s: %s  [j/k]select [Enter]send [Esc]cancel", agentName, strings.Join(items, " | ")))
	}
	if m.messageMode {
		agentName := ""
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			agentName = agents[m.cursor].ID
		}
		runes := []rune(m.messageInput)
		before := string(runes[:m.inputCursor])
		after := string(runes[m.inputCursor:])
		help = helpStyle.Render(fmt.Sprintf(" Message %s: %s█%s  [Enter]send [Esc]cancel", agentName, before, after))
	}
	if m.killConfirm {
		agentName := ""
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			agentName = agents[m.cursor].ID
		}
		help = helpStyle.Render(fmt.Sprintf(" Kill agent %s? [y/N]", agentName))
	}

	// Flash messages on their own line above help bar
	flashLine := ""
	if m.flashMsg != "" && !m.nudgeMode && !m.messageMode && !m.killConfirm {
		style := flashOkStyle
		if m.flashIsErr {
			style = flashErrStyle
		}
		flashLine = style.Render(" " + m.flashMsg)
	}

	// Build final output
	var output string
	if flashLine != "" {
		output = fmt.Sprintf("%s\n%s\n%s\n%s", titleBar, content, flashLine, help)
	} else {
		output = fmt.Sprintf("%s\n%s\n%s", titleBar, content, help)
	}

	// Full-screen background fill
	lines := strings.Split(output, "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	bg := lipgloss.NewStyle().Background(colBg).Foreground(colFg)
	for i, l := range lines {
		w := lipgloss.Width(l)
		if w < m.width {
			lines[i] = l + bg.Render(strings.Repeat(" ", m.width-w))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) helpBar() string {
	// In agents/agent-detail views with agents present, m means "message" not "mail"
	mIsMessage := (m.view == viewAgents || m.view == viewAgentDetail) && len(m.data.agents) > 0

	var parts []string
	for _, tab := range helpBarTabs {
		label := tab.label
		if tab.view == viewMail && mIsMessage {
			label = "[m]essage"
		}
		if m.view == tab.view || (tab.view == viewAgents && m.view == viewAgentDetail) {
			parts = append(parts, helpActiveStyle.Render(label))
		} else {
			parts = append(parts, helpStyle.Render(label))
		}
	}
	tabLine := " " + strings.Join(parts, " ") + helpStyle.Render(" [Tab]cycle [Esc]back [/]search [q]uit")

	// Context-specific shortcuts on second line
	var ctx string
	switch m.view {
	case viewAgentDetail:
		ctx = "[n]udge [j/k]scroll"
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			a := agents[m.cursor]
			if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
				ctx += " [Enter]attach"
			}
		}
	case viewAgents:
		ctx = "[n]udge [o]utput [x]kill [Enter]detail"
	case viewKanban:
		ctx = "[h/←/→]column [j/k]row [Enter]detail"
	case viewIssues:
		ctx = "[Enter]detail"
	case viewMail:
		ctx = "[Enter]detail"
	case viewWorktrees:
		ctx = "[Enter]diff"
	case viewMemory:
		ctx = "[Enter]detail"
	case viewLogs:
		ctx = "[f]ilter [F]agent"
	case viewActivity:
		ctx = "[Enter]agent"
	case viewIssueDetail, viewMailDetail, viewMemoryDetail:
		ctx = "[j/k]scroll"
	case viewDiff:
		ctx = "[j/k]scroll"
	}

	if ctx != "" {
		return tabLine + "\n" + helpStyle.Render("  │ "+ctx)
	}
	return tabLine + "\n" + helpStyle.Render("  │")
}

// helpBarTabs maps substrings in the help bar to views for mouse click targeting.
var helpBarTabs = []struct {
	label string
	view  view
}{
	{"[a]gents", viewAgents},
	{"[i]ssues", viewIssues},
	{"[m]ail", viewMail},
	{"[d] memory", viewMemory},
	{"[w]orktrees", viewWorktrees},
	{"[b]oard", viewKanban},
	{"[t]activity", viewActivity},
	{"[l]ogs", viewLogs},
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x, y := msg.X, msg.Y
	lastRow := m.height - 2 // help bar is 2 lines; tabs are on the first

	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail {
			m.detailScroll--
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		}
		if m.view == viewDiff {
			m.diffScroll--
			if m.diffScroll < 0 {
				m.diffScroll = 0
			}
			return m, nil
		}
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case msg.Button == tea.MouseButtonWheelDown:
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail {
			m.detailScroll++
			return m, nil
		}
		if m.view == viewDiff {
			m.diffScroll++
			return m, nil
		}
		m.cursor++
		m.clampCursor()
		return m, nil

	case msg.Button == tea.MouseButtonLeft:
		// Click on help bar → switch view
		if y >= lastRow {
			mIsMessage := (m.view == viewAgents || m.view == viewAgentDetail) && len(m.data.agents) > 0
			offset := 1 // leading space
			for _, tab := range helpBarTabs {
				label := tab.label
				if tab.view == viewMail && mIsMessage {
					label = "[m]essage"
				}
				if x >= offset && x < offset+len(label) {
					if tab.view == viewMail && mIsMessage {
						// In agent context, clicking [m]essage triggers message mode
						m.messageMode = true
						m.messageInput = ""
						m.inputCursor = 0
					} else {
						m.switchView(tab.view)
					}
					return m, nil
				}
				offset += len(label) + 1 // +1 for space separator
			}
			return m, nil
		}

		// Click on list items in list views
		if isListView(m.view) {
			item := m.mouseToListIndex(y)
			if item >= 0 && item < m.listLen() {
				now := time.Now()
				doubleClick := item == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond
				m.cursor = item
				m.lastClickRow = item
				m.lastClickTime = now
				if doubleClick {
					return m.handleEnter()
				}
			}
			return m, nil
		}

		// Diff/logs: click sets scroll position
		if m.view == viewDiff {
			item := m.mouseToListIndex(y)
			if item >= 0 {
				m.diffScroll = item
			}
			return m, nil
		}
		if m.view == viewLogs {
			item := m.mouseToListIndex(y)
			if item >= 0 {
				m.cursor = item
				m.clampCursor()
			}
			return m, nil
		}

	}

	return m, nil
}

func isListView(v view) bool {
	switch v {
	case viewAgents, viewIssues, viewMail, viewMemory, viewWorktrees, viewActivity:
		return true
	}
	return false
}

// mouseToListIndex converts a screen Y coordinate to a list item index.
// The first listHeaderRows rows are fixed chrome (title, panel border, column header, separator).
func (m Model) mouseToListIndex(y int) int {
	idx := y - listHeaderRows
	if m.view == viewIssues {
		idx = m.adjustIssuesIndex(idx)
	}
	return idx
}

// adjustIssuesIndex accounts for the extra separator lines (blank + "RECENTLY DONE" + separator)
// inserted between active and done sections in the issues view.
func (m Model) adjustIssuesIndex(idx int) int {
	display := m.filteredIssues()
	activeCount := 0
	for _, iss := range display {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeCount++
		}
	}
	if activeCount < len(display) && idx > activeCount {
		if idx <= activeCount+issuesSectionGap {
			return -1
		}
		idx -= issuesSectionGap
	}
	return idx
}

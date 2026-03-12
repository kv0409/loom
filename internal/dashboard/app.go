package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
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
	nudgeInput       string
	messageMode      bool
	messageInput     string
	killConfirm      bool
	selectedWorktree int
	diffContent      string
	lr               *logReader
	lastClickTime    time.Time
	lastClickRow     int
	hoverRow         int // -1 = no hover
	detailScroll     int // scroll offset for agent detail output
	diffScroll       int // scroll offset for diff view
}

type tickMsg time.Time

func New(loomRoot string) Model {
	return Model{loomRoot: loomRoot, width: 80, height: 24, lr: newLogReader(loomRoot), hoverRow: -1, cursors: make(map[view]int)}
}

// switchView saves the current cursor position and switches to the target view,
// restoring its previously saved cursor position.
func (m *Model) switchView(target view) {
	m.cursors[m.view] = m.cursor
	m.view = target
	m.cursor = m.cursors[target]
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd())
}

// ProgramOptions returns the tea.ProgramOption set needed by the dashboard,
// including alt-screen and mouse support.
func ProgramOptions() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseAllMotion()}
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
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.hoverRow = -1

	// Nudge mode captures all input
	if m.nudgeMode {
		switch msg.String() {
		case "enter":
			if m.cursor < len(m.data.agents) && m.nudgeInput != "" {
				a := m.data.agents[m.cursor]
				daemon.Nudge(m.loomRoot, a.ID, m.nudgeInput)
			}
			m.nudgeMode = false
			m.nudgeInput = ""
		case "esc":
			m.nudgeMode = false
			m.nudgeInput = ""
		case "backspace":
			if len(m.nudgeInput) > 0 {
				m.nudgeInput = m.nudgeInput[:len(m.nudgeInput)-1]
			}
		default:
			if len(msg.String()) == 1 || msg.String() == " " {
				m.nudgeInput += msg.String()
			}
		}
		return m, nil
	}

	// Kill confirm mode captures all input
	if m.killConfirm {
		switch msg.String() {
		case "y", "Y":
			if m.cursor < len(m.data.agents) {
				a := m.data.agents[m.cursor]
				daemon.Kill(m.loomRoot, a.ID, false)
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
			if m.cursor < len(m.data.agents) && m.messageInput != "" {
				a := m.data.agents[m.cursor]
				daemon.Message(m.loomRoot, a.ID, m.messageInput)
			}
			m.messageMode = false
			m.messageInput = ""
		case "esc":
			m.messageMode = false
			m.messageInput = ""
		case "backspace":
			if len(m.messageInput) > 0 {
				m.messageInput = m.messageInput[:len(m.messageInput)-1]
			}
		default:
			if len(msg.String()) == 1 || msg.String() == " " {
				m.messageInput += msg.String()
			}
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
			m.switchView(viewOverview)
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
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.data.agents) > 0 {
			m.nudgeMode = true
			m.nudgeInput = ""
			return m, nil
		}
	case "x":
		if m.view == viewAgents && m.cursor < len(m.data.agents) {
			m.killConfirm = true
			return m, nil
		}
	case "o":
		if m.view == viewAgents && m.cursor < len(m.data.agents) {
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailScroll = 0
			return m, nil
		}
	case "tab":
		m.switchView(nextView(m.view))
		return m, nil
	case "j", "down":
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
	case "enter":
		return m.handleEnter()
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewAgents:
		if len(m.data.agents) > 0 {
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailScroll = 0
		}
	case viewAgentDetail:
		if m.cursor < len(m.data.agents) {
			a := m.data.agents[m.cursor]
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
		if len(m.displayIssues()) > 0 {
			m.cursors[m.view] = m.cursor
			m.view = viewIssueDetail
			m.detailScroll = 0
		}
	case viewMail:
		if m.cursor < len(m.data.messages) {
			m.cursors[m.view] = m.cursor
			m.view = viewMailDetail
			m.detailScroll = 0
		}
	case viewWorktrees:
		if m.cursor < len(m.data.worktrees) {
			m.cursors[m.view] = m.cursor
			m.selectedWorktree = m.cursor
			m.diffContent = fetchDiff(m.data.worktrees[m.cursor].Path)
			m.view = viewDiff
			m.diffScroll = 0
		}
	case viewMemory:
		if m.cursor < len(m.data.memories) {
			m.cursors[m.view] = m.cursor
			m.view = viewMemoryDetail
		}
	case viewActivity:
		if m.cursor < len(m.data.activity) {
			aid := m.data.activity[m.cursor].AgentID
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
		return len(m.data.agents)
	case viewIssues, viewIssueDetail:
		return len(m.displayIssues())
	case viewMail:
		return len(m.data.messages)
case viewMailDetail:
		return len(m.data.messages)
	case viewMemory, viewMemoryDetail:
		return len(m.data.memories)
	case viewWorktrees:
		return len(m.data.worktrees)
	case viewActivity:
		return len(m.data.activity)
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

	help := m.helpBar()
	if m.nudgeMode {
		agentName := ""
		if m.cursor < len(m.data.agents) {
			agentName = m.data.agents[m.cursor].ID
		}
		help = helpStyle.Render(fmt.Sprintf(" Nudge %s: %s█  [Enter]send [Esc]cancel", agentName, m.nudgeInput))
	}
	if m.messageMode {
		agentName := ""
		if m.cursor < len(m.data.agents) {
			agentName = m.data.agents[m.cursor].ID
		}
		help = helpStyle.Render(fmt.Sprintf(" Message %s: %s█  [Enter]send [Esc]cancel", agentName, m.messageInput))
	}
	if m.killConfirm {
		agentName := ""
		if m.cursor < len(m.data.agents) {
			agentName = m.data.agents[m.cursor].ID
		}
		help = helpStyle.Render(fmt.Sprintf(" Kill agent %s? [y/N]", agentName))
	}
	return fmt.Sprintf("%s\n%s\n%s", titleStyle.Render("── LOOM DASHBOARD ──"), content, help)
}

func (m Model) helpBar() string {
	var parts []string
	for _, tab := range helpBarTabs {
		if m.view == tab.view || (tab.view == viewAgents && m.view == viewAgentDetail) {
			parts = append(parts, helpActiveStyle.Render(tab.label))
		} else {
			parts = append(parts, helpStyle.Render(tab.label))
		}
	}
	suffix := helpStyle.Render(" [Tab]cycle [Esc]back [q]uit")
	base := " " + strings.Join(parts, " ") + suffix
	if m.view == viewAgentDetail {
		extra := " | [n]udge [j/k]scroll"
		if m.cursor < len(m.data.agents) {
			a := m.data.agents[m.cursor]
			if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
				extra += " [Enter]attach"
			}
		}
		base += helpStyle.Render(extra)
	} else if m.view == viewAgents {
		base += helpStyle.Render(" | [n]udge [m]essage [o]utput [x]kill [Enter]detail")
	}
	return base
}

// helpBarTabs maps substrings in the help bar to views for mouse click targeting.
var helpBarTabs = []struct {
	label string
	view  view
}{
	{"[a]gents", viewAgents},
	{"[i]ssues", viewIssues},
	{"[m]ail", viewMail},
	{"[d]ecisions", viewMemory},
	{"[w]orktrees", viewWorktrees},
	{"[b]oard", viewKanban},
	{"[t]activity", viewActivity},
	{"[l]ogs", viewLogs},
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x, y := msg.X, msg.Y
	lastRow := m.height - 1

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
			bar := " [a]gents [i]ssues [m]ail [d]ecisions [w]orktrees [b]oard [t]activity [l]ogs"
			for _, tab := range helpBarTabs {
				idx := strings.Index(bar, tab.label)
				if idx >= 0 && x >= idx && x < idx+len(tab.label) {
					m.switchView(tab.view)
					m.hoverRow = -1
					return m, nil
				}
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

	case msg.Button == tea.MouseButtonNone:
		// Motion — update hover row
		if isListView(m.view) {
			item := m.mouseToListIndex(y)
			if item >= 0 && item < m.listLen() {
				m.hoverRow = item
			} else {
				m.hoverRow = -1
			}
		} else {
			m.hoverRow = -1
		}
		_ = x // suppress unused
		return m, nil
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
// Layout: row 0 = title, row 1 = panel border, row 2 = column header, row 3 = separator, row 4+ = items.
func (m Model) mouseToListIndex(y int) int {
	idx := y - 4
	if m.view == viewIssues {
		// displayIssues inserts 3 extra lines (blank + RECENTLY DONE + separator)
		// between active and done sections. Adjust index for items past the gap.
		activeCount := 0
		for _, iss := range m.displayIssues() {
			if iss.Status != "done" && iss.Status != "cancelled" {
				activeCount++
			}
		}
		display := m.displayIssues()
		if activeCount < len(display) && idx > activeCount {
			// Clicks on the 3 separator lines map to nothing useful
			if idx <= activeCount+3 {
				return -1
			}
			idx -= 3
		}
	}
	return idx
}

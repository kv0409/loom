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
	viewMemory
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
	width            int
	height           int
	logFilter        int // 0=all, 1..N=index into agents
	nudgeMode        bool
	nudgeInput       string
	messageMode      bool
	messageInput     string
	selectedWorktree int
	diffContent      string
	lr               *logReader
	lastClickTime    time.Time
	lastClickRow     int
	hoverRow         int // -1 = no hover
}

type tickMsg time.Time

func New(loomRoot string) Model {
	return Model{loomRoot: loomRoot, width: 80, height: 24, lr: newLogReader(loomRoot), hoverRow: -1}
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
			m.view = viewAgents
			m.cursor = 0
		case viewIssueDetail:
			m.view = viewIssues
			m.cursor = 0
		case viewDiff:
			m.view = viewWorktrees
			m.cursor = m.selectedWorktree
		default:
			m.view = viewOverview
			m.cursor = 0
		}
		return m, nil
	case "a":
		m.view = viewAgents
		m.cursor = 0
		return m, nil
	case "i":
		m.view = viewIssues
		m.cursor = 0
		return m, nil
	case "m":
		if m.view == viewAgents && len(m.data.agents) > 0 {
			m.messageMode = true
			m.messageInput = ""
			return m, nil
		}
		m.view = viewMail
		m.cursor = 0
		return m, nil
	case "d":
		m.view = viewMemory
		m.cursor = 0
		return m, nil
	case "w":
		m.view = viewWorktrees
		m.cursor = 0
		return m, nil
	case "t":
		m.view = viewActivity
		m.cursor = 0
		return m, nil
	case "l":
		m.view = viewLogs
		m.cursor = 0
		return m, nil
	case "f":
		if m.view == viewLogs {
			m.logFilter = (m.logFilter + 1) % 5 // all, lifecycle, error, stderr, warn
			return m, nil
		}
	case "n":
		if m.view == viewAgents && len(m.data.agents) > 0 {
			m.nudgeMode = true
			m.nudgeInput = ""
			return m, nil
		}
	case "x":
		if m.view == viewAgents && m.cursor < len(m.data.agents) {
			a := m.data.agents[m.cursor]
			daemon.Kill(m.loomRoot, a.ID, false)
			return m, nil
		}
	case "o":
		if m.view == viewAgents && m.cursor < len(m.data.agents) {
			a := m.data.agents[m.cursor]
			if out, err := daemon.Output(m.loomRoot, a.ID, 50); err == nil {
				m.view = viewAgentDetail
				m.diffContent = out
			}
			return m, nil
		}
	case "tab":
		m.view = nextView(m.view)
		m.cursor = 0
		return m, nil
	case "j", "down":
		m.cursor++
		m.clampCursor()
		return m, nil
	case "b":
		m.view = viewKanban
		m.cursor = 0
		return m, nil
	case "k", "up":
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
			m.view = viewAgentDetail
		}
	case viewAgentDetail:
		if m.cursor < len(m.data.agents) {
			a := m.data.agents[m.cursor]
			c := exec.Command("loom", "attach", a.ID)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return m, tea.ExecProcess(c, func(err error) tea.Msg { return nil })
		}
	case viewIssues:
		if len(m.displayIssues()) > 0 {
			m.view = viewIssueDetail
		}
	case viewWorktrees:
		if m.cursor < len(m.data.worktrees) {
			m.selectedWorktree = m.cursor
			m.diffContent = fetchDiff(m.data.worktrees[m.cursor].Path)
			m.view = viewDiff
			m.cursor = 0
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
	case viewMemory:
		return len(m.data.memories)
	case viewWorktrees:
		return len(m.data.worktrees)
	case viewDiff:
		lines := len(splitLines(m.diffContent))
		if lines > 0 {
			return lines
		}
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
	case viewMemory:
		content = m.renderMemory()
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
	return fmt.Sprintf("%s\n%s\n%s", titleStyle.Render("── LOOM DASHBOARD ──"), content, help)
}

func (m Model) helpBar() string {
	base := " [a]gents [i]ssues [m]ail [d]ecisions [w]orktrees [b]oard [t]activity [l]ogs [Tab]cycle [Esc]back [q]uit"
	if m.view == viewAgents {
		base += " | [n]udge [m]essage [o]utput [x]kill [Enter]attach"
	}
	return helpStyle.Render(base)
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
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case msg.Button == tea.MouseButtonWheelDown:
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
					m.view = tab.view
					m.cursor = 0
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
		if m.view == viewDiff || m.view == viewLogs {
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
	case viewAgents, viewIssues, viewMail, viewMemory, viewWorktrees:
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

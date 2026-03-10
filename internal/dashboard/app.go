package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/karanagi/loom/internal/agent"
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
)

var viewOrder = []view{viewOverview, viewAgents, viewIssues, viewMail, viewMemory, viewActivity, viewLogs}

type data struct {
	agents    []*agent.Agent
	issues    []*issue.Issue
	worktrees []*worktree.Worktree
	messages  []*mail.Message
	memories  []*memory.Entry
	unread    int
	activity  []activityEntry
	logs      []logLine
}

type Model struct {
	loomRoot  string
	view      view
	data      data
	cursor    int
	width     int
	height    int
	logFilter int // 0=all, 1..N=index into agents
}

type tickMsg time.Time

func New(loomRoot string) Model {
	return Model{loomRoot: loomRoot, width: 80, height: 24}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refresh() tea.Cmd {
	root := m.loomRoot
	return func() tea.Msg {
		var d data
		d.agents, _ = agent.List(root)
		d.issues, _ = issue.List(root, issue.ListOpts{All: true})
		d.worktrees, _ = worktree.List(root)
		d.messages, _ = mail.Log(root, mail.LogOpts{})
		d.memories, _ = memory.List(root, memory.ListOpts{})
		d.unread = countUnread(root)
		d.activity = fetchActivity(d.agents)
		d.logs = readLogs(root)
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
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.view == viewAgentDetail || m.view == viewIssueDetail {
			if m.view == viewAgentDetail {
				m.view = viewAgents
			} else {
				m.view = viewIssues
			}
		} else {
			m.view = viewOverview
		}
		m.cursor = 0
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
		m.view = viewMail
		m.cursor = 0
		return m, nil
	case "d":
		m.view = viewMemory
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
			m.logFilter++
			if m.logFilter > len(m.data.agents) {
				m.logFilter = 0
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
	case viewIssues:
		if len(m.data.issues) > 0 {
			m.view = viewIssueDetail
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
		return len(m.data.issues)
	case viewMail:
		return len(m.data.messages)
	case viewMemory:
		return len(m.data.memories)
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
	}

	help := helpStyle.Render(" [a]gents [i]ssues [m]ail [d]ecisions [t]activity [l]ogs [Tab]cycle [Esc]back [q]uit")
	return fmt.Sprintf("%s\n%s\n%s", titleStyle.Render("── LOOM DASHBOARD ──"), content, help)
}

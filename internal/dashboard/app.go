package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/dashboard/backend"
	"github.com/karanagi/loom/internal/nudge"
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

type Model struct {
	loomRoot         string
	view             view
	data             backend.Snapshot
	cursor           int
	cursors          map[view]int // per-view cursor positions
	width            int
	height           int
	logFilter        int // 0=all, 1=lifecycle, 2=error, 3=stderr, 4=warn
	logAgentFilter   int // 0=all, 1..N = specific agent
	nudgeMode        bool
	nudgeCursor      int
	messageMode      bool
	messageTI        textinput.Model
	killConfirm      bool
	selectedWorktree int
	diffContent      string
	kanbanCol        int // selected column in kanban view
	kanbanRow        int // selected row within column
	backend          backend.Backend
	lastClickTime    time.Time
	lastClickRow     int
	detailScroll     int // scroll offset for agent detail output
	diffScroll       int // scroll offset for diff view
	flashMsg         string
	flashIsErr       bool
	searchMode       bool
	searchTI         textinput.Model
	help             help.Model
	keys             keyMap
	reloading        bool // set when quitting due to binary hot-reload
	refreshed        bool // set after first data message received
	composeMode      bool
	composeForm      *huh.Form
	composeData      *composeData
	agentOutputCache []backend.ACPEvent // cached events for current agent detail
	agentOutputID    string             // agent ID the cache belongs to
	diffLoading      bool               // true while diff is being fetched
	errorShown       bool               // set after first error flash to avoid repeating
}

type tickMsg time.Time
type clearFlashMsg struct{}
type binaryReloadMsg struct{}

type daemonResultMsg struct {
	flash string
	isErr bool
}

type diffResultMsg struct {
	content string
}

type agentOutputMsg struct {
	agentID string
	events  []backend.ACPEvent
	err     error
}

type sendMailResultMsg struct {
	flash string
	isErr bool
}

func clearFlashAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearFlashMsg{} })
}

func New(loomRoot string) Model {
	h := help.New()
	h.Styles.ShortKey = helpStyle.Bold(true)
	h.Styles.ShortDesc = helpStyle
	h.Styles.ShortSeparator = helpStyle

	msgTI := textinput.New()
	msgTI.Prompt = ""

	searchTI := textinput.New()
	searchTI.Prompt = ""

	return Model{loomRoot: loomRoot, width: 80, height: 24, backend: backend.NewFileBackend(loomRoot), cursors: make(map[view]int), help: h, keys: defaultKeyMap(), messageTI: msgTI, searchTI: searchTI}
}

// Reloading reports whether the dashboard exited due to a binary hot-reload.
func (m Model) Reloading() bool { return m.reloading }

// switchView saves the current cursor position and switches to the target view,
// restoring its previously saved cursor position.
func (m *Model) switchView(target view) {
	m.cursors[m.view] = m.cursor
	m.view = target
	m.cursor = m.cursors[target]
	// Activity view: auto-scroll to bottom (latest entries) on first open.
	if target == viewActivity && m.cursor == 0 && len(m.data.Activity) > 0 {
		m.cursor = len(m.data.Activity) - 1
	}
	m.searchTI.SetValue("")
	m.searchMode = false
}

func (m *Model) setFlash(msg string, isErr bool) tea.Cmd {
	m.flashMsg = msg
	m.flashIsErr = isErr
	return clearFlashAfter(3 * time.Second)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd(), watchBinary())
}

// watchBinary polls the loom binary's mtime every 2s and sends binaryReloadMsg when it changes.
func watchBinary() tea.Cmd {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	info, err := os.Stat(exe)
	if err != nil {
		return nil
	}
	return watchBinaryFrom(info.ModTime())
}

func watchBinaryFrom(mtime time.Time) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return watchBinaryTickMsg{mtime: mtime}
		}
		info, err := os.Stat(exe)
		if err != nil {
			return watchBinaryTickMsg{mtime: mtime}
		}
		if info.ModTime().After(mtime) {
			return binaryReloadMsg{}
		}
		return watchBinaryTickMsg{mtime: info.ModTime()}
	})
}

type watchBinaryTickMsg struct{ mtime time.Time }

// ProgramOptions returns the tea.ProgramOption set needed by the dashboard,
// including alt-screen and mouse support.
func ProgramOptions() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refresh() tea.Cmd {
	b := m.backend
	return func() tea.Msg {
		return b.Load()
	}
}

func daemonCmd(loomRoot string, fn func() error, successFlash string) tea.Cmd {
	return func() tea.Msg {
		if err := fn(); err != nil {
			return daemonResultMsg{flash: err.Error(), isErr: true}
		}
		return daemonResultMsg{flash: successFlash, isErr: false}
	}
}

func diffCmd(b backend.Backend, wtPath string) tea.Cmd {
	return func() tea.Msg {
		return diffResultMsg{content: b.Diff(wtPath)}
	}
}

func agentOutputCmd(b backend.Backend, loomRoot, agentID string) tea.Cmd {
	return func() tea.Msg {
		events, err := b.AgentOutput(loomRoot, agentID)
		return agentOutputMsg{agentID: agentID, events: events, err: err}
	}
}

func sendMailCmd(b backend.Backend, loomRoot, from, to, subject, body, typ, priority, ref string) tea.Cmd {
	return func() tea.Msg {
		err := b.SendMail(loomRoot, from, to, subject, body, typ, priority, ref)
		if err != nil {
			return sendMailResultMsg{flash: fmt.Sprintf("Send failed: %s", err), isErr: true}
		}
		return sendMailResultMsg{flash: fmt.Sprintf("Sent to %s", to), isErr: false}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Compose modal captures all input while active.
	// Resize, tick, and data messages are also handled for the dashboard.
	if m.composeMode && m.composeForm != nil {
		// Intercept keys before huh sees them.
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "ctrl+s":
				return m.composeSend()
			case "esc":
				m.composeMode = false
				return m, m.setFlash("Compose cancelled", false)
			}
		}

		form, cmd := m.composeForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.composeForm = f
		}
		switch m.composeForm.State {
		case huh.StateCompleted:
			// Form completed (user tabbed past last field) — treat as send.
			return m.composeSend()
		case huh.StateAborted:
			m.composeMode = false
			return m, m.setFlash("Compose cancelled", false)
		}
		// Also process dashboard-level messages alongside the form.
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.help.Width = msg.Width
			m.composeForm.WithWidth(min(56, msg.Width-8))
		case tickMsg:
			return m, tea.Batch(cmd, m.refresh(), tickCmd())
		case watchBinaryTickMsg:
			return m, tea.Batch(cmd, watchBinaryFrom(msg.mtime))
		case binaryReloadMsg:
			m.reloading = true
			return m, tea.Quit
		case backend.Snapshot:
			m.data = msg
			m.refreshed = true
			m.clampCursor()
			if m.view != viewAgentDetail && m.view != viewIssueDetail && m.view != viewMailDetail && m.view != viewMemoryDetail {
				m.detailScroll = 0
			}
			if m.view != viewDiff {
				m.diffScroll = 0
			}
			if len(msg.Errors) > 0 && !m.errorShown {
				m.errorShown = true
			}
		case daemonResultMsg:
			return m, m.setFlash(msg.flash, msg.isErr)
		case diffResultMsg:
			m.diffContent = msg.content
			m.diffLoading = false
		case agentOutputMsg:
			if m.view == viewAgentDetail && msg.agentID == m.agentOutputID {
				m.agentOutputCache = msg.events
			}
		case sendMailResultMsg:
			return m, m.setFlash(msg.flash, msg.isErr)
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil
	case tickMsg:
		cmds := []tea.Cmd{m.refresh(), tickCmd()}
		if m.view == viewAgentDetail {
			agents := m.filteredAgents()
			if m.cursor < len(agents) {
				a := agents[m.cursor]
				if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
					cmds = append(cmds, agentOutputCmd(m.backend, m.loomRoot, a.ID))
				}
			}
		}
		return m, tea.Batch(cmds...)
	case watchBinaryTickMsg:
		return m, watchBinaryFrom(msg.mtime)
	case binaryReloadMsg:
		m.reloading = true
		return m, tea.Quit
	case backend.Snapshot:
		m.data = msg
		m.refreshed = true
		m.clampCursor()
		if m.view != viewAgentDetail && m.view != viewIssueDetail && m.view != viewMailDetail && m.view != viewMemoryDetail {
			m.detailScroll = 0
		}
		if m.view != viewDiff {
			m.diffScroll = 0
		}
		if len(msg.Errors) > 0 && !m.errorShown {
			m.errorShown = true
			return m, m.setFlash(fmt.Sprintf("%d data error(s): %s", len(msg.Errors), msg.Errors[0]), true)
		}
		return m, nil
	case clearFlashMsg:
		m.flashMsg = ""
		return m, nil
	case daemonResultMsg:
		return m, m.setFlash(msg.flash, msg.isErr)
	case diffResultMsg:
		m.diffContent = msg.content
		m.diffLoading = false
		return m, nil
	case agentOutputMsg:
		if m.view == viewAgentDetail && msg.agentID == m.agentOutputID {
			m.agentOutputCache = msg.events
		}
		return m, nil
	case sendMailResultMsg:
		return m, m.setFlash(msg.flash, msg.isErr)
	}

	// Forward to active textinput
	var cmd tea.Cmd
	if m.messageMode {
		m.messageTI, cmd = m.messageTI.Update(msg)
	} else if m.searchMode {
		m.searchTI, cmd = m.searchTI.Update(msg)
	}
	return m, cmd
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
				m.nudgeMode = false
				lr := m.loomRoot
				return m, daemonCmd(lr, func() error {
					return daemon.Nudge(lr, a.ID, nt.Message)
				}, fmt.Sprintf("Nudged %s: %s", a.ID, nt.Label))
			}
			m.nudgeMode = false
		case "esc":
			m.nudgeMode = false
			return m, m.setFlash("Nudge cancelled", false)
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
				m.killConfirm = false
				lr := m.loomRoot
				return m, daemonCmd(lr, func() error {
					return daemon.Kill(lr, a.ID, false)
				}, fmt.Sprintf("Killed %s", a.ID))
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
			if m.cursor < len(agents) && m.messageTI.Value() != "" {
				a := agents[m.cursor]
				msgText := m.messageTI.Value()
				m.messageMode = false
				m.messageTI.SetValue("")
				m.messageTI.Blur()
				lr := m.loomRoot
				return m, daemonCmd(lr, func() error {
					return daemon.Message(lr, a.ID, msgText)
				}, fmt.Sprintf("Messaged %s", a.ID))
			}
			m.messageMode = false
			m.messageTI.SetValue("")
			m.messageTI.Blur()
		case "esc":
			m.messageMode = false
			m.messageTI.SetValue("")
			m.messageTI.Blur()
		default:
			var cmd tea.Cmd
			m.messageTI, cmd = m.messageTI.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Search mode captures all input
	if m.searchMode {
		switch msg.String() {
		case "enter":
			m.searchMode = false
			m.searchTI.Blur()
			m.cursor = 0
			m.clampCursor()
		case "esc":
			m.searchMode = false
			m.searchTI.SetValue("")
			m.searchTI.Blur()
			m.cursor = 0
			m.clampCursor()
		default:
			var cmd tea.Cmd
			m.searchTI, cmd = m.searchTI.Update(msg)
			m.cursor = 0
			m.clampCursor()
			return m, cmd
		}
		return m, nil
	}

	switch msg.String() {
	case keyQuit, keyQuitCtrl:
		return m, tea.Quit
	case keyEsc:
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
			if m.searchTI.Value() != "" {
				m.searchTI.SetValue("")
				m.cursor = 0
				m.clampCursor()
			} else {
				m.switchView(viewOverview)
			}
		}
		return m, nil
	case keyViewOverview, keyViewOverview2:
		m.switchView(viewOverview)
		return m, nil
	case keyViewAgents:
		m.switchView(viewAgents)
		return m, nil
	case keyViewIssues:
		m.switchView(viewIssues)
		return m, nil
	case keyViewMail: // "m": message-compose in agents/agent-detail; mail view elsewhere
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.data.Agents) > 0 {
			m.messageMode = true
			m.messageTI.SetValue("")
			m.messageTI.Focus()
			return m, nil
		}
		m.switchView(viewMail)
		return m, nil
	case keyViewMemory:
		m.switchView(viewMemory)
		return m, nil
	case keyViewWorktrees:
		m.switchView(viewWorktrees)
		return m, nil
	case keyViewActivity:
		m.switchView(viewActivity)
		return m, nil
	case keyViewLogs: // "l": logs view — NOT a kanban-right alias (conflict resolved)
		m.switchView(viewLogs)
		return m, nil
	case keyLogsFilter:
		if m.view == viewLogs {
			m.logFilter = (m.logFilter + 1) % 5 // all, lifecycle, error, stderr, warn
		}
		return m, nil
	case keyLogsAgentFilter:
		if m.view == viewLogs {
			n := m.countLogAgents()
			m.logAgentFilter = (m.logAgentFilter + 1) % (n + 1) // 0=all, 1..n=agent
		}
		return m, nil
	case keyAgentNudge:
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.filteredAgents()) > 0 {
			m.nudgeMode = true
			m.nudgeCursor = 0
			return m, nil
		}
	case keyAgentKill:
		if m.view == viewAgents && m.cursor < len(m.filteredAgents()) {
			m.killConfirm = true
			return m, nil
		}
	case keyAgentOutput:
		if m.view == viewAgents && m.cursor < len(m.filteredAgents()) {
			a := m.filteredAgents()[m.cursor]
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailScroll = 0
			m.agentOutputCache = nil
			m.agentOutputID = a.ID
			if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
				return m, agentOutputCmd(m.backend, m.loomRoot, a.ID)
			}
			return m, nil
		}
	case keySearch:
		if isSearchableView(m.view) {
			m.searchMode = true
			m.searchTI.SetValue("")
			m.searchTI.Focus()
			return m, nil
		}
	case keyCompose:
		if m.view == viewMail || m.view == viewMailDetail {
			return m.openCompose("")
		}
		return m, nil
	case keyComposeReply:
		if m.view == viewMailDetail {
			msgs := m.filteredMessages()
			if m.cursor < len(msgs) {
				return m.openCompose(msgs[m.cursor].From)
			}
		} else if m.view == viewMail {
			return m.openCompose("")
		}
	case keyTab:
		m.switchView(nextView(m.view))
		return m, nil
	case keyVimDown, keyDown:
		if m.view == viewKanban {
			m.kanbanRow++
			m.clampKanbanRow()
			return m, nil
		}
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail || m.view == viewMemoryDetail {
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
	case keyViewKanban:
		m.switchView(viewKanban)
		return m, nil
	case keyVimUp, keyUp:
		if m.view == viewKanban {
			m.kanbanRow--
			if m.kanbanRow < 0 {
				m.kanbanRow = 0
			}
			return m, nil
		}
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail || m.view == viewMemoryDetail {
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
	case keyKanbanLeft, keyLeft: // "h" or left-arrow: kanban column left (kanban only)
		if m.view == viewKanban {
			m.kanbanCol--
			if m.kanbanCol < 0 {
				m.kanbanCol = 0
			}
			m.clampKanbanRow()
		}
		return m, nil
	case keyKanbanRight: // right-arrow only: kanban column right ("l" alias removed)
		if m.view == viewKanban {
			m.kanbanCol++
			if m.kanbanCol >= len(kanbanColumns) {
				m.kanbanCol = len(kanbanColumns) - 1
			}
			m.clampKanbanRow()
		}
		return m, nil
	case keyEnter:
		return m.handleEnter()
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewAgents:
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			a := agents[m.cursor]
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailScroll = 0
			m.agentOutputCache = nil
			m.agentOutputID = a.ID
			if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
				return m, agentOutputCmd(m.backend, m.loomRoot, a.ID)
			}
		}
	case viewAgentDetail:
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			a := agents[m.cursor]
			// ACP agents have no tmux pane — Enter is a no-op in detail view
			if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
				c := exec.Command("loom", "attach", "--", a.ID)
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
			wtPath := worktrees[m.cursor].Path
			m.cursors[m.view] = m.cursor
			m.selectedWorktree = m.cursor
			m.diffContent = ""
			m.diffLoading = true
			m.view = viewDiff
			m.diffScroll = 0
			m.cursor = 0
			return m, diffCmd(m.backend, wtPath)
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
			m.searchTI.SetValue("")
			for i, a := range m.data.Agents {
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
			for i, is := range m.data.Issues {
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

// composeSend validates the compose form fields and sends the message.
// Called by Ctrl+S shortcut or when the form reaches StateCompleted.
func (m Model) composeSend() (tea.Model, tea.Cmd) {
	m.composeMode = false
	if m.composeData.To == "" {
		return m, m.setFlash("Send failed: 'To' is required", true)
	}
	if m.composeData.Subject == "" {
		return m, m.setFlash("Send failed: 'Subject' is required", true)
	}
	return m, sendMailCmd(m.backend, m.loomRoot, "dashboard",
		m.composeData.To, m.composeData.Subject, m.composeData.Body,
		m.composeData.Type, m.composeData.Priority, "")
}

func (m Model) openCompose(replyTo string) (tea.Model, tea.Cmd) {
	var ids []string
	for _, a := range m.data.Agents {
		ids = append(ids, a.ID)
	}
	cd := &composeData{}
	m.composeData = cd
	m.composeForm = newComposeForm(cd, ids, replyTo).WithWidth(min(56, m.width-8))
	m.composeMode = true
	return m, m.composeForm.Init()
}

func (m *Model) clampCursor() {
	max := m.listLen() - 1
	if max < 0 {
		max = 0
	}
	if m.cursor > max {
		m.cursor = max
	}
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
	if m.diffScroll < 0 {
		m.diffScroll = 0
	}
}

func (m Model) listLen() int {
	switch m.view {
	case viewAgents, viewAgentDetail:
		return len(m.filteredAgents())
	case viewIssues, viewIssueDetail:
		return len(m.filteredIssues())
	case viewMail, viewMailDetail:
		return len(m.filteredMessages())
	case viewMemory, viewMemoryDetail:
		return len(m.filteredMemories())
	case viewWorktrees:
		return len(m.filteredWorktrees())
	case viewActivity:
		return len(m.filteredActivity())
	case viewLogs:
		return len(m.filteredLogLines())
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

	// Full-width title bar. titleStyle has Padding(0,2) adding 4 horizontal
	// chars, so content area is m.width-4 to fill the terminal exactly.
	left := " ◈ LOOM DASHBOARD"
	right := fmt.Sprintf("%d agents  %d unread ", len(m.data.Agents), m.data.Unread)
	contentW := m.width - 4
	padding := contentW - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		padding = 1
	}
	titleBar := titleStyle.Width(m.width).Render(left + strings.Repeat(" ", padding) + right)

	help := m.helpBar()
	if m.searchMode {
		searchBox := searchBoxStyle
		help = searchBox.Render("/ "+m.searchTI.View()) + helpStyle.Render("  [Enter]filter [Esc]cancel")
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
		help = helpStyle.Render(fmt.Sprintf(" Message %s: %s  [Enter]send [Esc]cancel", agentName, m.messageTI.View()))
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

	// Daemon unavailability banner (shown after first refresh, between title and content)
	daemonBanner := ""
	if m.refreshed && !m.data.DaemonOK {
		daemonBanner = flashErrStyle.Render(" ⚠ daemon restarting — reconnecting...")
	}

	// Build final output
	var output string
	if daemonBanner != "" && flashLine != "" {
		output = fmt.Sprintf("%s\n%s\n%s\n%s\n%s", titleBar, daemonBanner, content, flashLine, help)
	} else if daemonBanner != "" {
		output = fmt.Sprintf("%s\n%s\n%s\n%s", titleBar, daemonBanner, content, help)
	} else if flashLine != "" {
		output = fmt.Sprintf("%s\n%s\n%s\n%s", titleBar, content, flashLine, help)
	} else {
		output = fmt.Sprintf("%s\n%s\n%s", titleBar, content, help)
	}

	// Compose modal overlay replaces normal output.
	if m.composeMode && m.composeForm != nil {
		output = renderComposeOverlay(m.composeForm, m.width, m.height)
	}

	// Full-screen background fill
	lines := splitLines(output)
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i, l := range lines {
		w := lipgloss.Width(l)
		if w < m.width {
			lines[i] = l + strings.Repeat(" ", m.width-w)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) helpBar() string {
	base := " " + m.help.View(m.keys)

	var ctx string
	switch m.view {
	case viewAgentDetail:
		ctx = "[n]udge [m]essage [j/k]scroll"
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			a := agents[m.cursor]
			if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
				ctx += " [Enter]attach"
			}
		}
	case viewAgents:
		ctx = "[n]udge [m]essage [o]utput [x]kill [Enter]detail"
	case viewKanban:
		ctx = "[h/←/→]column [j/k]row [Enter]detail"
	case viewIssues:
		ctx = "[Enter]detail"
	case viewMail:
		ctx = "[c]ompose [Enter]detail"
	case viewWorktrees:
		ctx = "[Enter]diff"
	case viewMemory:
		ctx = "[Enter]detail"
	case viewLogs:
		ctx = "[/]search [f]ilter [F]agent"
	case viewActivity:
		ctx = "[Enter]agent"
	case viewIssueDetail, viewMemoryDetail:
		ctx = "[j/k]scroll"
	case viewMailDetail:
		ctx = "[c]ompose [r]eply [j/k]scroll"
	case viewDiff:
		ctx = "[j/k]scroll"
	}

	if ctx != "" {
		return base + " │ " + helpStyle.Render(ctx)
	}
	return base
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	_, y := msg.X, msg.Y

	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail || m.view == viewMemoryDetail {
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
		if m.view == viewAgentDetail || m.view == viewMailDetail || m.view == viewIssueDetail || m.view == viewMemoryDetail {
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

func isSearchableView(v view) bool {
	return isListView(v) || v == viewLogs
}

// mouseToListIndex converts a screen Y coordinate to a list item index,
// accounting for the viewport scroll offset so clicks target the correct item.
func (m Model) mouseToListIndex(y int) int {
	idx := y - listHeaderRows
	if idx < 0 {
		return -1
	}
	if m.view == viewIssues {
		return m.adjustIssuesIndex(idx)
	}
	vRows := visibleRows(m.height, 9)
	start, _ := listViewport(m.cursor, m.listLen(), vRows)
	return start + idx
}

// adjustIssuesIndex converts a screen-relative row index to an absolute
// display item index, accounting for the viewport scroll offset and the
// separator lines (blank + "RECENTLY DONE" + separator) between active
// and done sections.
func (m Model) adjustIssuesIndex(idx int) int {
	display := m.filteredIssues()
	activeCount := 0
	for _, iss := range display {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeCount++
		}
	}
	vRows := visibleRows(m.height, 9)
	start, end := issuesViewport(m.cursor, len(display), vRows, activeCount)

	// If the separator is visible in the current viewport, adjust for it.
	if activeCount < len(display) && activeCount >= start && activeCount < end {
		sepScreenPos := activeCount - start
		if idx >= sepScreenPos && idx < sepScreenPos+issuesSectionGap {
			return -1
		}
		if idx >= sepScreenPos+issuesSectionGap {
			return start + idx - issuesSectionGap
		}
	}
	return start + idx
}

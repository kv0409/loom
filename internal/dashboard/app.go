package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/dashboard/backend"
	"github.com/karanagi/loom/internal/issue"
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

var viewOrder = []view{viewOverview, viewAgents, viewIssues, viewMail, viewMemory, viewWorktrees}

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
	loomRoot             string
	view                 view
	data                 backend.Snapshot
	cursor               int
	cursors              map[view]int // per-view cursor positions
	width                int
	height               int
	nudgeMode            bool
	nudgeCursor          int
	messageMode          bool
	messageTI            textinput.Model
	killConfirm          bool
	selectedWorktreeName string
	diffContent          string
	kanbanCol            int // selected column in kanban view
	kanbanRow            int // selected row within column
	backend              backend.Backend
	lastClickTime        time.Time
	lastClickRow         int
	detailVP             viewport.Model
	diffVP               viewport.Model
	detailYOff           int  // desired Y offset for detail viewport
	diffYOff             int  // desired Y offset for diff viewport
	diffXOff             int  // desired X offset for diff viewport
	detailAutoScroll     bool // auto-scroll agent detail output to bottom
	flashMsg             string
	flashIsErr           bool
	searchMode           bool
	searchTI             textinput.Model
	help                 help.Model
	keys                 keyMap
	reloading            bool // set when quitting due to binary hot-reload
	refreshed            bool // set after first data message received
	composeMode          bool
	composeForm          *huh.Form
	composeData          *composeData
	issueComposeMode     bool
	issueComposeForm     *huh.Form
	issueComposeData     *issueComposeData
	chatMode             bool               // true when chat pane is open
	chatTI               textinput.Model    // text input for chat pane
	chatYOff             int                // scroll offset for chat history
	agentOutputCache     []backend.ACPEvent // cached events for current agent detail
	agentOutputID        string             // agent ID the cache belongs to
	diffLoading          bool               // true while diff is being fetched
	errorShown           bool               // set after first error flash to avoid repeating
	heartbeatTimeoutSec  int                // from config; used for countdown donut
	spinner              spinner.Model      // animated spinner for loading states
	quitConfirmMode      bool               // true when quit confirmation dialog is shown
	stopRequested        bool               // true when user chose "stop session + quit"
	proposalCursor       int                // selected proposal in overview proposals panel
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

type proposalResultMsg struct {
	flash string
	isErr bool
}

type createIssueResultMsg struct {
	flash string
	isErr bool
}

func clearFlashAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearFlashMsg{} })
}

func New(loomRoot string, heartbeatTimeoutSec int) Model {
	h := help.New()
	h.Styles.ShortKey = helpStyle.Bold(true)
	h.Styles.ShortDesc = helpStyle
	h.Styles.ShortSeparator = helpStyle

	msgTI := textinput.New()
	msgTI.Prompt = ""

	searchTI := textinput.New()
	searchTI.Prompt = ""

	chatTI := textinput.New()
	chatTI.Prompt = "❯ "
	chatTI.Placeholder = "message orchestrator..."

	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle))

	dvp := viewport.New(viewport.WithWidth(78), viewport.WithHeight(18))
	dvp.MouseWheelEnabled = true

	dfvp := viewport.New(viewport.WithWidth(78), viewport.WithHeight(18))
	dfvp.MouseWheelEnabled = true
	dfvp.SetHorizontalStep(8)

	return Model{loomRoot: loomRoot, width: 80, height: 24, backend: backend.NewFileBackend(loomRoot), cursors: make(map[view]int), help: h, keys: defaultKeyMap(), messageTI: msgTI, searchTI: searchTI, chatTI: chatTI, heartbeatTimeoutSec: heartbeatTimeoutSec, spinner: sp, detailVP: dvp, diffVP: dfvp}
}

// Reloading reports whether the dashboard exited due to a binary hot-reload.
func (m Model) Reloading() bool { return m.reloading }

// StopRequested reports whether the user chose to stop the loom session on quit.
func (m Model) StopRequested() bool { return m.stopRequested }

// switchView saves the current cursor position and switches to the target view,
// restoring its previously saved cursor position.
func (m *Model) switchView(target view) {
	m.cursors[m.view] = m.cursor
	m.view = target
	m.cursor = m.cursors[target]
	m.searchTI.SetValue("")
	m.searchMode = false
}

func (m *Model) setFlash(msg string, isErr bool) tea.Cmd {
	m.flashMsg = msg
	m.flashIsErr = isErr
	return clearFlashAfter(3 * time.Second)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd(), watchBinary(), m.spinner.Tick)
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

func (m Model) restartDaemon() (tea.Model, tea.Cmd) {
	lr := m.loomRoot
	return m, daemonCmd(lr, func() error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding executable: %w", err)
		}
		logPath := filepath.Join(lr, "logs", "daemon.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening daemon log: %w", err)
		}
		child := exec.Command(self, "start", "--resume")
		child.Env = append(os.Environ(), "LOOM_DAEMON=1")
		child.Stdout = logFile
		child.Stderr = logFile
		if err := child.Start(); err != nil {
			logFile.Close()
			return fmt.Errorf("starting daemon: %w", err)
		}
		logFile.Close()
		return nil
	}, "Daemon starting...")
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
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
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
			m.help.SetWidth(msg.Width)
			vpW := panelWidth(msg.Width) - 2
			vpH := scrollViewport(msg.Height)
			m.detailVP.SetWidth(vpW)
			m.detailVP.SetHeight(vpH)
			m.diffVP.SetWidth(vpW)
			m.diffVP.SetHeight(vpH)
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
			if msg.HeartbeatTimeoutSec > 0 {
				m.heartbeatTimeoutSec = msg.HeartbeatTimeoutSec
			}
			m.clampCursor()
			if m.view != viewAgentDetail && m.view != viewIssueDetail && m.view != viewMemoryDetail && m.view != viewMailDetail {
				m.detailYOff = 0
			}
			if m.view != viewDiff {
				m.diffYOff = 0
				m.diffXOff = 0
			}
			m.applyAutoScroll()
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
				m.applyAutoScroll()
			}
		case sendMailResultMsg:
			return m, m.setFlash(msg.flash, msg.isErr)
		}
		return m, cmd
	}

	// Issue compose modal captures all input while active.
	if m.issueComposeMode && m.issueComposeForm != nil {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "ctrl+s":
				return m.issueComposeSubmit()
			case "esc":
				m.issueComposeMode = false
				return m, m.setFlash("Create cancelled", false)
			}
		}

		form, cmd := m.issueComposeForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.issueComposeForm = f
		}
		switch m.issueComposeForm.State {
		case huh.StateCompleted:
			return m.issueComposeSubmit()
		case huh.StateAborted:
			m.issueComposeMode = false
			return m, m.setFlash("Create cancelled", false)
		}
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.help.SetWidth(msg.Width)
			vpW := panelWidth(msg.Width) - 2
			vpH := scrollViewport(msg.Height)
			m.detailVP.SetWidth(vpW)
			m.detailVP.SetHeight(vpH)
			m.diffVP.SetWidth(vpW)
			m.diffVP.SetHeight(vpH)
			m.issueComposeForm.WithWidth(min(56, msg.Width-8))
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
			if msg.HeartbeatTimeoutSec > 0 {
				m.heartbeatTimeoutSec = msg.HeartbeatTimeoutSec
			}
			m.clampCursor()
			if m.view != viewAgentDetail && m.view != viewIssueDetail && m.view != viewMemoryDetail && m.view != viewMailDetail {
				m.detailYOff = 0
			}
			if m.view != viewDiff {
				m.diffYOff = 0
				m.diffXOff = 0
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
		case createIssueResultMsg:
			return m, m.setFlash(msg.flash, msg.isErr)
		case sendMailResultMsg:
			return m, m.setFlash(msg.flash, msg.isErr)
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		m.chatTI.SetWidth(panelWidth(msg.Width) - 6)
		vpW := panelWidth(msg.Width) - 2
		vpH := scrollViewport(msg.Height)
		m.detailVP.SetWidth(vpW)
		m.detailVP.SetHeight(vpH)
		m.diffVP.SetWidth(vpW)
		m.diffVP.SetHeight(vpH)
		return m, nil
	case tickMsg:
		cmds := []tea.Cmd{m.refresh(), tickCmd()}
		if m.view == viewAgentDetail {
			agents := m.filteredAgents()
			if m.cursor < len(agents) {
				a := agents[m.cursor]
				cmds = append(cmds, agentOutputCmd(m.backend, m.loomRoot, a.ID))
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
		if msg.HeartbeatTimeoutSec > 0 {
			m.heartbeatTimeoutSec = msg.HeartbeatTimeoutSec
		}
		m.clampCursor()
		if len(m.data.Proposals) > 0 && m.proposalCursor >= len(m.data.Proposals) {
			m.proposalCursor = len(m.data.Proposals) - 1
		}
		if m.view != viewAgentDetail && m.view != viewIssueDetail && m.view != viewMemoryDetail && m.view != viewMailDetail {
			m.detailYOff = 0
		}
		if m.view != viewDiff {
			m.diffYOff = 0
			m.diffXOff = 0
		}
		m.applyAutoScroll()
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
			m.applyAutoScroll()
		}
		return m, nil
	case sendMailResultMsg:
		return m, m.setFlash(msg.flash, msg.isErr)
	case createIssueResultMsg:
		return m, m.setFlash(msg.flash, msg.isErr)
	case proposalResultMsg:
		return m, m.setFlash(msg.flash, msg.isErr)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Forward to active textinput
	var cmd tea.Cmd
	if m.messageMode {
		m.messageTI, cmd = m.messageTI.Update(msg)
	} else if m.searchMode {
		m.searchTI, cmd = m.searchTI.Update(msg)
	} else if m.chatMode {
		m.chatTI, cmd = m.chatTI.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Quit confirmation dialog captures all input
	if m.quitConfirmMode {
		switch msg.String() {
		case keyQuitConfirmStop:
			m.stopRequested = true
			return m, tea.Quit
		case keyQuitConfirmQuit:
			return m, tea.Quit
		case keyQuitConfirmCancel:
			m.quitConfirmMode = false
		default:
			m.quitConfirmMode = false
		}
		return m, nil
	}

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
		case "down":
			if m.nudgeCursor < len(nudge.Types)-1 {
				m.nudgeCursor++
			}
		case "up":
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

	// Chat mode captures all input
	if m.chatMode {
		switch msg.String() {
		case "enter":
			text := m.chatTI.Value()
			if text != "" {
				m.chatTI.SetValue("")
				return m, sendMailCmd(m.backend, m.loomRoot, "dashboard", "orchestrator", text, "", "task", "normal", "")
			}
		case "esc":
			m.chatMode = false
			m.chatTI.Blur()
		default:
			var cmd tea.Cmd
			m.chatTI, cmd = m.chatTI.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch msg.String() {
	case keyQuit, keyQuitCtrl:
		m.quitConfirmMode = true
		return m, nil
	case keyEsc:
		switch m.view {
		case viewAgentDetail:
			m.switchView(viewAgents)
		case viewIssueDetail:
			m.switchView(viewIssues)
		case viewMemoryDetail:
			m.switchView(viewMemory)
		case viewMailDetail:
			m.switchView(viewMail)
		case viewDiff:
			m.view = viewWorktrees
			m.cursor = 0
			for i, wt := range m.filteredWorktrees() {
				if wt.Name == m.selectedWorktreeName {
					m.cursor = i
					break
				}
			}
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
	case "m": // message-compose in agents/agent-detail
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.data.Agents) > 0 {
			m.messageMode = true
			m.messageTI.SetValue("")
			m.messageTI.Focus()
			return m, nil
		}
		return m, nil
	case keyViewMemory:
		m.switchView(viewMemory)
		return m, nil
	case keyViewMail:
		m.switchView(viewMail)
		return m, nil
	case keyViewWorktrees:
		m.switchView(viewWorktrees)
		return m, nil
	case keyAgentNudge:
		if (m.view == viewAgents || m.view == viewAgentDetail) && len(m.filteredAgents()) > 0 {
			m.nudgeMode = true
			m.nudgeCursor = 0
			return m, nil
		}
	case keyAgentKill: // also keyProposalDismiss (same key, different view)
		if m.view == viewOverview && len(m.data.Proposals) > 0 {
			return m.respondProposal("dismissed")
		}
		if m.view == viewAgents && m.cursor < len(m.filteredAgents()) {
			m.killConfirm = true
			return m, nil
		}
	case keyProposalAccept:
		if m.view == viewOverview && len(m.data.Proposals) > 0 {
			return m.respondProposal("accepted")
		}
	case keyProposalReject:
		if m.view == viewOverview && len(m.data.Proposals) > 0 {
			return m.respondProposal("rejected")
		}
	case keyMailCompose: // also keyIssueCreate (same key, different view)
		if m.view == viewMail || m.view == viewMailDetail {
			return m.openCompose("")
		}
		if m.view == viewIssues {
			return m.openIssueCompose()
		}
	case keyRestart: // also keyMailReply (same key "r")
		if m.refreshed && !m.data.DaemonOK {
			return m.restartDaemon()
		}
		if m.view == viewMailDetail {
			messages := m.sortedMessages()
			if m.cursor < len(messages) {
				return m.openCompose(messages[m.cursor].From)
			}
		}
	case keyAgentOutput:
		if m.view == viewAgents && m.cursor < len(m.filteredAgents()) {
			a := m.filteredAgents()[m.cursor]
			m.cursors[m.view] = m.cursor
			m.view = viewAgentDetail
			m.detailYOff = 0
			m.detailAutoScroll = true
			m.agentOutputCache = nil
			m.agentOutputID = a.ID
			return m, agentOutputCmd(m.backend, m.loomRoot, a.ID)
		}
	case keySearch:
		if isSearchableView(m.view) {
			m.searchMode = true
			m.searchTI.SetValue("")
			m.searchTI.Focus()
			return m, nil
		}
	case keyChat:
		m.chatMode = true
		m.chatTI.SetValue("")
		m.chatTI.Focus()
		m.chatYOff = 0
		return m, nil
	case keyTab:
		m.switchView(nextView(m.view))
		return m, nil
	case keyDown:
		switch m.view {
		case viewOverview:
			if len(m.data.Proposals) > 0 && m.proposalCursor < len(m.data.Proposals)-1 {
				m.proposalCursor++
			}
			return m, nil
		case viewAgentDetail, viewIssueDetail, viewMemoryDetail, viewMailDetail:
			if m.view == viewAgentDetail {
				m.detailAutoScroll = false
			}
			m.detailYOff++
			return m, nil
		case viewDiff:
			m.diffYOff++
			return m, nil
		}
		m.cursor++
		m.clampCursor()
		return m, nil
	case keyUp:
		switch m.view {
		case viewOverview:
			if m.proposalCursor > 0 {
				m.proposalCursor--
			}
			return m, nil
		case viewAgentDetail, viewIssueDetail, viewMemoryDetail, viewMailDetail:
			if m.view == viewAgentDetail {
				m.detailAutoScroll = false
			}
			m.detailYOff--
			if m.detailYOff < 0 {
				m.detailYOff = 0
			}
			return m, nil
		case viewDiff:
			m.diffYOff--
			if m.diffYOff < 0 {
				m.diffYOff = 0
			}
			return m, nil
		}
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil
	case keyGoBottom:
		if m.view == viewAgentDetail {
			m.detailAutoScroll = true
			m.applyAutoScroll()
			return m, nil
		}
	case "h", keyLeft: // diff hscroll left
		if m.view == viewDiff {
			m.diffXOff -= 8
			if m.diffXOff < 0 {
				m.diffXOff = 0
			}
		}
		return m, nil
	case keyRight: // diff hscroll right
		if m.view == viewDiff {
			m.diffXOff += 8
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
			m.detailYOff = 0
			m.detailAutoScroll = true
			m.agentOutputCache = nil
			m.agentOutputID = a.ID
			return m, agentOutputCmd(m.backend, m.loomRoot, a.ID)
		}
	case viewAgentDetail:
		// ACP-only: no tmux pane to attach to
	case viewIssues:
		if len(m.filteredIssues()) > 0 {
			m.cursors[m.view] = m.cursor
			m.view = viewIssueDetail
			m.detailYOff = 0
		}
	case viewWorktrees:
		worktrees := m.filteredWorktrees()
		if m.cursor < len(worktrees) {
			wt := worktrees[m.cursor]
			m.cursors[m.view] = m.cursor
			m.selectedWorktreeName = wt.Name
			m.diffContent = ""
			m.diffLoading = true
			m.view = viewDiff
			m.diffYOff = 0
			m.diffXOff = 0
			m.cursor = 0
			return m, diffCmd(m.backend, wt.Path)
		}
	case viewMemory:
		memories := m.filteredMemories()
		if m.cursor < len(memories) {
			m.cursors[m.view] = m.cursor
			m.view = viewMemoryDetail
			m.detailYOff = 0
		}
	case viewMail:
		messages := m.sortedMessages()
		if m.cursor < len(messages) {
			m.cursors[m.view] = m.cursor
			m.view = viewMailDetail
			m.detailYOff = 0
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

func (m Model) openIssueCompose() (tea.Model, tea.Cmd) {
	var ids []string
	for _, iss := range m.data.Issues {
		ids = append(ids, iss.ID)
	}
	cd := &issueComposeData{}
	m.issueComposeData = cd
	m.issueComposeForm = newIssueForm(cd, ids).WithWidth(min(56, m.width-8))
	m.issueComposeMode = true
	return m, m.issueComposeForm.Init()
}

func (m Model) issueComposeSubmit() (tea.Model, tea.Cmd) {
	m.issueComposeMode = false
	if strings.TrimSpace(m.issueComposeData.Title) == "" {
		return m, m.setFlash("Create failed: 'Title' is required", true)
	}
	cd := m.issueComposeData
	lr := m.loomRoot
	var deps []string
	if cd.DependsOn != "" {
		for _, d := range strings.Split(cd.DependsOn, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				deps = append(deps, d)
			}
		}
	}
	return m, func() tea.Msg {
		iss, err := issue.Create(lr, cd.Title, issue.CreateOpts{
			Type:        cd.Type,
			Priority:    cd.Priority,
			Description: cd.Description,
			Parent:      cd.Parent,
			DependsOn:   deps,
		})
		if err != nil {
			return createIssueResultMsg{flash: fmt.Sprintf("Create failed: %s", err), isErr: true}
		}
		daemon.RefreshBestEffort(lr, daemon.RefreshOpts{IssueIDs: []string{iss.ID}})
		return createIssueResultMsg{flash: fmt.Sprintf("Created %s", iss.ID), isErr: false}
	}
}

func (m Model) respondProposal(action string) (tea.Model, tea.Cmd) {
	if m.proposalCursor >= len(m.data.Proposals) {
		return m, nil
	}
	p := m.data.Proposals[m.proposalCursor]
	b := m.backend
	lr := m.loomRoot
	id := p.ID
	return m, func() tea.Msg {
		if err := b.RespondProposal(lr, id, action, ""); err != nil {
			return proposalResultMsg{flash: err.Error(), isErr: true}
		}
		return proposalResultMsg{flash: fmt.Sprintf("Proposal %s %s", id, action), isErr: false}
	}
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
	case viewMemory, viewMemoryDetail:
		return len(m.filteredMemories())
	case viewMail, viewMailDetail:
		return len(m.sortedMessages())
	case viewWorktrees:
		return len(m.filteredWorktrees())
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

func (m Model) View() tea.View {
	// Minimum terminal size guard
	if m.width < minTermWidth || m.height < minTermHeight {
		msg := fmt.Sprintf("Terminal too small (%d×%d)\nNeed at least %d×%d", m.width, m.height, minTermWidth, minTermHeight)
		v := tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg))
		v.AltScreen = true
		v.WindowTitle = "Loom Dashboard"
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	// When chat pane is active, reduce height for render functions only.
	// Save original for renderChatPane(), padding calc, and final truncation.
	fullHeight := m.height
	if m.chatMode {
		m.height = fullHeight - chatPaneHeight(fullHeight)
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
	case viewMemory:
		content = m.renderMemory()
	case viewMemoryDetail:
		content = m.renderMemoryDetail()
	case viewMail:
		content = m.renderMail()
	case viewMailDetail:
		content = m.renderMailDetail()
	case viewWorktrees:
		content = m.renderWorktrees()
	case viewDiff:
		content = m.renderDiff()
	case viewKanban:
		content = m.renderKanban()
	}

	// Restore original height for chat pane, padding, and truncation.
	m.height = fullHeight

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
	if m.chatMode {
		help = helpStyle.Render(" Chat: [Enter]send [Esc]close")
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
		reason := m.data.ShutdownReason
		var bannerText string
		switch reason {
		case backend.ShutdownIdle:
			bannerText = " ⚠ Daemon stopped: idle timeout (press r to restart)"
		case backend.ShutdownSignal:
			bannerText = " ⚠ Daemon stopped: received SIGTERM (press r to restart)"
		default:
			bannerText = " ⚠ Daemon stopped: unexpected exit (press r to restart)"
		}
		daemonBanner = flashErrStyle.Render(bannerText)
	}

	// Build top section (title + optional banner + content)
	top := titleBar + "\n"
	if daemonBanner != "" {
		top += daemonBanner + "\n"
	}
	top += content

	// Chat pane: append below main content when active
	chatPane := ""
	if m.chatMode {
		chatPane = m.renderChatPane()
	}

	// Build bottom section (optional flash + help), pinned to terminal bottom
	bottom := ""
	if flashLine != "" {
		bottom = flashLine + "\n"
	}
	bottom += help

	// Pad between top and bottom so help bar sits at the last line
	topLines := splitLines(top)
	chatLines := splitLines(chatPane)
	bottomLines := splitLines(bottom)
	totalUsed := len(topLines) + len(chatLines) + len(bottomLines)
	pad := m.height - totalUsed
	if pad < 0 {
		pad = 0
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, topLines...)
	for i := 0; i < pad; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, chatLines...)
	lines = append(lines, bottomLines...)

	// Compose modal overlay replaces normal output.
	if m.composeMode && m.composeForm != nil {
		output := renderComposeOverlay(m.composeForm, m.width, m.height)
		lines = splitLines(output)
		for len(lines) < m.height {
			lines = append(lines, "")
		}
	}

	// Issue compose modal overlay replaces normal output.
	if m.issueComposeMode && m.issueComposeForm != nil {
		output := renderIssueComposeOverlay(m.issueComposeForm, m.width, m.height)
		lines = splitLines(output)
		for len(lines) < m.height {
			lines = append(lines, "")
		}
	}

	// Message overlay replaces normal output.
	if m.messageMode {
		agentName := ""
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			agentName = agents[m.cursor].ID
		}
		output := renderMessageOverlay(agentName, m.messageTI, m.width, m.height)
		lines = splitLines(output)
		for len(lines) < m.height {
			lines = append(lines, "")
		}
	}

	// Nudge overlay replaces normal output.
	if m.nudgeMode {
		agentName := ""
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			agentName = agents[m.cursor].ID
		}
		var labels []string
		for _, nt := range nudge.Types {
			labels = append(labels, nt.Label)
		}
		output := renderNudgeOverlay(agentName, labels, m.nudgeCursor, m.width, m.height)
		lines = splitLines(output)
		for len(lines) < m.height {
			lines = append(lines, "")
		}
	}

	// Quit confirmation overlay replaces normal output.
	if m.quitConfirmMode {
		output := renderQuitConfirmOverlay(m.width, m.height)
		lines = splitLines(output)
		for len(lines) < m.height {
			lines = append(lines, "")
		}
	}

	// Kill confirmation overlay replaces normal output.
	if m.killConfirm {
		agentName := ""
		agents := m.filteredAgents()
		if m.cursor < len(agents) {
			agentName = agents[m.cursor].ID
		}
		output := renderKillConfirmOverlay(agentName, m.width, m.height)
		lines = splitLines(output)
		for len(lines) < m.height {
			lines = append(lines, "")
		}
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
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.WindowTitle = "Loom Dashboard"
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) helpBar() string {
	base := " " + m.help.View(m.keys)

	var ctx string
	switch m.view {
	case viewOverview:
		if len(m.data.Proposals) > 0 {
			ctx = "[y]accept [R]eject [x]dismiss [j/k]select"
		}
	case viewAgentDetail:
		ctx = "[n]udge [m]essage [j/k]scroll [G]bottom"
	case viewAgents:
		ctx = "[n]udge [m]essage [o]utput [x]kill [Enter]detail"
	case viewIssues:
		ctx = "[c]reate [Enter]detail"
	case viewWorktrees:
		ctx = "[Enter]diff"
	case viewMemory:
		ctx = "[Enter]detail"
	case viewMail:
		ctx = "[c]ompose [Enter]detail"
	case viewMailDetail:
		ctx = "[c]ompose [r]eply [j/k]scroll"
	case viewIssueDetail, viewMemoryDetail:
		ctx = "[j/k]scroll"
	case viewDiff:
		ctx = "[j/k]scroll [←/→]hscroll"
	}

	if ctx != "" {
		return base + " │ " + helpStyle.Render(ctx)
	}
	return base
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewAgentDetail, viewIssueDetail, viewMemoryDetail, viewMailDetail:
		if m.view == viewAgentDetail {
			m.detailAutoScroll = false
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			m.detailYOff--
			if m.detailYOff < 0 {
				m.detailYOff = 0
			}
		case tea.MouseWheelDown:
			m.detailYOff++
		}
		return m, nil
	case viewDiff:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.diffYOff--
			if m.diffYOff < 0 {
				m.diffYOff = 0
			}
		case tea.MouseWheelDown:
			m.diffYOff++
		}
		return m, nil
	}

	switch msg.Button {
	case tea.MouseWheelUp:
		m.cursor--
		if m.cursor < 0 {
			m.cursor = 0
		}
	case tea.MouseWheelDown:
		m.cursor++
		m.clampCursor()
	}
	return m, nil
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	y := msg.Y

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

	// Diff/logs: click scrolls viewport
	if m.view == viewDiff {
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

func isSearchableView(v view) bool {
	return isListView(v)
}

// applyAutoScroll sets detailYOff to the bottom of the output viewport
// when detailAutoScroll is true and we're viewing agent detail.
func (m *Model) applyAutoScroll() {
	if !m.detailAutoScroll || m.view != viewAgentDetail {
		return
	}
	agents := m.filteredAgents()
	if m.cursor >= len(agents) {
		return
	}
	a := agents[m.cursor]
	outputLines := m.renderAgentOutput(a, detailContentWidth(m.width))
	headerLines := m.renderAgentHeader(a)
	footerLines := m.renderAgentFooter(a)
	vpH := agentDetailVPHeight(m.height, len(headerLines), len(footerLines))
	if vpH < 1 {
		vpH = 1
	}
	maxOff := len(outputLines) - vpH
	if maxOff < 0 {
		maxOff = 0
	}
	m.detailYOff = maxOff
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

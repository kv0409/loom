package dashboard

// Key binding constants — single source of truth for all keyboard shortcuts.
//
// Shared keys are active in every view. View-specific keys only apply when
// the named view is active. Where a key would conflict between shared and
// view-specific use, the view-specific handler takes precedence (checked
// first inside handleKey).

// Shared / global shortcuts (always active outside modal modes).
const (
	keyQuit     = "q"
	keyQuitCtrl = "ctrl+c"
	keyEsc      = "esc"
	keyTab      = "tab"
	keyEnter    = "enter"
	keySearch   = "/"
	keyDown     = "down"
	keyUp       = "up"
	keyLeft     = "left"
	keyRight    = "right"
	keyVimDown  = "j"
	keyVimUp    = "k"

	// View-switch shortcuts (shared, work from any non-modal view).
	keyViewOverview  = "0"
	keyViewOverview2 = "H"
	keyViewAgents    = "a"
	keyViewIssues    = "i"
	// keyViewMail: "m" is shared ONLY when not in agents/agent-detail view.
	// In agents/agent-detail, "m" opens the message-compose modal instead.
	// See keyAgentMessage below.
	keyViewMail     = "m"
	keyViewMemory   = "d"
	keyViewWorktrees = "w"
	keyViewActivity = "t"
	// keyViewLogs: "l" is shared ONLY when not in kanban view.
	// In kanban, right-column navigation uses keyRight / keyKanbanRight instead.
	// The old "l" alias for kanban-right is intentionally removed to eliminate
	// the conflict documented in DISC-008 / DISC-023 / DISC-042.
	keyViewLogs = "l"
	keyViewKanban = "b"
)

// View-specific shortcuts — only meaningful in the named context.
const (
	// Agents / agent-detail view.
	keyAgentNudge    = "n"
	keyAgentOutput   = "o"
	keyAgentKill     = "x"
	keyAgentMessage  = "m" // shadows keyViewMail when in agents/agent-detail

	// Logs view.
	keyLogsFilter      = "f"
	keyLogsAgentFilter = "F"

	// Kanban view.
	// Left column: h or left-arrow.  Right column: right-arrow ONLY (not "l").
	// This resolves the l=logs vs l=kanban-right conflict.
	keyKanbanLeft  = "h"     // also keyLeft (arrow)
	keyKanbanRight = "right" // arrow only — "l" is NOT a kanban-right alias
)

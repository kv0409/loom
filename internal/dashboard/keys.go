package dashboard

import (
	"charm.land/bubbles/v2/key"
)

// keyMap implements help.KeyMap for the bubbles/help widget.
type keyMap struct {
	Tab         key.Binding
	Search      key.Binding
	Chat        key.Binding
	Esc         key.Binding
	Quit        key.Binding
	GoBottom    key.Binding
	IssueCreate key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Tab:         key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle")),
		Search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Chat:        key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "chat")),
		Esc:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		GoBottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		IssueCreate: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create issue")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Search, k.Chat, k.Esc, k.Quit}
}
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

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


	// Chat pane.
	keyChat = ":"

	// View-switch shortcuts (shared, work from any non-modal view).
	keyViewOverview  = "0"
	keyViewOverview2 = "H"
	keyViewAgents    = "a"
	keyViewIssues    = "i"
	keyViewMail      = "M"
	keyViewMemory    = "d"
	keyViewWorktrees = "w"
)

// View-specific shortcuts — only meaningful in the named context.
const (
	// Agents / agent-detail view.
	keyAgentNudge  = "n"
	keyAgentOutput = "o"
	keyAgentKill   = "x"

	// Mail view.
	keyMailCompose = "c"
	keyMailReply   = "r"

	// Issues view.
	keyIssueCreate = "c"

	// Detail views.
	keyGoBottom = "G"
)

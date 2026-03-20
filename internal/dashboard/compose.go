package dashboard

import (
	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// composeData holds the form field values bound to the huh form.
type composeData struct {
	To       string
	Subject  string
	Type     string
	Priority string
	Body     string
}

// mailTypes are the allowed message types for the compose form.
var mailTypes = []huh.Option[string]{
	huh.NewOption("status", "status"),
	huh.NewOption("completion", "completion"),
	huh.NewOption("blocker", "blocker"),
	huh.NewOption("question", "question"),
	huh.NewOption("task", "task"),
	huh.NewOption("review-request", "review-request"),
	huh.NewOption("review-result", "review-result"),
	huh.NewOption("escalation", "escalation"),
	huh.NewOption("nudge", "nudge"),
}

// mailPriorities are the allowed priority levels.
var mailPriorities = []huh.Option[string]{
	huh.NewOption("normal", "normal"),
	huh.NewOption("critical", "critical"),
	huh.NewOption("low", "low"),
}

// newComposeForm builds a huh.Form for composing a mail message.
// agentIDs provides autocomplete suggestions for the To field.
// replyTo pre-fills the To field when replying.
func newComposeForm(cd *composeData, agentIDs []string, replyTo string) *huh.Form {
	cd.To = replyTo
	cd.Type = "status"
	cd.Priority = "normal"

	f := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("To").
				Value(&cd.To).
				Placeholder("agent-id").
				Suggestions(agentIDs),
			huh.NewInput().
				Title("Subject").
				Value(&cd.Subject).
				Placeholder("subject line"),
			huh.NewSelect[string]().
				Title("Type").
				Options(mailTypes...).
				Value(&cd.Type),
			huh.NewSelect[string]().
				Title("Priority").
				Options(mailPriorities...).
				Value(&cd.Priority),
			huh.NewText().
				Title("Body").
				Value(&cd.Body).
				Placeholder("message body (optional)").
				Lines(4),
		),
	).WithTheme(loomTheme{}).WithKeyMap(composeKeyMap())

	return f
}

// composeKeyMap returns a custom huh KeyMap that rebinds AcceptSuggestion
// from the default ctrl+e to tab, so pressing Tab on an Input with suggestions
// both accepts the suggestion and advances to the next field.
func composeKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Input.AcceptSuggestion = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "complete"),
	)
	return km
}

// loomTheme implements huh.Theme for the Tokyo Night palette.
type loomTheme struct{}

func (loomTheme) Theme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)

	t.Focused.Base = lipgloss.NewStyle().
		PaddingLeft(1).
		BorderStyle(lipgloss.ThickBorder()).
		BorderLeft(true).
		BorderForeground(colBlue)
	t.Focused.Title = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	t.Focused.Description = lipgloss.NewStyle().Foreground(colGray)
	t.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(colRed).SetString(" *")
	t.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(colRed)
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(colBlue).SetString("> ")
	t.Focused.Option = lipgloss.NewStyle().Foreground(colFg)
	t.Focused.NextIndicator = lipgloss.NewStyle().Foreground(colGray).MarginLeft(1).SetString("→")
	t.Focused.PrevIndicator = lipgloss.NewStyle().Foreground(colGray).MarginRight(1).SetString("←")
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(colBlue)
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(colGray)
	t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(colFg)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(colBlue)
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Foreground(colBg).Background(colBlue).Bold(true).Padding(0, 2).MarginRight(1)
	t.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(colFg).Background(colSubtle).Padding(0, 2).MarginRight(1)
	t.Focused.Card = t.Focused.Base

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = lipgloss.NewStyle().Foreground(colGray)
	t.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(colGray)
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Help.ShortKey = lipgloss.NewStyle().Foreground(colGray)
	t.Help.ShortDesc = lipgloss.NewStyle().Foreground(colSubtle)
	t.Help.ShortSeparator = lipgloss.NewStyle().Foreground(colSubtle)

	return t
}

// renderComposeOverlay renders the compose form as a centered overlay.
func renderComposeOverlay(form *huh.Form, width, height int) string {
	formW := min(60, width-4)
	formView := form.View()

	composeTitle := composeTitleStyle.Render("✉ COMPOSE MESSAGE")

	content := lipgloss.JoinVertical(lipgloss.Left, composeTitle, formView)
	box := overlayStyle.Width(formW).Render(content)

	hint := composeHintStyle.Render("  ") +
		composeKeyStyle.Render("tab") + composeHintStyle.Render(" next/accept suggestion · ") +
		composeKeyStyle.Render("shift+tab") + composeHintStyle.Render(" prev · ") +
		composeKeyStyle.Render("ctrl+s") + composeHintStyle.Render(" send · ") +
		composeKeyStyle.Render("esc") + composeHintStyle.Render(" cancel")

	overlay := lipgloss.JoinVertical(lipgloss.Center, box, hint)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, overlay)
}

// issueComposeData holds the form field values bound to the issue compose form.
type issueComposeData struct {
	Title       string
	Type        string
	Priority    string
	Description string
	Parent      string
	DependsOn   string
}

// issueTypes are the allowed issue types.
var issueTypes = []huh.Option[string]{
	huh.NewOption("task", "task"),
	huh.NewOption("epic", "epic"),
	huh.NewOption("bug", "bug"),
	huh.NewOption("spike", "spike"),
}

// issuePriorities are the allowed priority levels for issues.
var issuePriorities = []huh.Option[string]{
	huh.NewOption("normal", "normal"),
	huh.NewOption("critical", "critical"),
	huh.NewOption("high", "high"),
	huh.NewOption("low", "low"),
}

// newIssueForm builds a huh.Form for creating an issue.
// issueIDs provides autocomplete suggestions for Parent and DependsOn fields.
func newIssueForm(cd *issueComposeData, issueIDs []string) *huh.Form {
	cd.Type = "task"
	cd.Priority = "normal"

	f := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Value(&cd.Title).
				Placeholder("issue title (required)"),
			huh.NewSelect[string]().
				Title("Type").
				Options(issueTypes...).
				Value(&cd.Type),
			huh.NewSelect[string]().
				Title("Priority").
				Options(issuePriorities...).
				Value(&cd.Priority),
			huh.NewText().
				Title("Description").
				Value(&cd.Description).
				Placeholder("description (optional)").
				Lines(6),
			huh.NewInput().
				Title("Parent").
				Value(&cd.Parent).
				Placeholder("parent issue ID (optional)").
				Suggestions(issueIDs),
			huh.NewInput().
				Title("Depends On").
				Value(&cd.DependsOn).
				Placeholder("comma-separated IDs (optional)").
				Suggestions(issueIDs),
		),
	).WithTheme(loomTheme{}).WithKeyMap(composeKeyMap())

	return f
}

// renderIssueComposeOverlay renders the issue compose form as a centered overlay.
func renderIssueComposeOverlay(form *huh.Form, width, height int) string {
	formW := min(60, width-4)
	formView := form.View()

	title := composeTitleStyle.Render("📋 CREATE ISSUE")

	content := lipgloss.JoinVertical(lipgloss.Left, title, formView)
	box := overlayStyle.Width(formW).Render(content)

	hint := composeHintStyle.Render("  ") +
		composeKeyStyle.Render("tab") + composeHintStyle.Render(" next/accept suggestion · ") +
		composeKeyStyle.Render("shift+tab") + composeHintStyle.Render(" prev · ") +
		composeKeyStyle.Render("ctrl+s") + composeHintStyle.Render(" create · ") +
		composeKeyStyle.Render("esc") + composeHintStyle.Render(" cancel")

	overlay := lipgloss.JoinVertical(lipgloss.Center, box, hint)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, overlay)
}

// renderQuitConfirmOverlay renders the quit confirmation dialog as a centered overlay.
func renderQuitConfirmOverlay(width, height int) string {
	formW := min(48, width-4)

	title := composeTitleStyle.Render("⏻ QUIT DASHBOARD")

	body := quitBodyStyle.Render("The loom session is still running.\nWhat would you like to do?")

	options := "\n" +
		composeKeyStyle.Render("[s]") + composeHintStyle.Render(" Stop session + quit") + "\n" +
		composeKeyStyle.Render("[q]") + composeHintStyle.Render(" Quit dashboard only") + "\n" +
		composeKeyStyle.Render("[esc]") + composeHintStyle.Render(" Cancel")

	content := lipgloss.JoinVertical(lipgloss.Left, title, body, options)
	box := overlayStyle.Width(formW).Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

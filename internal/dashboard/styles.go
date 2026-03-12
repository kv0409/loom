package dashboard

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var wtPrefixRe = regexp.MustCompile(`^LOOM-\d+-\d+-`)

func slugFromWorktree(name string) string {
	return wtPrefixRe.ReplaceAllString(name, "")
}

// Tokyo Night truecolor palette
var (
	colBlue    = lipgloss.Color("#7AA2F7")
	colGreen   = lipgloss.Color("#9ECE6A")
	colYellow  = lipgloss.Color("#E0AF68")
	colRed     = lipgloss.Color("#F7768E")
	colCyan    = lipgloss.Color("#7DCFFF")
	colMagenta = lipgloss.Color("#BB9AF7")
	colOrange  = lipgloss.Color("#FF9E64")
	colTeal    = lipgloss.Color("#73DACA")
	colGray    = lipgloss.Color("#565F89")
	colFg      = lipgloss.Color("#C0CAF5")
	colSubtle  = lipgloss.Color("#414868")
	colSelBg   = lipgloss.Color("#292E42")
)

// Semantic styles
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colBlue)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(colMagenta)
	activeStyle   = lipgloss.NewStyle().Foreground(colGreen)
	blockedStyle  = lipgloss.NewStyle().Foreground(colRed)
	reviewStyle   = lipgloss.NewStyle().Foreground(colCyan)
	deadStyle     = lipgloss.NewStyle().Foreground(colRed)
	idleStyle     = lipgloss.NewStyle().Foreground(colGray)
	selectedStyle = lipgloss.NewStyle().Bold(true).Background(colSelBg).Foreground(colFg)
	hoverStyle    = lipgloss.NewStyle().Background(colSubtle).Foreground(colFg)
	helpStyle     = lipgloss.NewStyle().Foreground(colSubtle)
	helpActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(colBlue)
	flashOkStyle  = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	flashErrStyle = lipgloss.NewStyle().Bold(true).Foreground(colRed)
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSubtle)
)

// Panel header colors by section type
var (
	panelAgents   = lipgloss.NewStyle().Bold(true).Foreground(colTeal)
	panelIssues   = lipgloss.NewStyle().Bold(true).Foreground(colYellow)
	panelMail     = lipgloss.NewStyle().Bold(true).Foreground(colOrange)
	panelMemory   = lipgloss.NewStyle().Bold(true).Foreground(colMagenta)
	panelWorktree = lipgloss.NewStyle().Bold(true).Foreground(colCyan)
	panelDiff     = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	panelActivity = lipgloss.NewStyle().Bold(true).Foreground(colBlue)
	panelLogs     = lipgloss.NewStyle().Bold(true).Foreground(colGray)
)

// Status-specific colors and glyphs
var statusColors = map[string]lipgloss.Color{
	"open":        colFg,
	"assigned":    colBlue,
	"in-progress": colTeal,
	"active":      colGreen,
	"done":        colGreen,
	"blocked":     colRed,
	"review":      colCyan,
	"error":       colRed,
	"dead":        colOrange,
	"cancelled":   colGray,
}

var statusGlyphs = map[string]string{
	"open":        "○",
	"assigned":    "◆",
	"in-progress": "▶",
	"active":      "▶",
	"done":        "✔",
	"blocked":     "⛔",
	"review":      "◎",
	"error":       "✖",
	"dead":        "✖",
	"cancelled":   "─",
}

func statusStyle(status string) lipgloss.Style {
	if c, ok := statusColors[status]; ok {
		return lipgloss.NewStyle().Foreground(c)
	}
	return idleStyle
}

func statusIndicator(status string) string {
	glyph := "●"
	if g, ok := statusGlyphs[status]; ok {
		glyph = g
	}
	return statusStyle(status).Render(glyph)
}

func truncateLines(s string, maxW int) string {
	lines := splitLines(s)
	trunc := lipgloss.NewStyle().MaxWidth(maxW)
	for i, l := range lines {
		if lipgloss.Width(l) > maxW {
			lines[i] = trunc.Render(l)
		}
	}
	return joinLines(lines)
}

func panel(title string, content string, width int) string {
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	content = truncateLines(content, innerW)
	s := borderStyle.Width(innerW).Render(content)
	if title != "" {
		lines := splitLines(s)
		if len(lines) > 0 {
			t := panelTitleStyle(title).Render(" " + panelIcon(title) + title + " ")
			tLen := lipgloss.Width(t)
			bc := lipgloss.NewStyle().Foreground(colSubtle)
			remaining := innerW - tLen - 1
			if remaining < 0 {
				remaining = 0
			}
			lines[0] = bc.Render("╭─") + t + bc.Render(strings.Repeat("─", remaining)+"╮")
			s = joinLines(lines)
		}
	}
	return s
}

// panelIcon returns a unicode icon prefix for the given panel title.
func panelIcon(title string) string {
	t := strings.ToUpper(title)
	switch {
	case strings.Contains(t, "AGENT"):
		return "🤖 "
	case strings.Contains(t, "ISSUE"):
		return "📋 "
	case strings.Contains(t, "MAIL"):
		return "📬 "
	case strings.Contains(t, "MEMORY"):
		return "🧠 "
	case strings.Contains(t, "ACTIVITY"):
		return "📊 "
	case strings.Contains(t, "LOG"):
		return "📝 "
	case strings.Contains(t, "WORKTREE"), strings.Contains(t, "DIFF"):
		return "🌳 "
	case strings.Contains(t, "KANBAN"), strings.Contains(t, "BOARD"):
		return "📌 "
	default:
		return ""
	}
}

// panelTitleStyle picks a color based on panel title keyword.
func panelTitleStyle(title string) lipgloss.Style {
	t := strings.ToUpper(title)
	switch {
	case strings.Contains(t, "AGENT"):
		return panelAgents
	case strings.Contains(t, "ISSUE"), strings.Contains(t, "KANBAN"):
		return panelIssues
	case strings.Contains(t, "MAIL"):
		return panelMail
	case strings.Contains(t, "MEMORY"):
		return panelMemory
	case strings.Contains(t, "WORKTREE"):
		return panelWorktree
	case strings.Contains(t, "DIFF"):
		return panelDiff
	case strings.Contains(t, "ACTIVITY"):
		return panelActivity
	case strings.Contains(t, "LOG"):
		return panelLogs
	default:
		return titleStyle
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

package dashboard

import (
	"fmt"
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
	colBg      = lipgloss.Color("#1A1B26")
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
	titleStyle    = lipgloss.NewStyle().Bold(true).Background(colBlue).Foreground(colBg).Padding(0, 2)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(colMagenta)
	activeStyle   = lipgloss.NewStyle().Foreground(colGreen)
	blockedStyle  = lipgloss.NewStyle().Foreground(colRed)
	reviewStyle   = lipgloss.NewStyle().Foreground(colCyan)
	deadStyle     = lipgloss.NewStyle().Foreground(colRed)
	idleStyle     = lipgloss.NewStyle().Foreground(colGray)
	selectedStyle = lipgloss.NewStyle().Bold(true).Background(colSelBg).Foreground(colFg)
	helpStyle     = lipgloss.NewStyle().Foreground(colSubtle)
	flashOkStyle  = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	flashErrStyle = lipgloss.NewStyle().Bold(true).Foreground(colRed)
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSubtle)
)

// Panel header colors by section type
var (
	panelAgents   = lipgloss.NewStyle().Bold(true).Background(colTeal).Foreground(colBg).Padding(0, 1)
	panelIssues   = lipgloss.NewStyle().Bold(true).Background(colYellow).Foreground(colBg).Padding(0, 1)
	panelMail     = lipgloss.NewStyle().Bold(true).Background(colOrange).Foreground(colBg).Padding(0, 1)
	panelMemory   = lipgloss.NewStyle().Bold(true).Background(colMagenta).Foreground(colBg).Padding(0, 1)
	panelWorktree = lipgloss.NewStyle().Bold(true).Background(colCyan).Foreground(colBg).Padding(0, 1)
	panelDiff     = lipgloss.NewStyle().Bold(true).Background(colGreen).Foreground(colBg).Padding(0, 1)
	panelActivity = lipgloss.NewStyle().Bold(true).Background(colBlue).Foreground(colBg).Padding(0, 1)
	panelLogs     = lipgloss.NewStyle().Bold(true).Background(colGray).Foreground(colBg).Padding(0, 1)
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
	"blocked":     "⊘",
	"review":      "◎",
	"error":       "✖",
	"dead":        "✖",
	"cancelled":   "─",
}

// typeGlyphs maps issue types to single-width Unicode icons.
var typeGlyphs = map[string]string{
	"epic":  "◈",
	"task":  "●",
	"bug":   "✦",
	"spike": "◇",
}

func statusStyle(status string) lipgloss.Style {
	if c, ok := statusColors[status]; ok {
		return lipgloss.NewStyle().Foreground(c)
	}
	return idleStyle
}

func statusPillStyle(status string) lipgloss.Style {
	c, ok := statusColors[status]
	if !ok {
		c = colGray
	}
	fg := colBg
	if status == "open" || status == "cancelled" {
		fg = colSelBg
	}
	return lipgloss.NewStyle().
		Background(c).
		Foreground(fg).
		Bold(true).
		Padding(0, 1)
}

func statusIndicator(status string) string {
	glyph := "●"
	if g, ok := statusGlyphs[status]; ok {
		glyph = g
	}
	return statusStyle(status).Render(glyph)
}

func typeGlyph(issueType string) string {
	if g, ok := typeGlyphs[issueType]; ok {
		return g
	}
	return "●"
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
		return "◈ "
	case strings.Contains(t, "ISSUE"):
		return "◇ "
	case strings.Contains(t, "MAIL"):
		return "▸ "
	case strings.Contains(t, "MEMORY"):
		return "◉ "
	case strings.Contains(t, "ACTIVITY"):
		return "▪ "
	case strings.Contains(t, "STATUS"):
		return "≈ "
	case strings.Contains(t, "LOG"):
		return "≡ "
	case strings.Contains(t, "WORKTREE"), strings.Contains(t, "DIFF"):
		return "⌥ "
	case strings.Contains(t, "KANBAN"), strings.Contains(t, "BOARD"):
		return "▦ "
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

func truncate(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	if n <= 3 {
		return "..."
	}
	w := 0
	for i, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > n-3 {
			return s[:i] + "..."
		}
		w += rw
	}
	return s
}

// Diff view styles
var (
	diffAdd    = lipgloss.NewStyle().Foreground(colGreen)
	diffDel    = lipgloss.NewStyle().Foreground(colRed)
	diffHunk   = lipgloss.NewStyle().Foreground(colCyan)
	diffHeader = lipgloss.NewStyle().Bold(true).Foreground(colYellow)
)

// Progress bar styles
var (
	barFill  = lipgloss.NewStyle().Foreground(colTeal)
	barEmpty = lipgloss.NewStyle().Foreground(colSubtle)
	barLabel = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
)

// searchBoxStyle is used for the inline search input in the help bar.
var searchBoxStyle = lipgloss.NewStyle().Background(colSelBg).Foreground(colFg).Padding(0, 1)

// heartbeatStyle returns a color style based on heartbeat freshness string.
func heartbeatStyle(ago string) lipgloss.Style {
	if ago == "never" {
		return lipgloss.NewStyle().Foreground(colRed)
	}
	if strings.HasSuffix(ago, "s") {
		return lipgloss.NewStyle().Foreground(colGreen)
	}
	if strings.HasSuffix(ago, "m") {
		return lipgloss.NewStyle().Foreground(colYellow)
	}
	return lipgloss.NewStyle().Foreground(colRed)
}

// selectedRow renders line with selectedStyle, replacing the leading two-space
// indent with a "▸ " prefix so the cursor is visible across all list views.
func selectedRow(line string) string {
	runes := []rune(line)
	if len(runes) >= 2 {
		return selectedStyle.Render("▸" + string(runes[1:]))
	}
	return selectedStyle.Render(line)
}

var emptyMsgStyle = lipgloss.NewStyle().Foreground(colGray).Italic(true)

func renderEmpty(msg string, width int) string {
	centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	return centered.Render(emptyMsgStyle.Render(msg)) + "\n"
}

func renderViewport(lines []string, scroll, viewH int) (string, int, int) {
	total := len(lines)
	maxScroll := total - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + viewH
	if end > total {
		end = total
	}
	return strings.Join(lines[scroll:end], "\n"), scroll, total
}

func scrollIndicator(scroll, viewH, total int) string {
	if total <= viewH {
		return ""
	}
	above := scroll
	below := total - viewH - scroll
	if below < 0 {
		below = 0
	}
	return idleStyle.Render(fmt.Sprintf(" ↑%d ↓%d", above, below))
}

const statusPillWidth = 13

func statusPill(status string) string {
	return statusPillStyle(status).Width(statusPillWidth).Render(status)
}

// detailViewH returns the number of visible lines for a detail-view panel
// given the terminal height. Accounts for title bar, panel chrome, and help bar.
func detailViewH(height int) int {
	h := height - 6
	if h < 1 {
		h = 1
	}
	return h
}

// listViewport returns the start and end indices for a cursor-following list
// viewport. visibleRows is the number of rows available for list items.
func listViewport(cursor, total, visibleRows int) (start, end int) {
	if visibleRows < 1 {
		visibleRows = 1
	}
	start = cursor - visibleRows + 1
	if start < 0 {
		start = 0
	}
	end = start + visibleRows
	if end > total {
		end = total
	}
	return start, end
}


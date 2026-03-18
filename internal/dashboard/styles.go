package dashboard

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var wtPrefixRe = regexp.MustCompile(`^LOOM-\d+(?:-\d+)+-`)

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
	headerStyle   = lipgloss.NewStyle().Bold(true).Background(colSelBg).Foreground(colFg).Padding(0, 1)
	activeStyle   = lipgloss.NewStyle().Foreground(colGreen)
	blockedStyle  = lipgloss.NewStyle().Foreground(colRed)
	reviewStyle   = lipgloss.NewStyle().Foreground(colCyan)
	deadStyle     = lipgloss.NewStyle().Foreground(colRed)
	idleStyle     = lipgloss.NewStyle().Foreground(colGray)
	selectedStyle = lipgloss.NewStyle().Bold(true).Background(colSubtle).Foreground(colFg)
	helpStyle     = lipgloss.NewStyle().Foreground(colSubtle)
	flashOkStyle  = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	flashErrStyle = lipgloss.NewStyle().Bold(true).Foreground(colRed)
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSubtle)
	overlayStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBlue).Background(colBg).Padding(1, 2)
)

// Panel header style — inverted: blue background + dark foreground, like a mini title bar
var panelTitle = lipgloss.NewStyle().Bold(true).Background(colBlue).Foreground(colBg).Padding(0, 1)

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
			t := panelTitle.Render(" " + panelIcon(title) + title + " ")
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

// plural returns the simple English plural of a singular noun.
// Handles: "discovery" → "discoveries", "worktree" → "worktrees".
func plural(n int, singular string) string {
	if n == 1 {
		return singular
	}
	if strings.HasSuffix(singular, "y") && !strings.HasSuffix(singular, "ey") {
		return singular[:len(singular)-1] + "ies"
	}
	return singular + "s"
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

// panelNoTruncate renders a bordered panel like panel() but skips line
// truncation. Used by the diff view which applies its own horizontal
// scrolling before rendering.
func panelNoTruncate(title string, content string, width int) string {
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	// Clamp lines to innerW using MaxWidth so the border isn't broken,
	// but only AFTER the caller has already shifted content horizontally.
	content = truncateLines(content, innerW)
	s := borderStyle.Width(innerW).Render(content)
	if title != "" {
		lines := splitLines(s)
		if len(lines) > 0 {
			t := panelTitle.Render(" " + panelIcon(title) + title + " ")
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

// hshiftLine shifts a single plain-text line by offset runes to the right,
// dropping the first offset runes. Returns the remaining string.
func hshiftLine(line string, offset int) string {
	if offset <= 0 {
		return line
	}
	runes := []rune(line)
	if offset >= len(runes) {
		return ""
	}
	return string(runes[offset:])
}

// diffHScrollStep is the number of columns each left/right press shifts.
const diffHScrollStep = 8

var (
	diffAdd    = lipgloss.NewStyle().Foreground(colGreen)
	diffDel    = lipgloss.NewStyle().Foreground(colRed)
	diffHunk   = lipgloss.NewStyle().Foreground(colCyan)
	diffHeader = lipgloss.NewStyle().Bold(true).Foreground(colYellow)
)

// Overview stats line style — bold foreground so it reads as a glanceable summary.
var statsLineStyle = lipgloss.NewStyle().Bold(true).Foreground(colFg)

// Progress bar styles
var (
	barLabel = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
)

// searchBoxStyle is used for the inline search input in the help bar.
var searchBoxStyle = lipgloss.NewStyle().Background(colSelBg).Foreground(colFg).Padding(0, 1)

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

// Activity view styles
var (
	activityTimeStyle  = lipgloss.NewStyle().Foreground(colGray)
	activityLabelStyle = lipgloss.NewStyle().Bold(true).Width(5)
	activityIconStyle  = lipgloss.NewStyle().Bold(true).Width(2)
)

// Table styles used by newStyledTable and newStyledTableHeaderless in render_helpers.go
var (
	tableHeaderStyle             = lipgloss.NewStyle().Bold(true).Background(colSubtle).Foreground(colFg).Padding(0, 1)
	tableCellStyle               = lipgloss.NewStyle().Foreground(colFg).Padding(0, 1)
	tableSelectedStyle           = lipgloss.NewStyle().Bold(true).Background(colSelBg)
	tableHeaderlessHeaderStyle   = lipgloss.NewStyle()
	tableHeaderlessSelectedStyle = lipgloss.NewStyle().Foreground(colFg).Padding(0, 1)
)

// Compose overlay styles
var (
	composeTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colBlue).MarginBottom(1)
	composeHintStyle  = lipgloss.NewStyle().Foreground(colGray)
	composeKeyStyle   = lipgloss.NewStyle().Foreground(colFg).Bold(true)
)

// agentColor returns the role-based color for an agent ID.
// Role is the prefix before the first dash-digit sequence (e.g. "builder" from "builder-001").
func agentColor(id string) lipgloss.Color {
	role := id
	for i := 1; i < len(id); i++ {
		if id[i-1] == '-' && id[i] >= '0' && id[i] <= '9' {
			role = id[:i-1]
			break
		}
	}
	switch role {
	case "orchestrator":
		return colOrange
	case "lead":
		return colMagenta
	case "builder":
		return colBlue
	case "reviewer":
		return colGreen
	case "explorer":
		return colTeal
	case "researcher":
		return colYellow
	default:
		return colFg
	}
}

// agentPillFor renders a pill displaying displayText but using colorID for the
// role-based background color. Use this when the display text has been truncated
// and would no longer match a role in agentColor.
func agentPillFor(displayText, colorID string) string {
	return lipgloss.NewStyle().
		Background(agentColor(colorID)).
		Foreground(colBg).
		Bold(true).
		Padding(0, 1).
		Render(displayText)
}

// agentPillSelected renders the same pill geometry without a filled background
// so the table's selection background can read as one continuous row.
func agentPillSelected(displayText, colorID string) string {
	return lipgloss.NewStyle().
		Foreground(agentColor(colorID)).
		Bold(true).
		Padding(0, 1).
		Render(displayText)
}

// agentPillPlain returns a plain-text string with the same visual width as agentPill(id).
// The pill's Padding(0,1) adds 1 space each side, so we mirror that here.
func agentPillPlain(id string) string {
	return " " + id + " "
}

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

func statusPillSelected(status string) string {
	return statusPillStyle(status).Width(statusPillWidth).Background(colSelBg).Render(status)
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

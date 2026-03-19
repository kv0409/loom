package dashboard

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	helpStyle     = lipgloss.NewStyle().Foreground(colSubtle)
	flashOkStyle  = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	flashErrStyle = lipgloss.NewStyle().Bold(true).Foreground(colRed)
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSubtle)
	overlayStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBlue).Background(colBg).Padding(1, 2)
)

// Panel header style — inverted: blue background + dark foreground, like a mini title bar
var panelTitle = lipgloss.NewStyle().Bold(true).Background(colBlue).Foreground(colBg).Padding(0, 1)

// Status-specific colors and glyphs
var statusColors = map[string]color.Color{
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

// heartbeatDonut returns a colored donut glyph based on elapsed/timeout fraction.
func heartbeatDonut(elapsed, timeout time.Duration) string {
	if timeout <= 0 {
		return statusIndicator("active")
	}
	frac := float64(elapsed) / float64(timeout)
	var glyph string
	var c color.Color
	switch {
	case frac < 0.2:
		glyph, c = "●", colGreen
	case frac < 0.4:
		glyph, c = "◕", colGreen
	case frac < 0.6:
		glyph, c = "◑", colYellow
	case frac < 0.8:
		glyph, c = "◔", colYellow
	default:
		glyph, c = "○", colRed
	}
	return lipgloss.NewStyle().Foreground(c).Render(glyph)
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
	s := borderStyle.Width(width).Render(content)
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
	if n <= 0 {
		return ""
	}
	return ansi.Truncate(s, n, "...")
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
	s := borderStyle.Width(width).Render(content)
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

// spinnerStyle is used for the inline loading spinner.
var spinnerStyle = lipgloss.NewStyle().Foreground(colBlue)

var emptyMsgStyle = lipgloss.NewStyle().Foreground(colGray).Italic(true)

// Activity view styles
var (
	activityTimeStyle = lipgloss.NewStyle().Foreground(colGray)
	activityIconStyle = lipgloss.NewStyle().Bold(true).Width(2)
)

// mailPriorityColor returns the foreground color for a mail priority level.
func mailPriorityColor(priority string) color.Color {
	switch priority {
	case "critical":
		return colRed
	case "normal":
		return colCyan
	default:
		return colGray
	}
}

// CellStyler is a callback that returns the lipgloss.Style for a given table
// cell. row and col are 0-based data indices; isSelected is true when the row
// matches the current cursor. Views provide a CellStyler to apply per-cell
// foreground colors (e.g. agent color, status color) while letting the table
// handle selection background uniformly.
type CellStyler func(row, col int, isSelected bool) lipgloss.Style

// ColWidth pins a table column to a fixed cell width. The lipgloss/table
// resizer honours Style.Width and excludes fixed columns from proportional
// expansion/shrinking, so remaining space flows to flexible columns.
type ColWidth struct {
	Col   int
	Width int
}

// Table styles used by newLGTable and newLGTableHeaderless in render_helpers.go.
var (
	lgTableHeaderStyle   = lipgloss.NewStyle().Bold(true).Background(colSubtle).Foreground(colFg).Padding(0, 1)
	lgTableCellStyle     = lipgloss.NewStyle().Foreground(colFg).Padding(0, 1)
	lgTableSelectedStyle = lipgloss.NewStyle().Bold(true).Background(colSelBg).Foreground(colFg).Padding(0, 1)
)

// Tree connector style — subtle foreground for enumerator/indenter glyphs.
var treeConnectorStyle = lipgloss.NewStyle().Foreground(colSubtle)

// Compose overlay styles
var (
	composeTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colBlue).MarginBottom(1)
	composeHintStyle  = lipgloss.NewStyle().Foreground(colGray)
	composeKeyStyle   = lipgloss.NewStyle().Foreground(colFg).Bold(true)
)

// agentColor returns the role-based color for an agent ID.
// Role is the prefix before the first dash-digit sequence (e.g. "builder" from "builder-001").
func agentColor(id string) color.Color {
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

func renderEmpty(msg string, width int) string {
	centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	return centered.Render(emptyMsgStyle.Render(msg)) + "\n"
}

func vpScrollIndicator(vp viewport.Model) string {
	total := vp.TotalLineCount()
	visible := vp.VisibleLineCount()
	if total <= visible {
		return ""
	}
	above := vp.YOffset()
	below := total - visible - above
	if below < 0 {
		below = 0
	}
	return idleStyle.Render(fmt.Sprintf(" ↑%d ↓%d", above, below))
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

package cli

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"charm.land/lipgloss/v2"
	lgtable "charm.land/lipgloss/v2/table"
)

// Tokyo Night palette (matches dashboard/styles.go)
var (
	colGreen   = lipgloss.Color("#9ECE6A")
	colYellow  = lipgloss.Color("#E0AF68")
	colRed     = lipgloss.Color("#F7768E")
	colGray    = lipgloss.Color("#565F89")
	colBlue    = lipgloss.Color("#7AA2F7")
	colCyan    = lipgloss.Color("#7DCFFF")
	colMagenta = lipgloss.Color("#BB9AF7")
	colOrange  = lipgloss.Color("#FF9E64")
	colTeal    = lipgloss.Color("#73DACA")
	colFg      = lipgloss.Color("#C0CAF5")
	colSubtle  = lipgloss.Color("#414868")
	colSelBg   = lipgloss.Color("#292E42")
)

func noColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

func colored(s string, style lipgloss.Style) string {
	if noColor() {
		return s
	}
	return style.Render(s)
}

// PrintSuccess prints a green checkmark line: "✓ msg" or "✓ msg — id"
func PrintSuccess(msg string, id ...string) {
	line := "✓ " + msg
	if len(id) > 0 && id[0] != "" {
		line += colored(" — "+id[0], lipgloss.NewStyle().Foreground(colBlue))
	}
	fmt.Println(colored(line, lipgloss.NewStyle().Foreground(colGreen)))
}

// PrintWarning prints a yellow warning line: "! msg" or "! msg — hint"
func PrintWarning(msg string, hint ...string) {
	line := "! " + msg
	if len(hint) > 0 && hint[0] != "" {
		line += " — " + hint[0]
	}
	fmt.Println(colored(line, lipgloss.NewStyle().Foreground(colYellow)))
}

// PrintError prints a red error line to stderr: "✗ msg" or "✗ msg — hint"
func PrintError(msg string, hint ...string) {
	line := "✗ " + msg
	if len(hint) > 0 && hint[0] != "" {
		line += " — " + hint[0]
	}
	fmt.Fprintln(os.Stderr, colored(line, lipgloss.NewStyle().Foreground(colRed)))
}

// PrintInfo prints dimmed informational text.
func PrintInfo(msg string) {
	fmt.Println(colored(msg, lipgloss.NewStyle().Foreground(colGray)))
}

// ---------------------------------------------------------------------------
// CLITable
// ---------------------------------------------------------------------------

var (
	cliHeaderStyle = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	cliCellStyle   = lipgloss.NewStyle().Foreground(colFg)
)

// CLITable renders a styled table. Falls back to tabwriter when NO_COLOR is set.
func CLITable(headers []string, rows [][]string) string {
	if noColor() {
		return plainTable(headers, rows)
	}
	t := lgtable.New().
		Headers(headers...).
		Rows(rows...).
		Wrap(false).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		BorderRow(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == lgtable.HeaderRow {
				return cliHeaderStyle
			}
			return cliCellStyle
		})
	return t.Render()
}

func plainTable(headers []string, rows [][]string) string {
	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	w.Flush()
	return strings.TrimRight(buf.String(), "\n")
}

// ---------------------------------------------------------------------------
// DetailView
// ---------------------------------------------------------------------------

// DetailField is a label-value pair for DetailView.
type DetailField struct {
	Label string
	Value string
}

var (
	detailLabelStyle = lipgloss.NewStyle().Foreground(colGray)
	detailValueStyle = lipgloss.NewStyle().Foreground(colFg)
)

// DetailView renders aligned key-value fields. Fields with empty values are skipped.
func DetailView(fields []DetailField) string {
	// Filter and find max label width.
	var visible []DetailField
	maxW := 0
	for _, f := range fields {
		if f.Value == "" {
			continue
		}
		visible = append(visible, f)
		if len(f.Label) > maxW {
			maxW = len(f.Label)
		}
	}
	var b strings.Builder
	for i, f := range visible {
		padded := f.Label + ":" + strings.Repeat(" ", maxW-len(f.Label)+1)
		if noColor() {
			b.WriteString(padded + f.Value)
		} else {
			b.WriteString(detailLabelStyle.Render(padded) + detailValueStyle.Render(f.Value))
		}
		if i < len(visible)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Styled pill formatters
// ---------------------------------------------------------------------------

var statusColorMap = map[string]color.Color{
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

// IssueText returns an issue ID colored with colBlue.
func IssueText(id string) string {
	if id == "" {
		return ""
	}
	return colored(id, lipgloss.NewStyle().Foreground(colBlue))
}

// StatusText returns color-coded status text.
func StatusText(status string) string {
	if status == "" {
		return ""
	}
	c, ok := statusColorMap[status]
	if !ok {
		c = colFg
	}
	return colored(status, lipgloss.NewStyle().Foreground(c))
}

var priorityColorMap = map[string]color.Color{
	"critical": colRed,
	"high":     colOrange,
	"normal":   colFg,
	"low":      colGray,
}

// PriorityText returns color-coded priority text.
func PriorityText(priority string) string {
	if priority == "" {
		return ""
	}
	c, ok := priorityColorMap[priority]
	if !ok {
		c = colFg
	}
	return colored(priority, lipgloss.NewStyle().Foreground(c))
}

// AgentText returns agent-colored text based on role prefix.
func AgentText(id string) string {
	if id == "" {
		return ""
	}
	var c color.Color
	switch {
	case strings.HasPrefix(id, "orchestrator"):
		c = colOrange
	case strings.HasPrefix(id, "lead-"):
		c = colMagenta
	case strings.HasPrefix(id, "builder-"):
		c = colBlue
	case strings.HasPrefix(id, "reviewer-"):
		c = colCyan
	case strings.HasPrefix(id, "explorer-"):
		c = colTeal
	case strings.HasPrefix(id, "researcher-"):
		c = colGreen
	default:
		c = colFg
	}
	return colored(id, lipgloss.NewStyle().Foreground(c))
}

// TimeFmt formats a time as a relative string like "2m ago", "1h ago", "3d ago".
func TimeFmt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", max(1, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ---------------------------------------------------------------------------
// Exported color accessors for use in cmd/loom/main.go
// ---------------------------------------------------------------------------

// Colored applies a lipgloss style to a string, respecting NO_COLOR.
func Colored(s string, style lipgloss.Style) string {
	return colored(s, style)
}

// Style constructors for specific colors.
func StyleSubtle() lipgloss.Style  { return lipgloss.NewStyle().Foreground(colSubtle) }
func StyleBlue() lipgloss.Style    { return lipgloss.NewStyle().Foreground(colBlue) }
func StyleGreen() lipgloss.Style   { return lipgloss.NewStyle().Foreground(colGreen) }
func StyleRed() lipgloss.Style     { return lipgloss.NewStyle().Foreground(colRed) }
func StyleGray() lipgloss.Style    { return lipgloss.NewStyle().Foreground(colGray) }
func StyleOrange() lipgloss.Style  { return lipgloss.NewStyle().Foreground(colOrange) }
func StyleTeal() lipgloss.Style    { return lipgloss.NewStyle().Foreground(colTeal) }
func StyleFg() lipgloss.Style      { return lipgloss.NewStyle().Foreground(colFg) }
func StyleYellow() lipgloss.Style  { return lipgloss.NewStyle().Foreground(colYellow) }

package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// newStyledTable creates a bubbles/table.Model styled with Tokyo Night colors.
// It is used render-only: rebuilt each frame, not persisted in the dashboard Model.
func newStyledTable(cols []table.Column, rows []table.Row, height int) table.Model {
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(height+1), // +1 for header row
		table.WithFocused(true),
	)
	t.SetStyles(table.Styles{
		Header:   tableHeaderStyle,
		Cell:     tableCellStyle,
		Selected: tableSelectedStyle,
	})
	return t
}

// newStyledTableHeaderless creates a bubbles/table.Model with no visible header row.
// Use tableBodyView() to render it — this strips the (invisible) header line from View().
func newStyledTableHeaderless(cols []table.Column, rows []table.Row, height int) table.Model {
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(height+1), // +1: invisible header still occupies one viewport line
		table.WithFocused(false),
	)
	t.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle(),                          // zero-height: no padding, no bold
		Cell:     tableCellStyle,                               // Padding(0,1) per cell
		Selected: lipgloss.NewStyle().Foreground(colFg).Padding(0, 1),
	})
	return t
}

// tableBodyView renders only the rows portion of a headerless table,
// stripping the empty header line that bubbles/table always prepends.
func tableBodyView(t table.Model) string {
	v := t.View()
	if i := strings.Index(v, "\n"); i >= 0 {
		return v[i+1:]
	}
	return v
}

// fmtTime formats a time as a human-readable "ago" string.
// short=true → "30s", "5m", "2h"  (used in overview compact cells)
// short=false → "30s ago", "5m ago", "2h ago"  (used in detail/table views)
func fmtTime(t time.Time, short bool) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	var s string
	switch {
	case d < time.Minute:
		s = fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		s = fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		s = fmt.Sprintf("%dh", int(d.Hours()))
	}
	if short {
		return s
	}
	return s + " ago"
}

// fmtTimeFull formats a time as an absolute timestamp string.
func fmtTimeFull(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04:05")
}

// cellPlaceholder returns a unique placeholder string of the given visual width.
// It uses a \x00 prefix + index suffix so it cannot collide with real cell content.
// Callers pass these as plain-text values in table.Row, then styledTableView replaces
// each placeholder with the corresponding styled string.
func cellPlaceholder(index, width int) string {
	tag := fmt.Sprintf("\x00%d\x00", index)
	pad := width - len(tag)
	if pad < 0 {
		pad = 0
	}
	return tag + strings.Repeat(" ", pad)
}

// styledTableView renders t.View() and replaces each placeholder cell value with its
// styled equivalent. Pass pairs as (placeholder, styled) in any order.
// This is necessary because bubbles/table calls runewidth.Truncate on cell values, which
// counts ANSI escape bytes as display width and mangles pre-styled strings.
func styledTableView(t table.Model, replacements [][2]string) string {
	out := t.View()
	for _, r := range replacements {
		out = strings.Replace(out, r[0], r[1], 1)
	}
	return out
}

// styledTableBodyView is like styledTableView but strips the header line (for headerless tables).
func styledTableBodyView(t table.Model, replacements [][2]string) string {
	out := styledTableView(t, replacements)
	if i := strings.Index(out, "\n"); i >= 0 {
		return out[i+1:]
	}
	return out
}

// colWidths computes proportional column widths from a list of (percent, min) pairs
// given the available pixel budget. Each entry is {pct: percentage of avail, min: minimum width}.
func colWidths(avail int, cols []struct{ pct, min int }) []int {
	out := make([]int, len(cols))
	for i, c := range cols {
		w := avail * c.pct / 100
		if w < c.min {
			w = c.min
		}
		out[i] = w
	}
	return out
}


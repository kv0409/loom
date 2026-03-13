package dashboard

import (
	"fmt"
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
		Header:   lipgloss.NewStyle().Bold(true).Background(colSubtle).Foreground(colFg).Padding(0, 1),
		Cell:     lipgloss.NewStyle().Foreground(colFg).Padding(0, 1),
		Selected: lipgloss.NewStyle().Bold(true).Background(colSubtle).Foreground(colFg),
	})
	return t
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


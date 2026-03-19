package dashboard

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	lgtable "charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
)

// fmtTime formats a time as a human-readable relative string.
// short=true → "30s", "5m", "2h"  (used in overview compact cells)
// short=false → "30s", "5m", "2h"  (used in detail/table views)
func fmtTime(t time.Time, short bool) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// fmtTimeFull formats a time as an absolute timestamp string.
func fmtTimeFull(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04:05")
}

// newLGTable creates a borderless lipgloss/table with headers, styled via StyleFunc.
// selectedRow is the data-row index (0-based) that should be highlighted, or -1 for none.
// width is the total available width (typically availableWidth(m.width)).
func newLGTable(headers []string, rows [][]string, selectedRow, width int) *lgtable.Table {
	return lgtable.New().
		Headers(headers...).
		Rows(rows...).
		Width(width).
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
				return lgTableHeaderStyle
			}
			if row == selectedRow {
				return lgTableSelectedStyle
			}
			return lgTableCellStyle
		})
}

// newLGTableHeaderless creates a borderless lipgloss/table with no headers.
// selectedRow is the data-row index (0-based) that should be highlighted, or -1 for none.
// width is the total available width.
func newLGTableHeaderless(rows [][]string, selectedRow, width int) *lgtable.Table {
	return lgtable.New().
		Rows(rows...).
		Width(width).
		Wrap(false).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		BorderRow(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == selectedRow {
				return lgTableSelectedStyle
			}
			return lgTableCellStyle
		})
}

// wordWrap splits s into segments of at most width runes, breaking on spaces where possible.
func wordWrap(s string, width int) []string {
	if width <= 0 || len(s) == 0 {
		return []string{s}
	}
	wrapped := ansi.Wrap(s, width, " ")
	return strings.Split(wrapped, "\n")
}

// wrapLines word-wraps multi-line text, prefixing each output line with indent.
// Returns the wrapped lines as a slice (one element per display line).
func wrapLines(text string, maxW int, indent string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			out = append(out, "")
			continue
		}
		for _, seg := range wordWrap(line, maxW) {
			out = append(out, indent+seg)
		}
	}
	return out
}




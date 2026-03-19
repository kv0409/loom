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
// styler is an optional CellStyler callback; when non-nil it controls per-cell styling
// (the view decides foreground colors based on data). When nil, the default header/cell/selected
// styles are applied.
// Variadic ColWidth hints pin specific columns to a fixed width; the resizer
// excludes them from proportional expansion so remaining space flows to flexible columns.
func newLGTable(headers []string, rows [][]string, selectedRow, width int, styler CellStyler, fixedCols ...ColWidth) *lgtable.Table {
	fixed := colWidthMap(fixedCols)
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
			var base lipgloss.Style
			if row == lgtable.HeaderRow {
				base = lgTableHeaderStyle
			} else if styler != nil {
				base = styler(row, col, row == selectedRow)
			} else if row == selectedRow {
				base = lgTableSelectedStyle
			} else {
				base = lgTableCellStyle
			}
			if w, ok := fixed[col]; ok {
				base = base.Width(w)
			}
			return base
		})
}

// newLGTableHeaderless creates a borderless lipgloss/table with no headers.
// selectedRow is the data-row index (0-based) that should be highlighted, or -1 for none.
// width is the total available width.
// styler is an optional CellStyler callback; when non-nil it controls per-cell styling.
// Variadic ColWidth hints pin specific columns to a fixed width.
func newLGTableHeaderless(rows [][]string, selectedRow, width int, styler CellStyler, fixedCols ...ColWidth) *lgtable.Table {
	fixed := colWidthMap(fixedCols)
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
			var base lipgloss.Style
			if styler != nil {
				base = styler(row, col, row == selectedRow)
			} else if row == selectedRow {
				base = lgTableSelectedStyle
			} else {
				base = lgTableCellStyle
			}
			if w, ok := fixed[col]; ok {
				base = base.Width(w)
			}
			return base
		})
}

// colWidthMap converts a slice of ColWidth hints into a col→width lookup.
func colWidthMap(cw []ColWidth) map[int]int {
	if len(cw) == 0 {
		return nil
	}
	m := make(map[int]int, len(cw))
	for _, c := range cw {
		m[c.Col] = c.Width
	}
	return m
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




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
		Header:   tableHeaderlessHeaderStyle,
		Cell:     tableCellStyle,
		Selected: tableHeaderlessSelectedStyle,
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
	pad := width - lipgloss.Width(tag)
	if pad < 0 {
		pad = 0
	}
	return tag + strings.Repeat(" ", pad)
}

// fixSelectedRowBg re-applies the selection background after inner ANSI resets
// so the highlight spans the full row. bubbles/table wraps the selected row with
// Selected style, but inner cell resets (\x1b[0m) break the background.
func fixSelectedRowBg(out string) string {
	selBg := lipgloss.NewStyle().Background(colSelBg).Render("")
	idx := strings.Index(selBg, "m")
	if idx < 0 {
		return out
	}
	bgSeq := selBg[:idx+1]
	reset := "\x1b[0m"
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, bgSeq) || strings.HasPrefix(line, "\x1b[1;48;2;") {
			parts := strings.Split(line, reset)
			if len(parts) > 2 {
				lines[i] = strings.Join(parts[:len(parts)-1], reset+bgSeq) + reset
			}
		}
	}
	return strings.Join(lines, "\n")
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
	return fixSelectedRowBg(out)
}

// styledTableBodyView is like styledTableView but strips the header line (for headerless tables).
func styledTableBodyView(t table.Model, replacements [][2]string) string {
	out := styledTableView(t, replacements)
	if i := strings.Index(out, "\n"); i >= 0 {
		return out[i+1:]
	}
	return out
}

// wordWrap splits s into segments of at most width runes, breaking on spaces where possible.
func wordWrap(s string, width int) []string {
	if width <= 0 || len(s) == 0 {
		return []string{s}
	}
	var segments []string
	for len(s) > 0 {
		runes := []rune(s)
		if len(runes) <= width {
			segments = append(segments, s)
			break
		}
		cut := width
		prefix := string(runes[:width])
		if idx := strings.LastIndex(prefix, " "); idx > 0 {
			cut = len([]rune(prefix[:idx])) + 1
		}
		segments = append(segments, strings.TrimRight(string(runes[:cut]), " "))
		s = strings.TrimLeft(string(runes[cut:]), " ")
	}
	return segments
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




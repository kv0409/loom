package dashboard

import "strings"

// availableWidth returns the usable column width for a panel of the given
// terminal width (subtracts 6 for indent + inter-column spacing), enforcing a
// minimum of 40.
func availableWidth(w int) int {
	avail := w - 6
	if avail < 40 {
		return 40
	}
	return avail
}

// panelWidth returns the standard width passed to panel(), spanning the full terminal.
func panelWidth(w int) int { return w }

// separatorWidth returns the width of the horizontal separator line drawn
// inside a panel (max(20, w-6)).
func separatorWidth(w int) int {
	if w-6 < 20 {
		return 20
	}
	return w - 6
}

// separator returns a "  ─────" line of the correct width for terminal width w.
func separator(w int) string {
	return "  " + strings.Repeat("─", separatorWidth(w)) + "\n"
}

// proportionalWidth returns max(minW, avail*pct/100).
func proportionalWidth(avail, pct, minW int) int {
	v := avail * pct / 100
	if v < minW {
		return minW
	}
	return v
}

// visibleRows returns the number of scrollable rows for a list view given the
// terminal height and the number of fixed header rows consumed above the list.
// headerRows is typically 9 for tab-based views; enforces a minimum of 1.
func visibleRows(h, headerRows int) int {
	v := h - headerRows
	if v < 1 {
		return 1
	}
	return v
}

// scrollViewport returns the number of scrollable rows for a detail/panel view
// (h - 6), enforcing a minimum of 1.
func scrollViewport(h int) int { return visibleRows(h, 6) }

// detailContentWidth returns the usable text-wrap width for detail/body fields
// inside a panel (w - 8).
func detailContentWidth(w int) int { return max(40, w-8) }

// issuesViewport returns start/end for the issues list, accounting for the
// "RECENTLY DONE" separator that consumes issuesSectionGap extra lines when
// visible. It recalculates with reduced rows so the cursor stays visible.
func issuesViewport(cursor, total, vRows, activeCount int) (start, end int) {
	start, end = listViewport(cursor, total, vRows)
	if activeCount < total && activeCount >= start && activeCount < end {
		reduced := vRows - issuesSectionGap
		if reduced < 1 {
			reduced = 1
		}
		start, end = listViewport(cursor, total, reduced)
	}
	return start, end
}

// sectionCursor reports the cursor position relative to a list section.
// When the cursor is outside the section, selected is false and the returned
// row is undefined.
func sectionCursor(cursor, start, end int) (row int, selected bool) {
	if cursor < start || cursor >= end {
		return 0, false
	}
	return cursor - start, true
}

// agentDetailVPHeight returns the viewport height for the scrollable output
// section in agent detail, subtracting the fixed header lines, footer lines,
// panel border (2), output section header (1), and blank separators (2).
func agentDetailVPHeight(termH, headerLines, footerLines int) int {
	// panel border top/bottom = 2, title bar = 1, blank before output = 1,
	// output header line = 1, blank before footer = 1, help bar = 1
	fixed := headerLines + footerLines + 7
	v := termH - fixed
	if v < 1 {
		return 1
	}
	return v
}
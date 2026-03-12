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

// panelWidth returns the standard content width passed to panel() (w - 2).
func panelWidth(w int) int { return w - 2 }

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

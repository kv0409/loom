package dashboard

import "testing"

func TestIssuesViewport_CursorVisibleWithSeparator(t *testing.T) {
	// 8 active + 5 done = 13 total, separator at index 8, 10 visible rows.
	// Cursor at 9 (second done item) must remain in [start, end).
	start, end := issuesViewport(9, 13, 10, 8)
	if 9 < start || 9 >= end {
		t.Errorf("cursor 9 outside viewport [%d, %d)", start, end)
	}
}

func TestIssuesViewport_CursorAtSeparatorBoundary(t *testing.T) {
	// Cursor exactly at activeCount (first done item).
	start, end := issuesViewport(8, 13, 10, 8)
	if 8 < start || 8 >= end {
		t.Errorf("cursor 8 outside viewport [%d, %d)", start, end)
	}
}

func TestIssuesViewport_NoSeparatorWhenAllActive(t *testing.T) {
	// No done items — activeCount == total, no separator.
	start, end := issuesViewport(5, 10, 8, 10)
	if start != 0 || end != 8 {
		t.Errorf("expected [0, 8), got [%d, %d)", start, end)
	}
}

func TestIssuesViewport_SeparatorOutOfView(t *testing.T) {
	// Separator at index 2, cursor at 9 — separator scrolled off top.
	// Should use full visibleRows since separator isn't rendered.
	start, end := issuesViewport(9, 13, 6, 2)
	if 9 < start || 9 >= end {
		t.Errorf("cursor 9 outside viewport [%d, %d)", start, end)
	}
	if end-start > 6 {
		t.Errorf("viewport wider than visibleRows: [%d, %d)", start, end)
	}
}

func TestIssuesViewport_ReducedViewportDoesNotExceedTotal(t *testing.T) {
	// Small list: 3 active + 1 done, 10 visible rows.
	start, end := issuesViewport(3, 4, 10, 3)
	if end > 4 {
		t.Errorf("end %d exceeds total 4", end)
	}
	if 3 < start || 3 >= end {
		t.Errorf("cursor 3 outside viewport [%d, %d)", start, end)
	}
}

func TestSectionCursor_InactiveSectionReturnsFalse(t *testing.T) {
	row, ok := sectionCursor(1, 0, 1)
	if ok {
		t.Fatalf("expected active section to be inactive when cursor is outside it, got row=%d", row)
	}

	row, ok = sectionCursor(1, 1, 2)
	if !ok {
		t.Fatal("expected done section to be active")
	}
	if row != 0 {
		t.Fatalf("expected done section row 0, got %d", row)
	}
}

package dashboard

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestListViewport_CursorAtStart(t *testing.T) {
	start, end := listViewport(0, 20, 10)
	if start != 0 {
		t.Errorf("expected start=0, got %d", start)
	}
	if end != 10 {
		t.Errorf("expected end=10, got %d", end)
	}
}

func TestListViewport_CursorAtEnd(t *testing.T) {
	start, end := listViewport(19, 20, 10)
	if start != 10 {
		t.Errorf("expected start=10, got %d", start)
	}
	if end != 20 {
		t.Errorf("expected end=20, got %d", end)
	}
}

func TestListViewport_CursorInRange(t *testing.T) {
	start, end := listViewport(5, 20, 10)
	if 5 < start || 5 >= end {
		t.Errorf("cursor 5 not in viewport [%d, %d)", start, end)
	}
}

func TestListViewport_TotalLessThanVisible(t *testing.T) {
	start, end := listViewport(2, 5, 10)
	if start != 0 {
		t.Errorf("expected start=0 when total < visible, got %d", start)
	}
	if end != 5 {
		t.Errorf("expected end=total=5, got %d", end)
	}
}

func TestListViewport_ZeroVisibleRows(t *testing.T) {
	// visibleRows < 1 should be clamped to 1.
	start, end := listViewport(0, 5, 0)
	if end-start < 1 {
		t.Errorf("expected at least 1 visible row, got [%d, %d)", start, end)
	}
}

func TestListViewport_CursorAlwaysVisible(t *testing.T) {
	// Fuzz: cursor should always be in [start, end) for any valid cursor.
	for total := 1; total <= 30; total++ {
		for vis := 1; vis <= total+5; vis++ {
			for cur := 0; cur < total; cur++ {
				start, end := listViewport(cur, total, vis)
				if cur < start || cur >= end {
					t.Errorf("cursor %d outside [%d, %d) (total=%d, vis=%d)", cur, start, end, total, vis)
				}
			}
		}
	}
}

func TestTruncate_Short(t *testing.T) {
	s := "hello"
	if got := truncate(s, 10); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTruncate_Exact(t *testing.T) {
	s := "hello"
	if got := truncate(s, 5); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTruncate_TooLong(t *testing.T) {
	s := "hello world"
	got := truncate(s, 8)
	if len(got) > 8 {
		t.Errorf("truncated string too long: %q (len %d)", got, len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("expected ... suffix, got %q", got)
	}
}

func TestTruncate_VerySmall(t *testing.T) {
	got := truncate("hello", 3)
	if got != "..." {
		t.Errorf("expected '...', got %q", got)
	}
}

func TestWordWrap_NoWrap(t *testing.T) {
	segs := wordWrap("short", 20)
	if len(segs) != 1 || segs[0] != "short" {
		t.Errorf("unexpected: %v", segs)
	}
}

func TestWordWrap_BreaksOnSpace(t *testing.T) {
	segs := wordWrap("hello world foo", 11)
	if len(segs) < 2 {
		t.Errorf("expected multiple segments, got %v", segs)
	}
	for _, s := range segs {
		if len(s) > 11 {
			t.Errorf("segment too long: %q", s)
		}
	}
}

func TestWordWrap_ZeroWidth(t *testing.T) {
	segs := wordWrap("hello", 0)
	if len(segs) != 1 || segs[0] != "hello" {
		t.Errorf("zero width should return original: %v", segs)
	}
}

func TestWordWrap_Empty(t *testing.T) {
	segs := wordWrap("", 10)
	if len(segs) != 1 || segs[0] != "" {
		t.Errorf("empty string should return [\"\"], got %v", segs)
	}
}

func TestWordWrap_UTF8ByteSlicing(t *testing.T) {
	// 30 multi-byte runes, width=10 → should split into rune-safe segments.
	input := strings.Repeat("日", 30)
	segs := wordWrap(input, 10)
	for i, seg := range segs {
		runes := []rune(seg)
		if len(runes) > 10 {
			t.Errorf("segment %d has %d runes (max 10): %q", i, len(runes), seg)
		}
		for j, r := range seg {
			if r == 0xFFFD {
				t.Fatalf("segment %d has replacement char at byte %d — invalid UTF-8", i, j)
			}
		}
	}
	// Reassembled content must equal original.
	if got := strings.Join(segs, ""); got != input {
		t.Errorf("reassembled segments don't match original")
	}
}

func TestCellPlaceholder_WidthMatchesRequested(t *testing.T) {
	for _, width := range []int{5, 10, 15, 20} {
		for idx := 0; idx < 20; idx++ {
			ph := cellPlaceholder(idx, width)
			got := lipgloss.Width(ph)
			if got != width {
				t.Errorf("cellPlaceholder(%d, %d): lipgloss.Width=%d, want %d", idx, width, got, width)
			}
		}
	}
}

func TestWrapLines_SingleLine(t *testing.T) {
	got := wrapLines("hello world", 20, "  ")
	if len(got) != 1 || got[0] != "  hello world" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestWrapLines_MultiLine(t *testing.T) {
	got := wrapLines("line one\nline two", 20, "    ")
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(got), got)
	}
	if got[0] != "    line one" || got[1] != "    line two" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestWrapLines_WrapsLongLine(t *testing.T) {
	got := wrapLines("hello world foo bar", 11, "  ")
	if len(got) < 2 {
		t.Fatalf("expected wrapping, got %v", got)
	}
	for i, line := range got {
		if line != "" && !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d missing indent: %q", i, line)
		}
	}
}

func TestWrapLines_EmptyLines(t *testing.T) {
	got := wrapLines("above\n\nbelow", 20, "  ")
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(got), got)
	}
	if got[1] != "" {
		t.Errorf("expected empty middle line, got %q", got[1])
	}
}

func TestWrapLines_UTF8(t *testing.T) {
	input := strings.Repeat("日", 20)
	got := wrapLines(input, 10, "  ")
	for i, line := range got {
		if line != "" && !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d missing indent: %q", i, line)
		}
	}
	// Reassemble without indent and verify content preserved.
	var content string
	for _, line := range got {
		content += strings.TrimPrefix(line, "  ")
	}
	if content != input {
		t.Errorf("content mismatch after wrap")
	}
}
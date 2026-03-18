package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestRenderViewport_BasicScroll(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	content, scroll, total := renderViewport(lines, 0, 3)
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if scroll != 0 {
		t.Errorf("expected scroll=0, got %d", scroll)
	}
	if content != "a\nb\nc" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestRenderViewport_ScrollClamped(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	content, scroll, _ := renderViewport(lines, 100, 3)
	if scroll != 2 { // maxScroll = 5-3 = 2
		t.Errorf("expected scroll clamped to 2, got %d", scroll)
	}
	if content != "c\nd\ne" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestRenderViewport_NegativeScroll(t *testing.T) {
	lines := []string{"a", "b"}
	_, scroll, _ := renderViewport(lines, -5, 3)
	if scroll != 0 {
		t.Errorf("expected scroll clamped to 0, got %d", scroll)
	}
}

func TestScrollIndicator_AllVisible(t *testing.T) {
	result := scrollIndicator(0, 10, 5)
	if result != "" {
		t.Errorf("expected empty indicator when all visible, got %q", result)
	}
}

func TestScrollIndicator_Scrolled(t *testing.T) {
	result := scrollIndicator(3, 5, 10)
	if result == "" {
		t.Error("expected non-empty scroll indicator")
	}
}

func TestColWidths_Basic(t *testing.T) {
	widths := colWidths(100, []struct{ pct, min int }{{50, 10}, {30, 10}, {20, 10}})
	if widths[0] != 50 {
		t.Errorf("expected 50, got %d", widths[0])
	}
	if widths[1] != 30 {
		t.Errorf("expected 30, got %d", widths[1])
	}
	if widths[2] != 20 {
		t.Errorf("expected 20, got %d", widths[2])
	}
}

func TestColWidths_MinEnforced(t *testing.T) {
	widths := colWidths(20, []struct{ pct, min int }{{10, 15}})
	if widths[0] != 15 { // 20*10/100=2, but min is 15
		t.Errorf("expected min=15 enforced, got %d", widths[0])
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
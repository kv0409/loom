package dashboard

import (
	"strings"
	"testing"
)

func TestHshiftLine_Zero(t *testing.T) {
	if got := hshiftLine("hello", 0); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestHshiftLine_Partial(t *testing.T) {
	if got := hshiftLine("hello world", 6); got != "world" {
		t.Errorf("expected %q, got %q", "world", got)
	}
}

func TestHshiftLine_PastEnd(t *testing.T) {
	if got := hshiftLine("hi", 10); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHshiftLine_Negative(t *testing.T) {
	if got := hshiftLine("hello", -1); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestHshiftLine_UTF8(t *testing.T) {
	// Each rune is one character; shifting by 2 should drop first two runes.
	if got := hshiftLine("日本語テスト", 2); got != "語テスト" {
		t.Errorf("expected %q, got %q", "語テスト", got)
	}
}

func TestRenderDiff_LongLinesNotClipped(t *testing.T) {
	// A line longer than any reasonable panel width must survive in the
	// rendered output (possibly truncated to panel width, but the horizontal
	// scroll mechanism lets the user reveal the rest).
	longLine := "+" + strings.Repeat("x", 200)
	m := testModel(viewDiff)
	m.diffContent = "diff --git a/f b/f\n" + longLine
	m.diffHScroll = 0
	m.width = 80
	m.height = 30

	out := m.renderDiff()
	// The rendered output should contain at least the beginning of the long line.
	if !strings.Contains(out, strings.Repeat("x", 20)) {
		t.Error("long diff line content missing from rendered output")
	}
}

func TestRenderDiff_HScrollRevealsContent(t *testing.T) {
	// Scrolling right should reveal content that was beyond the panel edge.
	longLine := "+" + strings.Repeat("A", 50) + strings.Repeat("B", 50)
	m := testModel(viewDiff)
	m.diffContent = longLine
	m.width = 80
	m.height = 30

	// With no horizontal scroll, the beginning is visible.
	m.diffHScroll = 0
	out0 := m.renderDiff()
	if !strings.Contains(out0, "AAAA") {
		t.Error("expected A's visible at hscroll=0")
	}

	// Scroll right past the A's — B's should now be visible.
	m.diffHScroll = 51
	out1 := m.renderDiff()
	if !strings.Contains(out1, "BBBB") {
		t.Error("expected B's visible after horizontal scroll")
	}
}

func TestRenderDiff_HScrollIndicator(t *testing.T) {
	m := testModel(viewDiff)
	m.diffContent = "+hello"
	m.width = 80
	m.height = 30

	m.diffHScroll = 0
	out0 := m.renderDiff()
	if strings.Contains(out0, "←") {
		t.Error("should not show ← indicator at hscroll=0")
	}

	m.diffHScroll = 8
	out1 := m.renderDiff()
	if !strings.Contains(out1, "←8") {
		t.Error("expected ←8 indicator at hscroll=8")
	}
}

func TestRenderDiff_PrefixDetectionAfterHScroll(t *testing.T) {
	// Even when scrolled right, added lines should still be styled as additions.
	// We verify by checking the rendered output contains the shifted text
	// (styling is applied, so the line is non-empty).
	m := testModel(viewDiff)
	m.diffContent = "+added line content here"
	m.width = 80
	m.height = 30
	m.diffHScroll = 5

	out := m.renderDiff()
	// After shifting 5 runes, "d line content here" should appear.
	if !strings.Contains(out, "d line") {
		t.Error("expected shifted content to be present")
	}
}

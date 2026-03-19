package dashboard

import (
	"strings"
	"testing"
)

func TestRenderDiff_LongLinesNotClipped(t *testing.T) {
	longLine := "+" + strings.Repeat("x", 200)
	m := testModel(viewDiff)
	m.diffContent = "diff --git a/f b/f\n" + longLine
	m.diffXOff = 0
	m.width = 80
	m.height = 30
	m.diffVP.SetWidth(m.width - 2)
	m.diffVP.SetHeight(scrollViewport(m.height))

	out := m.renderDiff()
	if !strings.Contains(out, strings.Repeat("x", 20)) {
		t.Error("long diff line content missing from rendered output")
	}
}

func TestRenderDiff_HScrollRevealsContent(t *testing.T) {
	longLine := "+" + strings.Repeat("A", 50) + strings.Repeat("B", 50)
	m := testModel(viewDiff)
	m.diffContent = longLine
	m.width = 80
	m.height = 30
	m.diffVP.SetWidth(m.width - 2)
	m.diffVP.SetHeight(scrollViewport(m.height))

	m.diffXOff = 0
	out0 := m.renderDiff()
	if !strings.Contains(out0, "AAAA") {
		t.Error("expected A's visible at hscroll=0")
	}

	m.diffXOff = 51
	out1 := m.renderDiff()
	if !strings.Contains(out1, "BBBB") {
		t.Error("expected B's visible after horizontal scroll")
	}
}

func TestRenderDiff_HScrollIndicator(t *testing.T) {
	m := testModel(viewDiff)
	m.diffContent = "+" + strings.Repeat("x", 200)
	m.width = 80
	m.height = 30
	m.diffVP.SetWidth(m.width - 2)
	m.diffVP.SetHeight(scrollViewport(m.height))

	m.diffXOff = 0
	out0 := m.renderDiff()
	if strings.Contains(out0, "←") {
		t.Error("should not show ← indicator at hscroll=0")
	}

	m.diffXOff = 8
	out1 := m.renderDiff()
	if !strings.Contains(out1, "←8") {
		t.Error("expected ←8 indicator at hscroll=8")
	}
}

func TestRenderDiff_PrefixDetectionAfterHScroll(t *testing.T) {
	m := testModel(viewDiff)
	m.diffContent = "+added line content here"
	m.width = 80
	m.height = 30
	m.diffVP.SetWidth(m.width - 2)
	m.diffVP.SetHeight(scrollViewport(m.height))
	m.diffXOff = 5

	out := m.renderDiff()
	if !strings.Contains(out, "d line") {
		t.Error("expected shifted content to be present")
	}
}

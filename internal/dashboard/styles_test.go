package dashboard

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestSelectedRow_ASCIIPrefix(t *testing.T) {
	// Line starting with two ASCII spaces — the common case for most views.
	line := "  agent-001  active  idle"
	result := selectedRow(line)
	if !strings.Contains(result, "▸") {
		t.Errorf("expected ▸ prefix, got %q", result)
	}
	// The result must be valid UTF-8.
	if !utf8.ValidString(result) {
		t.Errorf("selectedRow produced invalid UTF-8: %q", result)
	}
}

func TestSelectedRow_MultiBytePrefixNotCorrupted(t *testing.T) {
	// If a line already starts with a multi-byte character (like ▸), selectedRow
	// must NOT byte-slice in the middle of the character.
	line := "▸ LOOM-001 task  some title"
	result := selectedRow(line)
	if !utf8.ValidString(result) {
		t.Errorf("selectedRow produced invalid UTF-8 for multi-byte prefix: raw bytes=%x", []byte(result))
	}
}

func TestSelectedRow_EmptyLine(t *testing.T) {
	result := selectedRow("")
	if !utf8.ValidString(result) {
		t.Errorf("selectedRow produced invalid UTF-8 for empty input")
	}
}

func TestSelectedRow_SingleChar(t *testing.T) {
	result := selectedRow("x")
	if !utf8.ValidString(result) {
		t.Errorf("selectedRow produced invalid UTF-8 for single char input")
	}
}

func TestStyledReplacementWidthParity(t *testing.T) {
	// agentPillPlain vs agentPillFor must have identical visual width.
	for _, id := range []string{"orchestrator", "builder-001", "b", "researcher-001"} {
		plain := agentPillPlain(id)
		styled := agentPillFor(id, id)
		if pw, sw := lipgloss.Width(plain), lipgloss.Width(styled); pw != sw {
			t.Errorf("agentPillPlain(%q) width=%d, agentPillFor width=%d", id, pw, sw)
		}
	}
}

func TestSelectedRow_UnicodeContent(t *testing.T) {
	// Lines with various Unicode characters that might appear in issues/agents.
	lines := []string{
		"  ○● LOOM-001 task title",
		"  ▶● LOOM-002 assigned",
		"  ✔◈ LOOM-003 done epic",
		"▸ ✦ LOOM-004 bug title", // already has ▸ prefix
	}
	for _, line := range lines {
		result := selectedRow(line)
		if !utf8.ValidString(result) {
			t.Errorf("selectedRow(%q) produced invalid UTF-8: raw=%x", line, []byte(result))
		}
	}
}

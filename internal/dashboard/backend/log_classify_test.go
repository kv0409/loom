package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyLogLine_Lifecycle(t *testing.T) {
	cat, _, ok := ClassifyLogLine("[acp] activating agent builder-001")
	if !ok || cat != "lifecycle" {
		t.Errorf("expected lifecycle, got %q ok=%v", cat, ok)
	}
}

func TestClassifyLogLine_Error(t *testing.T) {
	cat, _, ok := ClassifyLogLine("something failed badly")
	if !ok || cat != "error" {
		t.Errorf("expected error, got %q ok=%v", cat, ok)
	}
}

func TestClassifyLogLine_Stderr(t *testing.T) {
	cat, _, ok := ClassifyLogLine("[acp-stderr] some output")
	if !ok || cat != "stderr" {
		t.Errorf("expected stderr, got %q ok=%v", cat, ok)
	}
}

func TestClassifyLogLine_Warn(t *testing.T) {
	cat, _, ok := ClassifyLogLine("WARNING: disk full")
	if !ok || cat != "warn" {
		t.Errorf("expected warn, got %q ok=%v", cat, ok)
	}
}

func TestClassifyLogLine_Skipped(t *testing.T) {
	_, _, ok := ClassifyLogLine("[acp-notif] something")
	if ok {
		t.Error("expected acp-notif to be skipped")
	}
}

func TestClassifyLogLine_Unclassified(t *testing.T) {
	_, _, ok := ClassifyLogLine("just a normal line")
	if ok {
		t.Error("expected unclassified line to be skipped")
	}
}

func TestExtractAgent_ACP(t *testing.T) {
	got := ExtractAgent("[acp] builder-001: doing stuff")
	if got != "builder-001" {
		t.Errorf("expected builder-001, got %q", got)
	}
}

func TestExtractAgent_Activating(t *testing.T) {
	got := ExtractAgent("[acp] activating agent lead-002")
	if got != "lead-002" {
		t.Errorf("expected lead-002, got %q", got)
	}
}

func TestExtractAgent_None(t *testing.T) {
	got := ExtractAgent("no agent here")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func mkFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadDaemonLog_Basic(t *testing.T) {
	dir := t.TempDir()
	mkFile(t, dir, "logs/daemon.log",
		"[acp] activating agent builder-001\n"+
			"[acp-notif] skip this\n"+
			"something failed\n"+
			"WARNING: low memory\n"+
			"normal line ignored\n")

	lines := ReadDaemonLog(dir)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0].Category != "lifecycle" {
		t.Errorf("line 0: expected lifecycle, got %q", lines[0].Category)
	}
	if lines[1].Category != "error" {
		t.Errorf("line 1: expected error, got %q", lines[1].Category)
	}
	if lines[2].Category != "warn" {
		t.Errorf("line 2: expected warn, got %q", lines[2].Category)
	}
}

func TestReadDaemonLog_Empty(t *testing.T) {
	dir := t.TempDir()
	lines := ReadDaemonLog(dir)
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for missing log, got %d", len(lines))
	}
}

func TestReadDaemonLog_FilterByCategory(t *testing.T) {
	dir := t.TempDir()
	mkFile(t, dir, "logs/daemon.log",
		"[acp] activating agent builder-001\n"+
			"something failed\n"+
			"WARNING: low memory\n")

	lines := ReadDaemonLog(dir)
	var errors []LogLine
	for _, l := range lines {
		if l.Category == "error" {
			errors = append(errors, l)
		}
	}
	if len(errors) != 1 {
		t.Fatalf("expected 1 error line, got %d", len(errors))
	}
}

func TestReadDaemonLog_FilterByAgent(t *testing.T) {
	dir := t.TempDir()
	mkFile(t, dir, "logs/daemon.log",
		"[acp] builder-001: something failed\n"+
			"[acp] lead-002: process exited\n")

	lines := ReadDaemonLog(dir)
	var b001 []LogLine
	for _, l := range lines {
		if l.Agent == "builder-001" {
			b001 = append(b001, l)
		}
	}
	if len(b001) != 1 {
		t.Fatalf("expected 1 line for builder-001, got %d", len(b001))
	}
}

func TestLogSummary(t *testing.T) {
	lines := []LogLine{
		{Category: "error", Agent: "builder-001"},
		{Category: "error", Agent: "builder-001"},
		{Category: "warn", Agent: "lead-002"},
		{Category: "lifecycle", Agent: "builder-001"},
		{Category: "lifecycle", Agent: "lead-002"},
	}
	s := LogSummary(lines)
	if s.Errors != 2 {
		t.Errorf("expected 2 errors, got %d", s.Errors)
	}
	if s.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", s.Warnings)
	}
	if s.Total != 5 {
		t.Errorf("expected 5 total, got %d", s.Total)
	}
	if len(s.HotAgents) != 2 {
		t.Fatalf("expected 2 hot agents, got %d", len(s.HotAgents))
	}
	// builder-001 has 3 events, lead-002 has 2
	if s.HotAgents[0].Agent != "builder-001" || s.HotAgents[0].Count != 3 {
		t.Errorf("expected builder-001 with 3, got %s with %d", s.HotAgents[0].Agent, s.HotAgents[0].Count)
	}
}

func TestLogSummary_Empty(t *testing.T) {
	s := LogSummary(nil)
	if s.Total != 0 || len(s.HotAgents) != 0 {
		t.Errorf("expected empty summary, got total=%d agents=%d", s.Total, len(s.HotAgents))
	}
}

func TestLogSummary_TopThreeOnly(t *testing.T) {
	lines := []LogLine{
		{Category: "error", Agent: "a1"},
		{Category: "error", Agent: "a1"},
		{Category: "error", Agent: "a2"},
		{Category: "error", Agent: "a2"},
		{Category: "error", Agent: "a3"},
		{Category: "error", Agent: "a4"},
		{Category: "error", Agent: "a4"},
		{Category: "error", Agent: "a4"},
	}
	s := LogSummary(lines)
	if len(s.HotAgents) != 3 {
		t.Errorf("expected max 3 hot agents, got %d", len(s.HotAgents))
	}
}

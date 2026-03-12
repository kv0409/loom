package acp

import (
	"testing"
)

func TestReadOutputFile_NDJSONFiltersToolSummary(t *testing.T) {
	// NDJSON file with mixed event kinds — only tool_summary should be preferred.
	input := `{"kind":"token_chunk","ts":"12:00:01","content":"thinking..."}
{"kind":"tool_summary","ts":"12:00:02","content":"Called execute_bash: ls -la"}
{"kind":"token_chunk","ts":"12:00:03","content":"more tokens"}
{"kind":"tool_summary","ts":"12:00:04","content":"Called fs_read: main.go"}
{"kind":"message","ts":"12:00:05","content":"Done."}
`
	events := ReadOutputFile([]byte(input))

	// Verify all 5 events are parsed.
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify kinds are correctly assigned from NDJSON.
	wantKinds := []Kind{TokenChunk, ToolSummary, TokenChunk, ToolSummary, CompleteMessage}
	for i, ev := range events {
		if ev.Kind != wantKinds[i] {
			t.Errorf("event[%d]: want kind %v, got %v (content=%q)", i, wantKinds[i], ev.Kind, ev.Content)
		}
	}

	// Simulate fetchActivity logic: prefer last ToolSummary.
	var last *ACPEvent
	for i := range events {
		if events[i].Kind == ToolSummary {
			last = &events[i]
		}
	}
	if last == nil {
		t.Fatal("expected a ToolSummary event, got none")
	}
	if last.Content != "Called fs_read: main.go" {
		t.Errorf("expected last ToolSummary content %q, got %q", "Called fs_read: main.go", last.Content)
	}
}

func TestReadOutputFile_NDJSONOnlyTokenChunks_FallsBackToTokenChunk(t *testing.T) {
	// No tool_summary events — fetchActivity should fall back to last TokenChunk.
	input := `{"kind":"token_chunk","ts":"12:00:01","content":"first chunk"}
{"kind":"token_chunk","ts":"12:00:02","content":"second chunk"}
`
	events := ReadOutputFile([]byte(input))
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Simulate fetchActivity fallback logic.
	var lastTool, lastChunk *ACPEvent
	for i := range events {
		if events[i].Kind == ToolSummary {
			lastTool = &events[i]
		}
	}
	if lastTool != nil {
		t.Fatal("expected no ToolSummary events")
	}
	for i := range events {
		if events[i].Kind == TokenChunk {
			lastChunk = &events[i]
		}
	}
	if lastChunk == nil || lastChunk.Content != "second chunk" {
		t.Errorf("expected fallback to last TokenChunk %q, got %v", "second chunk", lastChunk)
	}
}

func TestReadOutputFile_MixedLegacyAndNDJSON(t *testing.T) {
	// Legacy lines before NDJSON lines (mixed file after daemon restart).
	input := `[12:00:00] [session_update]	legacy tool call
{"kind":"tool_summary","ts":"12:00:01","content":"ndjson tool call"}
`
	events := ReadOutputFile([]byte(input))

	var toolSummaries []ACPEvent
	for _, ev := range events {
		if ev.Kind == ToolSummary {
			toolSummaries = append(toolSummaries, ev)
		}
	}
	if len(toolSummaries) != 2 {
		t.Fatalf("expected 2 ToolSummary events (1 legacy + 1 NDJSON), got %d", len(toolSummaries))
	}
	if toolSummaries[0].Content != "legacy tool call" {
		t.Errorf("expected legacy content %q, got %q", "legacy tool call", toolSummaries[0].Content)
	}
	if toolSummaries[1].Content != "ndjson tool call" {
		t.Errorf("expected ndjson content %q, got %q", "ndjson tool call", toolSummaries[1].Content)
	}
}

func TestReadOutputFile_EmptyFile(t *testing.T) {
	events := ReadOutputFile([]byte(""))
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty file, got %d", len(events))
	}
}

func TestReadOutputFile_InvalidNDJSONLineSkipped(t *testing.T) {
	input := `{"kind":"tool_summary","content":"valid"}
{invalid json}
{"kind":"tool_summary","content":"also valid"}
`
	events := ReadOutputFile([]byte(input))
	// Invalid JSON line should be skipped; 2 valid events expected.
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(events), events)
	}
	for _, ev := range events {
		if ev.Kind != ToolSummary {
			t.Errorf("expected ToolSummary, got %v", ev.Kind)
		}
	}
}

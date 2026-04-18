package acp

import (
	"context"
	"fmt"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestDrainOutputReturnsAllEventsWhileRecentOutputStaysBounded(t *testing.T) {
	c := &Client{}

	for i := 0; i < 75; i++ {
		c.appendEvent(ACPEvent{Kind: ToolSummary, Content: fmt.Sprintf("event-%02d", i)})
	}

	recent := c.RecentOutput(100)
	if len(recent) != 50 {
		t.Fatalf("expected recent output to keep last 50 events, got %d", len(recent))
	}
	if recent[0].Content != "event-25" {
		t.Fatalf("expected recent output to start at event-25, got %q", recent[0].Content)
	}

	drained := c.DrainOutput()
	if len(drained) != 75 {
		t.Fatalf("expected drain to return all 75 events, got %d", len(drained))
	}
	if drained[0].Content != "event-00" || drained[74].Content != "event-74" {
		t.Fatalf("unexpected drained range: first=%q last=%q", drained[0].Content, drained[74].Content)
	}

	if drainedAgain := c.DrainOutput(); len(drainedAgain) != 0 {
		t.Fatalf("expected second drain to be empty, got %d events", len(drainedAgain))
	}
}

func TestHandleNotification_ToolCallTracking(t *testing.T) {
	c := &Client{toolCalls: make(map[string]*ToolCall)}
	ctx := context.Background()

	// Simulate tool_call creation.
	if err := c.SessionUpdate(ctx, acpsdk.SessionNotification{
		Update: acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
			ToolCallId: "call_001",
			Title:      "Reading main.go",
			Kind:       acpsdk.ToolKindRead,
			Status:     acpsdk.ToolCallStatusPending,
			Locations:  []acpsdk.ToolCallLocation{{Path: "/src/main.go"}},
		}},
	}); err != nil {
		t.Fatalf("SessionUpdate tool_call: %v", err)
	}

	// Simulate tool_call_update with status change.
	completed := acpsdk.ToolCallStatusCompleted
	if err := c.SessionUpdate(ctx, acpsdk.SessionNotification{
		Update: acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			ToolCallId: "call_001",
			Status:     &completed,
		}},
	}); err != nil {
		t.Fatalf("SessionUpdate tool_call_update: %v", err)
	}

	// Simulate a second tool call.
	if err := c.SessionUpdate(ctx, acpsdk.SessionNotification{
		Update: acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
			ToolCallId: "call_002",
			Title:      "Writing output.go",
			Kind:       acpsdk.ToolKindEdit,
			Status:     acpsdk.ToolCallStatusCompleted,
		}},
	}); err != nil {
		t.Fatalf("SessionUpdate tool_call 2: %v", err)
	}

	calls := c.RecentToolCalls()
	// call_001 creates one entry (tool_call with title), update doesn't add another.
	// call_002 creates one entry (tool_call with title).
	if len(calls) != 2 {
		t.Fatalf("expected 2 recent calls, got %d", len(calls))
	}
	if calls[0].Title != "Reading main.go" || calls[0].Kind != "read" {
		t.Errorf("call[0]: got title=%q kind=%q", calls[0].Title, calls[0].Kind)
	}
	if calls[1].Title != "Writing output.go" || calls[1].Kind != "edit" {
		t.Errorf("call[1]: got title=%q kind=%q", calls[1].Title, calls[1].Kind)
	}

	// Verify the update was applied to the tracked call.
	tc := c.toolCalls["call_001"]
	if tc.Status != "completed" {
		t.Errorf("call_001 status: expected completed, got %q", tc.Status)
	}
}

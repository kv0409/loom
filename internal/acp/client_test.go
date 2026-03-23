package acp

import (
	"encoding/json"
	"fmt"
	"testing"
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

	// Simulate tool_call creation.
	send(t, c, `{
		"update": {
			"sessionUpdate": "tool_call",
			"toolCallId": "call_001",
			"title": "Reading main.go",
			"kind": "read",
			"status": "pending",
			"locations": [{"path": "/src/main.go"}]
		}
	}`)

	// Simulate tool_call_update with status change.
	send(t, c, `{
		"update": {
			"sessionUpdate": "tool_call_update",
			"toolCallId": "call_001",
			"status": "completed"
		}
	}`)

	// Simulate a second tool call.
	send(t, c, `{
		"update": {
			"sessionUpdate": "tool_call",
			"toolCallId": "call_002",
			"title": "Writing output.go",
			"kind": "edit",
			"status": "completed"
		}
	}`)

	calls := c.RecentToolCalls()
	// call_001 creates entry on "pending" (has title), then "completed" adds another snapshot.
	// call_002 creates entry on "completed" (has title).
	if len(calls) != 3 {
		t.Fatalf("expected 3 recent calls, got %d", len(calls))
	}
	if calls[0].Title != "Reading main.go" || calls[0].Kind != "read" {
		t.Errorf("call[0]: got title=%q kind=%q", calls[0].Title, calls[0].Kind)
	}
	if calls[1].Status != "completed" {
		t.Errorf("call[1]: expected completed, got %q", calls[1].Status)
	}
	if calls[2].Title != "Writing output.go" || calls[2].Kind != "edit" {
		t.Errorf("call[2]: got title=%q kind=%q", calls[2].Title, calls[2].Kind)
	}
}

func send(t *testing.T, c *Client, params string) {
	t.Helper()
	c.handleNotification(&jsonRPCNotification{
		Method: "session/update",
		Params: json.RawMessage(params),
	})
}

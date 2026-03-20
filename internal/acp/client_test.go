package acp

import (
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

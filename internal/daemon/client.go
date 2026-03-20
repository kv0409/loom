package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
)

// call dials the daemon Unix socket, sends req, and returns the response.
func call(loomRoot string, req Request) (Response, error) {
	conn, err := net.Dial("unix", SockPath(loomRoot))
	if err != nil {
		return Response{}, fmt.Errorf("daemon dial: %w", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("daemon send: %w", err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("daemon recv: %w", err)
	}
	if !resp.OK {
		return resp, fmt.Errorf("daemon: %s", resp.Error)
	}
	return resp, nil
}

// Nudge sends a nudge notification to an agent via the daemon.
func Nudge(loomRoot, agentID, message string) error {
	_, err := call(loomRoot, Request{Action: "nudge", AgentID: agentID, Message: message})
	return err
}

// Message sends a raw message to an agent via the daemon.
func Message(loomRoot, agentID, message string) error {
	_, err := call(loomRoot, Request{Action: "message", AgentID: agentID, Message: message})
	return err
}

// Heartbeat updates an agent heartbeat via the daemon when available, or falls back
// to a direct file write when the daemon is not reachable.
func Heartbeat(loomRoot, agentID string) error {
	if _, err := os.Stat(SockPath(loomRoot)); err == nil {
		if _, err := call(loomRoot, Request{Action: "heartbeat", AgentID: agentID}); err == nil {
			return nil
		}
	}
	return agent.UpdateHeartbeat(loomRoot, agentID)
}

// Kill terminates an agent via the daemon.
func Kill(loomRoot, agentID string, cleanup bool) error {
	_, err := call(loomRoot, Request{Action: "kill", AgentID: agentID, Cleanup: cleanup})
	return err
}

// Cancel sends a session/cancel to an ACP agent via the daemon.
func Cancel(loomRoot, agentID string) error {
	_, err := call(loomRoot, Request{Action: "cancel", AgentID: agentID})
	return err
}

// Output retrieves recent output events from an agent via the daemon.
func Output(loomRoot, agentID string, lines int) ([]acp.ACPEvent, error) {
	resp, err := call(loomRoot, Request{Action: "output", AgentID: agentID, Lines: lines})
	if err != nil {
		return nil, err
	}
	// resp.Data is decoded as []interface{} from JSON; re-encode and decode into []acp.ACPEvent.
	b, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("output marshal: %w", err)
	}
	var events []acp.ACPEvent
	if err := json.Unmarshal(b, &events); err != nil {
		return nil, fmt.Errorf("output unmarshal: %w", err)
	}
	return events, nil
}

// Invalidate marks daemon cache sections dirty so the next watcher tick reloads them.
func Invalidate(loomRoot string, targets ...string) error {
	_, err := call(loomRoot, Request{Action: "invalidate", Targets: targets})
	return err
}

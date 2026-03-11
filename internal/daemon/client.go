package daemon

import (
	"encoding/json"
	"fmt"
	"net"
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

// Output retrieves recent output lines from an agent via the daemon.
func Output(loomRoot, agentID string, lines int) (string, error) {
	resp, err := call(loomRoot, Request{Action: "output", AgentID: agentID, Lines: lines})
	if err != nil {
		return "", err
	}
	s, _ := resp.Data.(string)
	return s, nil
}

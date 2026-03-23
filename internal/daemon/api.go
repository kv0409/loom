package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/karanagi/loom/internal/agent"
)

// SockPath returns the daemon Unix socket path for a given loom root.
func SockPath(loomRoot string) string {
	return filepath.Join(loomRoot, "daemon.sock")
}

// Request is the JSON wire format for daemon API calls.
type Request struct {
	Action     string   `json:"action"`
	AgentID    string   `json:"agent_id"`
	Message    string   `json:"message,omitempty"`
	Lines      int      `json:"lines,omitempty"`
	Cleanup    bool     `json:"cleanup,omitempty"`
	Targets    []string `json:"targets,omitempty"`
	IssueIDs   []string `json:"issue_ids,omitempty"`
	AgentIDs   []string `json:"agent_ids,omitempty"`
	MailAgents []string `json:"mail_agents,omitempty"`
}

// Response is the JSON wire format for daemon API replies.
type Response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func okResp(data any) Response    { return Response{OK: true, Data: data} }
func errResp(msg string) Response { return Response{OK: false, Error: msg} }

// startAPI opens the Unix socket listener and serves connections.
// Called from Daemon.Start(). The listener is closed by stopAPI.
func (d *Daemon) startAPI() error {
	sock := SockPath(d.LoomRoot)
	os.Remove(sock) // clean stale socket
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return fmt.Errorf("api listen: %w", err)
	}
	d.apiLn = ln
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-d.stop:
					return
				default:
					log.Printf("[api] accept: %v", err)
					continue
				}
			}
			go d.handleConn(conn)
		}
	}()
	return nil
}

// stopAPI closes the listener and removes the socket file.
func (d *Daemon) stopAPI() {
	if d.apiLn != nil {
		d.apiLn.Close()
	}
	os.Remove(SockPath(d.LoomRoot))
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(errResp("bad request: " + err.Error()))
		return
	}
	var resp Response
	switch req.Action {
	case "nudge":
		resp = d.apiNudge(req)
	case "message":
		resp = d.apiMessage(req)
	case "heartbeat":
		resp = d.apiHeartbeat(req)
	case "kill":
		resp = d.apiKill(req)
	case "cancel":
		resp = d.apiCancel(req)
	case "output":
		resp = d.apiOutput(req)
	case "invalidate":
		resp = d.apiInvalidate(req)
	case "refresh":
		resp = d.apiRefresh(req)
	case "snapshot":
		resp = d.apiSnapshot()
	default:
		resp = errResp("unknown action: " + req.Action)
	}
	json.NewEncoder(conn).Encode(resp)
}

func (d *Daemon) apiNudge(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	nr := d.notify(a, "[LOOM] Nudge: "+req.Message)
	if nr.Outcome == NotifyFailed {
		return errResp("notify failed: " + nr.Reason)
	}
	return okResp(nr)
}

func (d *Daemon) apiMessage(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	nr := d.notify(a, req.Message)
	if nr.Outcome == NotifyFailed {
		return errResp("notify failed: " + nr.Reason)
	}
	return okResp(nr)
}

func (d *Daemon) apiHeartbeat(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	a.Heartbeat = time.Now()
	if err := agent.Save(d.LoomRoot, a); err != nil {
		return errResp("heartbeat save failed: " + err.Error())
	}
	if d.state != nil {
		if err := d.state.storeAgent(a); err != nil {
			d.invalidateState(stateTargetAgents)
		} else {
			d.signalStateChange(stateTargetAgents)
		}
	}
	if d.state == nil {
		d.invalidateState(stateTargetAgents)
	}
	return okResp(nil)
}

func (d *Daemon) apiKill(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	d.UnregisterACPClient(req.AgentID)
	_ = a // loaded to verify existence
	refreshOpts := d.killRefreshOpts(req.AgentID, nil)
	if err := agent.Kill(d.LoomRoot, req.AgentID, req.Cleanup); err != nil {
		return errResp("kill failed: " + err.Error())
	}
	d.refreshCachedState(refreshOpts)
	return okResp(nil)
}

func (d *Daemon) apiCancel(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	if a.ACPSessionID == "" {
		return errResp("agent has no active ACP session")
	}
	d.mu.Lock()
	c := d.acpClients[req.AgentID]
	d.mu.Unlock()
	if c == nil {
		return errResp("no ACP client for agent")
	}
	if err := c.CancelSession(a.ACPSessionID); err != nil {
		return errResp("cancel failed: " + err.Error())
	}
	return okResp(nil)
}

func (d *Daemon) apiOutput(req Request) Response {
	_, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	lines := req.Lines
	if lines <= 0 {
		lines = 50
	}
	return okResp(d.GetACPOutput(req.AgentID, lines))
}

func (d *Daemon) apiInvalidate(req Request) Response {
	if err := d.invalidateTargets(req.Targets...); err != nil {
		return errResp("invalidate failed: " + err.Error())
	}
	return okResp(nil)
}

func (d *Daemon) apiRefresh(req Request) Response {
	if d.state == nil {
		return errResp("state unavailable")
	}
	if len(req.IssueIDs) == 0 && len(req.AgentIDs) == 0 && len(req.MailAgents) == 0 {
		return errResp("no refresh targets provided")
	}
	for _, id := range req.IssueIDs {
		if err := d.state.refreshIssue(id); err != nil {
			d.invalidateState(stateTargetIssues)
			return errResp("refresh issue failed: " + err.Error())
		}
	}
	for _, id := range req.AgentIDs {
		if err := d.state.refreshAgent(id); err != nil {
			d.invalidateState(stateTargetAgents)
			return errResp("refresh agent failed: " + err.Error())
		}
	}
	for _, agentID := range req.MailAgents {
		if err := d.state.refreshMailbox(agentID); err != nil {
			d.invalidateState(stateTargetMail)
			return errResp("refresh mailbox failed: " + err.Error())
		}
	}
	d.signalStateChange(refreshStateTargets(RefreshOpts{
		IssueIDs:   req.IssueIDs,
		AgentIDs:   req.AgentIDs,
		MailAgents: req.MailAgents,
	})...)
	return okResp(nil)
}

func (d *Daemon) apiSnapshot() Response {
	if d.state == nil {
		return errResp("state unavailable")
	}
	if err := d.state.syncAgents(); err != nil {
		return errResp("sync agents failed: " + err.Error())
	}
	if err := d.state.syncIssues(); err != nil {
		return errResp("sync issues failed: " + err.Error())
	}
	if err := d.state.syncMail(); err != nil {
		return errResp("sync mail failed: " + err.Error())
	}
	return okResp(ControlSnapshot{
		Agents:   d.state.agentsList(),
		Issues:   d.state.allIssues(),
		Unread:   d.state.unreadCount(),
		Activity: d.collectActivity(),
	})
}

func (d *Daemon) invalidateTargets(names ...string) error {
	if len(names) == 0 {
		d.invalidateState()
		return nil
	}

	targets := make([]stateTarget, 0, len(names))
	for _, name := range names {
		switch name {
		case "issues":
			targets = append(targets, stateTargetIssues)
		case "agents":
			targets = append(targets, stateTargetAgents)
		case "mail":
			targets = append(targets, stateTargetMail)
		default:
			return fmt.Errorf("unknown target %q", name)
		}
	}
	d.invalidateState(targets...)
	return nil
}

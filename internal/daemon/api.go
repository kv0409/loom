package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/tmux"
)

// SockPath returns the daemon Unix socket path for a given loom root.
func SockPath(loomRoot string) string {
	return filepath.Join(loomRoot, "daemon.sock")
}

// Request is the JSON wire format for daemon API calls.
type Request struct {
	Action  string `json:"action"`
	AgentID string `json:"agent_id"`
	Message string `json:"message,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Cleanup bool   `json:"cleanup,omitempty"`
}

// Response is the JSON wire format for daemon API replies.
type Response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func okResp(data any) Response  { return Response{OK: true, Data: data} }
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
	case "kill":
		resp = d.apiKill(req)
	case "output":
		resp = d.apiOutput(req)
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
	d.notify(a, "[LOOM] Nudge: "+req.Message)
	return okResp(nil)
}

func (d *Daemon) apiMessage(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	d.notify(a, req.Message)
	return okResp(nil)
}

func (d *Daemon) apiKill(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	if a.Config.KiroMode == "acp" {
		d.UnregisterACPClient(req.AgentID)
	}
	if err := agent.Kill(d.LoomRoot, req.AgentID, req.Cleanup); err != nil {
		return errResp("kill failed: " + err.Error())
	}
	return okResp(nil)
}

func (d *Daemon) apiOutput(req Request) Response {
	a, err := agent.Load(d.LoomRoot, req.AgentID)
	if err != nil {
		return errResp("agent not found: " + req.AgentID)
	}
	lines := req.Lines
	if lines <= 0 {
		lines = 50
	}
	if a.Config.KiroMode == "acp" {
		out := d.GetACPOutput(req.AgentID, lines)
		return okResp(strings.Join(out, "\n"))
	}
	if a.TmuxTarget == "" {
		return errResp("no tmux target for agent")
	}
	out, err := tmux.CapturePane(a.TmuxTarget)
	if err != nil {
		return errResp("capture failed: " + err.Error())
	}
	// Trim to requested line count from the end.
	parts := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return okResp(strings.Join(parts, "\n"))
}

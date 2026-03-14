// Package acp provides a JSON-RPC 2.0 client for communicating with kiro-cli
// running in ACP (Agent Client Protocol) mode over stdio.
package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// PermissionHandler decides whether to approve a server→client request.
// It receives the tool name and command string extracted from the request.
// Return true to approve, false to deny.
type PermissionHandler func(tool, command string) bool

// Client manages a kiro-cli ACP subprocess and its JSON-RPC lifecycle.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex // serialises writes
	nextID atomic.Int64
	exited atomic.Bool

	// Background reader delivers responses to waiting callers.
	pending   map[int64]chan jsonRPCResponse
	pendingMu sync.Mutex

	// Output buffer for dashboard.
	outMu         sync.Mutex
	lastResponses []ACPEvent

	// AgentID for audit logging.
	AgentID string

	// OnPermission is called for server→client permission requests.
	OnPermission PermissionHandler

	waitOnce sync.Once
	waitErr  error
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// jsonRPCNotification is a server-initiated notification (no ID).
type jsonRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// serverRequest is a JSON-RPC 2.0 request sent from server to client.
// ID is json.RawMessage because kiro-cli sends string UUIDs for permission
// request IDs, not integers.
type serverRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc %d: %s", e.Code, e.Message) }

// InitializeResult holds the response from the initialize call.
type InitializeResult struct {
	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// SessionResult holds the response from session/new.
type SessionResult struct {
	SessionID string `json:"sessionId"`
}

// PromptResult holds the response from session/prompt.
type PromptResult struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock is a single content element in a prompt response.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// NewClient spawns kiro-cli in ACP mode and returns a connected Client.
func NewClient(command string, workDir string, env []string, extraArgs ...string) (*Client, error) {
	args := append([]string{"acp"}, extraArgs...)
	cmd := exec.Command(command, args...)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if len(env) > 0 {
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %s: %w", command, err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[int64]chan jsonRPCResponse),
	}

	// Background reader: dispatches responses and captures notifications.
	go c.readLoop(bufio.NewReader(stdoutPipe))
	// Capture stderr for debugging.
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log.Printf("[acp-stderr] %s", scanner.Text())
		}
	}()
	go func() {
		c.waitOnce.Do(func() { c.waitErr = cmd.Wait() })
		log.Printf("[acp] process exited: pid=%d err=%v", cmd.Process.Pid, c.waitErr)
		c.exited.Store(true)
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	}()

	return c, nil
}

// readLoop continuously reads stdout and dispatches lines.
func (c *Client) readLoop(r *bufio.Reader) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return // pipe closed
		}
		if len(line) == 0 {
			continue
		}

		// Probe the raw JSON to determine message type.
		// Server requests have a "method" field; responses do not.
		var probe struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}

		hasID := len(probe.ID) > 0 && string(probe.ID) != "null"

		if hasID && probe.Method != "" {
			// Server→client request (e.g. permission prompt). ID may be string UUID.
			var req serverRequest
			if err := json.Unmarshal(line, &req); err != nil {
				continue
			}
			c.handleServerRequest(&req)
			continue
		}

		if hasID {
			// Response to one of our requests (integer ID).
			var resp jsonRPCResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			c.pendingMu.Lock()
			ch, ok := c.pending[resp.ID]
			c.pendingMu.Unlock()
			if ok {
				ch <- resp
			}
			continue
		}

		// Notification (no ID).
		var notif jsonRPCNotification
		if err := json.Unmarshal(line, &notif); err != nil {
			continue
		}
		c.handleNotification(&notif)
	}
}

// handleServerRequest processes a server→client JSON-RPC request (e.g. permission prompts).
// It extracts tool/command info, checks the deny list via OnPermission, sends an
// approve or deny response back, and logs the decision for audit.
func (c *Client) handleServerRequest(req *serverRequest) {
	// Extract tool and command from params for deny-list checking.
	var params struct {
		Tool    string `json:"tool"`
		Command string `json:"command"`
		Name    string `json:"name"`
		Action  struct {
			Tool    string `json:"tool"`
			Command string `json:"command"`
			Name    string `json:"name"`
		} `json:"action"`
	}
	_ = json.Unmarshal(req.Params, &params)

	tool := params.Tool
	if tool == "" {
		tool = params.Action.Tool
	}
	if tool == "" {
		tool = params.Name
	}
	if tool == "" {
		tool = params.Action.Name
	}
	command := params.Command
	if command == "" {
		command = params.Action.Command
	}

	approved := true
	if c.OnPermission != nil {
		approved = c.OnPermission(tool, command)
	}

	action := "approved"
	if !approved {
		action = "denied"
	}
	log.Printf("[acp-permission] agent=%s method=%s tool=%q command=%q decision=%s", c.AgentID, req.Method, tool, command, action)

	// Send JSON-RPC response back to kiro-cli.
	// kiro-cli expects: {"outcome": {"outcome": "selected", "optionId": "allow_once"|"deny"}}
	optionID := "allow_once"
	if !approved {
		optionID = "deny"
	}
	result := map[string]interface{}{
		"outcome": map[string]string{
			"outcome":  "selected",
			"optionId": optionID,
		},
	}
	resp := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  interface{}     `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[acp-permission] marshal error: %v", err)
		return
	}
	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		log.Printf("[acp-permission] write error: %v", err)
	}
}

// handleNotification processes server notifications and captures output.
func (c *Client) handleNotification(n *jsonRPCNotification) {
	switch n.Method {
	case "session/update":
		var params struct {
			Update struct {
				SessionUpdate string `json:"sessionUpdate"`
				Content       struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		}
		if err := json.Unmarshal(n.Params, &params); err != nil {
			return
		}
		if params.Update.Content.Text != "" {
			c.appendEvent(ACPEvent{
				Kind:    eventKind(params.Update.SessionUpdate),
				Content: params.Update.Content.Text,
			})
		}
	case "_kiro.dev/metadata", "_kiro.dev/mcp/server_initialized", "_kiro.dev/commands/available", "_kiro.dev/session/update":
		// Internal kiro events — skip.
	default:
		log.Printf("[acp-notif] %s", n.Method)
	}
}

func (c *Client) appendEvent(ev ACPEvent) {
	c.outMu.Lock()
	c.lastResponses = append(c.lastResponses, ev)
	if len(c.lastResponses) > 50 {
		c.lastResponses = c.lastResponses[len(c.lastResponses)-50:]
	}
	c.outMu.Unlock()
}

// call sends a JSON-RPC request and waits for the matching response.
func (c *Client) call(method string, params interface{}, result interface{}) error {
	id := c.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("acp: marshal %s: %w", method, err)
	}

	// Register a channel for the response before sending.
	ch := make(chan jsonRPCResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("acp: write %s: %w", method, err)
	}

	// Wait for the background reader to deliver our response.
	var resp jsonRPCResponse
	var ok bool
	select {
	case resp, ok = <-ch:
	case <-time.After(30 * time.Second):
		return fmt.Errorf("acp: %s: timed out after 30s", method)
	}
	if !ok {
		return fmt.Errorf("acp: %s: connection closed", method)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

// send fires a JSON-RPC request without waiting for a response.
func (c *Client) send(method string, params interface{}) error {
	id := c.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("acp: marshal %s: %w", method, err)
	}
	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("acp: write %s: %w", method, err)
	}
	return nil
}

// Initialize performs the ACP initialize handshake.
func (c *Client) Initialize() (*InitializeResult, error) {
	params := map[string]interface{}{
		"protocolVersion": 1,
		"clientCapabilities": map[string]interface{}{
			"fs": map[string]bool{
				"readTextFile":  true,
				"writeTextFile": true,
			},
			"terminal": true,
		},
		"clientInfo": map[string]string{
			"name":    "loom",
			"version": "0.1.0",
		},
	}
	var res InitializeResult
	if err := c.call("initialize", params, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// NewSession creates a new ACP session and returns the session ID.
func (c *Client) NewSession() (string, error) {
	params := map[string]interface{}{
		"cwd":        c.cmd.Dir,
		"mcpServers": []interface{}{},
	}
	var res SessionResult
	if err := c.call("session/new", params, &res); err != nil {
		return "", err
	}
	return res.SessionID, nil
}

// SendPrompt sends a text prompt to an existing session.
// Fire-and-forget: the agent works asynchronously, output captured via notifications.
func (c *Client) SendPrompt(sessionID string, text string) error {
	params := map[string]interface{}{
		"sessionId": sessionID,
		"prompt": []map[string]string{
			{"type": "text", "text": text},
		},
	}
	return c.send("session/prompt", params)
}

// CancelSession cancels an in-progress prompt on the given session.
func (c *Client) CancelSession(sessionID string) error {
	params := map[string]interface{}{
		"sessionId": sessionID,
	}
	return c.call("session/cancel", params, nil)
}

// LoadSession resumes an existing session by ID (e.g. after daemon restart).
func (c *Client) LoadSession(sessionID string) error {
	params := map[string]interface{}{
		"sessionId": sessionID,
	}
	return c.call("session/load", params, nil)
}

// SetMode switches the agent mode for an existing session.
func (c *Client) SetMode(sessionID string, mode string) error {
	params := map[string]interface{}{
		"sessionId": sessionID,
		"mode":      mode,
	}
	return c.call("session/set_mode", params, nil)
}

func resolveModelAlias(model string) string {
	if strings.Contains(model, "claude-") {
		return model
	}
	aliases := map[string]string{
		"sonnet": "claude-sonnet-4.6",
		"opus":   "claude-opus-4.6",
		"haiku":  "claude-haiku-4.5",
	}
	if full, ok := aliases[model]; ok {
		return full
	}
	return model
}

// SetModel changes the model for an existing session.
func (c *Client) SetModel(sessionID string, model string) error {
	params := map[string]interface{}{
		"sessionId": sessionID,
		"modelId":  resolveModelAlias(model),
	}
	return c.call("session/set_model", params, nil)
}

// RecentOutput returns the last n captured output lines.
// RecentOutput returns the last n captured output events.
func (c *Client) RecentOutput(n int) []ACPEvent {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if n <= 0 || len(c.lastResponses) == 0 {
		return nil
	}
	if n > len(c.lastResponses) {
		n = len(c.lastResponses)
	}
	out := make([]ACPEvent, n)
	copy(out, c.lastResponses[len(c.lastResponses)-n:])
	return out
}

// Close shuts down the subprocess by killing the process group.
func (c *Client) Close() error {
	c.stdin.Close()
	// Send SIGTERM to the process group so child processes (aim sandbox, etc.) also exit.
	if c.cmd.Process != nil {
		syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
	}
	// Give it a moment to exit gracefully.
	done := make(chan struct{})
	go func() {
		c.waitOnce.Do(func() { c.waitErr = c.cmd.Wait() })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		if c.cmd.Process != nil {
			syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		}
		c.waitOnce.Do(func() { c.waitErr = c.cmd.Wait() })
	}
	return c.waitErr
}

// PID returns the OS process ID of the kiro-cli subprocess.
func (c *Client) PID() int {
	return c.cmd.Process.Pid
}

// Exited reports whether the subprocess has exited.
func (c *Client) Exited() bool {
	return c.exited.Load()
}

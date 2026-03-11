// Package acp provides a JSON-RPC 2.0 client for communicating with kiro-cli
// running in ACP (Agent Client Protocol) mode over stdio.
package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client manages a kiro-cli ACP subprocess and its JSON-RPC lifecycle.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex // serialises writes
	nextID atomic.Int64
	exited atomic.Bool

	outMu         sync.Mutex
	lastResponses []string // last 50 responses
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
// The command arg should be the path to kiro-cli (e.g. from Config.Kiro.Command).
// Additional args (like --agent) can be passed via extraArgs.
func NewClient(command string, workDir string, env []string, extraArgs ...string) (*Client, error) {
	args := append([]string{"acp"}, extraArgs...)
	cmd := exec.Command(command, args...)
	cmd.Dir = workDir
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

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %s: %w", command, err)
	}

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
	}
	go func() {
		cmd.Wait()
		c.exited.Store(true)
	}()
	return c, nil
}

// call sends a JSON-RPC request and reads the response.
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

	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("acp: write %s: %w", method, err)
	}

	// Read newline-delimited response.
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("acp: read %s: %w", method, err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("acp: decode %s: %w", method, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

// Initialize performs the ACP initialize handshake.
func (c *Client) Initialize() (*InitializeResult, error) {
	params := map[string]interface{}{
		"protocolVersion": "2025-11-16",
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
	var res SessionResult
	if err := c.call("session/new", nil, &res); err != nil {
		return "", err
	}
	return res.SessionID, nil
}

// SendPrompt sends a text prompt to an existing session.
// Uses 'content' key per kiro-cli's deviation from the ACP spec (DISC-004).
func (c *Client) SendPrompt(sessionID string, text string) (*PromptResult, error) {
	params := map[string]interface{}{
		"sessionId": sessionID,
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
	var res PromptResult
	if err := c.call("session/prompt", params, &res); err != nil {
		return nil, err
	}
	c.recordResponse(&res)
	return &res, nil
}

// recordResponse extracts text content from a PromptResult and appends to the buffer.
func (c *Client) recordResponse(res *PromptResult) {
	var texts []string
	for _, b := range res.Content {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	if len(texts) == 0 {
		return
	}
	c.outMu.Lock()
	c.lastResponses = append(c.lastResponses, texts...)
	if len(c.lastResponses) > 50 {
		c.lastResponses = c.lastResponses[len(c.lastResponses)-50:]
	}
	c.outMu.Unlock()
}

// RecentOutput returns the last n captured response texts.
func (c *Client) RecentOutput(n int) []string {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if n <= 0 || len(c.lastResponses) == 0 {
		return nil
	}
	if n > len(c.lastResponses) {
		n = len(c.lastResponses)
	}
	out := make([]string, n)
	copy(out, c.lastResponses[len(c.lastResponses)-n:])
	return out
}

// Close shuts down the subprocess. It closes stdin and waits for exit.
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

// PID returns the OS process ID of the kiro-cli subprocess.
func (c *Client) PID() int {
	return c.cmd.Process.Pid
}

// Exited reports whether the subprocess has exited.
func (c *Client) Exited() bool {
	return c.exited.Load()
}

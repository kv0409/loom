// Package acp provides an ACP (Agent Client Protocol) client for
// communicating with kiro-cli over stdio. JSON-RPC transport is delegated
// to github.com/coder/acp-go-sdk; this package owns the subprocess
// lifecycle and captures streamed output for the dashboard.
package acp

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

// PermissionHandler decides whether to approve a server→client request.
type PermissionHandler func(tool, command string) bool

// InitializeResult is the subset of the ACP initialize response exposed to callers.
type InitializeResult struct {
	ServerInfo struct {
		Name    string
		Version string
	}
}

// Client manages a kiro-cli ACP subprocess and its JSON-RPC session.
type Client struct {
	AgentID      string
	OnPermission PermissionHandler

	cmd    *exec.Cmd
	conn   *acpsdk.ClientSideConnection
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	exited atomic.Bool

	waitOnce sync.Once
	waitErr  error

	outMu         sync.Mutex
	lastResponses []ACPEvent
	pendingOutput []ACPEvent
	toolCalls     map[string]*ToolCall
	recentCalls   []ToolCall
}

// NewClient spawns kiro-cli in ACP mode and returns a connected Client.
func NewClient(command string, workDir string, env []string, extraArgs ...string) (*Client, error) {
	cmd := exec.Command(command, append([]string{"acp"}, extraArgs...)...)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if len(env) > 0 {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %s: %w", command, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cmd: cmd, ctx: ctx, cancel: cancel,
		done:      make(chan struct{}),
		toolCalls: make(map[string]*ToolCall),
	}
	c.conn = acpsdk.NewClientSideConnection(c, stdin, stdout)

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[acp-stderr] %s", scanner.Text())
		}
	}()
	go func() {
		c.waitOnce.Do(func() { c.waitErr = cmd.Wait() })
		log.Printf("[acp] process exited: pid=%d err=%v", cmd.Process.Pid, c.waitErr)
		c.exited.Store(true)
		close(c.done)
		c.cancel()
	}()
	return c, nil
}

// callTimeout is the bound applied to synchronous RPCs. Prompt is excluded
// because turns may legitimately run for minutes.
const callTimeout = 30 * time.Second

// callCtx derives a bounded context from the client's root context so calls
// fail over promptly instead of hanging the daemon spawn path.
func (c *Client) callCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.ctx, callTimeout)
}

// Initialize performs the ACP initialize handshake.
func (c *Client) Initialize() (*InitializeResult, error) {
	ctx, cancel := c.callCtx()
	defer cancel()
	resp, err := c.conn.Initialize(ctx, acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		ClientInfo:      &acpsdk.Implementation{Name: "loom", Version: "0.1.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("acp: initialize: %w", err)
	}
	var out InitializeResult
	if resp.AgentInfo != nil {
		out.ServerInfo.Name = resp.AgentInfo.Name
		out.ServerInfo.Version = resp.AgentInfo.Version
	}
	return &out, nil
}

// NewSession creates a new ACP session and returns the session ID.
func (c *Client) NewSession() (string, error) {
	ctx, cancel := c.callCtx()
	defer cancel()
	resp, err := c.conn.NewSession(ctx, acpsdk.NewSessionRequest{
		Cwd:        c.cmd.Dir,
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		return "", fmt.Errorf("acp: session/new: %w", err)
	}
	return string(resp.SessionId), nil
}

// SendPrompt sends a text prompt to an existing session. Fire-and-forget:
// the SDK's Prompt blocks until the turn ends, so we dispatch it in a
// goroutine and return immediately to match the existing daemon contract.
func (c *Client) SendPrompt(sessionID string, text string) error {
	if c.exited.Load() {
		return fmt.Errorf("acp: process exited")
	}
	req := acpsdk.PromptRequest{
		SessionId: acpsdk.SessionId(sessionID),
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock(text)},
	}
	go func() {
		if _, err := c.conn.Prompt(c.ctx, req); err != nil {
			log.Printf("[acp] prompt: %v", err)
		}
	}()
	return nil
}

// CancelSession cancels an in-progress prompt on the given session.
func (c *Client) CancelSession(sessionID string) error {
	ctx, cancel := c.callCtx()
	defer cancel()
	if err := c.conn.Cancel(ctx, acpsdk.CancelNotification{SessionId: acpsdk.SessionId(sessionID)}); err != nil {
		return fmt.Errorf("acp: session/cancel: %w", err)
	}
	return nil
}

// LoadSession resumes an existing session by ID.
func (c *Client) LoadSession(sessionID string) error {
	ctx, cancel := c.callCtx()
	defer cancel()
	_, err := c.conn.LoadSession(ctx, acpsdk.LoadSessionRequest{
		Cwd:        c.cmd.Dir,
		McpServers: []acpsdk.McpServer{},
		SessionId:  acpsdk.SessionId(sessionID),
	})
	if err != nil {
		return fmt.Errorf("acp: session/load: %w", err)
	}
	return nil
}

// SetMode switches the agent mode for an existing session.
func (c *Client) SetMode(sessionID string, mode string) error {
	ctx, cancel := c.callCtx()
	defer cancel()
	_, err := c.conn.SetSessionMode(ctx, acpsdk.SetSessionModeRequest{
		SessionId: acpsdk.SessionId(sessionID),
		ModeId:    acpsdk.SessionModeId(mode),
	})
	if err != nil {
		return fmt.Errorf("acp: session/set_mode: %w", err)
	}
	return nil
}

// SetModel changes the model for an existing session.
func (c *Client) SetModel(sessionID string, model string) error {
	ctx, cancel := c.callCtx()
	defer cancel()
	_, err := c.conn.UnstableSetSessionModel(ctx, acpsdk.UnstableSetSessionModelRequest{
		SessionId: acpsdk.SessionId(sessionID),
		ModelId:   acpsdk.UnstableModelId(resolveModelAlias(model)),
	})
	if err != nil {
		return fmt.Errorf("acp: session/set_model: %w", err)
	}
	return nil
}

func resolveModelAlias(model string) string {
	if strings.Contains(model, "claude-") {
		return model
	}
	switch model {
	case "sonnet":
		return "claude-sonnet-4.6"
	case "opus":
		return "claude-opus-4.6"
	case "haiku":
		return "claude-haiku-4.5"
	}
	return model
}

// Close shuts down the subprocess by killing its process group.
func (c *Client) Close() error {
	c.cancel()
	if c.cmd.Process != nil {
		syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
	}
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
func (c *Client) PID() int { return c.cmd.Process.Pid }

// Exited reports whether the subprocess has exited.
func (c *Client) Exited() bool { return c.exited.Load() }

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

// DrainOutput returns all newly captured output events since the previous drain.
func (c *Client) DrainOutput() []ACPEvent {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if len(c.pendingOutput) == 0 {
		return nil
	}
	out := make([]ACPEvent, len(c.pendingOutput))
	copy(out, c.pendingOutput)
	c.pendingOutput = nil
	return out
}

// RecentToolCalls returns a snapshot of the recent tool calls ring buffer.
func (c *Client) RecentToolCalls() []ToolCall {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	out := make([]ToolCall, len(c.recentCalls))
	copy(out, c.recentCalls)
	return out
}

// --- acpsdk.Client interface -----------------------------------------------

// SessionUpdate captures streamed notifications into the output buffer.
func (c *Client) SessionUpdate(_ context.Context, n acpsdk.SessionNotification) error {
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		if t := textOf(u.AgentMessageChunk.Content); t != "" {
			c.appendEvent(ACPEvent{Kind: TokenChunk, Content: t})
		}
	case u.ToolCall != nil:
		tc := u.ToolCall
		c.trackToolCall(string(tc.ToolCallId), tc.Title, string(tc.Kind), string(tc.Status), locationPaths(tc.Locations), true)
		c.appendEvent(ACPEvent{Kind: ToolSummary, Title: tc.Title})
	case u.ToolCallUpdate != nil:
		tu := u.ToolCallUpdate
		title, kind, status := deref(tu.Title), derefKind(tu.Kind), derefStatus(tu.Status)
		c.trackToolCall(string(tu.ToolCallId), title, kind, status, locationPaths(tu.Locations), false)
		if title != "" {
			c.appendEvent(ACPEvent{Kind: ToolSummary, Title: title})
		}
	}
	return nil
}

// RequestPermission consults the configured PermissionHandler and selects an option.
func (c *Client) RequestPermission(_ context.Context, req acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	tool := deref(req.ToolCall.Title)
	if tool == "" {
		tool = derefKind(req.ToolCall.Kind)
	}
	command := extractCommand(req.ToolCall.RawInput)

	approved := true
	if c.OnPermission != nil {
		approved = c.OnPermission(tool, command)
	}
	decision := "approved"
	if !approved {
		decision = "denied"
	}
	log.Printf("[acp-permission] agent=%s method=%s tool=%q command=%q decision=%s",
		c.AgentID, "session/request_permission", tool, command, decision)

	return acpsdk.RequestPermissionResponse{
		Outcome: acpsdk.NewRequestPermissionOutcomeSelected(pickOptionID(req.Options, approved)),
	}, nil
}

// The remaining Client interface methods are not advertised by our
// InitializeRequest (fs={false,false}, terminal=false); a well-behaved agent
// will not invoke them. Return MethodNotFound for safety.
func (c *Client) ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (r acpsdk.ReadTextFileResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("fs/read_text_file")
}
func (c *Client) WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (r acpsdk.WriteTextFileResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("fs/write_text_file")
}
func (c *Client) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (r acpsdk.CreateTerminalResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("terminal/create")
}
func (c *Client) KillTerminal(context.Context, acpsdk.KillTerminalRequest) (r acpsdk.KillTerminalResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("terminal/kill")
}
func (c *Client) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (r acpsdk.TerminalOutputResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("terminal/output")
}
func (c *Client) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (r acpsdk.ReleaseTerminalResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("terminal/release")
}
func (c *Client) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (r acpsdk.WaitForTerminalExitResponse, _ error) {
	return r, acpsdk.NewMethodNotFound("terminal/wait_for_exit")
}

// --- internal helpers ------------------------------------------------------

// trackToolCall upserts a tool call; isNew=true with a fresh id+title adds
// a snapshot to the recent ring buffer.
func (c *Client) trackToolCall(id, title, kind, status string, locs []string, isNew bool) {
	if id == "" {
		return
	}
	ts := time.Now().Format("2006-01-02T15:04:05")
	c.outMu.Lock()
	defer c.outMu.Unlock()

	tc, exists := c.toolCalls[id]
	if !exists {
		tc = &ToolCall{ToolCallID: id, Timestamp: ts}
		c.toolCalls[id] = tc
	}
	if title != "" {
		tc.Title = title
	}
	if kind != "" {
		tc.Kind = kind
	}
	if status != "" {
		tc.Status = status
	}
	tc.Locations = append(tc.Locations, locs...)

	if isNew && !exists && title != "" {
		snap := *tc
		snap.Timestamp = ts
		c.recentCalls = append(c.recentCalls, snap)
		const maxRecent = 100
		if len(c.recentCalls) > maxRecent {
			c.recentCalls = c.recentCalls[len(c.recentCalls)-maxRecent:]
		}
	}
}

func (c *Client) appendEvent(ev ACPEvent) {
	c.outMu.Lock()
	c.lastResponses = append(c.lastResponses, ev)
	if len(c.lastResponses) > 50 {
		c.lastResponses = c.lastResponses[len(c.lastResponses)-50:]
	}
	c.pendingOutput = append(c.pendingOutput, ev)
	c.outMu.Unlock()
}

func textOf(b acpsdk.ContentBlock) string {
	if b.Text != nil {
		return b.Text.Text
	}
	return ""
}

func locationPaths(locs []acpsdk.ToolCallLocation) []string {
	if len(locs) == 0 {
		return nil
	}
	out := make([]string, 0, len(locs))
	for _, l := range locs {
		out = append(out, l.Path)
	}
	return out
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func derefKind(p *acpsdk.ToolKind) string {
	if p == nil {
		return ""
	}
	return string(*p)
}
func derefStatus(p *acpsdk.ToolCallStatus) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

// extractCommand pulls a shell command from a tool call's RawInput if present.
func extractCommand(raw any) string {
	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range []string{"command", "cmd"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// pickOptionID selects the permission option matching the decision, falling
// back to the first option on approve and the last on deny.
func pickOptionID(opts []acpsdk.PermissionOption, approved bool) acpsdk.PermissionOptionId {
	if len(opts) == 0 {
		return ""
	}
	if approved {
		for _, o := range opts {
			if o.Kind == acpsdk.PermissionOptionKindAllowOnce {
				return o.OptionId
			}
		}
		return opts[0].OptionId
	}
	for _, o := range opts {
		if o.Kind == acpsdk.PermissionOptionKindRejectOnce || o.Kind == acpsdk.PermissionOptionKindRejectAlways {
			return o.OptionId
		}
	}
	return opts[len(opts)-1].OptionId
}

var _ acpsdk.Client = (*Client)(nil)

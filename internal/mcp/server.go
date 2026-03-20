package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/lock"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
)

type Server struct {
	LoomRoot string
	AgentID  string
}

type request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func (s *Server) Run() error {
	log.SetOutput(os.Stderr)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("invalid JSON: %v", err)
			continue
		}
		resp := s.dispatch(&req)
		if resp == nil {
			continue // notification
		}
		out, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "%s\n", out)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (s *Server) dispatch(req *request) *response {
	switch req.Method {
	case "initialize":
		return s.resp(req, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "loom", "version": "0.1.0"},
		})
	case "notifications/initialized":
		return nil
	case "tools/list":
		return s.resp(req, map[string]interface{}{"tools": toolDefs()})
	case "tools/call":
		var p toolCallParams
		json.Unmarshal(req.Params, &p)
		text, err := s.callTool(p.Name, p.Arguments)
		if err != nil {
			return s.resp(req, map[string]interface{}{
				"isError": true,
				"content": []map[string]string{{"type": "text", "text": err.Error()}},
			})
		}
		return s.resp(req, map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": text}},
		})
	default:
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

func (s *Server) resp(req *request, result interface{}) *response {
	var id interface{}
	if req.ID != nil {
		json.Unmarshal(*req.ID, &id)
	}
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func str(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func required(args map[string]interface{}, key string) (string, error) {
	v := str(args, key)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func num(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func (s *Server) callTool(name string, args map[string]interface{}) (string, error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	switch name {
	case "loom_mail_send":
		to, err := required(args, "to")
		if err != nil {
			return "", err
		}
		subject, err := required(args, "subject")
		if err != nil {
			return "", err
		}
		resolvedTo, err := mail.ResolveRecipient(s.LoomRoot, to)
		if err != nil {
			return "", err
		}
		if err := mail.Send(s.LoomRoot, mail.SendOpts{
			From:     s.AgentID,
			To:       to,
			Subject:  subject,
			Body:     str(args, "body"),
			Type:     strOr(args, "type", "status"),
			Priority: strOr(args, "priority", "normal"),
			Ref:      str(args, "ref"),
		}); err != nil {
			return "", err
		}
		daemon.RefreshBestEffort(s.LoomRoot, daemon.RefreshOpts{MailAgents: []string{resolvedTo}})
		return fmt.Sprintf("Sent to %s: %s", to, subject), nil

	case "loom_mail_read":
		unread, _ := args["unread_only"].(bool)
		msgs, err := mail.Read(s.LoomRoot, mail.ReadOpts{Agent: s.AgentID, UnreadOnly: unread})
		if err != nil {
			return "", err
		}
		if len(msgs) == 0 {
			return "No messages", nil
		}
		markedRead := false
		for _, m := range msgs {
			if !m.Read {
				if err := mail.MarkRead(s.LoomRoot, s.AgentID, m.ID); err != nil {
					return "", err
				}
				markedRead = true
			}
		}
		if markedRead {
			daemon.RefreshBestEffort(s.LoomRoot, daemon.RefreshOpts{MailAgents: []string{s.AgentID}})
		}
		out, _ := json.MarshalIndent(msgs, "", "  ")
		return string(out), nil

	case "loom_mail_count":
		msgs, err := mail.Read(s.LoomRoot, mail.ReadOpts{Agent: s.AgentID, UnreadOnly: true})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d unread messages", len(msgs)), nil

	case "loom_issue_show":
		id, err := required(args, "id")
		if err != nil {
			return "", err
		}
		iss, err := issue.Load(s.LoomRoot, id)
		if err != nil {
			return "", err
		}
		out, _ := json.MarshalIndent(iss, "", "  ")
		return string(out), nil

	case "loom_issue_update":
		id, err := required(args, "id")
		if err != nil {
			return "", err
		}
		assignee := str(args, "assignee")
		unassign := boolVal(args, "unassign")
		if assignee != "" && unassign {
			return "", fmt.Errorf("assignee and unassign are mutually exclusive")
		}
		var current *issue.Issue
		if unassign || assignee != "" || str(args, "status") == "cancelled" || str(args, "status") == "done" {
			current, err = issue.Load(s.LoomRoot, id)
			if err != nil {
				return "", err
			}
		}
		refreshOpts := daemon.RefreshOpts{}
		if unassign {
			if err := agent.UnassignIssue(s.LoomRoot, id); err != nil {
				return "", err
			}
			refreshOpts.IssueIDs = appendUniqueString(refreshOpts.IssueIDs, id)
			if current != nil && current.Assignee != "" {
				refreshOpts.AgentIDs = appendUniqueString(refreshOpts.AgentIDs, current.Assignee)
			}
		}
		if assignee != "" {
			if err := agent.AssignIssue(s.LoomRoot, assignee, id); err != nil {
				return "", err
			}
			refreshOpts.IssueIDs = appendUniqueString(refreshOpts.IssueIDs, id)
			refreshOpts.AgentIDs = appendUniqueString(refreshOpts.AgentIDs, assignee)
			if current != nil && current.Assignee != "" && current.Assignee != assignee {
				refreshOpts.AgentIDs = appendUniqueString(refreshOpts.AgentIDs, current.Assignee)
			}
		}
		status := str(args, "status")
		if status == "cancelled" {
			cancelled, err := agent.CancelIssue(s.LoomRoot, id)
			if err != nil {
				return "", err
			}
			for _, ci := range cancelled {
				refreshOpts.IssueIDs = appendUniqueString(refreshOpts.IssueIDs, ci.IssueID)
				if ci.PreviousAssignee != "" {
					refreshOpts.AgentIDs = appendUniqueString(refreshOpts.AgentIDs, ci.PreviousAssignee)
				}
			}
			daemon.RefreshBestEffort(s.LoomRoot, refreshOpts)
			return fmt.Sprintf("Cancelled %s (%d issues affected)", id, len(cancelled)), nil
		}
		priority := str(args, "priority")
		dispatch := strMap(args, "dispatch")
		if status == "done" {
			info, err := agent.CloseIssue(s.LoomRoot, id, "")
			if err != nil {
				return "", err
			}
			refreshOpts.IssueIDs = appendUniqueString(refreshOpts.IssueIDs, id)
			if info != nil && info.PreviousAssignee != "" {
				refreshOpts.AgentIDs = appendUniqueString(refreshOpts.AgentIDs, info.PreviousAssignee)
			}
			daemon.RefreshBestEffort(s.LoomRoot, refreshOpts)
			return fmt.Sprintf("Closed %s", id), nil
		}
		if status != "" || priority != "" || len(dispatch) > 0 {
			if _, err := issue.Update(s.LoomRoot, id, issue.UpdateOpts{
				Status:   status,
				Priority: priority,
				Dispatch: dispatch,
			}); err != nil {
				return "", err
			}
			refreshOpts.IssueIDs = appendUniqueString(refreshOpts.IssueIDs, id)
		}
		daemon.RefreshBestEffort(s.LoomRoot, refreshOpts)
		return fmt.Sprintf("Updated %s", id), nil

	case "loom_issue_close":
		id, err := required(args, "id")
		if err != nil {
			return "", err
		}
		info, err := agent.CloseIssue(s.LoomRoot, id, str(args, "reason"))
		if err != nil {
			return "", err
		}
		refreshOpts := daemon.RefreshOpts{IssueIDs: []string{id}}
		if info != nil && info.PreviousAssignee != "" {
			refreshOpts.AgentIDs = append(refreshOpts.AgentIDs, info.PreviousAssignee)
		}
		daemon.RefreshBestEffort(s.LoomRoot, refreshOpts)
		return fmt.Sprintf("Closed %s", id), nil

	case "loom_issue_create":
		title, err := required(args, "title")
		if err != nil {
			return "", err
		}
		iss, err := issue.Create(s.LoomRoot, title, issue.CreateOpts{
			Type:        strOr(args, "type", "task"),
			Priority:    strOr(args, "priority", "normal"),
			Parent:      str(args, "parent"),
			Description: str(args, "description"),
			Dispatch:    strMap(args, "dispatch"),
		})
		if err != nil {
			return "", err
		}
		daemon.RefreshBestEffort(s.LoomRoot, daemon.RefreshOpts{IssueIDs: []string{iss.ID}})
		return fmt.Sprintf("Created %s: %s", iss.ID, iss.Title), nil

	case "loom_issue_list":
		if boolVal(args, "ready_only") {
			issues, err := issue.ListReady(s.LoomRoot)
			if err != nil {
				return "", err
			}
			out, _ := json.MarshalIndent(issues, "", "  ")
			return string(out), nil
		}
		issues, err := issue.List(s.LoomRoot, issue.ListOpts{
			Status:   str(args, "status"),
			Assignee: str(args, "assignee"),
			Type:     str(args, "type"),
			All:      boolVal(args, "all"),
		})
		if err != nil {
			return "", err
		}
		out, _ := json.MarshalIndent(issues, "", "  ")
		return string(out), nil

	case "loom_memory_add":
		entry, err := memory.Add(s.LoomRoot, memory.AddOpts{
			Type:      str(args, "type"),
			Title:     str(args, "title"),
			Context:   str(args, "context"),
			Rationale: str(args, "rationale"),
			Decision:  str(args, "decision"),
			Finding:   str(args, "finding"),
			Rule:      str(args, "rule"),
			Location:  str(args, "location"),
			By:        s.AgentID,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Added %s: %s", entry.ID, entry.Title), nil

	case "loom_memory_search":
		limit := num(args, "limit")
		if limit == 0 {
			limit = 5
		}
		results, err := memory.Search(s.LoomRoot, memory.SearchOpts{Query: str(args, "query"), Limit: limit})
		if err != nil {
			return "", err
		}
		out, _ := json.MarshalIndent(results, "", "  ")
		return string(out), nil

	case "loom_memory_list":
		entries, err := memory.List(s.LoomRoot, memory.ListOpts{
			Type:    str(args, "type"),
			Affects: str(args, "affects"),
		})
		if err != nil {
			return "", err
		}
		out, _ := json.MarshalIndent(entries, "", "  ")
		return string(out), nil

	case "loom_lock_acquire":
		file, err := required(args, "file")
		if err != nil {
			return "", err
		}
		if err := lock.Acquire(s.LoomRoot, lock.AcquireOpts{File: file, Agent: s.AgentID, Issue: str(args, "issue")}); err != nil {
			return "", err
		}
		return fmt.Sprintf("Locked %s", file), nil

	case "loom_lock_release":
		file, err := required(args, "file")
		if err != nil {
			return "", err
		}
		if err := lock.Release(s.LoomRoot, file); err != nil {
			return "", err
		}
		return fmt.Sprintf("Released %s", file), nil

	case "loom_lock_check":
		file, err := required(args, "file")
		if err != nil {
			return "", err
		}
		l, err := lock.Check(s.LoomRoot, file)
		if err != nil {
			return "", err
		}
		if l == nil {
			return fmt.Sprintf("%s is not locked", file), nil
		}
		out, _ := json.MarshalIndent(l, "", "  ")
		return string(out), nil

	case "loom_agent_heartbeat":
		if err := daemon.Heartbeat(s.LoomRoot, s.AgentID); err != nil {
			return "", err
		}
		return "Heartbeat updated", nil

	case "loom_agent_status":
		a, err := agent.Load(s.LoomRoot, s.AgentID)
		if err != nil {
			return "", err
		}
		out, _ := json.MarshalIndent(a, "", "  ")
		return string(out), nil

	case "loom_agent_kill":
		id, err := required(args, "id")
		if err != nil {
			return "", err
		}
		if id == s.AgentID {
			return "", fmt.Errorf("cannot kill self")
		}
		cleanup := boolVal(args, "cleanup")
		if err := agent.Kill(s.LoomRoot, id, cleanup); err != nil {
			return "", err
		}
		return fmt.Sprintf("Killed %s (cleanup=%v)", id, cleanup), nil

	case "loom_worktree_remove":
		name, err := required(args, "name")
		if err != nil {
			return "", err
		}
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			return "", fmt.Errorf("invalid worktree name: %s", name)
		}
		force := boolVal(args, "force")
		if err := worktree.Remove(s.LoomRoot, name, force); err != nil {
			return "", err
		}
		return fmt.Sprintf("Removed worktree %s", name), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func strOr(m map[string]interface{}, key, def string) string {
	if v := str(m, key); v != "" {
		return v
	}
	return def
}

func boolVal(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func strMap(m map[string]interface{}, key string) map[string]string {
	v, ok := m[key].(map[string]interface{})
	if !ok || len(v) == 0 {
		return nil
	}
	out := make(map[string]string, len(v))
	for k, val := range v {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

func appendUniqueString(items []string, value string) []string {
	if value == "" {
		return items
	}
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}

func toolDefs() []toolDef {
	return []toolDef{
		{Name: "loom_mail_send", Description: "Send a mail message to another agent", InputSchema: obj(
			props{"to": propStr("Recipient agent ID"), "subject": propStr("Message subject"), "body": propStr("Message body"),
				"type":     propEnum("Message type", "task", "status", "completion", "blocker", "review-request", "review-result", "question", "escalation"),
				"priority": propEnum("Priority", "critical", "normal", "low"), "ref": propStr("Related issue ID")},
			"to", "subject")},
		{Name: "loom_mail_read", Description: "Read inbox messages", InputSchema: obj(
			props{"unread_only": propBool("Only return unread messages")})},
		{Name: "loom_mail_count", Description: "Count unread messages in inbox", InputSchema: obj(props{})},
		{Name: "loom_issue_show", Description: "Get issue details by ID", InputSchema: obj(
			props{"id": propStr("Issue ID")}, "id")},
		{Name: "loom_issue_update", Description: "Update issue status, priority, or assignee", InputSchema: obj(
			props{"id": propStr("Issue ID"), "status": propStr("New status"), "priority": propStr("New priority"), "assignee": propStr("New assignee"),
				"unassign": propBool("Remove current assignee"),
				"dispatch": propObj("Dispatch key-value pairs (e.g. SKIP_REVIEW, MAX_AGENTS)")},
			"id")},
		{Name: "loom_issue_create", Description: "Create a new issue", InputSchema: obj(
			props{"title": propStr("Issue title"), "type": propEnum("Issue type", "epic", "task", "bug", "spike"),
				"priority": propEnum("Priority", "critical", "high", "normal", "low"), "parent": propStr("Parent issue ID"), "description": propStr("Description"),
				"dispatch": propObj("Dispatch key-value pairs (e.g. SKIP_REVIEW, MAX_AGENTS)")},
			"title")},
		{Name: "loom_issue_list", Description: "List issues with optional filters", InputSchema: obj(
			props{"status": propStr("Filter by status"), "assignee": propStr("Filter by assignee"), "type": propStr("Filter by type"), "all": propBool("Include closed/cancelled"), "ready_only": propBool("Only return open unassigned issues with resolved dependencies")})},
		{Name: "loom_issue_close", Description: "Close an issue (marks done, reconciles agent ownership)", InputSchema: obj(
			props{"id": propStr("Issue ID"), "reason": propStr("Close reason")},
			"id")},
		{Name: "loom_memory_add", Description: "Record a decision, discovery, or convention in shared memory", InputSchema: obj(
			props{"type": propEnum("Memory type", "decision", "discovery", "convention"), "title": propStr("Title"),
				"context": propStr("Context (decisions)"), "rationale": propStr("Rationale (decisions)"), "decision": propStr("Decision text"),
				"finding": propStr("Finding (discoveries)"), "rule": propStr("Rule (conventions)"), "location": propStr("Location (discoveries)")},
			"type", "title")},
		{Name: "loom_memory_search", Description: "Search the shared memory store", InputSchema: obj(
			props{"query": propStr("Search query"), "type": propEnum("Filter by type", "decision", "discovery", "convention"), "limit": propInt("Max results")},
			"query")},
		{Name: "loom_memory_list", Description: "List memory entries with optional filters", InputSchema: obj(
			props{"type": propEnum("Filter by type", "decision", "discovery", "convention"), "affects": propStr("Filter by affected issue ID")})},
		{Name: "loom_lock_acquire", Description: "Acquire a file lock", InputSchema: obj(
			props{"file": propStr("File path to lock"), "issue": propStr("Related issue ID")},
			"file")},
		{Name: "loom_lock_release", Description: "Release a file lock", InputSchema: obj(
			props{"file": propStr("File path to unlock")},
			"file")},
		{Name: "loom_lock_check", Description: "Check if a file is locked", InputSchema: obj(
			props{"file": propStr("File path to check")},
			"file")},
		{Name: "loom_agent_heartbeat", Description: "Update agent heartbeat timestamp", InputSchema: obj(props{})},
		{Name: "loom_agent_status", Description: "Get current agent status and info", InputSchema: obj(props{})},
		{Name: "loom_agent_kill", Description: "Kill an agent (stops tmux window, optionally removes worktree and branch, deregisters)", InputSchema: obj(
			props{"id": propStr("Agent ID to kill"), "cleanup": propBool("Also remove the agent's worktree and delete its branch")},
			"id")},
		{Name: "loom_worktree_remove", Description: "Remove a worktree directory and delete its branch", InputSchema: obj(
			props{"name": propStr("Worktree name (e.g. LOOM-001-01-login-form)"), "force": propBool("Force removal even if branch is unmerged")},
			"name")},
	}
}

// Schema helpers
type props = map[string]interface{}

func obj(properties props, required ...string) map[string]interface{} {
	s := map[string]interface{}{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func propStr(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}

func propEnum(desc string, vals ...string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc, "enum": vals}
}

func propBool(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": desc}
}

func propInt(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": desc}
}

func propObj(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "object", "description": desc, "additionalProperties": map[string]interface{}{"type": "string"}}
}

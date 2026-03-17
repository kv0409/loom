package dashboard

import (
	"strings"

	"github.com/karanagi/loom/internal/dashboard/backend"
)

// searchMatch returns true if query (case-insensitive) appears in any of the fields.
func searchMatch(query string, fields ...string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func (m Model) filteredAgents() []*backend.Agent {
	if m.searchTI.Value() == "" {
		return m.data.Agents
	}
	var out []*backend.Agent
	for _, a := range m.data.Agents {
		if searchMatch(m.searchTI.Value(), a.ID, a.Role, a.Status, a.WorktreeName, strings.Join(a.AssignedIssues, " ")) {
			out = append(out, a)
		}
	}
	return out
}

func (m Model) filteredIssues() []*backend.Issue {
	display := m.displayIssues()
	if m.searchTI.Value() == "" {
		return display
	}
	var out []*backend.Issue
	for _, iss := range display {
		if searchMatch(m.searchTI.Value(), iss.ID, iss.Title, iss.Status, iss.Assignee, iss.Type, iss.Description, iss.Parent, iss.Worktree, strings.Join(iss.DependsOn, " "), strings.Join(iss.Children, " ")) {
			out = append(out, iss)
		}
	}
	return out
}

func (m Model) filteredMessages() []*backend.Message {
	if m.searchTI.Value() == "" {
		return m.data.Messages
	}
	var out []*backend.Message
	for _, msg := range m.data.Messages {
		if searchMatch(m.searchTI.Value(), msg.From, msg.To, msg.Subject, msg.Type, msg.Body, msg.Ref, msg.Priority) {
			out = append(out, msg)
		}
	}
	return out
}

func (m Model) filteredMemories() []*backend.MemoryEntry {
	if m.searchTI.Value() == "" {
		return m.data.Memories
	}
	var out []*backend.MemoryEntry
	for _, e := range m.data.Memories {
		if searchMatch(m.searchTI.Value(), e.ID, e.Title, e.Type, m.backend.MemoryByField(e), e.Context, e.Decision, e.Rationale, e.Finding, e.Rule, e.Implications, e.Location, e.AppliesTo, strings.Join(e.Affects, " "), strings.Join(e.Tags, " "), m.backend.MemorySnippet(e)) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) filteredWorktrees() []*backend.Worktree {
	if m.searchTI.Value() == "" {
		return m.data.Worktrees
	}
	var out []*backend.Worktree
	for _, wt := range m.data.Worktrees {
		if searchMatch(m.searchTI.Value(), wt.Name, wt.Branch, wt.Agent, wt.Issue) {
			out = append(out, wt)
		}
	}
	return out
}

func (m Model) filteredActivity() []backend.ActivityEntry {
	if m.searchTI.Value() == "" {
		return m.data.Activity
	}
	var out []backend.ActivityEntry
	for _, e := range m.data.Activity {
		if searchMatch(m.searchTI.Value(), e.AgentID, e.Line) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) filteredLogs() []backend.LogLine {
	if m.searchTI.Value() == "" {
		return m.data.Logs
	}
	var out []backend.LogLine
	for _, entry := range m.data.Logs {
		if searchMatch(m.searchTI.Value(), entry.Category, entry.Agent, entry.Text) {
			out = append(out, entry)
		}
	}
	return out
}

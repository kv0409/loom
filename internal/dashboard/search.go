package dashboard

import (
	"strings"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
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

func (m Model) filteredAgents() []*agent.Agent {
	if m.searchQuery == "" {
		return m.data.agents
	}
	var out []*agent.Agent
	for _, a := range m.data.agents {
		if searchMatch(m.searchQuery, a.ID, a.Role, a.Status, a.WorktreeName, strings.Join(a.AssignedIssues, " ")) {
			out = append(out, a)
		}
	}
	return out
}

func (m Model) filteredIssues() []*issue.Issue {
	display := m.displayIssues()
	if m.searchQuery == "" {
		return display
	}
	var out []*issue.Issue
	for _, iss := range display {
		if searchMatch(m.searchQuery, iss.ID, iss.Title, iss.Status, iss.Assignee, iss.Type) {
			out = append(out, iss)
		}
	}
	return out
}

func (m Model) filteredMessages() []*mail.Message {
	if m.searchQuery == "" {
		return m.data.messages
	}
	var out []*mail.Message
	for _, msg := range m.data.messages {
		if searchMatch(m.searchQuery, msg.From, msg.To, msg.Subject, msg.Type) {
			out = append(out, msg)
		}
	}
	return out
}

func (m Model) filteredMemories() []*memory.Entry {
	if m.searchQuery == "" {
		return m.data.memories
	}
	var out []*memory.Entry
	for _, e := range m.data.memories {
		if searchMatch(m.searchQuery, e.ID, e.Title, e.Type, memory.ByField(e)) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) filteredWorktrees() []*worktree.Worktree {
	if m.searchQuery == "" {
		return m.data.worktrees
	}
	var out []*worktree.Worktree
	for _, wt := range m.data.worktrees {
		if searchMatch(m.searchQuery, wt.Name, wt.Branch, wt.Agent, wt.Issue) {
			out = append(out, wt)
		}
	}
	return out
}

func (m Model) filteredActivity() []activityEntry {
	if m.searchQuery == "" {
		return m.data.activity
	}
	var out []activityEntry
	for _, e := range m.data.activity {
		if searchMatch(m.searchQuery, e.AgentID, e.Line) {
			out = append(out, e)
		}
	}
	return out
}

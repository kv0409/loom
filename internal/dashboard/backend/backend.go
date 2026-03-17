package backend

import (
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
)

// AgentTreeNode holds tree-structure metadata for a single agent in the sorted tree.
type AgentTreeNode struct {
	Depth     int
	IsLast    bool
	Ancestors []bool // Ancestors[i] is true if the ancestor at depth i is the last child of its parent
}

// ActivityEntry is a single tool-use event from an agent's activity stream.
type ActivityEntry struct {
	AgentID string
	Line    string // original raw line (kept for search/filter)
	Time    string // display-ready relative time (e.g. "3s ago")
	Tool    string // tool label (e.g. "SHELL", "READ")
	Detail  string // cleaned-up args / summary
}

// LogLine is a single classified line from the daemon log.
type LogLine struct {
	Category string // "lifecycle", "error", "stderr", "warn", or ""
	Agent    string // extracted agent ID
	Text     string // full line
}

// Snapshot holds all dashboard state loaded in a single refresh cycle.
type Snapshot struct {
	Agents    []*agent.Agent
	AgentTree []AgentTreeNode
	Issues    []*issue.Issue
	Worktrees []*worktree.Worktree
	DiffStats map[string]*worktree.DiffStats
	Messages  []*mail.Message
	Memories  []*memory.Entry
	Unread    int
	Activity  []ActivityEntry
	Logs      []LogLine
	DaemonOK  bool
}

// Backend loads a complete dashboard snapshot.
type Backend interface {
	Load() Snapshot
}

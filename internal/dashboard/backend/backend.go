package backend

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
	Time    string // display-ready relative time (e.g. "3s")
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
	Agents              []*Agent
	AgentTree           []AgentTreeNode
	Issues              []*Issue
	Proposals           []*Proposal
	Worktrees           []*Worktree
	DiffStats           map[string]*DiffStats
	Messages            []*Message
	Memories            []*MemoryEntry
	Unread              int
	Activity            []ActivityEntry
	Logs                []LogLine
	DaemonOK            bool
	Errors              []string
	HeartbeatTimeoutSec int
}

// Backend loads a complete dashboard snapshot.
type Backend interface {
	Load() Snapshot
	AgentOutput(loomRoot, agentID string) ([]ACPEvent, error)
	Diff(wtPath string) string
	SendMail(loomRoot string, from, to, subject, body, typ, priority, ref string) error
	RespondProposal(loomRoot, id, action, feedback string) error
	MemorySnippet(e *MemoryEntry) string
	MemoryByField(e *MemoryEntry) string
}

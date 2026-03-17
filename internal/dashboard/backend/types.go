package backend

import (
	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
)

// Type aliases re-export domain types so frontend files only import backend.
type (
	Agent       = agent.Agent
	AgentConfig = agent.AgentConfig
	Issue       = issue.Issue
	Message     = mail.Message
	MemoryEntry = memory.Entry
	Worktree    = worktree.Worktree
	DiffStats   = worktree.DiffStats
	ACPEvent    = acp.ACPEvent
	ACPKind     = acp.Kind
)

// Re-export ACP kind constants so frontend doesn't import acp.
const (
	TokenChunk      = acp.TokenChunk
	ToolSummary     = acp.ToolSummary
	CompleteMessage = acp.CompleteMessage
)

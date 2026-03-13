package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
)

// --- renderMail ---

func TestRenderMail_Empty(t *testing.T) {
	m := testModel(viewMail)
	out := m.renderMail()
	if out == "" {
		t.Error("renderMail with empty data returned empty string")
	}
}

func TestRenderMail_WithData(t *testing.T) {
	m := testModel(viewMail)
	m.data.messages = []*mail.Message{
		{From: "alice", To: "bob", Type: "task", Subject: "fix bug", Timestamp: time.Now()},
		{From: "bob", To: "alice", Type: "reply", Subject: "done", Timestamp: time.Now()},
	}
	out := m.renderMail()
	if !strings.Contains(out, "MAIL") {
		t.Error("renderMail missing MAIL title")
	}
}

// --- renderMemory ---

func TestRenderMemory_Empty(t *testing.T) {
	m := testModel(viewMemory)
	out := m.renderMemory()
	if out == "" {
		t.Error("renderMemory with empty data returned empty string")
	}
}

func TestRenderMemory_WithData(t *testing.T) {
	m := testModel(viewMemory)
	m.data.memories = []*memory.Entry{
		{ID: "M1", Type: "decision", Title: "Use Go"},
		{ID: "M2", Type: "discovery", Title: "Found bug"},
	}
	out := m.renderMemory()
	if !strings.Contains(out, "MEMORY") {
		t.Error("renderMemory missing MEMORY title")
	}
}

// --- renderWorktrees ---

func TestRenderWorktrees_Empty(t *testing.T) {
	m := testModel(viewWorktrees)
	out := m.renderWorktrees()
	if out == "" {
		t.Error("renderWorktrees with empty data returned empty string")
	}
}

func TestRenderWorktrees_WithData(t *testing.T) {
	m := testModel(viewWorktrees)
	m.data.worktrees = []*worktree.Worktree{
		{Name: "LOOM-1-1-fix", Branch: "fix-branch", Agent: "builder-1", Issue: "I1"},
		{Name: "LOOM-2-1-feat", Branch: "feat-branch", Agent: "builder-2", Issue: "I2"},
	}
	m.data.diffStats = map[string]*worktree.DiffStats{
		"LOOM-1-1-fix": {FilesChanged: 3, Insertions: 10, Deletions: 5},
	}
	out := m.renderWorktrees()
	if !strings.Contains(out, "WORKTREES") {
		t.Error("renderWorktrees missing WORKTREES title")
	}
}

// --- renderActivity ---

func TestRenderActivity_Empty(t *testing.T) {
	m := testModel(viewActivity)
	out := m.renderActivity()
	if out == "" {
		t.Error("renderActivity with empty data returned empty string")
	}
}

func TestRenderActivity_WithData(t *testing.T) {
	m := testModel(viewActivity)
	m.data.activity = []activityEntry{
		{AgentID: "builder-1", Line: "Called execute_bash: go build"},
		{AgentID: "builder-2", Line: "Called fs_read: main.go"},
	}
	out := m.renderActivity()
	if !strings.Contains(out, "ACTIVITY") {
		t.Error("renderActivity missing ACTIVITY title")
	}
}

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupRoot creates a temp .loom root with the agents/ directory.
func setupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

func makeAgent(id, role string) *Agent {
	return &Agent{
		ID:        id,
		Role:      role,
		Status:    "active",
		SpawnedAt: time.Now(),
		Heartbeat: time.Now(),
	}
}

// --- Register / Load / Save ---

func TestRegisterAndLoad(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	a.AssignedIssues = []string{"LOOM-001"}

	if err := Register(root, a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// YAML file should exist.
	if _, err := os.Stat(agentPath(root, "builder-001")); err != nil {
		t.Fatalf("agent file not created: %v", err)
	}

	// Mailbox dir should exist.
	if _, err := os.Stat(mailboxDir(root, "builder-001")); err != nil {
		t.Fatalf("mailbox dir not created: %v", err)
	}

	loaded, err := Load(root, "builder-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != "builder-001" {
		t.Errorf("ID: got %q, want %q", loaded.ID, "builder-001")
	}
	if loaded.Role != "builder" {
		t.Errorf("Role: got %q, want %q", loaded.Role, "builder")
	}
	if len(loaded.AssignedIssues) != 1 || loaded.AssignedIssues[0] != "LOOM-001" {
		t.Errorf("AssignedIssues: got %v, want [LOOM-001]", loaded.AssignedIssues)
	}
}

func TestLoadNonexistent(t *testing.T) {
	root := setupRoot(t)
	_, err := Load(root, "no-such-agent")
	if err == nil {
		t.Fatal("expected error loading nonexistent agent")
	}
}

func TestSave(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("lead-001", "lead")
	if err := Register(root, a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	a.Status = "idle"
	if err := Save(root, a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(root, "lead-001")
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.Status != "idle" {
		t.Errorf("Status: got %q, want %q", loaded.Status, "idle")
	}
}

// --- List ---

func TestListEmpty(t *testing.T) {
	root := setupRoot(t)
	agents, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestListNoDir(t *testing.T) {
	// agents/ directory doesn't exist at all.
	root := t.TempDir()
	agents, err := List(root)
	if err != nil {
		t.Fatalf("List on missing dir: %v", err)
	}
	if agents != nil {
		t.Errorf("expected nil, got %v", agents)
	}
}

func TestListMultiple(t *testing.T) {
	root := setupRoot(t)
	now := time.Now()

	// Register agents with staggered spawn times.
	for i, id := range []string{"builder-001", "builder-002", "lead-001"} {
		a := makeAgent(id, "builder")
		if id == "lead-001" {
			a.Role = "lead"
		}
		a.SpawnedAt = now.Add(time.Duration(i) * time.Second)
		if err := Register(root, a); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	agents, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	// List sorts by SpawnedAt descending (newest first).
	if agents[0].ID != "lead-001" {
		t.Errorf("first agent: got %q, want %q", agents[0].ID, "lead-001")
	}
	if agents[2].ID != "builder-001" {
		t.Errorf("last agent: got %q, want %q", agents[2].ID, "builder-001")
	}
}

// --- Deregister ---

func TestDeregister(t *testing.T) {
	root := setupRoot(t)
	a := makeAgent("builder-001", "builder")
	if err := Register(root, a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := Deregister(root, "builder-001"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	if _, err := os.Stat(agentPath(root, "builder-001")); !os.IsNotExist(err) {
		t.Error("agent file should be removed after Deregister")
	}

	// Load should fail.
	if _, err := Load(root, "builder-001"); err == nil {
		t.Error("expected error loading deregistered agent")
	}
}

func TestDeregisterNonexistent(t *testing.T) {
	root := setupRoot(t)
	err := Deregister(root, "ghost")
	if err == nil {
		t.Fatal("expected error deregistering nonexistent agent")
	}
}

// --- NextID ---

func TestNextIDOrchestrator(t *testing.T) {
	root := setupRoot(t)
	id := NextID(root, "orchestrator")
	if id != "orchestrator" {
		t.Errorf("got %q, want %q", id, "orchestrator")
	}
}

func TestNextIDFirstOfRole(t *testing.T) {
	root := setupRoot(t)
	id := NextID(root, "builder")
	if id != "builder-001" {
		t.Errorf("got %q, want %q", id, "builder-001")
	}
}

func TestNextIDIncrementsExisting(t *testing.T) {
	root := setupRoot(t)

	// Register builder-001 and builder-003 (gap).
	for _, id := range []string{"builder-001", "builder-003"} {
		if err := Register(root, makeAgent(id, "builder")); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	id := NextID(root, "builder")
	if id != "builder-004" {
		t.Errorf("got %q, want %q", id, "builder-004")
	}
}

func TestNextIDIgnoresOtherRoles(t *testing.T) {
	root := setupRoot(t)

	// Register a lead — should not affect builder numbering.
	if err := Register(root, makeAgent("lead-005", "lead")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	id := NextID(root, "builder")
	if id != "builder-001" {
		t.Errorf("got %q, want %q", id, "builder-001")
	}
}

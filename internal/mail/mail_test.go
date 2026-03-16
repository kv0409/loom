package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/store"
)

// setupRoot creates a temp .loom root with the required directory structure.
func setupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"mail/inbox", "mail/archive", "agents"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSendReadRoundTrip(t *testing.T) {
	root := setupRoot(t)

	err := Send(root, SendOpts{
		From:    "lead-001",
		To:      "builder-001",
		Subject: "implement login",
		Body:    "Please build the login form",
		Type:    "task",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msgs, err := Read(root, ReadOpts{Agent: "builder-001"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	m := msgs[0]
	if m.From != "lead-001" {
		t.Errorf("From: want lead-001, got %s", m.From)
	}
	if m.To != "builder-001" {
		t.Errorf("To: want builder-001, got %s", m.To)
	}
	if m.Subject != "implement login" {
		t.Errorf("Subject: want 'implement login', got %q", m.Subject)
	}
	if m.Body != "Please build the login form" {
		t.Errorf("Body mismatch: got %q", m.Body)
	}
	if m.Type != "task" {
		t.Errorf("Type: want task, got %s", m.Type)
	}
	if m.Read {
		t.Error("new message should be unread")
	}
}

func TestReadUnreadOnly(t *testing.T) {
	root := setupRoot(t)

	// Send two messages
	Send(root, SendOpts{From: "a", To: "b", Subject: "first"})
	Send(root, SendOpts{From: "a", To: "b", Subject: "second"})

	msgs, _ := Read(root, ReadOpts{Agent: "b"})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Mark one as read
	MarkRead(root, "b", msgs[0].ID)

	unread, _ := Read(root, ReadOpts{Agent: "b", UnreadOnly: true})
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(unread))
	}
}

func TestReadEmptyInbox(t *testing.T) {
	root := setupRoot(t)

	msgs, err := Read(root, ReadOpts{Agent: "nobody"})
	if err != nil {
		t.Fatalf("Read non-existent inbox: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestMarkRead(t *testing.T) {
	root := setupRoot(t)

	Send(root, SendOpts{From: "a", To: "b", Subject: "hello"})
	msgs, _ := Read(root, ReadOpts{Agent: "b"})
	if msgs[0].Read {
		t.Fatal("message should start unread")
	}

	if err := MarkRead(root, "b", msgs[0].ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	msgs, _ = Read(root, ReadOpts{Agent: "b"})
	if !msgs[0].Read {
		t.Error("message should be marked read")
	}
}

func TestArchive(t *testing.T) {
	root := setupRoot(t)

	Send(root, SendOpts{From: "a", To: "b", Subject: "archive me"})
	msgs, _ := Read(root, ReadOpts{Agent: "b"})
	msgID := msgs[0].ID

	if err := Archive(root, "b", msgID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Inbox should be empty
	msgs, _ = Read(root, ReadOpts{Agent: "b"})
	if len(msgs) != 0 {
		t.Fatalf("expected 0 inbox messages after archive, got %d", len(msgs))
	}

	// Archived file should exist
	date := time.Now().Format("2006-01-02")
	archivePath := filepath.Join(root, "mail", "archive", date, msgID+".yaml")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Error("archived message file not found")
	}
}

func TestDeadAgentRerouting(t *testing.T) {
	root := setupRoot(t)

	// Register a dead agent with a parent
	dead := &agent.Agent{
		ID:        "builder-dead",
		Role:      "builder",
		Status:    "dead",
		SpawnedBy: "lead-001",
	}
	if err := agent.Register(root, dead); err != nil {
		t.Fatalf("Register dead agent: %v", err)
	}

	// Send to the dead agent — should reroute to parent
	err := Send(root, SendOpts{
		From:    "orchestrator",
		To:      "builder-dead",
		Subject: "rerouted message",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Dead agent's inbox should be empty
	msgs, _ := Read(root, ReadOpts{Agent: "builder-dead"})
	if len(msgs) != 0 {
		t.Errorf("dead agent inbox should be empty, got %d", len(msgs))
	}

	// Parent should have the message
	msgs, _ = Read(root, ReadOpts{Agent: "lead-001"})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in parent inbox, got %d", len(msgs))
	}
	if msgs[0].Subject != "rerouted message" {
		t.Errorf("Subject: want 'rerouted message', got %q", msgs[0].Subject)
	}
}

func TestCorruptedYAMLSkipped(t *testing.T) {
	root := setupRoot(t)

	// Send a valid message
	Send(root, SendOpts{From: "a", To: "b", Subject: "valid"})

	// Write a corrupted YAML file into the inbox
	inboxDir := filepath.Join(root, "mail", "inbox", "b")
	os.WriteFile(filepath.Join(inboxDir, "corrupted.yaml"), []byte("{{{{not yaml"), 0644)

	// Read should skip the corrupted file and return the valid one
	msgs, err := Read(root, ReadOpts{Agent: "b"})
	if err != nil {
		t.Fatalf("Read with corrupted file: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 valid message, got %d", len(msgs))
	}
	if msgs[0].Subject != "valid" {
		t.Errorf("Subject: want 'valid', got %q", msgs[0].Subject)
	}
}

func TestLogFilterByType(t *testing.T) {
	root := setupRoot(t)

	Send(root, SendOpts{From: "a", To: "b", Subject: "blocker msg", Type: "blocker"})
	Send(root, SendOpts{From: "a", To: "b", Subject: "completion msg", Type: "completion"})

	msgs, err := Log(root, LogOpts{Type: "blocker"})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 blocker message, got %d", len(msgs))
	}
	if msgs[0].Type != "blocker" {
		t.Errorf("Type: want blocker, got %s", msgs[0].Type)
	}
}

func TestLogFilterByAgent(t *testing.T) {
	root := setupRoot(t)

	Send(root, SendOpts{From: "lead-001", To: "builder-001", Subject: "task1"})
	Send(root, SendOpts{From: "lead-002", To: "builder-002", Subject: "task2"})

	msgs, err := Log(root, LogOpts{Agent: "builder-001"})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for builder-001, got %d", len(msgs))
	}
}

func TestLogFilterBySince(t *testing.T) {
	root := setupRoot(t)

	// Write an old message directly to bypass time.Now() in Send
	old := &Message{
		ID:        "old-msg",
		From:      "a",
		To:        "b",
		Subject:   "old",
		Timestamp: time.Now().Add(-2 * time.Hour),
	}
	dir := filepath.Join(root, "mail", "inbox", "b")
	os.MkdirAll(dir, 0755)
	store.WriteYAML(filepath.Join(dir, old.ID+".yaml"), old)

	// Send a recent message
	Send(root, SendOpts{From: "a", To: "b", Subject: "recent"})

	msgs, err := Log(root, LogOpts{Since: 1 * time.Hour})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 recent message, got %d", len(msgs))
	}
	if msgs[0].Subject != "recent" {
		t.Errorf("Subject: want 'recent', got %q", msgs[0].Subject)
	}
}

func TestLogIncludesArchived(t *testing.T) {
	root := setupRoot(t)

	Send(root, SendOpts{From: "a", To: "b", Subject: "will archive"})
	msgs, _ := Read(root, ReadOpts{Agent: "b"})
	Archive(root, "b", msgs[0].ID)

	// Send another that stays in inbox
	Send(root, SendOpts{From: "a", To: "b", Subject: "stays"})

	all, err := Log(root, LogOpts{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 messages (inbox + archive), got %d", len(all))
	}
}

func TestReadSortedNewestFirst(t *testing.T) {
	root := setupRoot(t)

	// Write messages with explicit timestamps
	dir := filepath.Join(root, "mail", "inbox", "b")
	os.MkdirAll(dir, 0755)

	now := time.Now()
	for i, subj := range []string{"oldest", "middle", "newest"} {
		m := &Message{
			ID:        subj,
			From:      "a",
			To:        "b",
			Subject:   subj,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
		}
		store.WriteYAML(filepath.Join(dir, m.ID+".yaml"), m)
	}

	msgs, _ := Read(root, ReadOpts{Agent: "b"})
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Subject != "newest" {
		t.Errorf("first message should be newest, got %q", msgs[0].Subject)
	}
	if msgs[2].Subject != "oldest" {
		t.Errorf("last message should be oldest, got %q", msgs[2].Subject)
	}
}

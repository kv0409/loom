package mail

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/store"
)

type Message struct {
	ID        string    `yaml:"id"`
	From      string    `yaml:"from"`
	To        string    `yaml:"to"`
	Type      string    `yaml:"type"`
	Priority  string    `yaml:"priority"`
	Timestamp time.Time `yaml:"timestamp"`
	Ref       string    `yaml:"ref,omitempty"`
	Subject   string    `yaml:"subject"`
	Body      string    `yaml:"body"`
	Read      bool      `yaml:"read"`
}

type SendOpts struct {
	From     string
	To       string
	Subject  string
	Body     string
	Type     string
	Priority string
	Ref      string
}

type ReadOpts struct {
	Agent      string
	UnreadOnly bool
}

type LogOpts struct {
	Agent string
	Type  string
	Since time.Duration
}

// ErrRecipientNotFound is returned when the recipient agent has no registration.
var ErrRecipientNotFound = fmt.Errorf("recipient agent not found")

func Send(loomRoot string, opts SendOpts) error {
	msg := &Message{
		From:     opts.From,
		To:       opts.To,
		Subject:  opts.Subject,
		Body:     opts.Body,
		Type:     opts.Type,
		Priority: opts.Priority,
		Ref:      opts.Ref,
	}
	msg.Timestamp = time.Now()

	nonce, err := randomNonce()
	if err != nil {
		return fmt.Errorf("generating mail nonce: %w", err)
	}
	msg.ID = fmt.Sprintf("%d-%s-%s-%s", msg.Timestamp.UnixNano(), msg.From, slug(msg.Subject), nonce)

	// Validate recipient exists
	to := msg.To
	a, err := agent.Load(loomRoot, to)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrRecipientNotFound, to)
	}

	// Route to parent if recipient is dead
	if a.Status == "dead" && a.SpawnedBy != "" {
		to = a.SpawnedBy
	}

	dir := filepath.Join(loomRoot, "mail", "inbox", to)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return store.WriteYAML(filepath.Join(dir, msg.ID+".yaml"), msg)
}

func randomNonce() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Read(loomRoot string, opts ReadOpts) ([]*Message, error) {
	dir := filepath.Join(loomRoot, "mail", "inbox", opts.Agent)
	files, err := store.ListYAMLFiles(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var msgs []*Message
	for _, f := range files {
		var m Message
		if err := store.ReadYAML(f, &m); err != nil {
			continue
		}
		if opts.UnreadOnly && m.Read {
			continue
		}
		msgs = append(msgs, &m)
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Timestamp.After(msgs[j].Timestamp) })
	return msgs, nil
}

func MarkRead(loomRoot string, agent string, msgID string) error {
	path := filepath.Join(loomRoot, "mail", "inbox", agent, msgID+".yaml")
	var m Message
	if err := store.ReadYAML(path, &m); err != nil {
		return err
	}
	m.Read = true
	return store.WriteYAML(path, &m)
}

func Archive(loomRoot string, agent string, msgID string) error {
	src := filepath.Join(loomRoot, "mail", "inbox", agent, msgID+".yaml")
	date := time.Now().Format("2006-01-02")
	dst := filepath.Join(loomRoot, "mail", "archive", date)
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return os.Rename(src, filepath.Join(dst, msgID+".yaml"))
}

// ArchiveAndRemoveInbox archives all messages in an agent's inbox, then removes the directory.
// Safe to call on empty or nonexistent inboxes.
func ArchiveAndRemoveInbox(loomRoot string, agentID string) error {
	dir := filepath.Join(loomRoot, "mail", "inbox", agentID)
	files, err := store.ListYAMLFiles(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	date := time.Now().Format("2006-01-02")
	dst := filepath.Join(loomRoot, "mail", "archive", date)
	if len(files) > 0 {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
	}
	for _, f := range files {
		name := filepath.Base(f)
		os.Rename(f, filepath.Join(dst, name))
	}
	return os.RemoveAll(dir)
}

func Log(loomRoot string, opts LogOpts) ([]*Message, error) {
	var msgs []*Message
	cutoff := time.Time{}
	if opts.Since > 0 {
		cutoff = time.Now().Add(-opts.Since)
	}

	collect := func(dir string) {
		files, err := store.ListYAMLFiles(dir)
		if err != nil {
			return
		}
		for _, f := range files {
			var m Message
			if err := store.ReadYAML(f, &m); err != nil {
				continue
			}
			if opts.Agent != "" && m.From != opts.Agent && m.To != opts.Agent {
				continue
			}
			if opts.Type != "" && m.Type != opts.Type {
				continue
			}
			if !cutoff.IsZero() && m.Timestamp.Before(cutoff) {
				continue
			}
			msgs = append(msgs, &m)
		}
	}

	// Walk inbox
	inboxRoot := filepath.Join(loomRoot, "mail", "inbox")
	if entries, err := os.ReadDir(inboxRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				collect(filepath.Join(inboxRoot, e.Name()))
			}
		}
	}

	// Walk archive
	archiveRoot := filepath.Join(loomRoot, "mail", "archive")
	if entries, err := os.ReadDir(archiveRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				collect(filepath.Join(archiveRoot, e.Name()))
			}
		}
	}

	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Timestamp.After(msgs[j].Timestamp) })
	return msgs, nil
}

func slug(subject string) string {
	words := strings.Fields(strings.ToLower(subject))
	if len(words) > 3 {
		words = words[:3]
	}
	return strings.Join(words, "-")
}

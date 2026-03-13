package mail

import (
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

func Send(loomRoot string, msg *Message) error {
	msg.Timestamp = time.Now()
	msg.ID = fmt.Sprintf("%d-%s-%s", msg.Timestamp.Unix(), msg.From, slug(msg.Subject))

	// Route to parent if recipient is dead or missing
	to := msg.To
	if a, err := agent.Load(loomRoot, to); err == nil && a.Status == "dead" && a.SpawnedBy != "" {
		to = a.SpawnedBy
	}

	dir := filepath.Join(loomRoot, "mail", "inbox", to)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return store.WriteYAML(filepath.Join(dir, msg.ID+".yaml"), msg)
}

func Read(loomRoot string, agent string, unreadOnly bool) ([]*Message, error) {
	dir := filepath.Join(loomRoot, "mail", "inbox", agent)
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
		if unreadOnly && m.Read {
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

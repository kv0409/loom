package proposal

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/store"
)

type Proposal struct {
	ID             string     `yaml:"id"`
	Title          string     `yaml:"title"`
	Description    string     `yaml:"description"`
	ProposedAction string     `yaml:"proposed_action"`
	Category       string     `yaml:"category"`
	Status         string     `yaml:"status"`
	CreatedAt      time.Time  `yaml:"created_at"`
	RespondedAt    *time.Time `yaml:"responded_at,omitempty"`
	Response       string     `yaml:"response,omitempty"`
	SessionID      string     `yaml:"session_id,omitempty"`
}

func proposalsDir(loomRoot string) string {
	return filepath.Join(loomRoot, "proposals")
}

func proposalPath(loomRoot, id string) string {
	return filepath.Join(proposalsDir(loomRoot), id+".yaml")
}

func Create(loomRoot, title, description, proposedAction, category, sessionID string) (*Proposal, error) {
	dir := proposalsDir(loomRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating proposals dir: %w", err)
	}
	n, err := store.NextCounter(filepath.Join(dir, "counter.txt"))
	if err != nil {
		return nil, fmt.Errorf("getting next counter: %w", err)
	}
	p := &Proposal{
		ID:             fmt.Sprintf("PROP-%03d", n),
		Title:          title,
		Description:    description,
		ProposedAction: proposedAction,
		Category:       category,
		Status:         "pending",
		CreatedAt:      time.Now(),
		SessionID:      sessionID,
	}
	if err := store.WriteYAML(proposalPath(loomRoot, p.ID), p); err != nil {
		return nil, fmt.Errorf("writing proposal: %w", err)
	}
	return p, nil
}

func Get(loomRoot, id string) (*Proposal, error) {
	p := &Proposal{}
	if err := store.ReadYAML(proposalPath(loomRoot, id), p); err != nil {
		return nil, fmt.Errorf("loading proposal %s: %w", id, err)
	}
	return p, nil
}

func List(loomRoot, status string) ([]*Proposal, error) {
	files, err := store.ListYAMLFiles(proposalsDir(loomRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var proposals []*Proposal
	for _, f := range files {
		p := &Proposal{}
		if err := store.ReadYAML(f, p); err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Base(f), err)
		}
		if status != "" && p.Status != status {
			continue
		}
		proposals = append(proposals, p)
	}
	sort.Slice(proposals, func(i, j int) bool {
		return proposals[i].CreatedAt.After(proposals[j].CreatedAt)
	})
	return proposals, nil
}

func Respond(loomRoot, id, action, feedback string) (*Proposal, error) {
	p, err := Get(loomRoot, id)
	if err != nil {
		return nil, err
	}
	if p.Status != "pending" {
		return nil, fmt.Errorf("proposal %s is already %s", id, p.Status)
	}
	switch action {
	case "accepted", "rejected", "dismissed":
	default:
		return nil, fmt.Errorf("invalid action %q: must be accepted, rejected, or dismissed", action)
	}
	now := time.Now()
	p.Status = action
	p.RespondedAt = &now
	p.Response = feedback
	if err := store.WriteYAML(proposalPath(loomRoot, p.ID), p); err != nil {
		return nil, fmt.Errorf("saving proposal: %w", err)
	}
	return p, nil
}

func CountPending(loomRoot string) (int, error) {
	proposals, err := List(loomRoot, "pending")
	if err != nil {
		return 0, err
	}
	return len(proposals), nil
}

func Exists(loomRoot, title string) (bool, error) {
	proposals, err := List(loomRoot, "")
	if err != nil {
		return false, err
	}
	h := titleHash(title)
	for _, p := range proposals {
		if titleHash(p.Title) == h {
			return true, nil
		}
	}
	return false, nil
}

func titleHash(title string) string {
	s := strings.TrimSpace(strings.ToLower(title))
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:8])
}

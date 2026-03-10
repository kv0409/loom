package issue

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/store"
)

type Issue struct {
	ID          string         `yaml:"id"`
	Title       string         `yaml:"title"`
	Description string         `yaml:"description"`
	Type        string         `yaml:"type"`
	Status      string         `yaml:"status"`
	Priority    string         `yaml:"priority"`
	Assignee    string         `yaml:"assignee"`
	Parent      string         `yaml:"parent"`
	DependsOn   []string       `yaml:"depends_on"`
	Worktree    string         `yaml:"worktree"`
	CreatedBy   string         `yaml:"created_by"`
	CreatedAt   time.Time      `yaml:"created_at"`
	UpdatedAt   time.Time      `yaml:"updated_at"`
	ClosedAt    *time.Time     `yaml:"closed_at,omitempty"`
	CloseReason string         `yaml:"close_reason,omitempty"`
	Children    []string       `yaml:"children,omitempty"`
	History     []HistoryEntry `yaml:"history"`
}

type HistoryEntry struct {
	At     time.Time `yaml:"at"`
	By     string    `yaml:"by"`
	Action string    `yaml:"action"`
	Detail string    `yaml:"detail,omitempty"`
}

type CreateOpts struct {
	Type        string
	Priority    string
	Parent      string
	Description string
	DependsOn   []string
}

type ListOpts struct {
	Status   string
	Assignee string
	Type     string
	Parent   string
	All      bool
}

type UpdateOpts struct {
	Status   string
	Priority string
	Assignee string
}

var validTransitions = map[string][]string{
	"open":        {"assigned"},
	"assigned":    {"in-progress"},
	"in-progress": {"review", "blocked", "done"},
	"blocked":     {"in-progress"},
	"review":      {"done", "in-progress"},
}

func issuesDir(loomRoot string) string {
	return filepath.Join(loomRoot, "issues")
}

func issuePath(loomRoot, id string) string {
	return filepath.Join(issuesDir(loomRoot), id+".yaml")
}

func Create(loomRoot string, title string, opts CreateOpts) (*Issue, error) {
	if opts.Type == "" {
		opts.Type = "task"
	}
	if opts.Priority == "" {
		opts.Priority = "normal"
	}

	now := time.Now()
	var id string

	if opts.Parent != "" {
		// Sub-issue: load parent, count children, generate sub-ID
		parent, err := Load(loomRoot, opts.Parent)
		if err != nil {
			return nil, fmt.Errorf("loading parent %s: %w", opts.Parent, err)
		}
		subNum := len(parent.Children) + 1
		id = fmt.Sprintf("%s-%02d", opts.Parent, subNum)

		parent.Children = append(parent.Children, id)
		parent.UpdatedAt = now
		if err := Save(loomRoot, parent); err != nil {
			return nil, fmt.Errorf("updating parent: %w", err)
		}
	} else {
		n, err := store.NextCounter(filepath.Join(issuesDir(loomRoot), "counter.txt"))
		if err != nil {
			return nil, fmt.Errorf("getting next counter: %w", err)
		}
		id = fmt.Sprintf("LOOM-%03d", n)
	}

	issue := &Issue{
		ID:          id,
		Title:       title,
		Description: opts.Description,
		Type:        opts.Type,
		Status:      "open",
		Priority:    opts.Priority,
		Parent:      opts.Parent,
		DependsOn:   opts.DependsOn,
		CreatedBy:   "human",
		CreatedAt:   now,
		UpdatedAt:   now,
		History: []HistoryEntry{
			{At: now, By: "human", Action: "created"},
		},
	}

	if err := store.WriteYAML(issuePath(loomRoot, id), issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func Load(loomRoot string, id string) (*Issue, error) {
	issue := &Issue{}
	if err := store.ReadYAML(issuePath(loomRoot, id), issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func Save(loomRoot string, issue *Issue) error {
	issue.UpdatedAt = time.Now()
	return store.WriteYAML(issuePath(loomRoot, issue.ID), issue)
}

func List(loomRoot string, opts ListOpts) ([]*Issue, error) {
	files, err := store.ListYAMLFiles(issuesDir(loomRoot))
	if err != nil {
		return nil, err
	}
	var issues []*Issue
	for _, f := range files {
		issue := &Issue{}
		if err := store.ReadYAML(f, issue); err != nil {
			continue
		}
		if issue.ID == "" {
			continue
		}
		if !opts.All && (issue.Status == "done" || issue.Status == "cancelled") {
			continue
		}
		if opts.Status != "" && issue.Status != opts.Status {
			continue
		}
		if opts.Assignee != "" && issue.Assignee != opts.Assignee {
			continue
		}
		if opts.Type != "" && issue.Type != opts.Type {
			continue
		}
		if opts.Parent != "" && issue.Parent != opts.Parent {
			continue
		}
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].UpdatedAt.After(issues[j].UpdatedAt)
	})
	return issues, nil
}

func Update(loomRoot string, id string, opts UpdateOpts) (*Issue, error) {
	issue, err := Load(loomRoot, id)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	if opts.Status != "" {
		if err := validateTransition(issue.Status, opts.Status); err != nil {
			return nil, err
		}
		issue.History = append(issue.History, HistoryEntry{
			At: now, By: "human", Action: "status_change",
			Detail: issue.Status + " → " + opts.Status,
		})
		issue.Status = opts.Status
	}
	if opts.Priority != "" {
		issue.History = append(issue.History, HistoryEntry{
			At: now, By: "human", Action: "priority_change",
			Detail: issue.Priority + " → " + opts.Priority,
		})
		issue.Priority = opts.Priority
	}
	if opts.Assignee != "" {
		issue.History = append(issue.History, HistoryEntry{
			At: now, By: "human", Action: "assigned",
			Detail: opts.Assignee,
		})
		issue.Assignee = opts.Assignee
	}

	if err := Save(loomRoot, issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func Close(loomRoot string, id string, reason string) (*Issue, error) {
	issue, err := Load(loomRoot, id)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	issue.Status = "done"
	issue.ClosedAt = &now
	issue.CloseReason = reason
	issue.History = append(issue.History, HistoryEntry{
		At: now, By: "human", Action: "closed", Detail: reason,
	})

	if err := Save(loomRoot, issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func validateTransition(from, to string) error {
	if to == "cancelled" {
		return nil
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions from %q", from)
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s → %s (allowed: %s)", from, to, strings.Join(allowed, ", "))
}

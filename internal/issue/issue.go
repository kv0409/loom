package issue

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/store"
)

// actor returns the current agent ID if running inside a loom agent, otherwise "human".
func actor() string {
	if id := os.Getenv("LOOM_AGENT_ID"); id != "" {
		return id
	}
	return "human"
}

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
	MergedAt    *time.Time     `yaml:"merged_at,omitempty"`
	CloseReason string         `yaml:"close_reason,omitempty"`
	Children    []string          `yaml:"children,omitempty"`
	Dispatch    map[string]string `yaml:"dispatch,omitempty"`
	History     []HistoryEntry    `yaml:"history"`
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
	Dispatch    map[string]string
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
	Dispatch map[string]string
}

var validTransitions = map[string][]string{
	"open":        {"assigned"},
	"assigned":    {"in-progress", "open"},
	"in-progress": {"review", "blocked", "done", "open"},
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
		subNum := 0
		for _, childID := range parent.Children {
			if n, err := strconv.Atoi(strings.TrimPrefix(childID, opts.Parent+"-")); err == nil && n > subNum {
				subNum = n
			}
		}
		subNum++
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
		Dispatch:    opts.Dispatch,
		CreatedBy:   actor(),
		CreatedAt:   now,
		UpdatedAt:   now,
		History: []HistoryEntry{
			{At: now, By: actor(), Action: "created"},
		},
	}

	if err := store.WriteYAML(issuePath(loomRoot, id), issue); err != nil {
		return nil, fmt.Errorf("creating issue: %w", err)
	}
	return issue, nil
}

func Load(loomRoot string, id string) (*Issue, error) {
	issue := &Issue{}
	if err := store.ReadYAML(issuePath(loomRoot, id), issue); err != nil {
		return nil, fmt.Errorf("loading issue %s: %w", id, err)
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
			At: now, By: actor(), Action: "status_change",
			Detail: issue.Status + " → " + opts.Status,
		})
		issue.Status = opts.Status
	}
	if opts.Priority != "" {
		issue.History = append(issue.History, HistoryEntry{
			At: now, By: actor(), Action: "priority_change",
			Detail: issue.Priority + " → " + opts.Priority,
		})
		issue.Priority = opts.Priority
	}
	if opts.Assignee != "" {
		if !issue.IsReady(loomRoot) {
			return nil, fmt.Errorf("cannot assign %s: has unresolved dependencies", id)
		}
		issue.History = append(issue.History, HistoryEntry{
			At: now, By: actor(), Action: "assigned",
			Detail: opts.Assignee,
		})
		issue.Assignee = opts.Assignee
	}
	if len(opts.Dispatch) > 0 {
		if issue.Dispatch == nil {
			issue.Dispatch = make(map[string]string)
		}
		var parts []string
		for k, v := range opts.Dispatch {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts)
		issue.History = append(issue.History, HistoryEntry{
			At: now, By: actor(), Action: "dispatch_change",
			Detail: strings.Join(parts, ", "),
		})
		for k, v := range opts.Dispatch {
			issue.Dispatch[k] = v
		}
	}

	if err := Save(loomRoot, issue); err != nil {
		return nil, err
	}
	return issue, nil
}

// ClosedInfo holds info about a closed issue for caller notification.
type ClosedInfo struct {
	IssueID          string
	PreviousAssignee string
}

func Close(loomRoot string, id string, reason string) (*ClosedInfo, error) {
	issue, err := Load(loomRoot, id)
	if err != nil {
		return nil, err
	}

	// Validate all children are in a terminal state before closing
	if len(issue.Children) > 0 {
		var openChildren []string
		for _, childID := range issue.Children {
			child, err := Load(loomRoot, childID)
			if err != nil {
				return nil, fmt.Errorf("loading child %s: %w", childID, err)
			}
			if child.Status != "done" && child.Status != "cancelled" {
				openChildren = append(openChildren, childID)
			}
		}
		if len(openChildren) > 0 {
			return nil, fmt.Errorf("cannot close %s: %d child issue(s) still open: %s",
				id, len(openChildren), strings.Join(openChildren, ", "))
		}
	}

	now := time.Now()
	prevAssignee := issue.Assignee

	issue.Status = "done"
	issue.Assignee = ""
	issue.ClosedAt = &now
	issue.CloseReason = reason
	issue.History = append(issue.History, HistoryEntry{
		At: now, By: actor(), Action: "closed", Detail: reason,
	})

	if err := Save(loomRoot, issue); err != nil {
		return nil, err
	}
	return &ClosedInfo{IssueID: id, PreviousAssignee: prevAssignee}, nil
}

// CancelledInfo holds info about a cancelled issue for caller notification.
type CancelledInfo struct {
	IssueID          string
	PreviousAssignee string
}

// Cancel cancels an issue and recursively cancels all active children.
// Returns info about every issue that was cancelled so the caller can notify agents.
func Cancel(loomRoot, id string) ([]CancelledInfo, error) {
	issue, err := Load(loomRoot, id)
	if err != nil {
		return nil, err
	}

	var result []CancelledInfo

	// Only cancel if the issue is in an active state
	activeStatuses := map[string]bool{"open": true, "assigned": true, "in-progress": true, "blocked": true, "review": true}
	if !activeStatuses[issue.Status] {
		return result, nil
	}

	now := time.Now()
	prevAssignee := issue.Assignee

	issue.History = append(issue.History, HistoryEntry{
		At: now, By: actor(), Action: "status_change",
		Detail: issue.Status + " → cancelled",
	})
	issue.Status = "cancelled"
	issue.Assignee = ""
	issue.ClosedAt = &now

	if err := Save(loomRoot, issue); err != nil {
		return nil, fmt.Errorf("saving cancelled issue %s: %w", id, err)
	}

	result = append(result, CancelledInfo{IssueID: id, PreviousAssignee: prevAssignee})

	// Recursively cancel active children
	for _, childID := range issue.Children {
		childResult, err := Cancel(loomRoot, childID)
		if err != nil {
			return nil, fmt.Errorf("cancelling child %s: %w", childID, err)
		}
		result = append(result, childResult...)
	}

	return result, nil
}

// IsReady returns true if all issues in DependsOn are in a terminal state
// (done or cancelled). An issue with no dependencies is always ready.
func (iss *Issue) IsReady(loomRoot string) bool {
	for _, depID := range iss.DependsOn {
		dep, err := Load(loomRoot, depID)
		if err != nil {
			return false // missing dependency → not ready
		}
		if dep.Status != "done" && dep.Status != "cancelled" {
			return false
		}
	}
	return true
}

// ListReady returns open, unassigned issues whose dependencies are all resolved.
func ListReady(loomRoot string) ([]*Issue, error) {
	issues, err := List(loomRoot, ListOpts{Status: "open"})
	if err != nil {
		return nil, err
	}
	var ready []*Issue
	for _, iss := range issues {
		if iss.Assignee == "" && iss.IsReady(loomRoot) {
			ready = append(ready, iss)
		}
	}
	return ready, nil
}

func validateTransition(from, to string) error {
	if to == "cancelled" {
		return fmt.Errorf("use issue.Cancel() to cancel issues (cascades to children)")
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

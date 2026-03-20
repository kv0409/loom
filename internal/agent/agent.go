package agent

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/store"
	"github.com/karanagi/loom/internal/worktree"
)

type Agent struct {
	ID             string      `yaml:"id"`
	Role           string      `yaml:"role"`
	Status         string      `yaml:"status"`
	PID            int         `yaml:"pid"`
	SpawnedBy      string      `yaml:"spawned_by"`
	SpawnedAt      time.Time   `yaml:"spawned_at"`
	Heartbeat      time.Time   `yaml:"heartbeat"`
	AssignedIssues []string    `yaml:"assigned_issues,omitempty"`
	WorktreeName   string      `yaml:"worktree,omitempty"`
	ACPSessionID   string      `yaml:"acp_session_id,omitempty"`
	InitialTask    string      `yaml:"initial_task,omitempty"`
	NudgeCount     int         `yaml:"nudge_count,omitempty"`
	LastNudge      time.Time   `yaml:"last_nudge,omitempty"`
	FileScope      []string    `yaml:"file_scope,omitempty"`
	Config         AgentConfig `yaml:"config"`
}

type AgentConfig struct {
	KiroMode   string `yaml:"kiro_mode"`
	MCPEnabled bool   `yaml:"mcp_enabled"`
	Model      string `yaml:"model,omitempty"`
}

type SpawnOpts struct {
	Role           string
	SpawnedBy      string
	AssignedIssues []string
	IssueSlug      string
	ExtraContext   map[string]string
	Mode           string
	Model          string
	FileScope      []string
}

func agentsDir(loomRoot string) string      { return filepath.Join(loomRoot, "agents") }
func agentPath(loomRoot, id string) string  { return filepath.Join(agentsDir(loomRoot), id+".yaml") }
func mailboxDir(loomRoot, id string) string { return filepath.Join(loomRoot, "mail", "inbox", id) }

func Register(loomRoot string, agent *Agent) error {
	if err := os.MkdirAll(agentsDir(loomRoot), 0755); err != nil {
		return fmt.Errorf("creating agents dir: %w", err)
	}
	if err := os.MkdirAll(mailboxDir(loomRoot, agent.ID), 0755); err != nil {
		return fmt.Errorf("creating mailbox dir: %w", err)
	}
	return store.WriteYAML(agentPath(loomRoot, agent.ID), agent)
}

func Load(loomRoot, id string) (*Agent, error) {
	a := &Agent{}
	if err := store.ReadYAML(agentPath(loomRoot, id), a); err != nil {
		return nil, err
	}
	return a, nil
}

func Save(loomRoot string, agent *Agent) error {
	return store.WriteYAML(agentPath(loomRoot, agent.ID), agent)
}

func List(loomRoot string) ([]*Agent, error) {
	files, err := store.ListYAMLFiles(agentsDir(loomRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var agents []*Agent
	for _, f := range files {
		a := &Agent{}
		if err := store.ReadYAML(f, a); err != nil {
			return nil, fmt.Errorf("reading agent %s: %w", filepath.Base(f), err)
		}
		if a.ID == "" {
			return nil, fmt.Errorf("reading agent %s: missing id", filepath.Base(f))
		}
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].SpawnedAt.After(agents[j].SpawnedAt)
	})
	return agents, nil
}

func Deregister(loomRoot, id string) error {
	return os.Remove(agentPath(loomRoot, id))
}

func UpdateHeartbeat(loomRoot, id string) error {
	a, err := Load(loomRoot, id)
	if err != nil {
		return fmt.Errorf("loading agent %s: %w", id, err)
	}
	a.Heartbeat = time.Now()
	return Save(loomRoot, a)
}

func nextID(loomRoot, role string) (string, error) {
	if role == "orchestrator" {
		return "orchestrator", nil
	}
	n, err := reserveRoleNumber(loomRoot, role)
	if err != nil {
		return "", fmt.Errorf("reserving next %s ID: %w", role, err)
	}
	return fmt.Sprintf("%s-%03d", role, n), nil
}

func NextID(loomRoot, role string) string {
	id, err := nextID(loomRoot, role)
	if err != nil {
		return ""
	}
	return id
}

func reserveRoleNumber(loomRoot, role string) (int, error) {
	if err := os.MkdirAll(agentsDir(loomRoot), 0755); err != nil {
		return 0, fmt.Errorf("creating agents dir: %w", err)
	}

	counterPath := filepath.Join(agentsDir(loomRoot), role+".counter.txt")
	f, err := os.OpenFile(counterPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return 0, err
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)

	data, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}

	current := strings.TrimSpace(string(data))
	n := 0
	if current == "" {
		n, err = maxRoleNumber(loomRoot, role)
		if err != nil {
			return 0, err
		}
	} else {
		n, err = strconv.Atoi(current)
		if err != nil {
			return 0, fmt.Errorf("invalid agent counter %s: %w", counterPath, err)
		}
	}

	n++
	if err := f.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	if _, err := f.WriteString(strconv.Itoa(n)); err != nil {
		return 0, err
	}
	return n, nil
}

func maxRoleNumber(loomRoot, role string) (int, error) {
	entries, err := os.ReadDir(agentsDir(loomRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	max := 0
	prefix := role + "-"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".yaml")
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		numStr := strings.TrimPrefix(id, prefix)
		if n, err := strconv.Atoi(numStr); err == nil && n > max {
			max = n
		}
	}
	return max, nil
}

// loadDispatchDirectives collects dispatch key=value pairs from all assigned
// issues and returns them as newline-separated "KEY=VALUE" lines.
func loadDispatchDirectives(loomRoot string, issueIDs []string) string {
	var lines []string
	for _, id := range issueIDs {
		iss, err := issue.Load(loomRoot, id)
		if err != nil || len(iss.Dispatch) == 0 {
			continue
		}
		keys := make([]string, 0, len(iss.Dispatch))
		for k := range iss.Dispatch {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, k+"="+iss.Dispatch[k])
		}
	}
	return strings.Join(lines, "\n")
}

func buildTaskMsg(loomRoot string, opts SpawnOpts) string {
	if opts.ExtraContext != nil {
		if task, ok := opts.ExtraContext["task"]; ok {
			if len(opts.FileScope) > 0 {
				task += "\nFile scope hints (focus your edits here): " + strings.Join(opts.FileScope, ", ")
			}
			return task
		}
	}
	if len(opts.AssignedIssues) > 0 {
		base := fmt.Sprintf("Your assigned issues: %s. Read them with loom issue show and begin work.",
			strings.Join(opts.AssignedIssues, ", "))
		switch opts.Role {
		case "lead":
			base += " Remember: verify scope across the full codebase before decomposing — search for all affected files, not just those named in the issue."
		case "reviewer":
			base += " Remember: check whether the fix covers all affected locations in the codebase, not just the ones named in the issue."
		}
		if len(opts.FileScope) > 0 {
			base += "\nFile scope hints (focus your edits here): " + strings.Join(opts.FileScope, ", ")
		}
		if directives := loadDispatchDirectives(loomRoot, opts.AssignedIssues); directives != "" {
			base += "\nDispatch directives:\n" + directives
		}
		return base
	}
	return ""
}

func Spawn(loomRoot string, opts SpawnOpts) (*Agent, error) {
	id, err := nextID(loomRoot, opts.Role)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(loomRoot)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	now := time.Now()

	// Resolve model: CLI flag > per-role config > empty (kiro-cli default).
	model := opts.Model
	if model == "" {
		model = cfg.Models.ForRole(opts.Role)
	}

	a := &Agent{
		ID:             id,
		Role:           opts.Role,
		SpawnedBy:      opts.SpawnedBy,
		SpawnedAt:      now,
		Heartbeat:      now,
		AssignedIssues: opts.AssignedIssues,
		FileScope:      opts.FileScope,
		Config: AgentConfig{
			KiroMode:   "acp",
			MCPEnabled: cfg.MCP.Enabled,
			Model:      model,
		},
	}

	// Create worktree for builders before registering.
	if opts.Role == "builder" && len(opts.AssignedIssues) > 0 {
		slug := opts.IssueSlug
		if slug == "" {
			slug = "work"
		}
		wt, err := worktree.Create(loomRoot, opts.AssignedIssues[0], slug, id)
		if err != nil {
			return nil, fmt.Errorf("creating worktree: %w", err)
		}
		a.WorktreeName = wt.Name
	}

	a.Status = "pending-acp"
	a.InitialTask = buildTaskMsg(loomRoot, opts)
	if err := Register(loomRoot, a); err != nil {
		return nil, fmt.Errorf("registering agent: %w", err)
	}
	for _, issID := range a.AssignedIssues {
		if err := AssignIssue(loomRoot, a.ID, issID); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func Kill(loomRoot, id string, cleanupWorktree bool) error {
	return killWithResolved(loomRoot, id, cleanupWorktree, nil)
}

// KillWithResolved kills an agent, cascading only to children whose assigned
// issues are all in the resolved set.
func KillWithResolved(loomRoot, id string, cleanupWorktree bool, resolved map[string]bool) error {
	return killWithResolved(loomRoot, id, cleanupWorktree, resolved)
}

// killWithResolved kills an agent, cascading to children only if all their
// assigned issues are in the resolved set. A nil resolved set skips the check
// (kills all children unconditionally).
func killWithResolved(loomRoot, id string, cleanupWorktree bool, resolved map[string]bool) error {
	a, err := Load(loomRoot, id)
	if err != nil {
		return fmt.Errorf("loading agent %s: %w", id, err)
	}
	// Cascade: kill children, skipping those with unresolved issues.
	children, err := listChildren(loomRoot, id)
	if err != nil {
		return fmt.Errorf("listing children of %s: %w", id, err)
	}
	for _, child := range children {
		if resolved != nil && !childIssuesResolved(loomRoot, child, resolved) {
			continue
		}
		if err := killWithResolved(loomRoot, child.ID, cleanupWorktree, resolved); err != nil {
			return err
		}
	}
	// Kill ACP process group by PID.
	if a.PID > 0 {
		if err := syscall.Kill(-a.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("sending SIGTERM to %s: %w", id, err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := syscall.Kill(-a.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("sending SIGKILL to %s: %w", id, err)
		}
	}
	if cleanupWorktree && a.WorktreeName != "" {
		wtPath := filepath.Join(loomRoot, "worktrees", a.WorktreeName)
		if worktree.HasDirtyFiles(wtPath) {
			worktree.SalvageCommit(wtPath, a.ID)
		}
		if err := worktree.Remove(loomRoot, a.WorktreeName, false); err != nil {
			if errors.Is(err, worktree.ErrUnmergedBranch) {
				log.Printf("[agent] preserving worktree %s: has unmerged commits", a.WorktreeName)
			} else if err2 := worktree.ForceRemove(loomRoot, a.WorktreeName); err2 != nil {
				log.Printf("[agent] failed to remove worktree %s: %v", a.WorktreeName, err2)
			}
		}
	}
	// Archive remaining mail before removing inbox
	if err := archiveInbox(loomRoot, id); err != nil {
		return fmt.Errorf("archiving inbox for %s: %w", id, err)
	}
	if err := UnassignAllIssues(loomRoot, a); err != nil {
		return fmt.Errorf("unassigning issues for %s: %w", id, err)
	}
	return Deregister(loomRoot, id)
}

// archiveInbox moves all messages from an agent's inbox to the archive, then removes the inbox dir.
func archiveInbox(loomRoot, agentID string) error {
	dir := filepath.Join(loomRoot, "mail", "inbox", agentID)
	files, err := store.ListYAMLFiles(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type archivedMove struct {
		src string
		dst string
	}
	var moved []archivedMove
	if len(files) > 0 {
		date := time.Now().Format("2006-01-02")
		dst := filepath.Join(loomRoot, "mail", "archive", date)
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		for _, f := range files {
			target := filepath.Join(dst, filepath.Base(f))
			if err := os.Rename(f, target); err != nil {
				for i := len(moved) - 1; i >= 0; i-- {
					if rollbackErr := os.Rename(moved[i].dst, moved[i].src); rollbackErr != nil {
						return fmt.Errorf("moving mail %s: %w (rollback failed for %s: %v)", filepath.Base(f), err, filepath.Base(moved[i].src), rollbackErr)
					}
				}
				return fmt.Errorf("moving mail %s: %w", filepath.Base(f), err)
			}
			moved = append(moved, archivedMove{src: f, dst: target})
		}
	}
	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// KillProcess kills the OS process (group) for a dead agent by PID.
// Returns true if a process was found and signalled.
func KillProcess(a *Agent) bool {
	if a.PID <= 0 {
		return false
	}
	// Check if process is still alive.
	if err := syscall.Kill(a.PID, 0); err != nil {
		return false
	}
	syscall.Kill(-a.PID, syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	// Force kill if still alive.
	if err := syscall.Kill(a.PID, 0); err == nil {
		syscall.Kill(-a.PID, syscall.SIGKILL)
	}
	return true
}

// AssignIssue assigns an issue to an agent, updating both agent and issue state.
// If the issue was previously assigned to a different agent, that agent's
// AssignedIssues list is cleaned up first.
func AssignIssue(loomRoot, agentID, issueID string) error {
	iss, err := issue.Load(loomRoot, issueID)
	if err != nil {
		return fmt.Errorf("loading issue %s: %w", issueID, err)
	}

	a, err := Load(loomRoot, agentID)
	if err != nil {
		return fmt.Errorf("loading agent %s: %w", agentID, err)
	}

	var prev *Agent
	var prevOriginal []string
	if iss.Assignee != "" && iss.Assignee != agentID {
		loadedPrev, err := Load(loomRoot, iss.Assignee)
		if err == nil {
			prev = loadedPrev
			prevOriginal = append(prevOriginal, prev.AssignedIssues...)
		} else {
			prev = nil
		}
	}

	newOriginal := append([]string(nil), a.AssignedIssues...)
	if !containsStr(a.AssignedIssues, issueID) {
		a.AssignedIssues = append(a.AssignedIssues, issueID)
	}
	if prev != nil {
		prev.AssignedIssues = removeStr(prev.AssignedIssues, issueID)
		if err := Save(loomRoot, prev); err != nil {
			return fmt.Errorf("saving previous agent %s: %w", prev.ID, err)
		}
	}
	if !sameStrings(newOriginal, a.AssignedIssues) {
		if err := Save(loomRoot, a); err != nil {
			if prev != nil {
				prev.AssignedIssues = prevOriginal
				_ = Save(loomRoot, prev)
			}
			return fmt.Errorf("saving agent %s: %w", agentID, err)
		}
	}

	// Update issue assignee.
	opts := issue.UpdateOpts{Assignee: agentID}
	if iss.Status == "open" {
		opts.Status = "assigned"
	}
	if _, err := issue.Update(loomRoot, issueID, opts); err != nil {
		a.AssignedIssues = newOriginal
		_ = Save(loomRoot, a)
		if prev != nil {
			prev.AssignedIssues = prevOriginal
			_ = Save(loomRoot, prev)
		}
		return fmt.Errorf("updating issue %s: %w", issueID, err)
	}
	return nil
}

// UnassignIssue clears the assignee on an issue and removes it from the
// current agent's AssignedIssues list.
func UnassignIssue(loomRoot, issueID string) error {
	iss, err := issue.Load(loomRoot, issueID)
	if err != nil {
		return fmt.Errorf("loading issue %s: %w", issueID, err)
	}
	if iss.Assignee == "" {
		return nil
	}

	prevAgent := iss.Assignee
	issueSnapshot := cloneIssueState(iss)
	a, err := Load(loomRoot, prevAgent)
	if err != nil {
		return fmt.Errorf("loading agent %s: %w", prevAgent, err)
	}
	now := time.Now()
	iss.Assignee = ""
	iss.History = append(iss.History, issue.HistoryEntry{
		At: now, By: prevAgent, Action: "unassigned",
	})
	if iss.Status == "assigned" || iss.Status == "in-progress" {
		iss.History = append(iss.History, issue.HistoryEntry{
			At: now, By: currentActor(), Action: "status_change",
			Detail: iss.Status + " → open",
		})
		iss.Status = "open"
	}
	if err := issue.Save(loomRoot, iss); err != nil {
		return fmt.Errorf("saving issue %s: %w", issueID, err)
	}

	a.AssignedIssues = removeStr(a.AssignedIssues, issueID)
	if err := Save(loomRoot, a); err != nil {
		if rollbackErr := restoreIssueSnapshot(loomRoot, issueSnapshot); rollbackErr != nil {
			return fmt.Errorf("saving agent %s: %w (also failed to restore issue %s: %v)", prevAgent, err, issueID, rollbackErr)
		}
		return fmt.Errorf("saving agent %s: %w", prevAgent, err)
	}
	return nil
}

// CancelIssue cancels an issue and reconciles agent ownership for all affected issues.
// Returns the list of cancelled issues (with previous assignees) for caller notification.
func CancelIssue(loomRoot, issueID string) ([]issue.CancelledInfo, error) {
	issueSnapshots := make(map[string]*issue.Issue)
	if err := collectIssueSnapshots(loomRoot, issueID, issueSnapshots); err != nil {
		return nil, err
	}

	cancelled, err := issue.Cancel(loomRoot, issueID)
	if err != nil {
		return nil, err
	}

	originalAgents := make(map[string]*Agent)
	updatedAgents := make(map[string]*Agent)
	for _, ci := range cancelled {
		if ci.PreviousAssignee == "" {
			continue
		}
		if _, ok := updatedAgents[ci.PreviousAssignee]; !ok {
			a, err := Load(loomRoot, ci.PreviousAssignee)
			if err != nil {
				if rollbackErr := restoreIssueSnapshots(loomRoot, issueSnapshots); rollbackErr != nil {
					return nil, fmt.Errorf("loading agent %s: %w (also failed to restore issues: %v)", ci.PreviousAssignee, err, rollbackErr)
				}
				return nil, fmt.Errorf("loading agent %s: %w", ci.PreviousAssignee, err)
			}
			originalAgents[ci.PreviousAssignee] = cloneAgent(a)
			updatedAgents[ci.PreviousAssignee] = cloneAgent(a)
		}
		updatedAgents[ci.PreviousAssignee].AssignedIssues = removeStr(updatedAgents[ci.PreviousAssignee].AssignedIssues, ci.IssueID)
	}

	if err := persistAgentSnapshots(loomRoot, originalAgents, updatedAgents); err != nil {
		if rollbackErr := restoreIssueSnapshots(loomRoot, issueSnapshots); rollbackErr != nil {
			return nil, fmt.Errorf("%v (also failed to restore issues: %v)", err, rollbackErr)
		}
		return nil, err
	}
	return cancelled, nil
}

// CloseIssue closes an issue and reconciles agent ownership.
// Returns info about the closed issue for caller notification.
func CloseIssue(loomRoot, issueID, reason string) (*issue.ClosedInfo, error) {
	current, err := issue.Load(loomRoot, issueID)
	if err != nil {
		return nil, err
	}
	issueSnapshot := cloneIssueState(current)

	var updatedAgent *Agent
	if current.Assignee != "" {
		a, err := Load(loomRoot, current.Assignee)
		if err != nil {
			return nil, fmt.Errorf("loading agent %s: %w", current.Assignee, err)
		}
		updatedAgent = cloneAgent(a)
	}

	info, err := issue.Close(loomRoot, issueID, reason)
	if err != nil {
		return nil, err
	}

	if info.PreviousAssignee != "" && updatedAgent != nil {
		updatedAgent.AssignedIssues = removeStr(updatedAgent.AssignedIssues, info.IssueID)
		if err := Save(loomRoot, updatedAgent); err != nil {
			if rollbackErr := restoreIssueSnapshot(loomRoot, issueSnapshot); rollbackErr != nil {
				return nil, fmt.Errorf("saving agent %s: %w (also failed to restore issue %s: %v)", info.PreviousAssignee, err, issueID, rollbackErr)
			}
			return nil, fmt.Errorf("saving agent %s: %w", info.PreviousAssignee, err)
		}
	}
	return info, nil
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func removeStr(ss []string, s string) []string {
	out := ss[:0]
	for _, v := range ss {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// UnassignAllIssues clears the assignee on each of the agent's assigned issues
// using UnassignIssue for consistent state sync.
func UnassignAllIssues(loomRoot string, a *Agent) error {
	for _, issID := range append([]string(nil), a.AssignedIssues...) {
		if err := UnassignIssue(loomRoot, issID); err != nil {
			return err
		}
	}
	return nil
}

func collectIssueSnapshots(loomRoot, issueID string, snapshots map[string]*issue.Issue) error {
	if _, ok := snapshots[issueID]; ok {
		return nil
	}
	iss, err := issue.Load(loomRoot, issueID)
	if err != nil {
		return fmt.Errorf("loading issue %s: %w", issueID, err)
	}
	snapshots[issueID] = cloneIssueState(iss)
	for _, childID := range iss.Children {
		if err := collectIssueSnapshots(loomRoot, childID, snapshots); err != nil {
			return err
		}
	}
	return nil
}

func persistAgentSnapshots(loomRoot string, original, updated map[string]*Agent) error {
	if len(updated) == 0 {
		return nil
	}
	keys := make([]string, 0, len(updated))
	for id := range updated {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	var saved []string
	for _, id := range keys {
		if sameStrings(original[id].AssignedIssues, updated[id].AssignedIssues) {
			continue
		}
		if err := Save(loomRoot, updated[id]); err != nil {
			for i := len(saved) - 1; i >= 0; i-- {
				restoreID := saved[i]
				if rollbackErr := Save(loomRoot, cloneAgent(original[restoreID])); rollbackErr != nil {
					return fmt.Errorf("saving agent %s: %w (also failed to restore agent %s: %v)", id, err, restoreID, rollbackErr)
				}
			}
			return fmt.Errorf("saving agent %s: %w", id, err)
		}
		saved = append(saved, id)
	}
	return nil
}

func restoreIssueSnapshot(loomRoot string, snapshot *issue.Issue) error {
	if snapshot == nil {
		return nil
	}
	return issue.Save(loomRoot, cloneIssueState(snapshot))
}

func restoreIssueSnapshots(loomRoot string, snapshots map[string]*issue.Issue) error {
	if len(snapshots) == 0 {
		return nil
	}
	keys := make([]string, 0, len(snapshots))
	for id := range snapshots {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if err := restoreIssueSnapshot(loomRoot, snapshots[id]); err != nil {
			return err
		}
	}
	return nil
}

func cloneAgent(a *Agent) *Agent {
	if a == nil {
		return nil
	}
	out := *a
	out.AssignedIssues = append([]string(nil), a.AssignedIssues...)
	out.FileScope = append([]string(nil), a.FileScope...)
	return &out
}

func cloneIssueState(iss *issue.Issue) *issue.Issue {
	if iss == nil {
		return nil
	}
	out := *iss
	out.DependsOn = append([]string(nil), iss.DependsOn...)
	out.Children = append([]string(nil), iss.Children...)
	out.History = append([]issue.HistoryEntry(nil), iss.History...)
	if iss.Dispatch != nil {
		out.Dispatch = make(map[string]string, len(iss.Dispatch))
		for k, v := range iss.Dispatch {
			out.Dispatch[k] = v
		}
	}
	if iss.ClosedAt != nil {
		closedAt := *iss.ClosedAt
		out.ClosedAt = &closedAt
	}
	if iss.MergedAt != nil {
		mergedAt := *iss.MergedAt
		out.MergedAt = &mergedAt
	}
	return &out
}

func currentActor() string {
	if id := os.Getenv("LOOM_AGENT_ID"); id != "" {
		return id
	}
	return "human"
}

func listChildren(loomRoot, parentID string) ([]*Agent, error) {
	all, err := List(loomRoot)
	if err != nil {
		return nil, err
	}
	var children []*Agent
	for _, a := range all {
		if a.SpawnedBy == parentID {
			children = append(children, a)
		}
	}
	return children, nil
}

// childIssuesResolved returns true if all of the child's assigned issues are
// in the resolved set (or the child has no assigned issues).
func childIssuesResolved(loomRoot string, a *Agent, resolved map[string]bool) bool {
	for _, issID := range a.AssignedIssues {
		if !resolved[issID] {
			return false
		}
	}
	return true
}

func RenderPrompt(loomRoot string, agent *Agent, extraContext map[string]string) (string, error) {
	tmplPath := filepath.Join(loomRoot, "templates", agent.Role+".md")
	data, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(agent.Role).Parse(string(data))
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	projectRoot := filepath.Dir(loomRoot)

	var wtPath, wtBranch string
	if agent.WorktreeName != "" {
		wts, _ := worktree.List(loomRoot)
		for _, wt := range wts {
			if wt.Name == agent.WorktreeName {
				wtPath = wt.Path
				wtBranch = wt.Branch
				break
			}
		}
	}

	vars := map[string]interface{}{
		"AgentID":            agent.ID,
		"Role":               agent.Role,
		"SpawnedBy":          agent.SpawnedBy,
		"AssignedIssues":     strings.Join(agent.AssignedIssues, ", "),
		"WorktreePath":       wtPath,
		"WorktreeBranch":     wtBranch,
		"MCPEnabled":         agent.Config.MCPEnabled,
		"ProjectRoot":        projectRoot,
		"LoomRoot":           loomRoot,
		"FileScope":          strings.Join(agent.FileScope, ", "),
		"DispatchDirectives": loadDispatchDirectives(loomRoot, agent.AssignedIssues),
	}
	for k, v := range extraContext {
		vars[k] = v
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("rendering template: %w", err)
	}
	return buf.String(), nil
}

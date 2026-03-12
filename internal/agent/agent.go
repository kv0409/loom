package agent

import (
	"fmt"
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
	"github.com/karanagi/loom/internal/tmux"
	"github.com/karanagi/loom/internal/worktree"
)

type Agent struct {
	ID             string      `yaml:"id"`
	Role           string      `yaml:"role"`
	Status         string      `yaml:"status"`
	PID            int         `yaml:"pid"`
	TmuxTarget     string      `yaml:"tmux_target"`
	SpawnedBy      string      `yaml:"spawned_by"`
	SpawnedAt      time.Time   `yaml:"spawned_at"`
	Heartbeat      time.Time   `yaml:"heartbeat"`
	AssignedIssues []string    `yaml:"assigned_issues,omitempty"`
	WorktreeName   string      `yaml:"worktree,omitempty"`
	ACPSessionID   string      `yaml:"acp_session_id,omitempty"`
	InitialTask    string      `yaml:"initial_task,omitempty"`
	NudgeCount     int         `yaml:"nudge_count,omitempty"`
	LastNudge      time.Time   `yaml:"last_nudge,omitempty"`
	Config         AgentConfig `yaml:"config"`
}

type AgentConfig struct {
	KiroMode   string `yaml:"kiro_mode"`
	MCPEnabled bool   `yaml:"mcp_enabled"`
}

type SpawnOpts struct {
	Role           string
	SpawnedBy      string
	AssignedIssues []string
	IssueSlug      string
	ExtraContext   map[string]string
	Mode           string
}

func agentsDir(loomRoot string) string  { return filepath.Join(loomRoot, "agents") }
func agentPath(loomRoot, id string) string { return filepath.Join(agentsDir(loomRoot), id+".yaml") }
func mailboxDir(loomRoot, id string) string { return filepath.Join(loomRoot, "mail", "inbox", id) }

func Register(loomRoot string, agent *Agent) error {
	if err := os.MkdirAll(agentsDir(loomRoot), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(mailboxDir(loomRoot, agent.ID), 0755); err != nil {
		return err
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
			continue
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
		return err
	}
	a.Heartbeat = time.Now()
	return Save(loomRoot, a)
}

func NextID(loomRoot, role string) string {
	if role == "orchestrator" {
		return "orchestrator"
	}
	agents, _ := List(loomRoot)
	max := 0
	prefix := role + "-"
	for _, a := range agents {
		if strings.HasPrefix(a.ID, prefix) {
			numStr := strings.TrimPrefix(a.ID, prefix)
			if n, err := strconv.Atoi(numStr); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%s-%03d", role, max+1)
}

func buildTaskMsg(opts SpawnOpts) string {
	if opts.ExtraContext != nil {
		if task, ok := opts.ExtraContext["task"]; ok {
			return task
		}
	}
	if len(opts.AssignedIssues) > 0 {
		return fmt.Sprintf("Your assigned issues: %s. Read them with loom issue show and begin work.",
			strings.Join(opts.AssignedIssues, ", "))
	}
	return ""
}

func Spawn(loomRoot string, opts SpawnOpts) (*Agent, error) {
	id := NextID(loomRoot, opts.Role)

	cfg, err := config.Load(loomRoot)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	now := time.Now()
	mode := opts.Mode
	if mode == "" {
		mode = cfg.Kiro.DefaultMode
	}
	if mode != "chat" && mode != "acp" {
		return nil, fmt.Errorf("invalid mode %q: must be chat or acp", mode)
	}

	a := &Agent{
		ID:             id,
		Role:           opts.Role,
		SpawnedBy:      opts.SpawnedBy,
		SpawnedAt:      now,
		Heartbeat:      now,
		AssignedIssues: opts.AssignedIssues,
		Config: AgentConfig{
			KiroMode:   mode,
			MCPEnabled: cfg.MCP.Enabled,
		},
	}

	// Create worktree for builders before registering (both modes need it).
	createWorktree := func() error {
		if opts.Role != "builder" || len(opts.AssignedIssues) == 0 {
			return nil
		}
		slug := opts.IssueSlug
		if slug == "" {
			slug = "work"
		}
		wt, err := worktree.Create(loomRoot, opts.AssignedIssues[0], slug, id)
		if err != nil {
			return fmt.Errorf("creating worktree: %w", err)
		}
		a.WorktreeName = wt.Name
		return nil
	}

	// --- ACP mode: register as pending-acp, daemon activates later ---
	if mode == "acp" {
		a.Status = "pending-acp"
		a.InitialTask = buildTaskMsg(opts)
		if err := createWorktree(); err != nil {
			return nil, err
		}
		if err := Register(loomRoot, a); err != nil {
			return nil, fmt.Errorf("registering agent: %w", err)
		}
		assignIssues(loomRoot, a)
		return a, nil
	}

	// --- Chat mode: tmux-based spawn (unchanged) ---
	target, err := tmux.NewWindow(cfg.Tmux.SessionName, id)
	if err != nil {
		return nil, fmt.Errorf("creating tmux window: %w", err)
	}
	a.Status = "spawning"
	a.TmuxTarget = target

	if err := Register(loomRoot, a); err != nil {
		tmux.KillWindow(target)
		return nil, fmt.Errorf("registering agent: %w", err)
	}
	assignIssues(loomRoot, a)

	if err := createWorktree(); err != nil {
		Deregister(loomRoot, id)
		tmux.KillWindow(target)
		return nil, err
	}
	if a.WorktreeName != "" {
		Save(loomRoot, a)
	}

	agentName := "loom-" + opts.Role
	projectRoot := filepath.Dir(loomRoot)

	envPrefix := fmt.Sprintf("LOOM_AGENT_ID=%s LOOM_ROOT=%s LOOM_PROJECT_ROOT=%s LOOM_ROLE=%s",
		id, loomRoot, projectRoot, opts.Role)
	if a.WorktreeName != "" {
		envPrefix += fmt.Sprintf(" LOOM_WORKTREE=%s", filepath.Join(loomRoot, "worktrees", a.WorktreeName))
	}
	if opts.SpawnedBy != "" {
		envPrefix += fmt.Sprintf(" LOOM_PARENT_AGENT=%s", opts.SpawnedBy)
	}

	taskMsg := buildTaskMsg(opts)
	kiroBase := fmt.Sprintf("%s %s %s --agent %s", envPrefix, cfg.Kiro.Command, mode, agentName)
	if taskMsg != "" {
		escaped := strings.ReplaceAll(taskMsg, "'", "'\\''")
		kiroBase += fmt.Sprintf(" '%s'", escaped)
	}

	var kiroCmd string
	if opts.Role == "builder" && a.WorktreeName != "" {
		kiroCmd = fmt.Sprintf("cd %s && %s", filepath.Join(loomRoot, "worktrees", a.WorktreeName), kiroBase)
	} else {
		kiroCmd = kiroBase
	}

	if err := tmux.RunInPane(target, kiroCmd); err != nil {
		Kill(loomRoot, id, true)
		return nil, fmt.Errorf("starting kiro: %w", err)
	}

	a.Status = "active"
	Save(loomRoot, a)

	return a, nil
}

func Kill(loomRoot, id string, cleanupWorktree bool) error {
	a, err := Load(loomRoot, id)
	if err != nil {
		return err
	}
	// Cascade: kill or warn children first
	children, _ := listChildren(loomRoot, id)
	for _, child := range children {
		if childSafeToKill(loomRoot, child) || child.TmuxTarget == "" {
			Kill(loomRoot, child.ID, cleanupWorktree)
		} else {
			tmux.SendKeys(child.TmuxTarget, "[LOOM] Shutdown")
		}
	}
	if a.TmuxTarget != "" {
		tmux.KillWindow(a.TmuxTarget)
	}
	// Kill ACP process group by PID (covers kiro-cli + aim sandbox + children).
	if a.PID > 0 && a.Config.KiroMode == "acp" {
		syscall.Kill(-a.PID, syscall.SIGTERM)
		// Brief grace period, then force kill.
		time.Sleep(500 * time.Millisecond)
		syscall.Kill(-a.PID, syscall.SIGKILL)
	}
	if cleanupWorktree && a.WorktreeName != "" {
		wtPath := filepath.Join(loomRoot, "worktrees", a.WorktreeName)
		if worktree.HasDirtyFiles(wtPath) {
			worktree.SalvageCommit(wtPath, a.ID)
		}
		if err := worktree.Remove(loomRoot, a.WorktreeName, true); err != nil {
			if err2 := worktree.ForceRemove(loomRoot, a.WorktreeName); err2 != nil {
				log.Printf("[agent] failed to remove worktree %s: %v", a.WorktreeName, err2)
			}
		}
	}
	// Purge mail inbox for the dead agent
	os.RemoveAll(filepath.Join(loomRoot, "mail", "inbox", id))
	unassignIssues(loomRoot, a)
	return Deregister(loomRoot, id)
}

// KillProcess kills the OS process (group) for a dead agent by PID.
// Returns true if a process was found and signalled.
func KillProcess(a *Agent) bool {
	if a.PID <= 0 || a.Config.KiroMode != "acp" {
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

// assignIssues sets the assignee on each of the agent's assigned issues.
func assignIssues(loomRoot string, a *Agent) {
	for _, issID := range a.AssignedIssues {
		iss, err := issue.Load(loomRoot, issID)
		if err != nil {
			continue
		}
		opts := issue.UpdateOpts{Assignee: a.ID}
		if iss.Status == "open" {
			opts.Status = "assigned"
		}
		issue.Update(loomRoot, issID, opts)
	}
}

// unassignIssues clears the assignee on each of the agent's assigned issues.
func unassignIssues(loomRoot string, a *Agent) {
	for _, issID := range a.AssignedIssues {
		iss, err := issue.Load(loomRoot, issID)
		if err != nil || iss.Assignee != a.ID {
			continue
		}
		iss.Assignee = ""
		if iss.Status == "assigned" || iss.Status == "in-progress" {
			iss.Status = "open"
		}
		issue.Save(loomRoot, iss)
	}
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

func childSafeToKill(loomRoot string, a *Agent) bool {
	// Check pane is idle (last non-empty line looks like a shell prompt)
	if a.TmuxTarget != "" {
		output, err := tmux.CapturePane(a.TmuxTarget)
		if err == nil && !paneIsIdle(output) {
			return false
		}
	}
	// Check worktree is clean
	if a.WorktreeName != "" {
		wtPath := filepath.Join(loomRoot, "worktrees", a.WorktreeName)
		stats, err := worktree.DiffStatsFor(wtPath)
		if err == nil && stats.FilesChanged > 0 {
			return false
		}
	}
	return true
}

func paneIsIdle(output string) bool {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return strings.HasSuffix(line, "$") || strings.HasSuffix(line, "%") || strings.HasSuffix(line, "#")
		}
	}
	return true // empty pane is idle
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
		"AgentID":        agent.ID,
		"Role":           agent.Role,
		"SpawnedBy":      agent.SpawnedBy,
		"AssignedIssues": strings.Join(agent.AssignedIssues, ", "),
		"WorktreePath":   wtPath,
		"WorktreeBranch": wtBranch,
		"MCPEnabled":     agent.Config.MCPEnabled,
		"ProjectRoot":    projectRoot,
		"LoomRoot":       loomRoot,
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

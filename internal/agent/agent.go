package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/karanagi/loom/internal/config"
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

func Spawn(loomRoot string, opts SpawnOpts) (*Agent, error) {
	id := NextID(loomRoot, opts.Role)

	cfg, err := config.Load(loomRoot)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	target, err := tmux.NewWindow(cfg.Tmux.SessionName, id)
	if err != nil {
		return nil, fmt.Errorf("creating tmux window: %w", err)
	}

	now := time.Now()
	agent := &Agent{
		ID:             id,
		Role:           opts.Role,
		Status:         "spawning",
		TmuxTarget:     target,
		SpawnedBy:      opts.SpawnedBy,
		SpawnedAt:      now,
		Heartbeat:      now,
		AssignedIssues: opts.AssignedIssues,
		Config: AgentConfig{
			KiroMode:   cfg.Kiro.DefaultMode,
			MCPEnabled: cfg.MCP.Enabled,
		},
	}

	if err := Register(loomRoot, agent); err != nil {
		tmux.KillWindow(target)
		return nil, fmt.Errorf("registering agent: %w", err)
	}

	if opts.Role == "builder" && len(opts.AssignedIssues) > 0 {
		issueID := opts.AssignedIssues[0]
		slug := opts.IssueSlug
		if slug == "" {
			slug = "work"
		}
		wt, err := worktree.Create(loomRoot, issueID, slug, id)
		if err != nil {
			Deregister(loomRoot, id)
			tmux.KillWindow(target)
			return nil, fmt.Errorf("creating worktree: %w", err)
		}
		agent.WorktreeName = wt.Name
		Save(loomRoot, agent)
	}

	kiroCmd := cfg.Kiro.Command + " " + cfg.Kiro.DefaultMode
	if err := tmux.RunInPane(target, kiroCmd); err != nil {
		Kill(loomRoot, id, true)
		return nil, fmt.Errorf("starting kiro: %w", err)
	}

	time.Sleep(3 * time.Second)

	prompt, err := RenderPrompt(loomRoot, agent, opts.ExtraContext)
	if err == nil && prompt != "" {
		tmux.RunInPane(target, prompt)
	}

	agent.Status = "active"
	Save(loomRoot, agent)

	return agent, nil
}

func Kill(loomRoot, id string, cleanupWorktree bool) error {
	a, err := Load(loomRoot, id)
	if err != nil {
		return err
	}
	if a.TmuxTarget != "" {
		tmux.KillWindow(a.TmuxTarget)
	}
	if cleanupWorktree && a.WorktreeName != "" {
		worktree.Remove(loomRoot, a.WorktreeName)
	}
	return Deregister(loomRoot, id)
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

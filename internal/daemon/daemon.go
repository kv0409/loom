package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/tmux"
)

type Daemon struct {
	LoomRoot   string
	Config     *config.Config
	stop       chan struct{}
	done       chan struct{}
	mu         sync.Mutex
	acpClients map[string]*acp.Client
}

func New(loomRoot string, cfg *config.Config) *Daemon {
	return &Daemon{
		LoomRoot:   loomRoot,
		Config:     cfg,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		acpClients: make(map[string]*acp.Client),
	}
}

// notify delivers a message to an agent. ACP agents receive a session/prompt;
// chat agents receive tmux send-keys.
func (d *Daemon) notify(a *agent.Agent, msg string) {
	if a.Config.KiroMode == "acp" {
		d.mu.Lock()
		c := d.acpClients[a.ID]
		d.mu.Unlock()
		if c != nil && a.ACPSessionID != "" {
			c.SendPrompt(a.ACPSessionID, msg)
		}
		return
	}
	if a.TmuxTarget != "" {
		tmux.RunInPane(a.TmuxTarget, msg)
	}
}

// isAlive checks whether an agent's backing process is still running.
func (d *Daemon) isAlive(a *agent.Agent) bool {
	if a.Config.KiroMode == "acp" {
		d.mu.Lock()
		c := d.acpClients[a.ID]
		d.mu.Unlock()
		return c != nil && !c.Exited()
	}
	_, err := tmux.CapturePane(a.TmuxTarget)
	return err == nil
}

func (d *Daemon) Start() error {
	var wg sync.WaitGroup
	wg.Add(6)
	go func() { defer wg.Done(); d.watchIssues() }()
	go func() { defer wg.Done(); d.watchMail() }()
	go func() { defer wg.Done(); d.watchHeartbeats() }()
	go func() { defer wg.Done(); d.watchDoneIssues() }()
	go func() { defer wg.Done(); d.watchInboxGC() }()
	go func() { defer wg.Done(); d.watchPendingAgents() }()
	go func() { wg.Wait(); close(d.done) }()
	return nil
}

func (d *Daemon) Stop() {
	close(d.stop)
	<-d.done
	d.mu.Lock()
	for id, c := range d.acpClients {
		c.Close()
		delete(d.acpClients, id)
	}
	d.mu.Unlock()
}

// RegisterACPClient associates an ACP client with an agent ID.
func (d *Daemon) RegisterACPClient(agentID string, c *acp.Client) {
	d.mu.Lock()
	d.acpClients[agentID] = c
	d.mu.Unlock()
}

// UnregisterACPClient removes and closes an ACP client for the given agent.
func (d *Daemon) UnregisterACPClient(agentID string) {
	d.mu.Lock()
	if c, ok := d.acpClients[agentID]; ok {
		c.Close()
		delete(d.acpClients, agentID)
	}
	d.mu.Unlock()
}

func (d *Daemon) watchPendingAgents() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			agents, err := agent.List(d.LoomRoot)
			if err != nil {
				continue
			}
			for _, a := range agents {
				if a.Status == "pending-acp" {
					d.activateACPAgent(a)
				}
			}
		}
	}
}

func (d *Daemon) activateACPAgent(a *agent.Agent) {
	projectRoot := filepath.Dir(d.LoomRoot)

	env := append(os.Environ(),
		"LOOM_AGENT_ID="+a.ID,
		"LOOM_ROOT="+d.LoomRoot,
		"LOOM_PROJECT_ROOT="+projectRoot,
		"LOOM_ROLE="+a.Role,
	)
	if a.SpawnedBy != "" {
		env = append(env, "LOOM_PARENT_AGENT="+a.SpawnedBy)
	}
	if a.WorktreeName != "" {
		env = append(env, "LOOM_WORKTREE="+filepath.Join(d.LoomRoot, "worktrees", a.WorktreeName))
	}

	extraArgs := []string{"--agent", "loom-" + a.Role}

	workDir := projectRoot
	if a.Role == "builder" && a.WorktreeName != "" {
		workDir = filepath.Join(d.LoomRoot, "worktrees", a.WorktreeName)
	}

	c, err := acp.NewClient(d.Config.Kiro.Command, workDir, env, extraArgs...)
	if err != nil {
		a.Status = "dead"
		agent.Save(d.LoomRoot, a)
		return
	}

	if _, err := c.Initialize(); err != nil {
		c.Close()
		a.Status = "dead"
		agent.Save(d.LoomRoot, a)
		return
	}

	sessionID, err := c.NewSession()
	if err != nil {
		c.Close()
		a.Status = "dead"
		agent.Save(d.LoomRoot, a)
		return
	}

	if a.InitialTask != "" {
		if _, err := c.SendPrompt(sessionID, a.InitialTask); err != nil {
			c.Close()
			a.Status = "dead"
			agent.Save(d.LoomRoot, a)
			return
		}
	}

	d.RegisterACPClient(a.ID, c)
	a.PID = c.PID()
	a.ACPSessionID = sessionID
	a.Status = "active"
	agent.Save(d.LoomRoot, a)
}

func (d *Daemon) watchIssues() {
	// Seed with existing issues so we only notify about NEW ones
	notified := make(map[string]bool)
	existing, _ := issue.List(d.LoomRoot, issue.ListOpts{All: true})
	for _, iss := range existing {
		notified[iss.ID] = true
	}
	ticker := time.NewTicker(time.Duration(d.Config.Polling.IssueIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			issues, err := issue.List(d.LoomRoot, issue.ListOpts{Status: "open"})
			if err != nil {
				continue
			}
			for _, iss := range issues {
				if iss.Assignee != "" || notified[iss.ID] {
					continue
				}
				notified[iss.ID] = true
				msg := "[LOOM] New issue " + iss.ID + ": " + iss.Title + ". Run: loom issue show " + iss.ID
				orch, err := agent.Load(d.LoomRoot, "orchestrator")
				if err != nil {
					continue
				}
				d.notify(orch, msg)
			}
		}
	}
}

func (d *Daemon) watchDoneIssues() {
	// Track last-known status for each issue to detect transitions to "done".
	statusCache := make(map[string]string)
	existing, _ := issue.List(d.LoomRoot, issue.ListOpts{All: true})
	for _, iss := range existing {
		statusCache[iss.ID] = iss.Status
	}
	ticker := time.NewTicker(time.Duration(d.Config.Polling.IssueIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			issues, err := issue.List(d.LoomRoot, issue.ListOpts{All: true})
			if err != nil {
				continue
			}
			for _, iss := range issues {
				prev := statusCache[iss.ID]
				statusCache[iss.ID] = iss.Status
				if iss.Status != "done" || prev == "done" {
					continue
				}
				// Issue just transitioned to done — kill assigned agents.
				agents, err := agent.List(d.LoomRoot)
				if err != nil {
					continue
				}
				for _, a := range agents {
					for _, aid := range a.AssignedIssues {
						if aid == iss.ID {
							agent.Kill(d.LoomRoot, a.ID, true)
							break
						}
					}
				}
			}
		}
	}
}

func (d *Daemon) watchMail() {
	const renotifyInterval = 30 * time.Second
	notifiedAt := make(map[string]time.Time)
	ticker := time.NewTicker(time.Duration(d.Config.Polling.MailIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			inboxRoot := filepath.Join(d.LoomRoot, "mail", "inbox")
			entries, err := os.ReadDir(inboxRoot)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				agentID := e.Name()
				msgs, err := mail.Read(d.LoomRoot, agentID, true)
				if err != nil {
					continue
				}
				if len(msgs) == 0 {
					continue
				}
				a, err := agent.Load(d.LoomRoot, agentID)
				if err != nil {
					continue
				}
				for _, m := range msgs {
					if t, ok := notifiedAt[m.ID]; ok && time.Since(t) < renotifyInterval {
						continue
					}
					notifiedAt[m.ID] = time.Now()
					msg := "[LOOM] New mail from " + m.From + ": " + m.Subject + ". Run: loom mail read"
					d.notify(a, msg)
				}
			}
		}
	}
}

func (d *Daemon) watchHeartbeats() {
	ticker := time.NewTicker(time.Duration(d.Config.Polling.HeartbeatIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			agents, err := agent.List(d.LoomRoot)
			if err != nil {
				continue
			}
			for _, a := range agents {
				if a.Status == "dead" || a.Status == "done" {
					continue
				}
				if d.isAlive(a) {
					continue
				}
				if a.Config.KiroMode == "acp" {
					d.UnregisterACPClient(a.ID)
				}
				a.Status = "dead"
				agent.Save(d.LoomRoot, a)
				parentID := a.SpawnedBy
				if parentID == "" {
					continue
				}
				parent, err := agent.Load(d.LoomRoot, parentID)
				if err != nil {
					continue
				}
				msg := "[LOOM] Agent " + a.ID + " is dead"
				d.notify(parent, msg)
			}
		}
	}
}

func (d *Daemon) watchInboxGC() {
	ticker := time.NewTicker(time.Duration(d.Config.Polling.HeartbeatIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			inboxRoot := filepath.Join(d.LoomRoot, "mail", "inbox")
			entries, err := os.ReadDir(inboxRoot)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if _, err := agent.Load(d.LoomRoot, e.Name()); err != nil {
					os.RemoveAll(filepath.Join(inboxRoot, e.Name()))
				}
			}
		}
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

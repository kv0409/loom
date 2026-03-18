package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/worktree"
)

type Daemon struct {
	LoomRoot   string
	Config     *config.Config
	stop       chan struct{}
	done       chan struct{}
	mu         sync.Mutex
	acpClients map[string]*acp.Client
	apiLn      net.Listener
	lastSeen     map[string]time.Time // ephemeral: detect heartbeat changes between ticks
	idleSince    map[string]time.Time // ephemeral: when agent became idle (no active issues)
	loggedAt     map[string]time.Time // rate-limit: last time a log key was emitted
	lastActivity time.Time            // ephemeral: last time any watcher observed activity
}

func New(loomRoot string, cfg *config.Config) *Daemon {
	return &Daemon{
		LoomRoot:     loomRoot,
		Config:       cfg,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		acpClients:   make(map[string]*acp.Client),
		lastSeen:     make(map[string]time.Time),
		idleSince:    make(map[string]time.Time),
		loggedAt:     make(map[string]time.Time),
		lastActivity: time.Now(),
	}
}

// rlog logs a message at most once per minute for the given key.
func (d *Daemon) rlog(key, format string, args ...any) {
	d.mu.Lock()
	last := d.loggedAt[key]
	if time.Since(last) < time.Minute {
		d.mu.Unlock()
		return
	}
	d.loggedAt[key] = time.Now()
	d.mu.Unlock()
	log.Printf(format, args...)
}

// touchActivity records that something meaningful happened, resetting the
// idle-shutdown timer.
func (d *Daemon) touchActivity() {
	d.mu.Lock()
	d.lastActivity = time.Now()
	d.mu.Unlock()
}

// NotifyOutcome describes the result of a notification attempt.
type NotifyOutcome string

const (
	NotifyDelivered NotifyOutcome = "delivered"
	NotifySkipped   NotifyOutcome = "skipped"
	NotifyFailed    NotifyOutcome = "failed"
)

// NotifyResult captures the outcome of a notify call.
type NotifyResult struct {
	Outcome NotifyOutcome `json:"outcome"`
	Reason  string        `json:"reason,omitempty"`
}

// notify delivers a message to an agent via ACP session/prompt and reports the outcome.
func (d *Daemon) notify(a *agent.Agent, msg string) NotifyResult {
	if a.ACPSessionID == "" {
		return NotifyResult{Outcome: NotifySkipped, Reason: "no active session"}
	}
	d.mu.Lock()
	c := d.acpClients[a.ID]
	d.mu.Unlock()
	if c == nil {
		return NotifyResult{Outcome: NotifySkipped, Reason: "no ACP client"}
	}
	if err := c.SendPrompt(a.ACPSessionID, msg); err != nil {
		return NotifyResult{Outcome: NotifyFailed, Reason: err.Error()}
	}
	return NotifyResult{Outcome: NotifyDelivered}
}

// logNotify calls notify and logs non-delivered outcomes. Used by internal
// lifecycle paths where the caller is fire-and-forget but should not be silent.
func (d *Daemon) logNotify(a *agent.Agent, msg string) NotifyResult {
	nr := d.notify(a, msg)
	if nr.Outcome != NotifyDelivered {
		log.Printf("[notify] %s to %s: %s (%s)", nr.Outcome, a.ID, nr.Reason, msg)
	}
	return nr
}

// isAlive checks whether an agent's backing process is still running.
func (d *Daemon) isAlive(a *agent.Agent) bool {
	d.mu.Lock()
	c := d.acpClients[a.ID]
	d.mu.Unlock()
	return c != nil && !c.Exited()
}

func (d *Daemon) Start() error {
	if err := d.startAPI(); err != nil {
		return fmt.Errorf("starting API: %w", err)
	}
	var wg sync.WaitGroup
	wg.Add(9)
	go func() { defer wg.Done(); d.watchIssues() }()
	go func() { defer wg.Done(); d.watchMail() }()
	go func() { defer wg.Done(); d.watchHeartbeats() }()
	go func() { defer wg.Done(); d.watchInboxGC() }()
	go func() { defer wg.Done(); d.watchPendingAgents() }()
	go func() { defer wg.Done(); d.watchACPOutput() }()
	go func() { defer wg.Done(); d.watchDoneIssues() }()
	go func() { defer wg.Done(); d.watchWorktreeGC() }()
	go func() { defer wg.Done(); d.watchIdleShutdown() }()
	go func() { wg.Wait(); close(d.done) }()
	return nil
}

func (d *Daemon) Stop() {
	close(d.stop)
	<-d.done
	d.stopAPI()
	d.mu.Lock()
	for id, c := range d.acpClients {
		c.Close()
		delete(d.acpClients, id)
	}
	d.mu.Unlock()
}

// Reload stops all watcher goroutines, reloads config from disk, then restarts
// the watchers. The acpClients map is preserved across the reload.
func (d *Daemon) Reload() error {
	log.Println("[daemon] reload: stopping goroutines")
	close(d.stop)
	<-d.done
	d.stopAPI()

	cfg, err := config.Load(d.LoomRoot)
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	d.Config = cfg

	// Fresh channels.
	d.stop = make(chan struct{})
	d.done = make(chan struct{})
	d.lastSeen = make(map[string]time.Time)
	d.idleSince = make(map[string]time.Time)
	d.loggedAt = make(map[string]time.Time)
	d.lastActivity = time.Now()

	log.Println("[daemon] reload: restarting goroutines")
	return d.Start()
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

// GetACPOutput returns the last n output events for the given agent.
func (d *Daemon) GetACPOutput(agentID string, n int) []acp.ACPEvent {
	d.mu.Lock()
	c := d.acpClients[agentID]
	d.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.RecentOutput(n)
}

func (d *Daemon) watchPendingAgents() {
	ticker := time.NewTicker(time.Duration(d.Config.Polling.PendingAgentsIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			agents, err := agent.List(d.LoomRoot)
			if err != nil {
				d.rlog("watchPendingAgents:list", "[pending-agents] agent.List failed: %v", err)
				continue
			}
			for _, a := range agents {
				if a.Status == "pending-acp" {
					d.touchActivity()
					a.Status = "activating"
					if err := agent.Save(d.LoomRoot, a); err != nil {
						log.Printf("[daemon] save agent %s: %v", a.ID, err)
					}
					go d.activateACPAgent(a)
				}
			}
		}
	}
}

func (d *Daemon) watchACPOutput() {
	ticker := time.NewTicker(time.Duration(d.Config.Polling.ACPOutputIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	// Track how many events we've already seen per agent (index-based dedup).
	lastCount := make(map[string]int)
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.mu.Lock()
			ids := make([]string, 0, len(d.acpClients))
			for id := range d.acpClients {
				ids = append(ids, id)
			}
			d.mu.Unlock()
			for _, id := range ids {
				events := d.GetACPOutput(id, 50)
				if len(events) == 0 {
					continue
				}
				// Only write events we haven't seen yet.
				prev := lastCount[id]
				if len(events) < prev {
					prev = 0 // client was replaced (new session)
				}
				if len(events) <= prev {
					continue
				}
				newEvents := events[prev:]
				lastCount[id] = len(events)

				p := filepath.Join(d.LoomRoot, "agents", id+".output")
				f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					d.rlog("watchACPOutput:open:"+id, "[acp-output] open output file for %s: %v", id, err)
					continue
				}
				ts := time.Now().Format("2006-01-02T15:04:05")
				enc := json.NewEncoder(f)
				for i := range newEvents {
					newEvents[i].Timestamp = ts
					enc.Encode(newEvents[i])
				}
				f.Close()

				// Rotate: keep last 200 non-empty lines (atomic via temp file).
				// Filter empty lines to avoid displacing real NDJSON events and
				// to ensure no mid-JSON-line truncation occurs during rotation.
				if raw, err := os.ReadFile(p); err == nil {
					split := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
					var all []string
					for _, line := range split {
						if line != "" {
							all = append(all, line)
						}
					}
					if len(all) > 200 {
						tmp := p + ".tmp"
						if err := os.WriteFile(tmp, []byte(strings.Join(all[len(all)-200:], "\n")+"\n"), 0644); err == nil {
							os.Rename(tmp, p)
						}
					}
				}
			}
		}
	}
}

func (d *Daemon) activateACPAgent(a *agent.Agent) {
	log.Printf("[acp] activating agent %s (role=%s)", a.ID, a.Role)
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
	if len(a.FileScope) > 0 {
		env = append(env, "LOOM_FILE_SCOPE="+strings.Join(a.FileScope, ","))
	}

	extraArgs := []string{"--agent", "loom-" + a.Role}

	workDir := projectRoot
	if a.Role == "builder" && a.WorktreeName != "" {
		workDir = filepath.Join(d.LoomRoot, "worktrees", a.WorktreeName)
	}

	log.Printf("[acp] %s: creating client cmd=%s workDir=%s args=%v", a.ID, d.Config.Kiro.Command, workDir, extraArgs)
	c, err := acp.NewClient(d.Config.Kiro.Command, workDir, env, extraArgs...)
	if err != nil {
		log.Printf("[acp] %s: NewClient failed: %v", a.ID, err)
		a.Status = "dead"
		if err := agent.Save(d.LoomRoot, a); err != nil {
			log.Printf("[daemon] save agent %s: %v", a.ID, err)
		}
		return
	}

	c.AgentID = a.ID
	deny := d.Config.Deny
	c.OnPermission = func(tool, command string) bool {
		return !deny.IsDenied(tool, command)
	}

	log.Printf("[acp] %s: calling Initialize", a.ID)
	if _, err := c.Initialize(); err != nil {
		log.Printf("[acp] %s: Initialize failed: %v", a.ID, err)
		c.Close()
		a.Status = "dead"
		if err := agent.Save(d.LoomRoot, a); err != nil {
			log.Printf("[daemon] save agent %s: %v", a.ID, err)
		}
		return
	}

	log.Printf("[acp] %s: calling NewSession", a.ID)
	sessionID, err := c.NewSession()
	if err != nil {
		log.Printf("[acp] %s: NewSession failed: %v", a.ID, err)
		c.Close()
		a.Status = "dead"
		if err := agent.Save(d.LoomRoot, a); err != nil {
			log.Printf("[daemon] save agent %s: %v", a.ID, err)
		}
		return
	}
	log.Printf("[acp] %s: session=%s, sending initial task", a.ID, sessionID)

	if a.InitialTask != "" {
		if err := c.SendPrompt(sessionID, a.InitialTask); err != nil {
			log.Printf("[acp] %s: SendPrompt failed: %v", a.ID, err)
			c.Close()
			a.Status = "dead"
			if err := agent.Save(d.LoomRoot, a); err != nil {
				log.Printf("[daemon] save agent %s: %v", a.ID, err)
			}
			return
		}
	}

	d.RegisterACPClient(a.ID, c)

	// Re-load from disk to avoid clobbering concurrent heartbeat updates.
	a, err = agent.Load(d.LoomRoot, a.ID)
	if err != nil {
		log.Printf("[acp] %s: re-load failed: %v", a.ID, err)
		return
	}
	a.PID = c.PID()
	a.ACPSessionID = sessionID
	a.Status = "active"
	if err := agent.Save(d.LoomRoot, a); err != nil {
		log.Printf("[daemon] save agent %s: %v", a.ID, err)
	}

	// Set the model after the agent is fully active so a timeout
	// doesn't block activation or kill the process during startup.
	if a.Config.Model != "" {
		go func() {
			log.Printf("[acp] %s: setting model to %s", a.ID, a.Config.Model)
			if err := c.SetModel(a.ACPSessionID, a.Config.Model); err != nil {
				log.Printf("[acp] %s: SetModel(%s) failed: %v (continuing with default)", a.ID, a.Config.Model, err)
			}
		}()
	}
}

func (d *Daemon) watchIssues() {
	// Track the UpdatedAt timestamp of each issue when it was last notified.
	// If an issue is reopened (e.g. after agent death), its UpdatedAt advances
	// and it becomes eligible for re-notification.
	notifiedAt := make(map[string]time.Time)
	existing, _ := issue.List(d.LoomRoot, issue.ListOpts{All: true})
	for _, iss := range existing {
		// Already assigned/progressed or terminal — seed with current timestamp.
		if iss.Status != "open" || iss.Assignee != "" {
			notifiedAt[iss.ID] = iss.UpdatedAt
			continue
		}
		// Open + unassigned + ready → already eligible, seed to avoid duplicate notify.
		if iss.IsReady(d.LoomRoot) {
			notifiedAt[iss.ID] = iss.UpdatedAt
		}
	}
	ticker := time.NewTicker(time.Duration(d.Config.Polling.IssueIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			issues, err := issue.ListReady(d.LoomRoot)
			if err != nil {
				d.rlog("watchIssues:list", "[issues] issue.ListReady failed: %v", err)
				continue
			}
			for _, iss := range issues {
				if prev, ok := notifiedAt[iss.ID]; ok && !iss.UpdatedAt.After(prev) {
					continue
				}
				notifiedAt[iss.ID] = iss.UpdatedAt
				d.touchActivity()
				msg := "[LOOM] New issue " + iss.ID + ": " + iss.Title + ". Run: loom issue show " + iss.ID
				orch, err := agent.Load(d.LoomRoot, "orchestrator")
				if err != nil {
					d.rlog("watchIssues:orch", "[issues] load orchestrator: %v", err)
					continue
				}
				d.logNotify(orch, msg)
			}
		}
	}
}

// allDescendantsResolved recursively checks that all children (and their
// children, etc.) of the given issue are done or cancelled.
func allDescendantsResolved(loomRoot, issueID string) bool {
	return allDescendantsResolvedVisited(loomRoot, issueID, make(map[string]bool))
}

func allDescendantsResolvedVisited(loomRoot, issueID string, visited map[string]bool) bool {
	if visited[issueID] {
		return false // cycle detected — treat as unresolved
	}
	visited[issueID] = true
	iss, err := issue.Load(loomRoot, issueID)
	if err != nil {
		return false
	}
	for _, childID := range iss.Children {
		child, err := issue.Load(loomRoot, childID)
		if err != nil {
			return false
		}
		if child.Status != "done" && child.Status != "cancelled" {
			return false
		}
		if len(child.Children) > 0 && !allDescendantsResolvedVisited(loomRoot, childID, visited) {
			return false
		}
	}
	return true
}

// watchDoneIssues polls parent issues and auto-closes them when all children
// are done or cancelled. It also notifies agents assigned to resolved issues
// to wrap up, and grace-kills them after 2 minutes if they're still alive.
func (d *Daemon) watchDoneIssues() {
	notifiedAgents := make(map[string]time.Time) // agentID → when notified
	ticker := time.NewTicker(time.Duration(d.Config.Polling.IssueIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			issues, err := issue.List(d.LoomRoot, issue.ListOpts{All: true})
			if err != nil {
				d.rlog("watchDoneIssues:list", "[done-issues] issue.List failed: %v", err)
				continue
			}

			// Build set of resolved issue IDs.
			resolved := make(map[string]bool)
			for _, iss := range issues {
				if iss.Status == "done" || iss.Status == "cancelled" {
					resolved[iss.ID] = true
				}
			}

			// Auto-close parents with all descendants resolved.
			for _, iss := range issues {
				if len(iss.Children) == 0 || iss.Status == "done" || iss.Status == "cancelled" {
					continue
				}
				if !allDescendantsResolved(d.LoomRoot, iss.ID) {
					continue
				}
				d.touchActivity()
				if _, err := issue.Close(d.LoomRoot, iss.ID, "all children resolved"); err != nil {
					d.rlog("watchDoneIssues:close:"+iss.ID, "[done-issues] auto-close %s failed: %v", iss.ID, err)
					continue
				}
				resolved[iss.ID] = true
				msg := "[LOOM] Issue " + iss.ID + " auto-closed: all children resolved."
				target := iss.Assignee
				if target == "" {
					target = "orchestrator"
				}
				a, err := agent.Load(d.LoomRoot, target)
				if err != nil {
					d.rlog("watchDoneIssues:load:"+target, "[done-issues] load agent %s: %v", target, err)
					continue
				}
				d.logNotify(a, msg)
			}

			// Notify agents on resolved issues to wrap up; grace-kill after 2 min.
			agents, err := agent.List(d.LoomRoot)
			if err != nil {
				d.rlog("watchDoneIssues:agents", "[done-issues] agent.List failed: %v", err)
				continue
			}
			for _, a := range agents {
				if a.Status != "active" || a.Role == "orchestrator" {
					continue
				}
				// Only notify/kill if the agent has at least one assigned issue
				// AND all of its assigned issues are resolved.
				if len(a.AssignedIssues) == 0 {
					continue
				}
				allResolved := true
				for _, issID := range a.AssignedIssues {
					if !resolved[issID] {
						allResolved = false
						break
					}
				}
				if !allResolved {
					continue
				}
				if t, ok := notifiedAgents[a.ID]; ok {
					if time.Since(t) > 2*time.Minute {
						log.Printf("[daemon] grace-killing %s: still alive 2m after issue resolved", a.ID)
						agent.KillWithResolved(d.LoomRoot, a.ID, true, resolved)
						delete(notifiedAgents, a.ID)
					}
					continue
				}
				notifiedAgents[a.ID] = time.Now()
				log.Printf("[daemon] notifying %s: assigned issue resolved, wrap up", a.ID)
				d.logNotify(a, "[LOOM] Your assigned issue is resolved. Wrap up any final work and exit.")
			}

			// Clean up tracking for agents that are gone.
			for id := range notifiedAgents {
				if _, err := agent.Load(d.LoomRoot, id); err != nil {
					delete(notifiedAgents, id)
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
				d.rlog("watchMail:readdir", "[mail] ReadDir inbox: %v", err)
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				agentID := e.Name()
				msgs, err := mail.Read(d.LoomRoot, mail.ReadOpts{Agent: agentID, UnreadOnly: true})
				if err != nil {
					d.rlog("watchMail:read:"+agentID, "[mail] Read inbox for %s: %v", agentID, err)
					continue
				}
				if len(msgs) == 0 {
					continue
				}
				d.touchActivity()
				a, err := agent.Load(d.LoomRoot, agentID)
				if err != nil {
					d.rlog("watchMail:load:"+agentID, "[mail] load agent %s: %v", agentID, err)
					continue
				}
				for _, m := range msgs {
					if t, ok := notifiedAt[m.ID]; ok && time.Since(t) < renotifyInterval {
						continue
					}
					notifiedAt[m.ID] = time.Now()
					msg := "[LOOM] New mail from " + m.From + ": " + m.Subject + ". Run: loom mail read"
					d.logNotify(a, msg)
				}
			}
		}
	}
}

func (d *Daemon) watchHeartbeats() {
	timeout := time.Duration(d.Config.Limits.HeartbeatTimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	idleTimeout := time.Duration(d.Config.Limits.IdleTimeoutSeconds) * time.Second
	if idleTimeout == 0 {
		idleTimeout = 600 * time.Second
	}
	ticker := time.NewTicker(time.Duration(d.Config.Polling.HeartbeatIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			agents, err := agent.List(d.LoomRoot)
			if err != nil {
				d.rlog("watchHeartbeats:list", "[heartbeat] agent.List failed: %v", err)
				continue
			}
			for _, a := range agents {
				if a.Status == "dead" || a.Status == "done" || a.Status == "pending-acp" || a.Status == "activating" {
					delete(d.lastSeen, a.ID)
					delete(d.idleSince, a.ID)
					continue
				}
				if !d.isAlive(a) {
					d.mu.Lock()
					_, hasClient := d.acpClients[a.ID]
					d.mu.Unlock()
					log.Printf("[heartbeat] marking %s dead: isAlive=false (hasClient=%v pid=%d)", a.ID, hasClient, a.PID)
					d.UnregisterACPClient(a.ID)
					agent.KillProcess(a)
					// Salvage and clean up worktree before marking dead.
					if a.WorktreeName != "" {
						wtPath := filepath.Join(d.LoomRoot, "worktrees", a.WorktreeName)
						if worktree.HasDirtyFiles(wtPath) {
							worktree.SalvageCommit(wtPath, a.ID)
						}
						if err := worktree.Remove(d.LoomRoot, a.WorktreeName, true); err != nil {
							worktree.ForceRemove(d.LoomRoot, a.WorktreeName)
						}
					}
					a.Status = "dead"
					a.NudgeCount = 0
					if err := agent.Save(d.LoomRoot, a); err != nil {
						log.Printf("[daemon] save agent %s: %v", a.ID, err)
					}
					agent.UnassignAllIssues(d.LoomRoot, a)
					delete(d.lastSeen, a.ID)
					delete(d.idleSince, a.ID)
					parentID := a.SpawnedBy
					if parentID == "" {
						continue
					}
					parent, err := agent.Load(d.LoomRoot, parentID)
					if err != nil {
						continue
					}
					d.logNotify(parent, "[LOOM] Agent "+a.ID+" is dead (worktree cleaned up)")
					continue
				}

				// Agent is alive — check for stale heartbeat.
				prev, tracked := d.lastSeen[a.ID]
				if !tracked {
					d.lastSeen[a.ID] = a.Heartbeat
					continue
				}

				// Heartbeat was refreshed since last check — reset nudge count.
				if a.Heartbeat.After(prev) {
					d.lastSeen[a.ID] = a.Heartbeat
					d.touchActivity()
					if a.NudgeCount > 0 {
						a.NudgeCount = 0
						if err := agent.Save(d.LoomRoot, a); err != nil {
							log.Printf("[daemon] save agent %s: %v", a.ID, err)
						}
					}
					continue
				}

				// Check if heartbeat is stale.
				if time.Since(a.Heartbeat) <= timeout {
					continue
				}

				if a.NudgeCount < 2 {
					// Skip stale-heartbeat nudge for the orchestrator when there
					// are no active (non-terminal) issues — it is legitimately idle.
					if a.Role == "orchestrator" && !d.hasActiveIssues() {
						continue
					}
					d.logNotify(a, "[LOOM] Heartbeat stale — are you stuck? Run loom agent heartbeat to confirm alive.")
					a.NudgeCount++
					a.LastNudge = time.Now()
					if err := agent.Save(d.LoomRoot, a); err != nil {
						log.Printf("[daemon] save agent %s: %v", a.ID, err)
					}
					if a.NudgeCount == 2 {
						if parentID := a.SpawnedBy; parentID != "" {
							parent, err := agent.Load(d.LoomRoot, parentID)
							if err == nil {
								d.logNotify(parent, "[LOOM] Agent "+a.ID+" unresponsive after 2 nudges.")
							}
						}
					}
				}
			}

			// Idle agent timeout: kill active non-orchestrator agents with no
			// active (non-done/cancelled) assigned issues.
			if idleTimeout > 0 {
				d.checkIdleAgents(agents, idleTimeout)
			}
		}
	}
}

// hasActiveIssues returns true if any issue is in the "open" state (i.e.
// awaiting dispatch by the orchestrator). Issues already assigned/in-progress/
// review are being handled by leads/builders, so the orchestrator is idle.
func (d *Daemon) hasActiveIssues() bool {
	ready, err := issue.ListReady(d.LoomRoot)
	if err != nil {
		return true // assume active on error to avoid suppressing nudges
	}
	return len(ready) > 0
}

// checkIdleAgents kills active non-orchestrator agents that have no active
// (non-done/cancelled) assigned issues for longer than idleTimeout.
func (d *Daemon) checkIdleAgents(agents []*agent.Agent, idleTimeout time.Duration) {
	for _, a := range agents {
		if a.Status != "active" || a.Role == "orchestrator" {
			delete(d.idleSince, a.ID)
			continue
		}
		// Check whether the agent has any active (non-terminal) assigned issues.
		hasActive := false
		for _, issID := range a.AssignedIssues {
			iss, err := issue.Load(d.LoomRoot, issID)
			if err != nil {
				continue
			}
			if iss.Status != "done" && iss.Status != "cancelled" && iss.Status != "merged" {
				hasActive = true
				break
			}
		}
		if hasActive {
			delete(d.idleSince, a.ID)
			continue
		}
		// Agent has no active issues — track when it became idle.
		if _, ok := d.idleSince[a.ID]; !ok {
			d.idleSince[a.ID] = time.Now()
			continue
		}
		if time.Since(d.idleSince[a.ID]) < idleTimeout {
			continue
		}
		// Idle timeout exceeded — kill the agent.
		log.Printf("[idle-timeout] killing %s: idle for %v with no active issues", a.ID, time.Since(d.idleSince[a.ID]))
		delete(d.idleSince, a.ID)
		agent.Kill(d.LoomRoot, a.ID, true)
		if parentID := a.SpawnedBy; parentID != "" {
			parent, err := agent.Load(d.LoomRoot, parentID)
			if err == nil {
				d.logNotify(parent, "[LOOM] Agent "+a.ID+" auto-killed: idle with no active issues for "+idleTimeout.String())
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
				d.rlog("watchInboxGC:readdir", "[inbox-gc] ReadDir inbox: %v", err)
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if _, err := agent.Load(d.LoomRoot, e.Name()); err != nil {
					mail.ArchiveAndRemoveInbox(d.LoomRoot, e.Name())
				}
			}
		}
	}
}

func (d *Daemon) watchWorktreeGC() {
	// Initial delay: let agents settle after startup.
	select {
	case <-d.stop:
		return
	case <-time.After(2 * time.Minute):
	}

	ticker := time.NewTicker(time.Duration(d.Config.Polling.WorktreeGCIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	d.runWorktreeGC()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.runWorktreeGC()
		}
	}
}

func (d *Daemon) runWorktreeGC() {
	orphans, err := worktree.Cleanup(d.LoomRoot)
	if err != nil {
		log.Printf("[gc] worktree cleanup list failed: %v", err)
		return
	}
	if len(orphans) == 0 {
		return
	}
	log.Printf("[gc] found %d orphan worktree(s): %v", len(orphans), orphans)

	for _, name := range orphans {
		issueID := worktree.ExtractIssueID(name)

		// Check issue state for safety.
		if issueID != "" {
			iss, err := issue.Load(d.LoomRoot, issueID)
			if err == nil {
				// Active issue — skip.
				if iss.Status != "done" && iss.Status != "cancelled" {
					continue
				}
				// Done but not merged — preserve (work not integrated yet).
				if iss.Status == "done" && !worktree.IsMerged(d.LoomRoot, name) {
					continue
				}
			}
			// err != nil means issue file missing — safe to remove.
		}

		// Check for dirty worktree — salvage if stale (>30 min).
		wtPath := filepath.Join(d.LoomRoot, "worktrees", name)
		if worktree.HasDirtyFiles(wtPath) {
			info, err := os.Stat(wtPath)
			if err != nil {
				continue
			}
			if time.Since(info.ModTime()) < 30*time.Minute {
				log.Printf("[gc] preserving dirty worktree %s: modified %v ago", name, time.Since(info.ModTime()))
				continue
			}
			log.Printf("[gc] salvaging stale dirty worktree %s", name)
			worktree.SalvageCommit(wtPath, "gc")
			if err := worktree.ForceRemove(d.LoomRoot, name); err != nil {
				log.Printf("[gc] failed to force-remove worktree %s: %v", name, err)
			}
			continue
		}

		log.Printf("[gc] removing orphan worktree %s", name)
		if err := worktree.Remove(d.LoomRoot, name, true); err != nil {
			log.Printf("[gc] failed to remove worktree %s: %v", name, err)
		}
	}
}

// watchIdleShutdown monitors lastActivity and triggers graceful shutdown
// (SIGTERM to self) when the daemon has been idle longer than
// Config.Polling.IdleShutdownSeconds. Disabled when the value is 0.
func (d *Daemon) watchIdleShutdown() {
	threshold := time.Duration(d.Config.Polling.IdleShutdownSeconds) * time.Second
	if threshold <= 0 {
		// Disabled — block until stop.
		<-d.stop
		return
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	warned := false
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.mu.Lock()
			idle := time.Since(d.lastActivity)
			d.mu.Unlock()
			if idle < threshold {
				warned = false
				continue
			}
			// Double-check: any non-terminal issues or active agents mean
			// the system is not truly idle.
			if d.hasNonTerminalIssues() || d.hasActiveAgents() {
				d.touchActivity()
				warned = false
				continue
			}
			if !warned {
				log.Printf("[idle-shutdown] daemon idle for %v (threshold %v), will shut down", idle.Truncate(time.Second), threshold)
				warned = true
			}
			log.Printf("[idle-shutdown] shutting down: no activity for %v", idle.Truncate(time.Second))
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGTERM)
			return
		}
	}
}

// hasNonTerminalIssues returns true if any issue is not done/cancelled.
func (d *Daemon) hasNonTerminalIssues() bool {
	issues, err := issue.List(d.LoomRoot, issue.ListOpts{All: true})
	if err != nil {
		return true // assume active on error
	}
	for _, iss := range issues {
		if iss.Status != "done" && iss.Status != "cancelled" {
			return true
		}
	}
	return false
}

// hasActiveAgents returns true if any non-orchestrator agent is active.
func (d *Daemon) hasActiveAgents() bool {
	agents, err := agent.List(d.LoomRoot)
	if err != nil {
		return true // assume active on error
	}
	for _, a := range agents {
		if a.Role == "orchestrator" {
			continue
		}
		if a.Status == "active" || a.Status == "activating" || a.Status == "pending-acp" {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

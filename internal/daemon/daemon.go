package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/tmux"
)

type Daemon struct {
	LoomRoot string
	Config   *config.Config
	stop     chan struct{}
	done     chan struct{}
}

func New(loomRoot string, cfg *config.Config) *Daemon {
	return &Daemon{
		LoomRoot: loomRoot,
		Config:   cfg,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (d *Daemon) Start() error {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); d.watchIssues() }()
	go func() { defer wg.Done(); d.watchMail() }()
	go func() { defer wg.Done(); d.watchHeartbeats() }()
	go func() { wg.Wait(); close(d.done) }()
	return nil
}

func (d *Daemon) Stop() {
	close(d.stop)
	<-d.done
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
				// Look up orchestrator's actual tmux target
				orch, err := agent.Load(d.LoomRoot, "orchestrator")
				if err != nil || orch.TmuxTarget == "" {
					continue
				}
				tmux.RunInPane(orch.TmuxTarget, msg)
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
				if err != nil || a.TmuxTarget == "" {
					continue
				}
				for _, m := range msgs {
					if t, ok := notifiedAt[m.ID]; ok && time.Since(t) < renotifyInterval {
						continue
					}
					notifiedAt[m.ID] = time.Now()
					msg := "[LOOM] New mail from " + m.From + ": " + m.Subject + ". Run: loom mail read"
					tmux.RunInPane(a.TmuxTarget, msg)
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
				// Check if tmux pane is still alive instead of relying on heartbeats
				_, err := tmux.CapturePane(a.TmuxTarget)
				if err != nil {
					// Pane is gone — agent is truly dead
					a.Status = "dead"
					agent.Save(d.LoomRoot, a)
					parentID := a.SpawnedBy
					if parentID == "" {
						continue
					}
					parent, err := agent.Load(d.LoomRoot, parentID)
					if err != nil || parent.TmuxTarget == "" {
						continue
					}
					msg := "[LOOM] Agent " + a.ID + " is dead (tmux pane gone)"
					tmux.RunInPane(parent.TmuxTarget, msg)
				}
			}
		}
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

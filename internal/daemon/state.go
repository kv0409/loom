package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/store"
)

type fileStamp struct {
	modTime time.Time
	size    int64
}

type stateTarget uint8

const (
	stateTargetIssues stateTarget = 1 << iota
	stateTargetAgents
	stateTargetMail
)

const defaultReconcileEvery = 30 * time.Second

type daemonState struct {
	loomRoot       string
	mu             sync.RWMutex
	now            func() time.Time
	reconcileEvery time.Duration
	dirty          stateTarget
	lastIssuesSync time.Time
	lastAgentsSync time.Time
	lastMailSync   time.Time
	issueStamp     map[string]fileStamp
	issues         map[string]*issue.Issue
	agentStamp     map[string]fileStamp
	agents         map[string]*agent.Agent
	mailStamp      map[string]fileStamp
	mailByAgent    map[string]map[string]*mail.Message
}

func newDaemonState(loomRoot string) *daemonState {
	return &daemonState{
		loomRoot:       loomRoot,
		now:            time.Now,
		reconcileEvery: defaultReconcileEvery,
		dirty:          stateTargetIssues | stateTargetAgents | stateTargetMail,
		issueStamp:     make(map[string]fileStamp),
		issues:         make(map[string]*issue.Issue),
		agentStamp:     make(map[string]fileStamp),
		agents:         make(map[string]*agent.Agent),
		mailStamp:      make(map[string]fileStamp),
		mailByAgent:    make(map[string]map[string]*mail.Message),
	}
}

func (s *daemonState) syncIssues() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.shouldSyncLocked(stateTargetIssues, s.lastIssuesSync) {
		return nil
	}

	files, err := listYAMLFilesOrEmpty(filepath.Join(s.loomRoot, "issues"))
	if err != nil {
		return fmt.Errorf("listing issues: %w", err)
	}

	seen := make(map[string]bool, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return fmt.Errorf("stat issue %s: %w", filepath.Base(f), err)
		}
		id := strings.TrimSuffix(filepath.Base(f), ".yaml")
		seen[id] = true
		stamp := fileStamp{modTime: info.ModTime(), size: info.Size()}
		if prev, ok := s.issueStamp[id]; ok && prev == stamp {
			continue
		}
		iss := &issue.Issue{}
		if err := store.ReadYAML(f, iss); err != nil {
			return fmt.Errorf("reading issue %s: %w", filepath.Base(f), err)
		}
		if iss.ID == "" {
			return fmt.Errorf("reading issue %s: missing id", filepath.Base(f))
		}
		s.issues[id] = cloneCachedIssue(iss)
		s.issueStamp[id] = stamp
	}

	for id := range s.issues {
		if seen[id] {
			continue
		}
		delete(s.issues, id)
		delete(s.issueStamp, id)
	}
	s.lastIssuesSync = s.now()
	s.dirty &^= stateTargetIssues
	return nil
}

func (s *daemonState) syncAgents() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.shouldSyncLocked(stateTargetAgents, s.lastAgentsSync) {
		return nil
	}

	files, err := listYAMLFilesOrEmpty(filepath.Join(s.loomRoot, "agents"))
	if err != nil {
		return fmt.Errorf("listing agents: %w", err)
	}

	seen := make(map[string]bool, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return fmt.Errorf("stat agent %s: %w", filepath.Base(f), err)
		}
		id := strings.TrimSuffix(filepath.Base(f), ".yaml")
		seen[id] = true
		stamp := fileStamp{modTime: info.ModTime(), size: info.Size()}
		if prev, ok := s.agentStamp[id]; ok && prev == stamp {
			continue
		}
		a := &agent.Agent{}
		if err := store.ReadYAML(f, a); err != nil {
			return fmt.Errorf("reading agent %s: %w", filepath.Base(f), err)
		}
		if a.ID == "" {
			return fmt.Errorf("reading agent %s: missing id", filepath.Base(f))
		}
		s.agents[id] = cloneCachedAgent(a)
		s.agentStamp[id] = stamp
	}

	for id := range s.agents {
		if seen[id] {
			continue
		}
		delete(s.agents, id)
		delete(s.agentStamp, id)
	}
	s.lastAgentsSync = s.now()
	s.dirty &^= stateTargetAgents
	return nil
}

func (s *daemonState) syncMail() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.shouldSyncLocked(stateTargetMail, s.lastMailSync) {
		return nil
	}

	inboxRoot := filepath.Join(s.loomRoot, "mail", "inbox")
	entries, err := os.ReadDir(inboxRoot)
	if err != nil {
		if os.IsNotExist(err) {
			s.mailStamp = make(map[string]fileStamp)
			s.mailByAgent = make(map[string]map[string]*mail.Message)
			s.lastMailSync = s.now()
			s.dirty &^= stateTargetMail
			return nil
		}
		return fmt.Errorf("reading inbox root: %w", err)
	}

	seen := make(map[string]bool)
	seenAgents := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		agentID := e.Name()
		seenAgents[agentID] = true
		files, err := listYAMLFilesOrEmpty(filepath.Join(inboxRoot, agentID))
		if err != nil {
			return fmt.Errorf("listing inbox for %s: %w", agentID, err)
		}
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				return fmt.Errorf("stat mail %s: %w", filepath.Base(f), err)
			}
			msgID := strings.TrimSuffix(filepath.Base(f), ".yaml")
			key := agentID + "/" + msgID
			seen[key] = true
			stamp := fileStamp{modTime: info.ModTime(), size: info.Size()}
			if prev, ok := s.mailStamp[key]; ok && prev == stamp {
				continue
			}
			msg := &mail.Message{}
			if err := store.ReadYAML(f, msg); err != nil {
				return fmt.Errorf("reading mail %s: %w", filepath.Base(f), err)
			}
			if msg.ID == "" {
				return fmt.Errorf("reading mail %s: missing id", filepath.Base(f))
			}
			if s.mailByAgent[agentID] == nil {
				s.mailByAgent[agentID] = make(map[string]*mail.Message)
			}
			s.mailByAgent[agentID][msgID] = cloneCachedMessage(msg)
			s.mailStamp[key] = stamp
		}
	}

	for key := range s.mailStamp {
		if seen[key] {
			continue
		}
		delete(s.mailStamp, key)
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		agentID, msgID := parts[0], parts[1]
		if bucket := s.mailByAgent[agentID]; bucket != nil {
			delete(bucket, msgID)
			if len(bucket) == 0 {
				delete(s.mailByAgent, agentID)
			}
		}
	}

	for agentID := range s.mailByAgent {
		if seenAgents[agentID] {
			continue
		}
		delete(s.mailByAgent, agentID)
	}
	s.lastMailSync = s.now()
	s.dirty &^= stateTargetMail
	return nil
}

func (s *daemonState) invalidate(targets ...stateTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(targets) == 0 {
		s.dirty = stateTargetIssues | stateTargetAgents | stateTargetMail
		return
	}
	for _, target := range targets {
		s.dirty |= target
	}
}

func (s *daemonState) shouldSyncLocked(target stateTarget, lastSync time.Time) bool {
	if s.dirty&target != 0 {
		return true
	}
	if lastSync.IsZero() {
		return true
	}
	if s.reconcileEvery <= 0 {
		return false
	}
	return s.now().Sub(lastSync) >= s.reconcileEvery
}

func (s *daemonState) allIssues() []*issue.Issue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*issue.Issue, 0, len(s.issues))
	for _, iss := range s.issues {
		out = append(out, cloneCachedIssue(iss))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *daemonState) readyIssues() []*issue.Issue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*issue.Issue
	for _, iss := range s.issues {
		if iss.Status != "open" || iss.Assignee != "" {
			continue
		}
		if !issueReadyFromCache(s.issues, iss) {
			continue
		}
		out = append(out, cloneCachedIssue(iss))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *daemonState) resolvedIssueSet() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]bool, len(s.issues))
	for _, iss := range s.issues {
		if iss.Status == "done" || iss.Status == "cancelled" {
			out[iss.ID] = true
		}
	}
	return out
}

func (s *daemonState) allDescendantsResolved(issueID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return descendantsResolvedFromCache(s.issues, issueID, make(map[string]bool))
}

func (s *daemonState) agentsList() []*agent.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*agent.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, cloneCachedAgent(a))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SpawnedAt.After(out[j].SpawnedAt)
	})
	return out
}

func (s *daemonState) agentByID(id string) *agent.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if a := s.agents[id]; a != nil {
		return cloneCachedAgent(a)
	}
	return nil
}

func (s *daemonState) issueByID(id string) *issue.Issue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if iss := s.issues[id]; iss != nil {
		return cloneCachedIssue(iss)
	}
	return nil
}

func (s *daemonState) mailAgentIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.mailByAgent))
	for agentID := range s.mailByAgent {
		ids = append(ids, agentID)
	}
	sort.Strings(ids)
	return ids
}

func (s *daemonState) unreadMessages(agentID string) []*mail.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*mail.Message
	for _, msg := range s.mailByAgent[agentID] {
		if msg.Read {
			continue
		}
		out = append(out, cloneCachedMessage(msg))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out
}

func (s *daemonState) storeIssue(iss *issue.Issue) error {
	info, err := os.Stat(filepath.Join(s.loomRoot, "issues", iss.ID+".yaml"))
	if err != nil {
		return fmt.Errorf("stat issue %s: %w", iss.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.issues[iss.ID] = cloneCachedIssue(iss)
	s.issueStamp[iss.ID] = fileStamp{modTime: info.ModTime(), size: info.Size()}
	s.lastIssuesSync = s.now()
	s.dirty &^= stateTargetIssues
	return nil
}

func (s *daemonState) storeAgent(a *agent.Agent) error {
	info, err := os.Stat(filepath.Join(s.loomRoot, "agents", a.ID+".yaml"))
	if err != nil {
		return fmt.Errorf("stat agent %s: %w", a.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[a.ID] = cloneCachedAgent(a)
	s.agentStamp[a.ID] = fileStamp{modTime: info.ModTime(), size: info.Size()}
	s.lastAgentsSync = s.now()
	s.dirty &^= stateTargetAgents
	return nil
}

func (s *daemonState) refreshIssue(id string) error {
	path := filepath.Join(s.loomRoot, "issues", id+".yaml")
	iss := &issue.Issue{}
	if err := store.ReadYAML(path, iss); err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.issues, id)
			delete(s.issueStamp, id)
			s.lastIssuesSync = s.now()
			s.dirty &^= stateTargetIssues
			return nil
		}
		return fmt.Errorf("reading issue %s: %w", id, err)
	}
	if iss.ID == "" {
		return fmt.Errorf("reading issue %s: missing id", id)
	}
	return s.storeIssue(iss)
}

func (s *daemonState) refreshAgent(id string) error {
	a, err := agent.Load(s.loomRoot, id)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.agents, id)
			delete(s.agentStamp, id)
			s.lastAgentsSync = s.now()
			s.dirty &^= stateTargetAgents
			return nil
		}
		return fmt.Errorf("reading agent %s: %w", id, err)
	}
	if a.ID == "" {
		return fmt.Errorf("reading agent %s: missing id", id)
	}
	return s.storeAgent(a)
}

func (s *daemonState) refreshMailbox(agentID string) error {
	files, err := listYAMLFilesOrEmpty(filepath.Join(s.loomRoot, "mail", "inbox", agentID))
	if err != nil {
		return fmt.Errorf("listing inbox for %s: %w", agentID, err)
	}

	bucket := make(map[string]*mail.Message, len(files))
	stamps := make(map[string]fileStamp, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return fmt.Errorf("stat mail %s: %w", filepath.Base(f), err)
		}
		msgID := strings.TrimSuffix(filepath.Base(f), ".yaml")
		msg := &mail.Message{}
		if err := store.ReadYAML(f, msg); err != nil {
			return fmt.Errorf("reading mail %s: %w", filepath.Base(f), err)
		}
		if msg.ID == "" {
			return fmt.Errorf("reading mail %s: missing id", filepath.Base(f))
		}
		bucket[msgID] = cloneCachedMessage(msg)
		stamps[msgID] = fileStamp{modTime: info.ModTime(), size: info.Size()}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := agentID + "/"
	for key := range s.mailStamp {
		if strings.HasPrefix(key, prefix) {
			delete(s.mailStamp, key)
		}
	}
	if len(bucket) == 0 {
		delete(s.mailByAgent, agentID)
	} else {
		s.mailByAgent[agentID] = bucket
		for msgID, stamp := range stamps {
			s.mailStamp[prefix+msgID] = stamp
		}
	}
	s.lastMailSync = s.now()
	s.dirty &^= stateTargetMail
	return nil
}

func listYAMLFilesOrEmpty(dir string) ([]string, error) {
	files, err := store.ListYAMLFiles(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return files, nil
}

func issueReadyFromCache(issues map[string]*issue.Issue, iss *issue.Issue) bool {
	for _, depID := range iss.DependsOn {
		dep := issues[depID]
		if dep == nil {
			return false
		}
		if dep.Status != "done" && dep.Status != "cancelled" {
			return false
		}
	}
	return true
}

func descendantsResolvedFromCache(issues map[string]*issue.Issue, issueID string, visited map[string]bool) bool {
	if visited[issueID] {
		return false
	}
	visited[issueID] = true
	iss := issues[issueID]
	if iss == nil {
		return false
	}
	for _, childID := range iss.Children {
		child := issues[childID]
		if child == nil {
			return false
		}
		if child.Status != "done" && child.Status != "cancelled" {
			return false
		}
		if len(child.Children) > 0 && !descendantsResolvedFromCache(issues, childID, visited) {
			return false
		}
	}
	return true
}

func cloneCachedAgent(a *agent.Agent) *agent.Agent {
	if a == nil {
		return nil
	}
	out := *a
	out.AssignedIssues = append([]string(nil), a.AssignedIssues...)
	out.FileScope = append([]string(nil), a.FileScope...)
	return &out
}

func cloneCachedIssue(iss *issue.Issue) *issue.Issue {
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

func cloneCachedMessage(msg *mail.Message) *mail.Message {
	if msg == nil {
		return nil
	}
	out := *msg
	return &out
}

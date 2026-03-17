package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/tmux"
	"github.com/karanagi/loom/internal/worktree"
)

// FileBackend implements Backend by reading directly from the .loom filesystem.
type FileBackend struct {
	root string
	lr   *logReader
}

// NewFileBackend creates a FileBackend rooted at the given .loom directory.
func NewFileBackend(loomRoot string) *FileBackend {
	return &FileBackend{root: loomRoot, lr: newLogReader(loomRoot)}
}

// Load reads all dashboard data from the filesystem and returns a Snapshot.
func (fb *FileBackend) Load() Snapshot {
	var s Snapshot
	s.Agents, _ = agent.List(fb.root)
	s.Issues, _ = issue.List(fb.root, issue.ListOpts{All: true})
	s.Worktrees, _ = worktree.List(fb.root)
	s.DiffStats = make(map[string]*worktree.DiffStats)
	for _, wt := range s.Worktrees {
		if ds, err := worktree.DiffStatsFor(wt.Path); err == nil {
			s.DiffStats[wt.Name] = ds
		}
	}
	s.Messages, _ = mail.Log(fb.root, mail.LogOpts{})
	s.Memories, _ = memory.List(fb.root, memory.ListOpts{})
	s.Unread = countUnread(fb.root)
	s.Agents, s.AgentTree = sortAgentTree(s.Agents)
	s.Activity = fetchActivity(fb.root, s.Agents)
	s.Logs = fb.lr.read()
	_, err := os.Stat(daemon.SockPath(fb.root))
	s.DaemonOK = err == nil
	return s
}

// toolLabels maps raw tool names to display labels (no lipgloss colors).
var toolLabels = map[string]string{
	"shell": "SHELL", "execute_bash": "SHELL",
	"read": "READ", "fs_read": "READ",
	"write": "WRITE", "fs_write": "WRITE",
	"grep": "FIND", "glob": "FIND",
	"code":    "CODE",
	"use_aws": "AWS", "aws": "AWS",
}

type timedEntry struct {
	ts    string // ISO datetime, HH:MM:SS, or "" for stable sort
	entry ActivityEntry
}

// extractTimestamp returns the timestamp prefix and remaining text from a .tools line.
func extractTimestamp(line string) (ts, rest string) {
	if len(line) >= 19 && line[4] == '-' && line[10] == 'T' && line[13] == ':' && line[16] == ':' {
		return line[:19], strings.TrimSpace(line[19:])
	}
	if len(line) >= 8 && line[2] == ':' && line[5] == ':' {
		return line[:8], strings.TrimSpace(line[8:])
	}
	return "", line
}

// parseToolFields extracts structured fields from a "tool: args" string.
func parseToolFields(rest, projectRoot string) (toolLabel string, detail string) {
	toolName := rest
	args := ""
	if idx := strings.Index(rest, ": "); idx != -1 {
		toolName = rest[:idx]
		args = rest[idx+2:]
	}

	label, ok := toolLabels[toolName]
	if !ok {
		switch {
		case strings.HasPrefix(toolName, "loom"):
			label = "LOOM"
		case strings.HasPrefix(toolName, "@") || strings.Contains(toolName, "/"):
			label = "TOOL"
		default:
			label = "TOOL"
		}
	}

	detail = cleanArgs(args, projectRoot)
	return label, detail
}

// cleanArgs strips project root paths, cd prefixes, and redirects from args.
func cleanArgs(args, projectRoot string) string {
	if projectRoot != "" {
		args = strings.ReplaceAll(args, projectRoot+"/", "")
		args = strings.ReplaceAll(args, projectRoot, ".")
	}
	if idx := strings.Index(args, " && "); idx != -1 {
		if strings.HasPrefix(strings.TrimSpace(args[:idx]), "cd ") {
			args = strings.TrimSpace(args[idx+4:])
		}
	}
	args = strings.TrimSuffix(strings.TrimSpace(args), "2>&1")
	return strings.TrimSpace(args)
}

// summarizeACPContent collapses raw ACP tool summary content into a clean one-liner.
func summarizeACPContent(content string) (toolLabel, detail string) {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "Called ") {
		rest := content[7:]
		if idx := strings.Index(rest, ": "); idx != -1 {
			toolName := rest[:idx]
			args := rest[idx+2:]
			if label, ok := toolLabels[toolName]; ok {
				return label, args
			}
			return "TOOL", args
		}
		return "TOOL", rest
	}

	content = reassembleJSON(content)
	var params map[string]interface{}
	if json.Unmarshal([]byte(content), &params) == nil {
		return summarizeJSONParams(params)
	}

	oneLine := strings.Join(strings.Fields(content), " ")
	const maxLen = 120
	if runes := []rune(oneLine); len(runes) > maxLen {
		oneLine = string(runes[:maxLen-1]) + "…"
	}
	return "TOOL", oneLine
}

// reassembleJSON joins multi-line JSON fragments into a single string.
func reassembleJSON(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}
	var sb strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx := strings.Index(trimmed, "] "); idx != -1 && strings.Contains(trimmed[:idx], "[") {
			trimmed = strings.TrimSpace(trimmed[idx+2:])
		}
		sb.WriteString(trimmed)
	}
	return sb.String()
}

// summarizeJSONParams produces a tool label and detail from parsed JSON tool call params.
func summarizeJSONParams(params map[string]interface{}) (string, string) {
	if p, ok := params["path"]; ok {
		if cmd, ok := params["command"]; ok {
			_ = p
			return "SHELL", fmt.Sprintf("%v", cmd)
		}
		if _, ok := params["content"]; ok {
			return "WRITE", fmt.Sprintf("%v", p)
		}
		return "READ", fmt.Sprintf("%v", p)
	}
	if cmd, ok := params["command"]; ok {
		return "SHELL", fmt.Sprintf("%v", cmd)
	}
	if pat, ok := params["pattern"]; ok {
		if _, ok := params["replacement"]; ok {
			return "FIND", fmt.Sprintf("replace %v", pat)
		}
		return "FIND", fmt.Sprintf("%v", pat)
	}
	if q, ok := params["query"]; ok {
		return "TOOL", fmt.Sprintf("query: %v", q)
	}

	var parts []string
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		if len(parts) >= 3 {
			break
		}
	}
	return "TOOL", strings.Join(parts, " ")
}

// buildEntry constructs an ActivityEntry from a raw .tools line.
func buildEntry(agentID, line, projectRoot string) ActivityEntry {
	ts, rest := extractTimestamp(line)
	tool, detail := parseToolFields(rest, projectRoot)
	return ActivityEntry{
		AgentID: agentID,
		Line:    line,
		Time:    relativeTime(ts),
		Tool:    tool,
		Detail:  detail,
	}
}

// fetchActivity returns all activity entries across agents, sorted chronologically.
func fetchActivity(loomRoot string, agents []*agent.Agent) []ActivityEntry {
	var timed []timedEntry
	projectRoot := filepath.Dir(loomRoot)

	seen := make(map[string]bool, len(agents))
	for _, a := range agents {
		seen[a.ID] = true
	}

	// Scan for orphaned .tools files.
	agentsDir := filepath.Join(loomRoot, "agents")
	if orphans, err := filepath.Glob(filepath.Join(agentsDir, "*.tools")); err == nil {
		for _, p := range orphans {
			id := strings.TrimSuffix(filepath.Base(p), ".tools")
			if seen[id] {
				continue
			}
			if raw, err := os.ReadFile(p); err == nil {
				for _, line := range strings.Split(string(raw), "\n") {
					if t := strings.TrimSpace(line); t != "" {
						ts, _ := extractTimestamp(t)
						timed = append(timed, timedEntry{ts: ts, entry: buildEntry(id, t, projectRoot)})
					}
				}
			}
		}
	}

	for _, a := range agents {
		toolsPath := filepath.Join(loomRoot, "agents", a.ID+".tools")
		if toolsRaw, err := os.ReadFile(toolsPath); err == nil {
			added := false
			for _, line := range strings.Split(string(toolsRaw), "\n") {
				if t := strings.TrimSpace(line); t != "" {
					ts, _ := extractTimestamp(t)
					timed = append(timed, timedEntry{ts: ts, entry: buildEntry(a.ID, t, projectRoot)})
					added = true
				}
			}
			if added {
				continue
			}
		}

		if a.Status == "dead" {
			continue
		}

		// ACP agents: read from .output files.
		if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
			outPath := filepath.Join(loomRoot, "agents", a.ID+".output")
			raw, err := os.ReadFile(outPath)
			if err != nil {
				continue
			}
			events := acp.ReadOutputFile(raw)
			var last *acp.ACPEvent
			for i := range events {
				if events[i].Kind == acp.ToolSummary {
					last = &events[i]
				}
			}
			if last == nil {
				var sb strings.Builder
				for i := range events {
					if events[i].Kind == acp.TokenChunk {
						sb.WriteString(events[i].Content)
					}
				}
				if sb.Len() > 0 {
					combined := acp.ACPEvent{Kind: acp.TokenChunk, Content: sb.String()}
					last = &combined
				}
			}
			if last != nil {
				text := last.Content
				const maxLen = 200
				if runes := []rune(text); len(runes) > maxLen {
					text = "…" + string(runes[len(runes)-(maxLen-1):])
				}
				tool, detail := summarizeACPContent(last.Content)
				ts := last.Timestamp
				timed = append(timed, timedEntry{ts: ts, entry: ActivityEntry{
					AgentID: a.ID,
					Line:    text,
					Time:    relativeTime(ts),
					Tool:    tool,
					Detail:  detail,
				}})
			}
			continue
		}

		// Chat agents: tmux pane scraping.
		out, err := tmux.CapturePane(a.TmuxTarget)
		if err != nil {
			continue
		}
		for _, line := range parseActivityLines(out) {
			timed = append(timed, timedEntry{entry: buildEntry(a.ID, line, projectRoot)})
		}
	}

	sort.SliceStable(timed, func(i, j int) bool {
		return timed[i].ts < timed[j].ts
	})

	entries := make([]ActivityEntry, len(timed))
	for i, te := range timed {
		entries[i] = te.entry
	}
	return entries
}

func parseActivityLines(raw string) []string {
	var results []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isActivityLine(trimmed) {
			results = append(results, trimmed)
		}
	}
	if len(results) > 20 {
		results = results[len(results)-20:]
	}
	return results
}

func isActivityLine(line string) bool {
	markers := []string{
		"⏺", "tool", "Tool", "invoke", "Invoke",
		"execute_bash", "fs_read", "fs_write", "grep", "glob",
		"search_symbols", "lookup_symbols", "pattern_search",
		"[LOOM]", "loom ", "git ", "commit", "Commit",
		"Reading", "Writing", "Creating", "running",
	}
	for _, m := range markers {
		if strings.Contains(line, m) {
			return true
		}
	}
	return false
}

// countUnread counts unread mail messages across all agent inboxes.
func countUnread(loomRoot string) int {
	var count int
	inboxRoot := filepath.Join(loomRoot, "mail", "inbox")
	entries, err := os.ReadDir(inboxRoot)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		msgs, err := mail.Read(loomRoot, mail.ReadOpts{Agent: e.Name(), UnreadOnly: true})
		if err == nil {
			count += len(msgs)
		}
	}
	return count
}

// sortAgentTree sorts agents into a depth-first tree order and returns tree metadata.
func sortAgentTree(agents []*agent.Agent) ([]*agent.Agent, []AgentTreeNode) {
	if len(agents) == 0 {
		return agents, nil
	}

	idSet := map[string]bool{}
	for _, a := range agents {
		idSet[a.ID] = true
	}
	children := map[string][]int{}
	var roots []int
	for i, a := range agents {
		if a.SpawnedBy == "" || !idSet[a.SpawnedBy] {
			roots = append(roots, i)
		} else {
			children[a.SpawnedBy] = append(children[a.SpawnedBy], i)
		}
	}

	sorted := make([]*agent.Agent, 0, len(agents))
	tree := make([]AgentTreeNode, 0, len(agents))

	var walk func(idx, depth int, isLast bool, ancestors []bool)
	walk = func(idx, depth int, isLast bool, ancestors []bool) {
		anc := make([]bool, len(ancestors))
		copy(anc, ancestors)
		sorted = append(sorted, agents[idx])
		tree = append(tree, AgentTreeNode{Depth: depth, IsLast: isLast, Ancestors: anc})
		kids := children[agents[idx].ID]
		nextAnc := append(anc, isLast)
		for j, kid := range kids {
			walk(kid, depth+1, j == len(kids)-1, nextAnc)
		}
	}

	for i, r := range roots {
		walk(r, 0, i == len(roots)-1, nil)
	}

	// Append any agents not reached.
	visited := map[int]bool{}
	for _, a := range sorted {
		for i, orig := range agents {
			if orig == a {
				visited[i] = true
				break
			}
		}
	}
	for i, a := range agents {
		if !visited[i] {
			sorted = append(sorted, a)
			tree = append(tree, AgentTreeNode{})
		}
	}

	return sorted, tree
}

// relativeTime converts a timestamp to a human-friendly relative string.
func relativeTime(ts string) string {
	if ts == "" {
		return ""
	}
	layouts := []string{"2006-01-02T15:04:05", "15:04:05"}
	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, ts)
		if err == nil {
			break
		}
	}
	if err != nil {
		return ts
	}
	// For time-only formats, assume today.
	if t.Year() == 0 {
		now := time.Now()
		t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func (fb *FileBackend) AgentOutput(loomRoot, agentID string) ([]ACPEvent, error) {
	outPath := filepath.Join(loomRoot, "agents", agentID+".output")
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	return acp.ReadOutputFile(raw), nil
}

func (fb *FileBackend) Diff(wtPath string) string {
	base := worktree.DefaultBranch(wtPath)
	cmd := exec.Command("git", "diff", base+"...HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return "(no diff available)"
	}
	if len(out) == 0 {
		return "(no changes)"
	}
	return string(out)
}

func (fb *FileBackend) SendMail(loomRoot string, from, to, subject, body, typ, priority, ref string) error {
	return mail.Send(loomRoot, mail.SendOpts{
		From:     from,
		To:       to,
		Subject:  subject,
		Body:     body,
		Type:     typ,
		Priority: priority,
		Ref:      ref,
	})
}

func (fb *FileBackend) MemorySnippet(e *MemoryEntry) string {
	return memory.Snippet(e)
}

func (fb *FileBackend) MemoryByField(e *MemoryEntry) string {
	return memory.ByField(e)
}

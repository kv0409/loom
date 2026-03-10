package dashboard

import (
	"fmt"
	"strings"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/tmux"
)

type activityEntry struct {
	AgentID string
	Line    string
}

// fetchActivity is called from refresh() (tea.Cmd), not from View().
func fetchActivity(agents []*agent.Agent) []activityEntry {
	var entries []activityEntry
	for _, a := range agents {
		if a.TmuxTarget == "" || a.Status == "dead" {
			continue
		}
		out, err := tmux.CapturePane(a.TmuxTarget)
		if err != nil {
			continue
		}
		for _, line := range parseActivityLines(out) {
			entries = append(entries, activityEntry{AgentID: a.ID, Line: line})
		}
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

func (m Model) renderActivity() string {
	entries := m.data.activity

	if len(entries) == 0 {
		return panel("ACTIVITY", "  (no activity detected)\n", m.width-2)
	}

	content := fmt.Sprintf("  %-16s %s\n", "AGENT", "ACTIVITY")
	content += "  " + strings.Repeat("─", m.width-6) + "\n"

	visible := m.height - 8
	if visible < 5 {
		visible = 5
	}
	start := 0
	if len(entries) > visible {
		start = len(entries) - visible
	}

	for i := start; i < len(entries); i++ {
		e := entries[i]
		agentLabel := idleStyle.Render(fmt.Sprintf("%-16s", e.AgentID))
		content += fmt.Sprintf("  %s %s\n", agentLabel, truncate(e.Line, m.width-22))
	}

	return panel(fmt.Sprintf("ACTIVITY (%d entries)", len(entries)), content, m.width-2)
}

package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/tmux"
)

type activityEntry struct {
	AgentID string
	Line    string
}

// fetchActivity is called from refresh() (tea.Cmd), not from View().
func fetchActivity(loomRoot string, agents []*agent.Agent) []activityEntry {
	var entries []activityEntry
	for _, a := range agents {
		if a.Status == "dead" {
			continue
		}

		// ACP agents: read from .output files
		if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
			outPath := filepath.Join(loomRoot, "agents", a.ID+".output")
			raw, err := os.ReadFile(outPath)
			if err != nil {
				continue
			}
			text := assembleChunks(string(raw))
			if text != "" {
				entries = append(entries, activityEntry{AgentID: a.ID, Line: text})
			}
			continue
		}

		// Chat agents: tmux pane scraping
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

// assembleChunks joins [agent_message_chunk] and [session_update] fragments into readable text.
// maxLen <= 0 means no limit.
func assembleChunks(raw string) string {
	return assembleChunksN(raw, 200)
}

func assembleChunksN(raw string, maxLen int) string {
	var parts []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip known prefixes from the notification handler.
		// Format is "[prefix] content" — remove prefix + the single format space,
		// but preserve any content-leading spaces (they're word boundaries).
		for _, prefix := range []string{"[agent_message_chunk] ", "[session_update] "} {
			if after, ok := strings.CutPrefix(line, prefix); ok {
				line = after
				break
			}
		}
		parts = append(parts, line)
	}
	joined := strings.Join(parts, "")
	if maxLen > 0 && len(joined) > maxLen {
		joined = "…" + joined[len(joined)-(maxLen-1):]
	}
	return joined
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

	// Proportional column widths.
	agentW := max(8, m.width*16/100)
	lineW := max(20, m.width-agentW-6)

	fmtStr := fmt.Sprintf("  %%-%ds %%s", agentW)
	content := fmt.Sprintf(fmtStr+"\n", "AGENT", "RECENT OUTPUT")
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
		line := fmt.Sprintf("  %-*s %s", agentW, truncate(e.AgentID, agentW), truncate(e.Line, lineW))
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		} else if i == m.hoverRow {
			line = hoverStyle.Render(line)
		} else {
			line = idleStyle.Render(line)
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("ACTIVITY (%d agents)", len(entries)), content, m.width-2)
}

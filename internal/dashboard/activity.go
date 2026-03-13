package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/karanagi/loom/internal/acp"
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
			events := acp.ReadOutputFile(raw)
			// Prefer last ToolSummary; fall back to last TokenChunk.
			var last *acp.ACPEvent
			for i := range events {
				if events[i].Kind == acp.ToolSummary {
					last = &events[i]
				}
			}
			if last == nil {
				for i := range events {
					if events[i].Kind == acp.TokenChunk {
						last = &events[i]
					}
				}
			}
			if last != nil {
				text := last.Content
				const maxLen = 200
				runes := []rune(text)
				if len(runes) > maxLen {
					text = "…" + string(runes[len(runes)-(maxLen-1):])
				}
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
	entries := m.filteredActivity()

	avail := availableWidth(m.width)
	agentW := proportionalWidth(avail, 16, 8)
	lineW := max(20, avail-agentW)

	cols := []table.Column{
		{Title: "AGENT", Width: agentW},
		{Title: "RECENT OUTPUT", Width: lineW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(entries), vRows)

	rows := make([]table.Row, 0, end-start)
	for i := start; i < end; i++ {
		e := entries[i]
		rows = append(rows, table.Row{e.AgentID, e.Line})
	}

	var content string
	if len(entries) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No activity detected", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = t.View() + "\n"
	}

	title := fmt.Sprintf("[t] ACTIVITY (%d agents)", len(entries))
	if m.searchQuery != "" {
		title = fmt.Sprintf("[t] ACTIVITY (%d/%d) filter: %s", len(entries), len(m.data.activity), m.searchQuery)
	}
	return panel(title, content, m.width-2)
}

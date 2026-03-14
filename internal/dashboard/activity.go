package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/tmux"
)

type activityEntry struct {
	AgentID string
	Line    string
}

type timedEntry struct {
	ts    string // HH:MM:SS or "" for stable sort
	entry activityEntry
}

// fetchActivity is called from refresh() (tea.Cmd), not from View().
// It returns all lines from .tools files across all agents, sorted chronologically.
// For agents without a .tools file it falls back to .output / tmux scraping (single entry).
func fetchActivity(loomRoot string, agents []*agent.Agent) []activityEntry {
	var timed []timedEntry

	// Build a set of agent IDs already in the list so we can detect orphans.
	seen := make(map[string]bool, len(agents))
	for _, a := range agents {
		seen[a.ID] = true
	}

	// Scan for orphaned .tools files (agents whose YAML was deleted after being killed).
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
						ts := ""
						if len(t) >= 8 && t[2] == ':' && t[5] == ':' {
							ts = t[:8]
						}
						timed = append(timed, timedEntry{ts: ts, entry: activityEntry{AgentID: id, Line: t}})
					}
				}
			}
		}
	}

	for _, a := range agents {
		// Always try .tools file first — it persists even after agent death.
		toolsPath := filepath.Join(loomRoot, "agents", a.ID+".tools")
		if toolsRaw, err := os.ReadFile(toolsPath); err == nil {
			added := false
			for _, line := range strings.Split(string(toolsRaw), "\n") {
				if t := strings.TrimSpace(line); t != "" {
					ts := ""
					if len(t) >= 8 && t[2] == ':' && t[5] == ':' {
						ts = t[:8]
					}
					timed = append(timed, timedEntry{ts: ts, entry: activityEntry{AgentID: a.ID, Line: t}})
					added = true
				}
			}
			if added {
				continue
			}
		}

		// Dead agents have no live pane or output stream — skip fallbacks.
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
			// Prefer last ToolSummary; fall back to concatenation of all TokenChunks.
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
				runes := []rune(text)
				if len(runes) > maxLen {
					text = "…" + string(runes[len(runes)-(maxLen-1):])
				}
				timed = append(timed, timedEntry{entry: activityEntry{AgentID: a.ID, Line: text}})
			}
			continue
		}

		// Chat agents: tmux pane scraping
		out, err := tmux.CapturePane(a.TmuxTarget)
		if err != nil {
			continue
		}
		for _, line := range parseActivityLines(out) {
			timed = append(timed, timedEntry{entry: activityEntry{AgentID: a.ID, Line: line}})
		}
	}

	// Sort by timestamp (stable to preserve original order for equal/empty timestamps).
	sort.SliceStable(timed, func(i, j int) bool {
		return timed[i].ts < timed[j].ts
	})

	entries := make([]activityEntry, len(timed))
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

// toolInfo maps a raw tool name to a display icon, icon color, compact label, and label color.
// labelColor follows the LOOM-083 spec; color is the existing icon badge color.
type toolInfo struct {
	icon       string
	color      lipgloss.Color
	label      string
	labelColor lipgloss.Color
}

var toolMap = map[string]toolInfo{
	"shell":        {"$", colCyan, "SHELL", colBlue},
	"execute_bash": {"$", colCyan, "SHELL", colBlue},
	"read":         {"r", colGreen, "READ", colGreen},
	"fs_read":      {"r", colGreen, "READ", colGreen},
	"write":        {"w", colYellow, "WRITE", colYellow},
	"fs_write":     {"w", colYellow, "WRITE", colYellow},
	"grep":         {"s", colMagenta, "FIND", colCyan},
	"glob":         {"s", colMagenta, "FIND", colCyan},
	"code":         {"<>", colBlue, "CODE", colBlue},
	"use_aws":      {"☁", colOrange, "AWS", colOrange},
	"aws":          {"☁", colOrange, "AWS", colOrange},
}

// formatToolLine parses a .tools line ("HH:MM:SS tool: args") and returns a
// styled string suitable for display in the activity table.
func formatToolLine(line string, width int, projectRoot string) string {
	// Split timestamp from the rest: "HH:MM:SS tool: args"
	timeStr := ""
	rest := line
	if len(line) >= 8 && line[2] == ':' && line[5] == ':' {
		timeStr = line[:8]
		rest = strings.TrimSpace(line[8:])
	}

	// Split "tool: args"
	toolName := rest
	args := ""
	if idx := strings.Index(rest, ": "); idx != -1 {
		toolName = rest[:idx]
		args = rest[idx+2:]
	}

	// Resolve display info
	info, ok := toolMap[toolName]
	if !ok {
		// Loom CLI lines
		if strings.HasPrefix(toolName, "loom") {
			info = toolInfo{"◆", colMagenta, "LOOM", colMagenta}
		} else if strings.HasPrefix(toolName, "@") || strings.Contains(toolName, "/") {
			// MCP tools: @server/tool
			info = toolInfo{"~", colTeal, "TOOL", colGray}
		} else {
			info = toolInfo{"·", colGray, "TOOL", colGray}
		}
	}

	// Clean up args: strip "cd /abs/path &&" prefix, "2>&1" suffix, replace project root
	if projectRoot != "" {
		args = strings.ReplaceAll(args, projectRoot+"/", "")
		args = strings.ReplaceAll(args, projectRoot, ".")
	}
	// Strip "cd /... && " prefix
	if idx := strings.Index(args, " && "); idx != -1 {
		candidate := strings.TrimSpace(args[:idx])
		if strings.HasPrefix(candidate, "cd ") {
			args = strings.TrimSpace(args[idx+4:])
		}
	}
	// Strip trailing " 2>&1"
	args = strings.TrimSuffix(strings.TrimSpace(args), "2>&1")
	args = strings.TrimSpace(args)

	// Render styled parts
	timePart := activityTimeStyle.Render(relativeTime(timeStr))
	label := activityLabelStyle.Foreground(info.labelColor).Render(info.label)
	badge := activityBadgeStyle.Foreground(info.color).Render("[" + info.icon + "] " + toolName)

	// Calculate remaining width for args
	usedW := lipgloss.Width(timePart) + 1 + lipgloss.Width(label) + 1 + lipgloss.Width(badge) + 1
	argW := width - usedW
	if argW < 4 {
		argW = 4
	}
	argPart := truncate(args, argW)

	return timePart + " " + label + " " + badge + " " + argPart
}

func (m Model) renderActivity() string {
	entries := m.filteredActivity()

	avail := availableWidth(m.width)
	const numColsActivity = 2
	avail -= numColsActivity * 2
	agentW := proportionalWidth(avail, 16, 8)
	lineW := max(20, avail-agentW)

	// Derive project root from loomRoot (.loom is inside the project root)
	projectRoot := filepath.Dir(m.loomRoot)

	cols := []table.Column{
		{Title: "AGENT", Width: agentW},
		{Title: "RECENT OUTPUT", Width: lineW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(entries), vRows)

	rows := make([]table.Row, 0, end-start)
	for i := start; i < end; i++ {
		e := entries[i]
		displayLine := formatToolLine(e.Line, lineW, projectRoot)
		agentCol := agentPill(e.AgentID)
		rows = append(rows, table.Row{agentCol, displayLine})
	}

	var content string
	if len(entries) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No activity detected", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = t.View() + "\n"
		if len(entries) > vRows {
			content += fmt.Sprintf("  ... and %d more\n", len(entries)-vRows)
		}
	}

	uniqueAgents := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		uniqueAgents[e.AgentID] = struct{}{}
	}
	title := fmt.Sprintf("[t] ACTIVITY (%d agents)", len(uniqueAgents))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[t] ACTIVITY (%d/%d) filter: %s", len(entries), len(m.data.activity), m.searchTI.Value())
	}
	return panel(title, content, m.width-2)
}

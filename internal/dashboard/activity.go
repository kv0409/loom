package dashboard

import (
	"encoding/json"
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
	Line    string // original raw line (kept for search/filter)
	Time    string // display-ready relative time (e.g. "3s ago")
	Tool    string // tool label (e.g. "SHELL", "READ")
	Detail  string // cleaned-up args / summary
}

type timedEntry struct {
	ts    string // ISO datetime, HH:MM:SS, or "" for stable sort
	entry activityEntry
}

// extractTimestamp returns the timestamp prefix and remaining text from a .tools line.
// Supports ISO "2006-01-02T15:04:05" (19 chars) and legacy "HH:MM:SS" (8 chars).
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

	info, ok := toolMap[toolName]
	if !ok {
		if strings.HasPrefix(toolName, "loom") {
			info = toolInfo{"◆", colMagenta, "LOOM", colMagenta}
		} else if strings.HasPrefix(toolName, "@") || strings.Contains(toolName, "/") {
			info = toolInfo{"~", colTeal, "TOOL", colGray}
		} else {
			info = toolInfo{"·", colGray, "TOOL", colGray}
		}
	}
	toolLabel = info.label

	detail = cleanArgs(args, projectRoot)
	return toolLabel, detail
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

// summarizeACPContent collapses raw ACP tool summary content into a clean
// "tool: args" one-liner. It handles JSON parameter fragments and "Called tool: args" format.
func summarizeACPContent(content string) (toolLabel, detail string) {
	content = strings.TrimSpace(content)

	// Handle "Called tool_name: args" format from ToolSummary events.
	if strings.HasPrefix(content, "Called ") {
		rest := content[7:]
		if idx := strings.Index(rest, ": "); idx != -1 {
			toolName := rest[:idx]
			args := rest[idx+2:]
			if info, ok := toolMap[toolName]; ok {
				return info.label, args
			}
			return "TOOL", args
		}
		return "TOOL", rest
	}

	// Try to parse as JSON object (raw tool call parameters).
	content = reassembleJSON(content)
	var params map[string]interface{}
	if json.Unmarshal([]byte(content), &params) == nil {
		return summarizeJSONParams(params)
	}

	// Fallback: return as-is, single-lined.
	oneLine := strings.Join(strings.Fields(content), " ")
	const maxLen = 120
	if runes := []rune(oneLine); len(runes) > maxLen {
		oneLine = string(runes[:maxLen-1]) + "…"
	}
	return "TOOL", oneLine
}

// reassembleJSON joins multi-line JSON fragments (from raw ACP output) into a single string.
func reassembleJSON(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}
	var sb strings.Builder
	for _, line := range lines {
		// Strip "TOOL [·] " prefix from each line if present.
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
	// Detect tool type from parameter keys.
	if p, ok := params["path"]; ok {
		if cmd, ok := params["command"]; ok {
			// shell/execute_bash with working_dir
			return "SHELL", fmt.Sprintf("%v", cmd)
		}
		// read/write by path
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

	// Generic: show first key=value pairs.
	var parts []string
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		if len(parts) >= 3 {
			break
		}
	}
	return "TOOL", strings.Join(parts, " ")
}

// buildEntry constructs an activityEntry from a raw .tools line.
func buildEntry(agentID, line, projectRoot string) activityEntry {
	ts, rest := extractTimestamp(line)
	tool, detail := parseToolFields(rest, projectRoot)
	return activityEntry{
		AgentID: agentID,
		Line:    line,
		Time:    relativeTime(ts),
		Tool:    tool,
		Detail:  detail,
	}
}

// fetchActivity is called from refresh() (tea.Cmd), not from View().
// It returns all lines from .tools files across all agents, sorted chronologically.
// For agents without a .tools file it falls back to .output / tmux scraping.
func fetchActivity(loomRoot string, agents []*agent.Agent) []activityEntry {
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

		// ACP agents: read from .output files, collapse into single-line summaries.
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
				timed = append(timed, timedEntry{ts: ts, entry: activityEntry{
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

// formatToolLine parses a .tools line ("TIMESTAMP tool: args") and returns a
// styled string suitable for display in the activity table.
// Kept for overview compact rendering where a single styled string is needed.
func formatToolLine(line string, width int, projectRoot string) string {
	timeStr, rest := extractTimestamp(line)
	toolName := rest
	args := ""
	if idx := strings.Index(rest, ": "); idx != -1 {
		toolName = rest[:idx]
		args = rest[idx+2:]
	}

	info, ok := toolMap[toolName]
	if !ok {
		if strings.HasPrefix(toolName, "loom") {
			info = toolInfo{"◆", colMagenta, "LOOM", colMagenta}
		} else if strings.HasPrefix(toolName, "@") || strings.Contains(toolName, "/") {
			info = toolInfo{"~", colTeal, "TOOL", colGray}
		} else {
			info = toolInfo{"·", colGray, "TOOL", colGray}
		}
	}

	args = cleanArgs(args, projectRoot)

	timePart := activityTimeStyle.Render(relativeTime(timeStr))
	label := activityLabelStyle.Foreground(info.labelColor).Render(info.label)
	badge := activityBadgeStyle.Foreground(info.color).Render("[" + info.icon + "] " + toolName)

	usedW := lipgloss.Width(timePart) + 1 + lipgloss.Width(label) + 1 + lipgloss.Width(badge) + 1
	argW := width - usedW
	if argW < 4 {
		argW = 4
	}
	argPart := truncate(args, argW)

	return timePart + " " + label + " " + badge + " " + argPart
}

// resolveToolInfo returns the toolInfo for a given label.
func resolveToolInfo(label string) toolInfo {
	for _, info := range toolMap {
		if info.label == label {
			return info
		}
	}
	switch label {
	case "LOOM":
		return toolInfo{"◆", colMagenta, "LOOM", colMagenta}
	default:
		return toolInfo{"·", colGray, "TOOL", colGray}
	}
}

func (m Model) renderActivity() string {
	entries := m.filteredActivity()

	avail := availableWidth(m.width)
	const numCols = 4
	avail -= numCols * 2 // table cell padding

	agentW := proportionalWidth(avail, 14, 8)
	timeW := proportionalWidth(avail, 10, 7)
	toolW := proportionalWidth(avail, 8, 5)
	detailW := max(10, avail-agentW-timeW-toolW)

	cols := []table.Column{
		{Title: "AGENT", Width: agentW},
		{Title: "TIME", Width: timeW},
		{Title: "TOOL", Width: toolW},
		{Title: "DETAIL", Width: detailW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(entries), vRows)

	rows := make([]table.Row, 0, end-start)
	var replacements [][2]string
	ri := 0
	for i := start; i < end; i++ {
		e := entries[i]
truncAgent := truncate(e.AgentID, agentW-2) // -2 for agentPill Padding(0,1)
		styledAgent := agentPillFor(truncAgent, e.AgentID)

		styledTime := activityTimeStyle.Render(truncate(e.Time, timeW))

		info := resolveToolInfo(e.Tool)
		styledTool := activityLabelStyle.Foreground(info.labelColor).Render(truncate(e.Tool, toolW))

		plainDetail := truncate(e.Detail, detailW)

		phAgent := cellPlaceholder(ri, lipgloss.Width(agentPillPlain(truncAgent)))
		phTime := cellPlaceholder(ri+1, lipgloss.Width(styledTime))
		phTool := cellPlaceholder(ri+2, lipgloss.Width(styledTool))
		rows = append(rows, table.Row{phAgent, phTime, phTool, plainDetail})
		replacements = append(replacements,
			[2]string{phAgent, styledAgent},
			[2]string{phTime, styledTime},
			[2]string{phTool, styledTool},
		)
		ri += 3
	}

	var content string
	if len(entries) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No activity detected", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = styledTableView(t, replacements) + "\n"
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
	return panel(title, content, panelWidth(m.width))
}

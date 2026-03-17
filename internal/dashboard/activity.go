package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

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
		title = fmt.Sprintf("[t] ACTIVITY (%d/%d) filter: %s", len(entries), len(m.data.Activity), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

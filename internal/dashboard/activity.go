package dashboard

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

// toolInfo maps a raw tool name to a display icon, icon color, compact label, and label color.
type toolInfo struct {
	icon       string
	color      color.Color
	label      string
	labelColor color.Color
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
	timeStr, rest := backend.ExtractTimestamp(line)
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

	args = backend.CleanArgs(args, projectRoot)

	timePart := activityTimeStyle.Render(backend.RelativeTime(timeStr))
	icon := activityIconStyle.Foreground(info.color).Render(info.icon)
	label := activityLabelStyle.Foreground(info.labelColor).Render(info.label)

	usedW := lipgloss.Width(timePart) + 1 + lipgloss.Width(icon) + 1 + lipgloss.Width(label) + 1
	argW := width - usedW
	if argW < 4 {
		argW = 4
	}
	argPart := truncate(args, argW)

	return timePart + " " + icon + " " + label + " " + argPart
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
	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(entries), vRows)

	headers := []string{"AGENT", "TIME", "TOOL", "DETAIL"}

	rows := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		e := entries[i]
		info := resolveToolInfo(e.Tool)
		rows = append(rows, []string{
			agentPillFor(e.AgentID, e.AgentID),
			activityTimeStyle.Render(e.Time),
			activityLabelStyle.Foreground(info.labelColor).Render(e.Tool),
			e.Detail,
		})
	}

	var content string
	if len(entries) == 0 {
		t := newLGTable(headers, nil, -1, avail)
		content = t.Render() + "\n" + renderEmpty("No activity detected", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail)
		content = t.Render() + "\n"
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

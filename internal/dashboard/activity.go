package dashboard

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

// toolInfo maps a tool label to a display icon and color.
type toolInfo struct {
	icon  string
	color color.Color
}

// toolMap maps uppercase tool labels (as produced by backend.parseToolFields)
// to a single-cell icon and color.
var toolMap = map[string]toolInfo{
	"SHELL": {"❯", colCyan},
	"READ":  {"←", colGreen},
	"WRITE": {"✎", colYellow},
	"FIND":  {"⌕", colCyan},
	"CODE":  {"◆", colBlue},
	"AWS":   {"☁", colOrange},
	"LOOM":  {"⚙", colMagenta},
}

// rawToolLabel maps raw tool names (as they appear in .tools files) to the
// uppercase labels used as toolMap keys.
var rawToolLabel = map[string]string{
	"shell": "SHELL", "execute_bash": "SHELL",
	"read": "READ", "fs_read": "READ",
	"write": "WRITE", "fs_write": "WRITE",
	"grep": "FIND", "glob": "FIND",
	"code":    "CODE",
	"use_aws": "AWS", "aws": "AWS",
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

	label := "TOOL"
	if l, ok := rawToolLabel[toolName]; ok {
		label = l
	} else if strings.HasPrefix(toolName, "loom") {
		label = "LOOM"
	}
	info := resolveToolInfo(label)

	args = backend.CleanArgs(args, projectRoot)

	timePart := activityTimeStyle.Render(backend.RelativeTime(timeStr))
	icon := activityIconStyle.Foreground(info.color).Render(info.icon)

	usedW := lipgloss.Width(timePart) + 1 + lipgloss.Width(icon) + 1
	argW := width - usedW
	if argW < 4 {
		argW = 4
	}
	argPart := truncate(args, argW)

	return timePart + " " + icon + " " + argPart
}

// resolveToolInfo returns the toolInfo for a given tool label.
func resolveToolInfo(label string) toolInfo {
	if info, ok := toolMap[label]; ok {
		return info
	}
	return toolInfo{"·", colGray}
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
			activityIconStyle.Foreground(info.color).Render(info.icon),
			e.Detail,
		})
	}

	var content string
	if len(entries) == 0 {
		t := newLGTable(headers, nil, -1, avail, nil)
		content = t.Render() + "\n" + renderEmpty("No activity detected", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail, nil)
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

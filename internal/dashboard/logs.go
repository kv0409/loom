package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type logLine struct {
	Agent string
	Text  string
}

// readLogs is called from refresh() (tea.Cmd), not from View().
func readLogs(loomRoot string) []logLine {
	logsDir := filepath.Join(loomRoot, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil
	}

	var lines []logLine
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		agentID := strings.TrimSuffix(e.Name(), ".log")
		data, err := os.ReadFile(filepath.Join(logsDir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				lines = append(lines, logLine{Agent: agentID, Text: trimmed})
			}
		}
	}

	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	return lines
}

func (m Model) renderLogs() string {
	filter := ""
	if m.logFilter > 0 && m.logFilter <= len(m.data.agents) {
		filter = m.data.agents[m.logFilter-1].ID
	}

	filterLabel := "all agents"
	if filter != "" {
		filterLabel = filter
	}
	header := fmt.Sprintf("  Filter: [%s]  (f to cycle)\n", filterLabel)
	header += "  " + strings.Repeat("─", m.width-6) + "\n"

	// Filter from pre-fetched data
	var lines []logLine
	for _, l := range m.data.logs {
		if filter == "" || l.Agent == filter {
			lines = append(lines, l)
		}
	}

	if len(lines) == 0 {
		header += "  (no log files found in .loom/logs/)\n"
		return panel("LOGS", header, m.width-2)
	}

	visible := m.height - 8
	if visible < 5 {
		visible = 5
	}
	start := 0
	if len(lines) > visible {
		start = len(lines) - visible
	}

	content := header
	for i := start; i < len(lines); i++ {
		l := lines[i]
		agentTag := idleStyle.Render(fmt.Sprintf("[%-14s]", l.Agent))
		content += fmt.Sprintf("  %s %s\n", agentTag, truncate(l.Text, m.width-22))
	}

	return panel(fmt.Sprintf("LOGS (%d lines)", len(lines)), content, m.width-2)
}

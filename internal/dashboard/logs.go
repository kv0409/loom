package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type logLine struct {
	Category string
	Text     string
}

// readLogs is called from refresh() (tea.Cmd), not from View().
func readLogs(loomRoot string) []logLine {
	data, err := os.ReadFile(filepath.Join(loomRoot, "logs", "daemon.log"))
	if err != nil {
		return nil
	}

	var lines []logLine
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		cat, text, ok := classifyLogLine(trimmed)
		if !ok {
			continue // skip noise
		}
		lines = append(lines, logLine{Category: cat, Text: text})
	}

	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	return lines
}

// classifyLogLine returns (category, display text, keep).
// Returns keep=false for noisy lines that should be filtered out.
func classifyLogLine(line string) (string, string, bool) {
	// Skip notification spam
	if strings.Contains(line, "[acp-notif]") {
		return "", "", false
	}

	// Agent lifecycle events
	if strings.Contains(line, "[acp] activating agent") {
		return "lifecycle", line, true
	}
	if strings.Contains(line, "calling Initialize") ||
		strings.Contains(line, "calling NewSession") ||
		strings.Contains(line, "sending initial task") {
		return "lifecycle", line, true
	}
	if strings.Contains(line, "process exited") {
		return "lifecycle", line, true
	}

	// Errors
	if strings.Contains(line, "failed") || strings.Contains(line, "Failed") ||
		strings.Contains(line, "error") || strings.Contains(line, "Error") {
		return "error", line, true
	}

	// Stderr from kiro-cli
	if strings.Contains(line, "[acp-stderr]") {
		return "stderr", line, true
	}

	// Warnings
	if strings.Contains(line, "Warning") || strings.Contains(line, "WARNING") {
		return "warn", line, true
	}

	// Session info
	if strings.Contains(line, "session=") {
		return "lifecycle", line, true
	}

	return "", "", false
}

func (m Model) renderLogs() string {
	filter := ""
	categories := []string{"", "lifecycle", "error", "stderr", "warn"}
	if m.logFilter > 0 && m.logFilter < len(categories) {
		filter = categories[m.logFilter]
	}

	filterLabel := "all"
	if filter != "" {
		filterLabel = filter
	}
	header := fmt.Sprintf("  Filter: [%s]  (f to cycle: all → lifecycle → error → stderr → warn)\n", filterLabel)
	header += "  " + strings.Repeat("─", m.width-6) + "\n"

	var lines []logLine
	for _, l := range m.data.logs {
		if filter == "" || l.Category == filter {
			lines = append(lines, l)
		}
	}

	if len(lines) == 0 {
		header += "  (no matching log entries)\n"
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
		tag := categoryTag(l.Category)
		content += fmt.Sprintf("  %s %s\n", tag, truncate(l.Text, m.width-16))
	}

	return panel(fmt.Sprintf("LOGS (%d events)", len(lines)), content, m.width-2)
}

func categoryTag(cat string) string {
	switch cat {
	case "error":
		return deadStyle.Render(fmt.Sprintf("%-10s", "ERROR"))
	case "stderr":
		return idleStyle.Render(fmt.Sprintf("%-10s", "STDERR"))
	case "warn":
		return idleStyle.Render(fmt.Sprintf("%-10s", "WARN"))
	default:
		return activeStyle.Render(fmt.Sprintf("%-10s", cat))
	}
}

package dashboard

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

func (m Model) countLogAgents() int {
	seen := map[string]bool{}
	for _, l := range m.data.Logs {
		if l.Agent != "" {
			seen[l.Agent] = true
		}
	}
	return len(seen)
}

func (m Model) filteredLogLines() []backend.LogLine {
	agentSet := map[string]bool{}
	for _, l := range m.data.Logs {
		if l.Agent != "" {
			agentSet[l.Agent] = true
		}
	}
	agents := make([]string, 0, len(agentSet))
	for a := range agentSet {
		agents = append(agents, a)
	}
	sort.Strings(agents)

	filter := ""
	categories := []string{"", "lifecycle", "error", "stderr", "warn"}
	if m.logFilter > 0 && m.logFilter < len(categories) {
		filter = categories[m.logFilter]
	}

	agentFilter := ""
	if m.logAgentFilter > 0 && m.logAgentFilter <= len(agents) {
		agentFilter = agents[m.logAgentFilter-1]
	}

	var out []backend.LogLine
	for _, l := range m.filteredLogs() {
		if filter != "" && l.Category != filter {
			continue
		}
		if agentFilter != "" && l.Agent != agentFilter {
			continue
		}
		out = append(out, l)
	}
	return out
}

func (m Model) renderLogs() string {
	lines := m.filteredLogLines()

	// Derive filter/agent labels for header display
	agentSet := map[string]bool{}
	for _, l := range m.data.Logs {
		if l.Agent != "" {
			agentSet[l.Agent] = true
		}
	}
	agents := make([]string, 0, len(agentSet))
	for a := range agentSet {
		agents = append(agents, a)
	}
	sort.Strings(agents)

	filter := ""
	categories := []string{"", "lifecycle", "error", "stderr", "warn"}
	if m.logFilter > 0 && m.logFilter < len(categories) {
		filter = categories[m.logFilter]
	}

	agentFilter := ""
	if m.logAgentFilter > 0 && m.logAgentFilter <= len(agents) {
		agentFilter = agents[m.logAgentFilter-1]
	}

	filterLabel := "all"
	if filter != "" {
		filterLabel = filter
	}
	agentLabel := "all"
	if agentFilter != "" {
		agentLabel = agentFilter
	}
	header := fmt.Sprintf("  Filter: [%s]  Agent: [%s]  (f=category, F=agent)\n", filterLabel, agentLabel)
	header += separator(m.width) + "\n"

	if len(lines) == 0 {
		header += renderEmpty("No matching log entries", availableWidth(m.width))
		return panel("[l] INVESTIGATION", header, panelWidth(m.width))
	}

	errorsCount := 0
	warnCount := 0
	hotAgents := map[string]int{}
	for _, line := range lines {
		if line.Category == "error" || line.Category == "stderr" {
			errorsCount++
		}
		if line.Category == "warn" {
			warnCount++
		}
		if line.Agent != "" {
			hotAgents[line.Agent]++
		}
	}
	type agentCount struct {
		agent string
		count int
	}
	var ranked []agentCount
	for agent, count := range hotAgents {
		ranked = append(ranked, agentCount{agent: agent, count: count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].agent < ranked[j].agent
	})

	visible := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(lines), visible)

	tagW := 10
	const numColsLogs = 2
	textW := m.width - tagW - 8 - numColsLogs*2
	if textW < 10 {
		textW = 10
	}
	cols := []table.Column{
		{Title: "CAT", Width: tagW},
		{Title: "MESSAGE", Width: textW},
	}
	rows := make([]table.Row, 0, end-start)
	var replacements [][2]string
	for i := start; i < end; i++ {
		l := lines[i]
		cat := l.Category
		if cat == "" {
			cat = "info"
		}
		styledCat := categoryTag(cat)
		ph := cellPlaceholder(i-start, lipgloss.Width(styledCat))
		rows = append(rows, table.Row{ph, truncate(l.Text, textW)})
		replacements = append(replacements, [2]string{ph, styledCat})
	}
	t := newStyledTable(cols, rows, end-start)

	content := header
	content += fmt.Sprintf("  %d errors · %d warnings · %d events\n", errorsCount, warnCount, len(lines))
	content += "\n  " + headerStyle.Render("HOT AGENTS") + "\n"
	if len(ranked) == 0 {
		content += "  No agent attribution available.\n\n"
	} else {
		for _, item := range ranked[:min(3, len(ranked))] {
			content += fmt.Sprintf("  %s · %d events\n", item.agent, item.count)
		}
		content += "\n"
	}
	content += "  " + headerStyle.Render("RECENT INCIDENTS") + "\n"
	content += styledTableView(t, replacements)
	return panel(fmt.Sprintf("[l] INVESTIGATION (%d events)", len(lines)), content, panelWidth(m.width))
}

func categoryTag(cat string) string {
	switch cat {
	case "error":
		return deadStyle.Render("ERROR")
	case "stderr":
		return idleStyle.Render("STDERR")
	case "warn":
		return idleStyle.Render("WARN")
	default:
		return activeStyle.Render(cat)
	}
}

package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderOverview() string {
	// Agent counts by status
	counts := map[string]int{}
	for _, a := range m.data.agents {
		counts[a.Status]++
	}
	agentSection := headerStyle.Render("AGENTS") + fmt.Sprintf(" (%d total)\n", len(m.data.agents))
	for _, s := range []string{"active", "idle", "blocked", "dead"} {
		if c := counts[s]; c > 0 {
			agentSection += fmt.Sprintf("  %s %d\n", statusStyle(s).Render(fmt.Sprintf("%-8s", s)), c)
		}
	}

	// Issue counts by status
	issueCounts := map[string]int{}
	for _, iss := range m.data.issues {
		issueCounts[iss.Status]++
	}
	issueSection := headerStyle.Render("ISSUES") + fmt.Sprintf(" (%d total)\n", len(m.data.issues))
	for _, s := range []string{"open", "assigned", "in-progress", "blocked", "review", "done"} {
		if c := issueCounts[s]; c > 0 {
			bar := strings.Repeat("█", min(c, 20))
			issueSection += fmt.Sprintf("  %s %s %d\n", statusStyle(s).Render(fmt.Sprintf("%-12s", s)), bar, c)
		}
	}

	// Worktrees
	wtSection := headerStyle.Render("WORKTREES") + fmt.Sprintf(" (%d active)\n", len(m.data.worktrees))
	for _, wt := range m.data.worktrees {
		wtSection += fmt.Sprintf("  %s  %s\n", wt.Name, idleStyle.Render(wt.Agent))
	}

	// Mail + Memory
	mailSection := headerStyle.Render("MAIL") + fmt.Sprintf(" (%d unread)\n", m.data.unread)
	limit := min(len(m.data.messages), 5)
	for _, msg := range m.data.messages[:limit] {
		mailSection += fmt.Sprintf("  %s %s→%s [%s] %s\n",
			idleStyle.Render(msg.Timestamp.Format("15:04")),
			msg.From, msg.To, msg.Type, truncate(msg.Subject, 30))
	}

	memCounts := map[string]int{}
	for _, e := range m.data.memories {
		memCounts[e.Type]++
	}
	memSection := headerStyle.Render("MEMORY") + fmt.Sprintf(" (%d entries)\n", len(m.data.memories))
	var parts []string
	for _, t := range []string{"decision", "discovery", "convention"} {
		if c := memCounts[t]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %ss", c, t))
		}
	}
	if len(parts) > 0 {
		memSection += "  " + strings.Join(parts, " · ") + "\n"
	}

	colW := max((m.width-4)/2, 30)
	left := lipgloss.NewStyle().Width(colW).Render(agentSection + "\n" + wtSection)
	right := lipgloss.NewStyle().Width(colW).Render(issueSection + "\n" + mailSection + "\n" + memSection)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

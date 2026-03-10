package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderOverview() string {
	colW := max((m.width-4)/2, 30)

	// Agent counts by status
	counts := map[string]int{}
	for _, a := range m.data.agents {
		counts[a.Status]++
	}
	agentContent := ""
	for _, s := range []string{"active", "idle", "blocked", "dead"} {
		if c := counts[s]; c > 0 {
			agentContent += fmt.Sprintf("  %s %s %d\n", statusIndicator(s), statusStyle(s).Render(fmt.Sprintf("%-8s", s)), c)
		}
	}
	agentPanel := panel(fmt.Sprintf("AGENTS (%d)", len(m.data.agents)), agentContent, colW)

	// Issue counts by status
	issueCounts := map[string]int{}
	for _, iss := range m.data.issues {
		issueCounts[iss.Status]++
	}
	issueContent := ""
	for _, s := range []string{"open", "assigned", "in-progress", "blocked", "review", "done"} {
		if c := issueCounts[s]; c > 0 {
			bar := strings.Repeat("█", min(c, 20))
			issueContent += fmt.Sprintf("  %s %s %s %d\n", statusIndicator(s), statusStyle(s).Render(fmt.Sprintf("%-12s", s)), bar, c)
		}
	}
	issuePanel := panel(fmt.Sprintf("ISSUES (%d)", len(m.data.issues)), issueContent, colW)

	// Worktrees
	wtContent := ""
	for _, wt := range m.data.worktrees {
		wtContent += fmt.Sprintf("  %s  %s\n", wt.Name, idleStyle.Render(wt.Agent))
	}
	if wtContent == "" {
		wtContent = "  (none)\n"
	}
	wtPanel := panel(fmt.Sprintf("WORKTREES (%d)", len(m.data.worktrees)), wtContent, colW)

	// Mail
	mailContent := ""
	limit := min(len(m.data.messages), 5)
	for _, msg := range m.data.messages[:limit] {
		mailContent += fmt.Sprintf("  %s %s→%s [%s] %s\n",
			idleStyle.Render(msg.Timestamp.Format("15:04")),
			msg.From, msg.To, msg.Type, truncate(msg.Subject, 30))
	}
	if mailContent == "" {
		mailContent = "  (none)\n"
	}
	mailPanel := panel(fmt.Sprintf("MAIL (%d unread)", m.data.unread), mailContent, colW)

	// Memory
	memCounts := map[string]int{}
	for _, e := range m.data.memories {
		memCounts[e.Type]++
	}
	memContent := ""
	var parts []string
	for _, t := range []string{"decision", "discovery", "convention"} {
		if c := memCounts[t]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %ss", c, t))
		}
	}
	if len(parts) > 0 {
		memContent = "  " + strings.Join(parts, " · ") + "\n"
	} else {
		memContent = "  (empty)\n"
	}
	memPanel := panel(fmt.Sprintf("MEMORY (%d)", len(m.data.memories)), memContent, colW)

	left := lipgloss.JoinVertical(lipgloss.Left, agentPanel, wtPanel, memPanel)
	right := lipgloss.JoinVertical(lipgloss.Left, issuePanel, mailPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
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

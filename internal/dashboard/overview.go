package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderOverview() string {
	colW := max((m.width-4)/2, 30)

	// Agent table: ID, role, status, heartbeat ago
	agentContent := ""
	for _, a := range m.data.agents {
		ago := timeAgo(a.Heartbeat)
		agentContent += fmt.Sprintf("  %s %-18s %-12s %s\n",
			statusIndicator(a.Status), truncate(a.ID, 18), a.Role, idleStyle.Render(ago))
	}
	if agentContent == "" {
		agentContent = "  (none)\n"
	}
	agentPanel := panel(fmt.Sprintf("AGENTS (%d)", len(m.data.agents)), agentContent, colW)

	// Issues: non-done, showing ID, title, status, assignee
	issueContent := ""
	issueCount := 0
	for _, iss := range m.data.issues {
		if iss.Status == "done" {
			continue
		}
		issueCount++
		assignee := iss.Assignee
		if assignee == "" {
			assignee = "-"
		}
		issueContent += fmt.Sprintf("  %s %-12s %-20s %s %s\n",
			statusIndicator(iss.Status), iss.ID, truncate(iss.Title, 20),
			statusStyle(iss.Status).Render(fmt.Sprintf("%-11s", iss.Status)), idleStyle.Render(assignee))
	}
	if issueContent == "" {
		issueContent = "  (none)\n"
	}
	issuePanel := panel(fmt.Sprintf("ISSUES (%d open)", issueCount), issueContent, colW)

	// Worktrees with DiffStats
	wtContent := ""
	for _, wt := range m.data.worktrees {
		diffStr := ""
		if ds := m.data.diffStats[wt.Name]; ds != nil && ds.FilesChanged > 0 {
			diffStr = fmt.Sprintf(" %df +%d -%d", ds.FilesChanged, ds.Insertions, ds.Deletions)
		}
		wtContent += fmt.Sprintf("  %s  %-14s %s%s\n",
			truncate(slugFromWorktree(wt.Name), 22), idleStyle.Render(truncate(wt.Agent, 14)),
			idleStyle.Render(truncate(wt.Branch, 20)), activeStyle.Render(diffStr))
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

	// Memory (kept as-is)
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

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

func truncate(s string, n int) string {
	if n <= 3 {
		return "..."
	}
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

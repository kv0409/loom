package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderOverview() string {
	colW := max((m.width-4)/2, 30)
	innerW := colW - 2

	// Agent table: ID, role, heartbeat ago
	// Layout: "  ● ID ROLE AGO" — overhead: 4 fixed + 2 field separators = 6
	aIdW := min(18, max(8, (innerW-6)*3/5))
	aRoleW := max(4, innerW-6-aIdW-4)
	agentContent := ""
	for _, a := range m.data.agents {
		ago := timeAgo(a.Heartbeat)
		agentContent += fmt.Sprintf("  %s %-*s %-*s %s\n",
			statusIndicator(a.Status), aIdW, truncate(a.ID, aIdW),
			aRoleW, truncate(a.Role, aRoleW), idleStyle.Render(ago))
	}
	if agentContent == "" {
		agentContent = "  No agents running. Use loom spawn to start.\n"
	}
	agentPanel := panel(fmt.Sprintf("AGENTS (%d)", len(m.data.agents)), agentContent, colW)

	// Issues: non-done, showing ID, title, status
	// Layout: "  ● ID TITLE STATUS" — overhead: 4 fixed + 2 field separators = 6
	iIdW := min(12, max(6, (innerW-6)/4))
	iStatusW := min(11, max(6, (innerW-6)/5))
	iTitleW := max(6, innerW-6-iIdW-iStatusW)
	issueContent := ""
	issueCount := 0
	for _, iss := range m.data.issues {
		if iss.Status == "done" || iss.Status == "cancelled" {
			continue
		}
		issueCount++
		issueContent += fmt.Sprintf("  %s %-*s %-*s %s\n",
			statusIndicator(iss.Status), iIdW, truncate(iss.ID, iIdW),
			iTitleW, truncate(iss.Title, iTitleW),
			statusStyle(iss.Status).Render(truncate(iss.Status, iStatusW)))
	}
	if issueContent == "" {
		issueContent = "  No open issues. Use loom issue create to add one.\n"
	}
	issuePanel := panel(fmt.Sprintf("ISSUES (%d open)", issueCount), issueContent, colW)

	// Worktrees with DiffStats
	wtSlugW := min(22, max(8, (innerW-4)/3))
	wtBranchW := min(20, max(6, (innerW-4)/3))
	wtContent := ""
	for _, wt := range m.data.worktrees {
		diffStr := ""
		if ds := m.data.diffStats[wt.Name]; ds != nil && ds.FilesChanged > 0 {
			diffStr = fmt.Sprintf(" %df +%d -%d", ds.FilesChanged, ds.Insertions, ds.Deletions)
		}
		wtContent += fmt.Sprintf("  %-*s %s%s\n",
			wtSlugW, truncate(slugFromWorktree(wt.Name), wtSlugW),
			idleStyle.Render(truncate(wt.Branch, wtBranchW)), activeStyle.Render(diffStr))
	}
	if wtContent == "" {
		wtContent = "  No worktrees active. Builders create them automatically.\n"
	}
	wtPanel := panel(fmt.Sprintf("WORKTREES (%d)", len(m.data.worktrees)), wtContent, colW)

	// Mail
	mailContent := ""
	limit := min(len(m.data.messages), 5)
	mailFromW := min(12, max(4, (innerW-10)/4))
	mailSubjW := max(6, innerW-10-mailFromW*2-8)
	for _, msg := range m.data.messages[:limit] {
		mailContent += fmt.Sprintf("  %s %s→%s [%s] %s\n",
			idleStyle.Render(msg.Timestamp.Format("15:04")),
			truncate(msg.From, mailFromW), truncate(msg.To, mailFromW),
			msg.Type, truncate(msg.Subject, mailSubjW))
	}
	if mailContent == "" {
		mailContent = "  No messages yet. Agents communicate via loom mail.\n"
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


package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// overviewRowBudget returns the max item rows each panel can show.
// It reserves 2 rows for title+helpbar, and 3 rows border overhead per panel.
func (m Model) overviewRowBudget(panelCount int) int {
	usable := m.height - 2 // title bar + help bar
	var perPanel int
	if m.width < 80 {
		perPanel = (usable / panelCount) - 3
	} else {
		perPanel = (usable / 3) - 3 // taller column has 3 panels
	}
	if perPanel < 1 {
		perPanel = 1
	}
	return perPanel
}

// capContent limits content lines to maxRows, appending a "... and N more" hint if truncated.
func capContent(lines []string, maxRows int) string {
	if len(lines) <= maxRows || maxRows <= 0 {
		return linesToContent(lines)
	}
	show := maxRows - 1 // reserve 1 row for the hint
	if show < 0 {
		show = 0
	}
	remaining := len(lines) - show
	result := linesToContent(lines[:show])
	result += fmt.Sprintf("  ... and %d more\n", remaining)
	return result
}

func linesToContent(lines []string) string {
	s := ""
	for _, l := range lines {
		s += l + "\n"
	}
	return s
}

func (m Model) renderOverview() string {
	stacked := m.width < 80
	colW := max((m.width-4)/2, 30)
	if stacked {
		colW = max(m.width-2, 20)
	}
	innerW := colW - 2

	panelCount := 5
	budget := m.overviewRowBudget(panelCount)

	// Agent table
	aIdW := min(18, max(8, (innerW-6)*3/5))
	aRoleW := max(4, innerW-6-aIdW-4)
	var agentLines []string
	for _, a := range m.data.agents {
		ago := timeAgo(a.Heartbeat)
		agentLines = append(agentLines, fmt.Sprintf("  %s %-*s %-*s %s",
			statusIndicator(a.Status), aIdW, truncate(a.ID, aIdW),
			aRoleW, truncate(a.Role, aRoleW), idleStyle.Render(ago)))
	}
	agentContent := capContent(agentLines, budget)
	if agentContent == "" {
		agentContent = "  No agents running. Use loom spawn to start.\n"
	}
	agentPanel := panel(fmt.Sprintf("AGENTS (%d)", len(m.data.agents)), agentContent, colW)

	// Issues (non-done)
	iIdW := min(12, max(6, (innerW-6)/4))
	iStatusW := min(11, max(6, (innerW-6)/5))
	iTitleW := max(6, innerW-6-iIdW-iStatusW)
	var issueLines []string
	for _, iss := range m.data.issues {
		if iss.Status == "done" || iss.Status == "cancelled" {
			continue
		}
		issueLines = append(issueLines, fmt.Sprintf("  %s %-*s %-*s %s",
			statusIndicator(iss.Status), iIdW, truncate(iss.ID, iIdW),
			iTitleW, truncate(iss.Title, iTitleW),
			statusStyle(iss.Status).Render(truncate(iss.Status, iStatusW))))
	}
	issueContent := capContent(issueLines, budget)
	if issueContent == "" {
		issueContent = "  No open issues. Use loom issue create to add one.\n"
	}
	issuePanel := panel(fmt.Sprintf("ISSUES (%d open)", len(issueLines)), issueContent, colW)

	// Worktrees
	wtSlugW := min(22, max(8, (innerW-4)/3))
	wtBranchW := min(20, max(6, (innerW-4)/3))
	var wtLines []string
	for _, wt := range m.data.worktrees {
		diffStr := ""
		if ds := m.data.diffStats[wt.Name]; ds != nil && ds.FilesChanged > 0 {
			diffStr = fmt.Sprintf(" %df +%d -%d", ds.FilesChanged, ds.Insertions, ds.Deletions)
		}
		wtLines = append(wtLines, fmt.Sprintf("  %-*s %s%s",
			wtSlugW, truncate(slugFromWorktree(wt.Name), wtSlugW),
			idleStyle.Render(truncate(wt.Branch, wtBranchW)), activeStyle.Render(diffStr)))
	}
	wtContent := capContent(wtLines, budget)
	if wtContent == "" {
		wtContent = "  No worktrees active. Builders create them automatically.\n"
	}
	wtPanel := panel(fmt.Sprintf("WORKTREES (%d)", len(m.data.worktrees)), wtContent, colW)

	// Mail
	mailFromW := min(12, max(4, (innerW-10)/4))
	mailSubjW := max(6, innerW-10-mailFromW*2-8)
	var mailLines []string
	for _, msg := range m.data.messages[:min(len(m.data.messages), 20)] {
		mailLines = append(mailLines, fmt.Sprintf("  %s %s→%s [%s] %s",
			idleStyle.Render(msg.Timestamp.Format("15:04")),
			truncate(msg.From, mailFromW), truncate(msg.To, mailFromW),
			msg.Type, truncate(msg.Subject, mailSubjW)))
	}
	mailContent := capContent(mailLines, budget)
	if mailContent == "" {
		mailContent = "  No messages yet. Agents communicate via loom mail.\n"
	}
	mailPanel := panel(fmt.Sprintf("MAIL (%d unread)", m.data.unread), mailContent, colW)

	// Memory (single summary line — no truncation needed)
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

	if stacked {
		return lipgloss.JoinVertical(lipgloss.Left, agentPanel, issuePanel, wtPanel, mailPanel, memPanel)
	}

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


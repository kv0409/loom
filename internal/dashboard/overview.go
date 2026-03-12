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
	usable := m.height - 3 // title bar (1) + help bar (2)
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

	panelCount := 6
	budget := m.overviewRowBudget(panelCount)
	agentBudget := budget + budget/2 // agents get 1.5x budget (primary panel)

	// Agent table (primary — prominent with age, heartbeat, task)
	aIdW := min(16, max(8, (innerW-12)*2/5))
	aRoleW := max(4, min(10, (innerW-12)/5))
	aAgeW := max(4, 6)
	aHbW := max(4, 6)
	var agentLines []string
	for _, a := range m.data.agents {
		hb := timeAgo(a.Heartbeat)
		age := timeAgo(a.SpawnedAt)
		task := idleStyle.Render("idle")
		if len(a.AssignedIssues) > 0 {
			task = activeStyle.Render(a.AssignedIssues[0])
		}
		agentLines = append(agentLines, fmt.Sprintf("  %s %-*s %-*s %s %s %s",
			statusIndicator(a.Status), aIdW, truncate(a.ID, aIdW),
			aRoleW, truncate(a.Role, aRoleW),
			idleStyle.Render(fmt.Sprintf("⏱ %-*s", aAgeW, age)),
			heartbeatStyle(hb).Render(fmt.Sprintf("♥ %-*s", aHbW, hb)),
			task))
	}
	agentContent := capContent(agentLines, agentBudget)
	if agentContent == "" {
		agentContent = renderEmpty("No agents running — loom spawn to start", innerW)
	} else {
		agentContent = "\n" + agentContent
	}
	agentPanel := panel(fmt.Sprintf("AGENTS (%d)", len(m.data.agents)), agentContent, colW)

	// Issues (simplified — indicator + ID + title only, no status text)
	iIdW := min(14, max(6, (innerW-4)/4))
	iTitleW := max(6, innerW-4-iIdW)
	var issueLines []string
	for _, iss := range m.data.issues {
		if iss.Status == "done" || iss.Status == "cancelled" {
			continue
		}
		issueLines = append(issueLines, fmt.Sprintf("  %s %-*s %s",
			statusIndicator(iss.Status), iIdW, truncate(iss.ID, iIdW),
			truncate(iss.Title, iTitleW)))
	}
	issueContent := capContent(issueLines, budget)
	if issueContent == "" {
		issueContent = renderEmpty("No open issues — loom issue create to add one", innerW)
	} else {
		issueContent = "\n" + issueContent
	}
	issuePanel := panel(fmt.Sprintf("ISSUES (%d open)", len(issueLines)), issueContent, colW)

	// Worktrees (simplified — name + issue only, no branch/diff)
	wtSlugW := max(8, (innerW-4)*2/3)
	wtIssueW := max(6, innerW-4-wtSlugW)
	var wtLines []string
	for _, wt := range m.data.worktrees {
		issueID := wt.Issue
		if issueID == "" {
			issueID = "—"
		}
		wtLines = append(wtLines, fmt.Sprintf("  %-*s %s",
			wtSlugW, truncate(slugFromWorktree(wt.Name), wtSlugW),
			idleStyle.Render(truncate(issueID, wtIssueW))))
	}
	wtContent := capContent(wtLines, budget)
	if wtContent == "" {
		wtContent = renderEmpty("No worktrees active — builders create them automatically", innerW)
	} else {
		wtContent = "\n" + wtContent
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
		mailContent = renderEmpty("No messages yet — agents communicate via loom mail", innerW)
	} else {
		mailContent = "\n" + mailContent
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
		memContent = renderEmpty("empty", innerW)
	}
	memPanel := panel(fmt.Sprintf("MEMORY (%d)", len(m.data.memories)), memContent, colW)

	// Live activity section
	actPanel := m.renderActivityOverview(colW, budget)

	if stacked {
		return lipgloss.JoinVertical(lipgloss.Left, agentPanel, issuePanel, wtPanel, mailPanel, memPanel, actPanel)
	}

	left := lipgloss.JoinVertical(lipgloss.Left, agentPanel, wtPanel, memPanel)
	right := lipgloss.JoinVertical(lipgloss.Left, issuePanel, mailPanel, actPanel)

	leftH := lipgloss.Height(left)
	rightH := lipgloss.Height(right)
	if leftH < rightH {
		left += strings.Repeat("\n", rightH-leftH)
	} else if rightH < leftH {
		right += strings.Repeat("\n", leftH-rightH)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// heartbeatStyle returns a color style based on heartbeat freshness string.
func heartbeatStyle(ago string) lipgloss.Style {
	if strings.HasSuffix(ago, "s") || ago == "never" {
		if ago == "never" {
			return lipgloss.NewStyle().Foreground(colRed)
		}
		return lipgloss.NewStyle().Foreground(colGreen)
	}
	if strings.HasSuffix(ago, "m") {
		return lipgloss.NewStyle().Foreground(colYellow)
	}
	return lipgloss.NewStyle().Foreground(colRed)
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

// renderActivityOverview builds a compact live activity panel for the overview.
func (m Model) renderActivityOverview(colW, budget int) string {
	var lines []string

	// Recent mail (last few from→to with type)
	msgs := m.data.messages
	mailLimit := min(3, len(msgs))
	for i := 0; i < mailLimit; i++ {
		msg := msgs[i]
		lines = append(lines, fmt.Sprintf("  ▸ %s %s→%s [%s]",
			idleStyle.Render(msg.Timestamp.Format("15:04")),
			truncate(msg.From, 12), truncate(msg.To, 12), msg.Type))
	}

	// Recent agent tool calls from activity data
	toolLimit := min(3, len(m.data.activity))
	for i := len(m.data.activity) - toolLimit; i < len(m.data.activity); i++ {
		e := m.data.activity[i]
		lines = append(lines, fmt.Sprintf("  ↯ %-12s %s",
			truncate(e.AgentID, 12), truncate(e.Line, colW-20)))
	}

	content := capContent(lines, budget)
	if content == "" {
		content = renderEmpty("No recent activity", colW-2)
	} else {
		content = "\n" + content
	}
	unique := map[string]struct{}{}
	for _, e := range m.data.activity {
		unique[e.AgentID] = struct{}{}
	}
	return panel(fmt.Sprintf("ACTIVITY (%d agents, %d msgs)", len(unique), len(m.data.messages)), content, colW)
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


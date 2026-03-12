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

// agentsBandBudget returns the row budget for the full-width AGENTS band (~40% of usable height).
func (m Model) agentsBandBudget() int {
	usable := m.height - 3 // title bar (1) + help bar (2)
	budget := (usable * 40 / 100) - 3
	if budget < 1 {
		budget = 1
	}
	return budget
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
	fullW := max(m.width-2, 20)
	innerW := fullW - 2

	agentBudget := m.agentsBandBudget()

	// --- AGENTS band (full width, ~40% height, no task truncation) ---
	aIdW := min(16, max(8, (innerW-12)*2/5))
	aRoleW := max(4, min(10, (innerW-12)/5))
	aAgeW := max(4, 6)
	aHbW := max(4, 6)
	// task column gets remaining space — no truncation cap
	fixedCols := 2 + 1 + aIdW + 1 + aRoleW + 1 + 2 + aAgeW + 1 + 2 + aHbW + 1
	aTaskW := max(8, innerW-fixedCols)

	var agentLines []string
	for _, a := range m.data.agents {
		hb := timeAgo(a.Heartbeat)
		age := timeAgo(a.SpawnedAt)
		task := idleStyle.Render("idle")
		if len(a.AssignedIssues) > 0 {
			// join all assigned issues — no truncation
			taskStr := strings.Join(a.AssignedIssues, ", ")
			if lipgloss.Width(taskStr) > aTaskW {
				taskStr = truncate(taskStr, aTaskW)
			}
			task = activeStyle.Render(taskStr)
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
	agentPanel := panel(fmt.Sprintf("[a] AGENTS (%d)", len(m.data.agents)), agentContent, fullW)

	// --- Remaining panels (lower ~60%) ---
	stacked := m.width < 80
	colW := max((m.width-4)/2, 30)
	if stacked {
		colW = max(m.width-2, 20)
	}
	colInnerW := colW - 2

	// remaining vertical budget for lower panels
	usable := m.height - 3
	lowerUsable := usable - agentBudget - 3 // subtract agents band height
	panelCount := 4
	if stacked {
		panelCount = 3
	}
	budget := (lowerUsable/panelCount - 3)
	if budget < 1 {
		budget = 1
	}

	// Issues (simplified — indicator + ID + title only)
	iIdW := min(14, max(6, (colInnerW-4)/4))
	iTitleW := max(6, colInnerW-4-iIdW)
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
		issueContent = renderEmpty("No open issues — loom issue create to add one", colInnerW)
	} else {
		issueContent = "\n" + issueContent
	}
	issuePanel := panel(fmt.Sprintf("[i] ISSUES (%d open)", len(issueLines)), issueContent, colW)

	// Worktrees (simplified)
	wtSlugW := max(8, (colInnerW-4)*2/3)
	wtIssueW := max(6, colInnerW-4-wtSlugW)
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
		wtContent = renderEmpty("No worktrees active — builders create them automatically", colInnerW)
	} else {
		wtContent = "\n" + wtContent
	}
	wtPanel := panel(fmt.Sprintf("[w] WORKTREES (%d)", len(m.data.worktrees)), wtContent, colW)

	// Mail
	mailFromW := min(12, max(4, (colInnerW-10)/4))
	mailSubjW := max(6, colInnerW-10-mailFromW*2-8)
	var mailLines []string
	for _, msg := range m.data.messages[:min(len(m.data.messages), 20)] {
		mailLines = append(mailLines, fmt.Sprintf("  %s %s→%s [%s] %s",
			idleStyle.Render(msg.Timestamp.Format("15:04")),
			truncate(msg.From, mailFromW), truncate(msg.To, mailFromW),
			msg.Type, truncate(msg.Subject, mailSubjW)))
	}
	mailContent := capContent(mailLines, budget)
	if mailContent == "" {
		mailContent = renderEmpty("No messages yet — agents communicate via loom mail", colInnerW)
	} else {
		mailContent = "\n" + mailContent
	}
	mailPanel := panel(fmt.Sprintf("[m] MAIL (%d unread)", m.data.unread), mailContent, colW)

	// Memory
	memCounts := map[string]int{}
	for _, e := range m.data.memories {
		memCounts[e.Type]++
	}
	var parts []string
	for _, t := range []string{"decision", "discovery", "convention"} {
		if c := memCounts[t]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %ss", c, t))
		}
	}
	memContent := ""
	if len(parts) > 0 {
		memContent = "  " + strings.Join(parts, " · ") + "\n"
	} else {
		memContent = renderEmpty("empty", colInnerW)
	}
	memPanel := panel(fmt.Sprintf("[d] MEMORY (%d)", len(m.data.memories)), memContent, colW)

	// Activity
	actPanel := m.renderActivityOverview(colW, budget)

	var lowerSection string
	if stacked {
		hint := lipgloss.NewStyle().Foreground(colYellow).
			Render("  ↔ resize terminal ≥ 80 cols for memory & activity panels")
		lowerSection = lipgloss.JoinVertical(lipgloss.Left, issuePanel, mailPanel, wtPanel, hint)
	} else {
		left := lipgloss.JoinVertical(lipgloss.Left, wtPanel, memPanel)
		right := lipgloss.JoinVertical(lipgloss.Left, issuePanel, mailPanel, actPanel)

		leftH := lipgloss.Height(left)
		rightH := lipgloss.Height(right)
		if leftH < rightH {
			left += strings.Repeat("\n", rightH-leftH)
		} else if rightH < leftH {
			right += strings.Repeat("\n", leftH-rightH)
		}
		lowerSection = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}

	return lipgloss.JoinVertical(lipgloss.Left, agentPanel, lowerSection)
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
// Shows only ToolSummary lines (human-readable tool use); mail is in the MAIL panel.
func (m Model) renderActivityOverview(colW, budget int) string {
	var lines []string
	toolLimit := min(budget, len(m.data.activity))
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
	return panel(fmt.Sprintf("[t] ACTIVITY (%d agents)", len(unique)), content, colW)
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


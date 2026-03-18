package dashboard

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

// agentsBandBudget returns the row budget for the full-width AGENTS band (~40% of usable height).
func (m Model) agentsBandBudget() int {
	usable := m.height - 1 - lipgloss.Height(m.helpBar()) // title bar (1) + help bar
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
	fullW := max(panelWidth(m.width), 20)
	usable := m.height - 1 - lipgloss.Height(m.helpBar())
	attentionBudget := max(5, usable/3)
	flightBudget := max(6, usable/3)
	signalBudget := usable - attentionBudget - flightBudget - 3
	if signalBudget < 4 {
		signalBudget = 4
	}

	attention := m.renderAttentionOverview(fullW, attentionBudget)
	flight := m.renderFlightOverview(fullW, flightBudget)
	signal := m.renderActivityOverview(fullW, signalBudget)

	return lipgloss.JoinVertical(lipgloss.Left, attention, flight, signal)
}

func (m Model) renderAttentionOverview(fullW, budget int) string {
	var blocked []*backend.Issue
	var review []*backend.Issue
	for _, iss := range m.data.Issues {
		switch iss.Status {
		case "blocked":
			blocked = append(blocked, iss)
		case "review":
			review = append(review, iss)
		}
	}

	var dead []*backend.Agent
	for _, a := range m.data.Agents {
		if a.Status == "dead" || a.Status == "error" {
			dead = append(dead, a)
		}
	}

	var lines []string
	if len(blocked) > 0 {
		lines = append(lines, blockedStyle.Render(fmt.Sprintf("  %d blocked issue%s need intervention", len(blocked), suffix(len(blocked)))))
		for _, iss := range blocked[:min(3, len(blocked))] {
			line := fmt.Sprintf("    %s %s", iss.ID, truncate(iss.Title, fullW-14))
			if iss.Assignee != "" {
				line += idleStyle.Render(" · " + iss.Assignee)
			}
			lines = append(lines, line)
		}
	}
	if len(review) > 0 {
		lines = append(lines, reviewStyle.Render(fmt.Sprintf("  %d issue%s waiting on review", len(review), suffix(len(review)))))
		for _, iss := range review[:min(3, len(review))] {
			lines = append(lines, fmt.Sprintf("    %s %s", iss.ID, truncate(iss.Title, fullW-14)))
		}
	}
	if len(dead) > 0 {
		lines = append(lines, deadStyle.Render(fmt.Sprintf("  %d agent%s offline or errored", len(dead), suffix(len(dead)))))
		for _, a := range dead[:min(2, len(dead))] {
			issues := "no assigned issue"
			if len(a.AssignedIssues) > 0 {
				issues = strings.Join(a.AssignedIssues, ", ")
			}
			lines = append(lines, fmt.Sprintf("    %s %s", a.ID, idleStyle.Render("· "+truncate(issues, fullW-18))))
		}
	}
	if m.data.Unread > 0 {
		lines = append(lines, barLabel.Render(fmt.Sprintf("  %d unread message%s waiting in inboxes", m.data.Unread, suffix(m.data.Unread))))
	}
	if len(lines) == 0 {
		return panel("NEEDS ATTENTION", renderEmpty("No active blockers, dead agents, or unread messages", fullW-2), fullW)
	}
	return panel("NEEDS ATTENTION", capContent(lines, budget), fullW)
}

func (m Model) renderFlightOverview(fullW, budget int) string {
	projectRoot := filepath.Dir(m.loomRoot)
	lastActivity := map[string]string{}
	for _, e := range m.data.Activity {
		lastActivity[e.AgentID] = e.Line
	}

	activeIssues := 0
	for _, iss := range m.data.Issues {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeIssues++
		}
	}

	statsText := fmt.Sprintf("  %d active issue%s · %d running agent%s · %d worktree%s", activeIssues, suffix(activeIssues), len(m.data.Agents), suffix(len(m.data.Agents)), len(m.data.Worktrees), suffix(len(m.data.Worktrees)))
	lines := []string{
		statsLineStyle.Render(statsText),
		strings.TrimRight(separator(fullW), "\n"),
	}

	shown := 0
	for _, a := range m.data.Agents {
		if a.Status == "dead" || a.Status == "error" {
			continue
		}
		label := statusIndicator(a.Status) + " " + agentPillFor(truncate(a.ID, 16), a.ID)
		focus := idleStyle.Render("idle")
		if line, ok := lastActivity[a.ID]; ok && line != "" {
			focus = activeStyle.Render(truncate(formatToolLine(line, fullW-26, projectRoot), fullW-26))
		} else if len(a.AssignedIssues) > 0 {
			focus = activeStyle.Render(strings.Join(a.AssignedIssues, ", "))
		}
		lines = append(lines, fmt.Sprintf("  %s  %s", label, focus))
		shown++
		if shown >= budget-2 {
			break
		}
	}
	if shown == 0 {
		lines = append(lines, "  No active agents yet.")
	}

	return panel("IN FLIGHT", capContent(lines, budget), fullW)
}

func suffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// renderActivityOverview builds a compact live activity panel for the overview.
// Shows only ToolSummary lines (human-readable tool use); mail excluded.
// Uses 4-column layout (AGENT, TIME, TOOL, DETAIL) matching renderActivity.
func (m Model) renderActivityOverview(colW, budget int) string {
	innerW := colW - 2 // panel border (1 each side)
	const numCols = 4
	innerW -= numCols * 2 // table cell padding

	agentW := 16 // "orchestrator" (12) + pill padding (2) + cell padding (2) = 16
	timeW := 7
	toolW := 5
	detailW := max(8, innerW-agentW-timeW-toolW)

	cols := []table.Column{
		{Title: "", Width: agentW},
		{Title: "", Width: timeW},
		{Title: "", Width: toolW},
		{Title: "", Width: detailW},
	}

	toolLimit := min(budget, len(m.data.Activity))
	rows := make([]table.Row, 0, toolLimit)
	var replacements [][2]string
	ri := 0
	for i := len(m.data.Activity) - toolLimit; i < len(m.data.Activity); i++ {
		e := m.data.Activity[i]
		truncAgent := truncate(e.AgentID, agentW-2) // -2 for agentPill Padding(0,1)
		styledAgent := agentPillFor(truncAgent, e.AgentID)
		styledTime := activityTimeStyle.Render(truncate(e.Time, timeW))
		info := resolveToolInfo(e.Tool)
		styledTool := activityLabelStyle.Foreground(info.labelColor).Render(truncate(e.Tool, toolW))
		plainDetail := truncate(e.Detail, detailW)

		phAgent := cellPlaceholder(ri, lipgloss.Width(agentPillPlain(truncAgent)))
		phTime := cellPlaceholder(ri+1, lipgloss.Width(styledTime))
		phTool := cellPlaceholder(ri+2, lipgloss.Width(styledTool))
		rows = append(rows, table.Row{phAgent, phTime, phTool, plainDetail})
		replacements = append(replacements,
			[2]string{phAgent, styledAgent},
			[2]string{phTime, styledTime},
			[2]string{phTool, styledTool},
		)
		ri += 3
	}

	unique := map[string]struct{}{}
	for _, e := range m.data.Activity {
		unique[e.AgentID] = struct{}{}
	}

	var content string
	if len(rows) == 0 {
		content = renderEmpty("No recent activity", colW-2)
	} else {
		t := newStyledTableHeaderless(cols, rows, len(rows))
		content = "\n" + styledTableBodyView(t, replacements)
	}
	return panel(fmt.Sprintf("LATEST SIGNAL (%d agents)", len(unique)), content, colW)
}

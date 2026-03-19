package dashboard

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
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
	lastActivity := map[string]string{}
	for _, e := range m.data.Activity {
		if e.Detail != "" {
			lastActivity[e.AgentID] = e.Detail
		}
	}

	activeIssues := 0
	for _, iss := range m.data.Issues {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeIssues++
		}
	}

	statsText := fmt.Sprintf("  %d active issue%s · %d running agent%s · %d worktree%s", activeIssues, suffix(activeIssues), len(m.data.Agents), suffix(len(m.data.Agents)), len(m.data.Worktrees), suffix(len(m.data.Worktrees)))
	header := statsLineStyle.Render(statsText) + "\n" + strings.TrimRight(separator(fullW), "\n")

	type flightAgent struct {
		agent       *backend.Agent
		hasActivity bool
	}
	var agents []flightAgent
	maxRows := budget - 2
	if maxRows < 1 {
		maxRows = 1
	}
	for _, a := range m.data.Agents {
		if a.Status == "dead" || a.Status == "error" {
			continue
		}
		_, hasAct := lastActivity[a.ID]
		agents = append(agents, flightAgent{agent: a, hasActivity: hasAct})
		if len(agents) >= maxRows {
			break
		}
	}

	if len(agents) == 0 {
		return panel("IN FLIGHT", header+"\n  No active agents yet.\n", fullW)
	}

	timeout := time.Duration(m.heartbeatTimeoutSec) * time.Second
	rows := make([][]string, len(agents))
	for i, fa := range agents {
		a := fa.agent
		// Glyph: heartbeat donut for active/in-progress, statusGlyph otherwise
		glyph := statusGlyphs[a.Status]
		if glyph == "" {
			glyph = "●"
		}
		if a.Status == "active" || a.Status == "in-progress" {
			glyph = heartbeatGlyph(time.Since(a.Heartbeat), timeout)
		}
		// HB
		hb := fmtTime(a.Heartbeat, true)
		// Issues
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = strings.Join(a.AssignedIssues, ",")
		}
		// Focus
		focus := "idle"
		if detail, ok := lastActivity[a.ID]; ok {
			focus = detail
		} else if len(a.AssignedIssues) > 0 {
			focus = strings.Join(a.AssignedIssues, ", ")
		}
		rows[i] = []string{glyph, truncate(a.ID, 16), hb, issues, focus}
	}

	styler := func(row, col int, _ bool) lipgloss.Style {
		base := lgTableCellStyle
		if row >= len(agents) {
			return base
		}
		a := agents[row].agent
		switch col {
		case 0: // glyph
			if a.Status == "active" || a.Status == "in-progress" {
				return base.Foreground(heartbeatColor(time.Since(a.Heartbeat), timeout))
			}
			if c, ok := statusColors[a.Status]; ok {
				return base.Foreground(c)
			}
		case 1: // agent ID
			return base.Foreground(agentColor(a.ID)).Bold(true)
		case 2: // HB
			if a.Status == "active" || a.Status == "in-progress" {
				elapsed := time.Since(a.Heartbeat)
				if timeout > 0 && float64(elapsed)/float64(timeout) >= 0.8 {
					return base.Foreground(colRed)
				}
			}
			return base.Foreground(colGray)
		case 3: // issues
			return base.Foreground(colFg)
		case 4: // focus
			if agents[row].hasActivity {
				return base.Foreground(colGreen)
			}
			return base.Foreground(colGray)
		}
		return base
	}

	innerW := fullW - 2
	t := newLGTableHeaderless(rows, -1, innerW, styler, ColWidth{0, 3}, ColWidth{1, 18}, ColWidth{2, 5}, ColWidth{3, 14})
	content := header + "\n" + t.Render()
	return panel("IN FLIGHT", content, fullW)
}

func suffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// renderActivityOverview builds a compact live activity panel for the overview.
// Shows only ToolSummary lines (human-readable tool use); mail excluded.
// Uses 4-column layout: AGENT, TIME, TOOL, DETAIL. Icon colored via CellStyler.
func (m Model) renderActivityOverview(colW, budget int) string {
	innerW := colW - 2 // panel border (1 each side)

	toolLimit := min(budget, len(m.data.Activity))
	rows := make([][]string, 0, toolLimit)
	for i := len(m.data.Activity) - toolLimit; i < len(m.data.Activity); i++ {
		e := m.data.Activity[i]
		rows = append(rows, []string{e.AgentID, e.Time, resolveToolInfo(e.Tool).icon, e.Detail})
	}

	activityStart := len(m.data.Activity) - toolLimit
	styler := func(row, col int, _ bool) lipgloss.Style {
		base := lgTableCellStyle
		dataIdx := activityStart + row
		if dataIdx >= len(m.data.Activity) {
			return base
		}
		e := m.data.Activity[dataIdx]
		switch col {
		case 0:
			return base.Foreground(agentColor(e.AgentID)).Bold(true)
		case 1:
			return base.Foreground(colGray)
		case 2:
			return base.Foreground(resolveToolInfo(e.Tool).color).Bold(true)
		}
		return base
	}

	unique := map[string]struct{}{}
	for _, e := range m.data.Activity {
		unique[e.AgentID] = struct{}{}
	}

	var content string
	if len(rows) == 0 {
		content = renderEmpty("No recent activity", colW-2)
	} else {
		t := newLGTableHeaderless(rows, -1, innerW, styler, ColWidth{0, 16}, ColWidth{1, 5}, ColWidth{2, 3})
		content = "\n" + t.Render()
	}
	return panel(fmt.Sprintf("LATEST SIGNAL (%d agents)", len(unique)), content, colW)
}

package dashboard

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/issue"
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
	var blocked []*issue.Issue
	var review []*issue.Issue
	for _, iss := range m.data.issues {
		switch iss.Status {
		case "blocked":
			blocked = append(blocked, iss)
		case "review":
			review = append(review, iss)
		}
	}

	var dead []*agent.Agent
	for _, a := range m.data.agents {
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
	if m.data.unread > 0 {
		lines = append(lines, barLabel.Render(fmt.Sprintf("  %d unread message%s waiting in inboxes", m.data.unread, suffix(m.data.unread))))
	}
	if len(lines) == 0 {
		return panel("NEEDS ATTENTION", renderEmpty("No active blockers, dead agents, or unread messages", fullW-2), fullW)
	}
	return panel("NEEDS ATTENTION", capContent(lines, budget), fullW)
}

func (m Model) renderFlightOverview(fullW, budget int) string {
	projectRoot := filepath.Dir(m.loomRoot)
	lastActivity := map[string]string{}
	for _, e := range m.data.activity {
		lastActivity[e.AgentID] = e.Line
	}

	activeIssues := 0
	for _, iss := range m.data.issues {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeIssues++
		}
	}

	lines := []string{
		fmt.Sprintf("  %d active issue%s · %d running agent%s · %d worktree%s", activeIssues, suffix(activeIssues), len(m.data.agents), suffix(len(m.data.agents)), len(m.data.worktrees), suffix(len(m.data.worktrees))),
	}

	shown := 0
	for _, a := range m.data.agents {
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

// renderStatusBar builds the full-width STATUS BAR band:
// Line 1: issue counts by status + worktree count + memory counts
// Lines 2-4: per-parent progress bars for active parent issues (max 3)
func (m Model) renderStatusBar(fullW int) string {
	innerW := fullW - 2

	// --- Line 1: counts summary ---
	statusCounts := map[string]int{}
	for _, iss := range m.data.issues {
		if iss.Status != "done" && iss.Status != "cancelled" && iss.Parent == "" {
			statusCounts[iss.Status]++
		}
	}
	// Also count all non-done/cancelled issues (including sub-issues) for display
	allStatusCounts := map[string]int{}
	for _, iss := range m.data.issues {
		if iss.Status != "done" && iss.Status != "cancelled" {
			allStatusCounts[iss.Status]++
		}
	}
	doneCount := 0
	for _, iss := range m.data.issues {
		if iss.Status == "done" {
			doneCount++
		}
	}

	var countParts []string
	for _, s := range []string{"in-progress", "review", "assigned", "blocked", "open"} {
		if c := allStatusCounts[s]; c > 0 {
			countParts = append(countParts, statusStyle(s).Render(fmt.Sprintf("%d %s", c, s)))
		}
	}
	if doneCount > 0 {
		countParts = append(countParts, idleStyle.Render(fmt.Sprintf("%d done", doneCount)))
	}

	memCounts := map[string]int{}
	for _, e := range m.data.memories {
		memCounts[e.Type]++
	}
	var memParts []string
	for _, t := range []string{"decision", "discovery", "convention"} {
		if c := memCounts[t]; c > 0 {
			memParts = append(memParts, fmt.Sprintf("%d %s", c, plural(c, t)))
		}
	}

	summaryParts := countParts
	if len(m.data.worktrees) > 0 {
		n := len(m.data.worktrees)
		summaryParts = append(summaryParts, idleStyle.Render(fmt.Sprintf("%d %s", n, plural(n, "worktree"))))
	}
	if len(memParts) > 0 {
		summaryParts = append(summaryParts, idleStyle.Render(strings.Join(memParts, " · ")))
	}

	sep := idleStyle.Render(" · ")
	summaryLine := "  " + strings.Join(summaryParts, sep)

	// --- Lines 2-4: progress bars for active parent issues ---
	type parentProgress struct {
		id       string
		title    string
		done     int
		total    int
		children []string
	}

	// Build a map of issue ID → issue for quick lookup
	issueMap := map[string]*issue.Issue{}
	for _, iss := range m.data.issues {
		issueMap[iss.ID] = iss
	}

	var parents []parentProgress
	for _, iss := range m.data.issues {
		if iss.Status == "done" || iss.Status == "cancelled" {
			continue
		}
		if len(iss.Children) == 0 {
			continue
		}
		// Count done children
		done := 0
		for _, cid := range iss.Children {
			if c, ok := issueMap[cid]; ok && (c.Status == "done" || c.Status == "cancelled") {
				done++
			}
		}
		parents = append(parents, parentProgress{
			id:       iss.ID,
			title:    iss.Title,
			done:     done,
			total:    len(iss.Children),
			children: iss.Children,
		})
	}

	const maxBars = 3
	shown := parents
	overflow := 0
	if len(parents) > maxBars {
		shown = parents[:maxBars]
		overflow = len(parents) - maxBars
	}

	// stackedBar renders a continuous stacked bar of width barW using largest-remainder rounding.
	// Visual groups left-to-right: done(█) → active(▓) → blocked(▓) → remaining(░)
	// Active merges assigned + in-progress + review.
	stackedBar := func(counts map[string]int, total, barW int) string {
		type stage struct {
			char  string
			count int
			style lipgloss.Style
		}
		stages := []stage{
			{"█", counts["done"] + counts["cancelled"], barSegDone},
			{"▓", counts["assigned"] + counts["in-progress"] + counts["review"], barSegActive},
			{"▓", counts["blocked"], barSegBlocked},
			{"░", counts["open"], barSegRemaining},
		}
		type entry struct {
			exact     float64
			floor     int
			remainder float64
		}
		entries := make([]entry, len(stages))
		allocated := 0
		for i, s := range stages {
			exact := 0.0
			if total > 0 {
				exact = float64(s.count) * float64(barW) / float64(total)
			}
			entries[i] = entry{exact, int(exact), exact - float64(int(exact))}
			allocated += int(exact)
		}
		rem := barW - allocated
		order := make([]int, len(entries))
		for i := range order {
			order[i] = i
		}
		for i := 0; i < len(order)-1; i++ {
			for j := i + 1; j < len(order); j++ {
				if entries[order[j]].remainder > entries[order[i]].remainder {
					order[i], order[j] = order[j], order[i]
				}
			}
		}
		widths := make([]int, len(stages))
		for i, idx := range order {
			widths[idx] = entries[idx].floor
			if i < rem {
				widths[idx]++
			}
		}
		var bar string
		for i, s := range stages {
			if widths[i] > 0 {
				bar += s.style.Render(strings.Repeat(s.char, widths[i]))
			}
		}
		return bar
	}

	// Column widths: id=14, bar=20, fraction=7, title=remainder
	const idW, barW, fracW = 14, 20, 7
	titleW := max(20, innerW-2-idW-barW-fracW-4) // 4 = cell padding (0,1) × 4 cols × 2 sides / 2

	var rows []table.Row
	var statusReplacements [][2]string
	ri := 0
	for _, p := range shown {
		childCounts := map[string]int{}
		for _, cid := range p.children {
			if c, ok := issueMap[cid]; ok {
				childCounts[c.Status]++
			}
		}
		bar := stackedBar(childCounts, p.total, barW)
		styledID := barLabel.Render(truncate(p.id, idW))
		styledTitle := idleStyle.Render(truncate(p.title, titleW))
		phID := cellPlaceholder(ri, lipgloss.Width(styledID))
		phBar := cellPlaceholder(ri+1, barW)
		phTitle := cellPlaceholder(ri+2, lipgloss.Width(styledTitle))
		fraction := fmt.Sprintf("%d/%d", p.done, p.total)
		rows = append(rows, table.Row{phID, phBar, fraction, phTitle})
		statusReplacements = append(statusReplacements,
			[2]string{phID, styledID},
			[2]string{phBar, bar},
			[2]string{phTitle, styledTitle},
		)
		ri += 3
	}
	if overflow > 0 {
		overflowText := fmt.Sprintf("… +%d more", overflow)
		styledOverflow := idleStyle.Render(overflowText)
		phOverflow := cellPlaceholder(ri, lipgloss.Width(styledOverflow))
		rows = append(rows, table.Row{phOverflow, "", "", ""})
		statusReplacements = append(statusReplacements, [2]string{phOverflow, styledOverflow})
	}

	cols := []table.Column{
		{Title: "", Width: idW},
		{Title: "", Width: barW},
		{Title: "", Width: fracW},
		{Title: "", Width: titleW},
	}
	tbl := newStyledTableHeaderless(cols, rows, len(rows))

	content := "\n" + summaryLine + "\n"
	if len(rows) > 0 {
		content += styledTableBodyView(tbl, statusReplacements) + "\n"
	}

	return panel("[s] STATUS", content, fullW)
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

	toolLimit := min(budget, len(m.data.activity))
	rows := make([]table.Row, 0, toolLimit)
	var replacements [][2]string
	ri := 0
	for i := len(m.data.activity) - toolLimit; i < len(m.data.activity); i++ {
		e := m.data.activity[i]
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
	for _, e := range m.data.activity {
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

// wordWrap splits s into segments of at most width runes, breaking on spaces where possible.
func wordWrap(s string, width int) []string {
	if width <= 0 || len(s) == 0 {
		return []string{s}
	}
	var segments []string
	for len(s) > 0 {
		runes := []rune(s)
		if len(runes) <= width {
			segments = append(segments, s)
			break
		}
		cut := width
		prefix := string(runes[:width])
		if idx := strings.LastIndex(prefix, " "); idx > 0 {
			cut = len([]rune(prefix[:idx])) + 1
		}
		segments = append(segments, strings.TrimRight(string(runes[:cut]), " "))
		s = strings.TrimLeft(string(runes[cut:]), " ")
	}
	return segments
}


package dashboard

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
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
	innerW := fullW - 2

	agentBudget := m.agentsBandBudget()

	// --- AGENTS band (full width, ~40% height, no task truncation) ---
	aIdW := min(16, max(8, (innerW-12)*2/5))
	aAgeW := max(4, 8)  // "⏱ " (2) + value (6)
	aHbW := max(4, 8)   // "♥ " (2) + value (6)
	// task column gets remaining space — no truncation cap
	// table adds 2-char padding per cell (Padding(0,1) = 1 each side), 4 cols × 2 = 8
	// +4 = glyph(1) + space(1) + agentPill Padding(0,1)(+2)
	fixedCols := aIdW + aAgeW + aHbW + 4 + 8
	aTaskW := max(8, innerW-fixedCols)

	projectRoot := filepath.Dir(m.loomRoot)

	// Build a map of agent ID → last ACP activity line for quick lookup.
	lastActivity := map[string]string{}
	for _, e := range m.data.activity {
		lastActivity[e.AgentID] = e.Line
	}

	agentCols := []table.Column{
		{Title: "", Width: aIdW + 4},  // glyph + space + agentPill (with Padding(0,1))
		{Title: "", Width: aAgeW},
		{Title: "", Width: aHbW},
		{Title: "", Width: aTaskW},
	}
	agentRows := make([]table.Row, 0, len(m.data.agents))
	var agentReplacements [][2]string
	ri := 0
	for _, a := range m.data.agents {
		hb := fmtTime(a.Heartbeat, true)
		age := fmtTime(a.SpawnedAt, true)
		styledTask := idleStyle.Render("idle")
		taskW := lipgloss.Width(styledTask)
		if line, ok := lastActivity[a.ID]; ok && line != "" {
			styledTask = formatToolLine(line, aTaskW, projectRoot)
			taskW = lipgloss.Width(styledTask)
		} else if len(a.AssignedIssues) > 0 {
			taskStr := truncate(strings.Join(a.AssignedIssues, ", "), aTaskW)
			styledTask = activeStyle.Render(taskStr)
			taskW = lipgloss.Width(styledTask)
		}
		glyph := statusGlyphs[a.Status]
		if glyph == "" {
			glyph = "●"
		}
		truncID := truncate(a.ID, aIdW)
		styledID := statusIndicator(a.Status) + " " + agentPillFor(truncID, a.ID)
		styledAge := idleStyle.Render("⏱ " + age)
		styledHb := heartbeatStyle(hb).Render("♥ " + hb)
		phID := cellPlaceholder(ri, lipgloss.Width(glyph+" "+agentPillPlain(truncID)))
		phAge := cellPlaceholder(ri+1, lipgloss.Width(styledAge))
		phHb := cellPlaceholder(ri+2, lipgloss.Width(styledHb))
		phTask := cellPlaceholder(ri+3, taskW)
		agentRows = append(agentRows, table.Row{phID, phAge, phHb, phTask})
		agentReplacements = append(agentReplacements,
			[2]string{phID, styledID},
			[2]string{phAge, styledAge},
			[2]string{phHb, styledHb},
			[2]string{phTask, styledTask},
		)
		ri += 4
	}

	var agentContent string
	if len(agentRows) == 0 {
		agentContent = renderEmpty("No agents running — loom spawn to start", innerW)
	} else {
		rows := agentRows
		repl := agentReplacements
		if agentBudget > 0 && len(rows) > agentBudget {
			rows = rows[:agentBudget]
			repl = repl[:agentBudget*4]
		}
		t := newStyledTableHeaderless(agentCols, rows, len(rows))
		agentContent = "\n" + styledTableBodyView(t, repl) + "\n"
		if len(agentRows) > agentBudget && agentBudget > 0 {
			agentContent += fmt.Sprintf("  ... and %d more\n", len(agentRows)-agentBudget)
		}
	}
	agentPanel := panel(fmt.Sprintf("[a] AGENTS (%d)", len(m.data.agents)), agentContent, fullW)

	// --- STATUS BAR band (full width, 1-4 lines) ---
	statusBar := m.renderStatusBar(fullW)

	// --- ACTIVITY band (remaining space) ---
	usable := m.height - 3
	agentPanelH := lipgloss.Height(agentPanel)
	statusBarH := lipgloss.Height(statusBar)
	actBudget := usable - agentPanelH - statusBarH - 3
	if actBudget < 1 {
		actBudget = 1
	}
	actPanel := m.renderActivityOverview(fullW, actBudget)

	return lipgloss.JoinVertical(lipgloss.Left, agentPanel, statusBar, actPanel)
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
	return panel(fmt.Sprintf("[t] ACTIVITY (%d agents)", len(unique)), content, colW)
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


package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/issue"
)

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
			memParts = append(memParts, fmt.Sprintf("%d %ss", c, t))
		}
	}

	summaryParts := countParts
	if len(m.data.worktrees) > 0 {
		summaryParts = append(summaryParts, idleStyle.Render(fmt.Sprintf("%d worktrees", len(m.data.worktrees))))
	}
	if len(memParts) > 0 {
		summaryParts = append(summaryParts, idleStyle.Render(strings.Join(memParts, " · ")))
	}

	sep := idleStyle.Render(" · ")
	summaryLine := "  " + strings.Join(summaryParts, sep)

	// --- Lines 2-4: progress bars for active parent issues ---
	type parentProgress struct {
		id    string
		title string
		done  int
		total int
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
			id:    iss.ID,
			title: iss.Title,
			done:  done,
			total: len(iss.Children),
		})
	}

	var barLines []string
	const maxBars = 3
	shown := parents
	overflow := 0
	if len(parents) > maxBars {
		shown = parents[:maxBars]
		overflow = len(parents) - maxBars
	}

	barFill := lipgloss.NewStyle().Foreground(colTeal)
	barEmpty := lipgloss.NewStyle().Foreground(colSubtle)
	barLabel := lipgloss.NewStyle().Foreground(colBlue).Bold(true)

	for _, p := range shown {
		idStr := barLabel.Render(fmt.Sprintf("%-14s", truncate(p.id, 14)))
		fraction := fmt.Sprintf("%d/%d", p.done, p.total)
		// bar width: innerW minus id(14) minus fraction(~5) minus title minus spacing
		fractionW := len(fraction)
		barW := 10
		titleMaxW := innerW - 14 - 1 - barW - 1 - fractionW - 2
		if titleMaxW < 6 {
			titleMaxW = 6
		}
		titleStr := idleStyle.Render(truncate(p.title, titleMaxW))

		filled := 0
		if p.total > 0 {
			filled = p.done * barW / p.total
		}
		bar := barFill.Render(strings.Repeat("■", filled)) +
			barEmpty.Render(strings.Repeat("░", barW-filled))

		barLines = append(barLines, fmt.Sprintf("  %s %s %-*s %s",
			idStr, bar, fractionW, fraction, titleStr))
	}
	if overflow > 0 {
		barLines = append(barLines, idleStyle.Render(fmt.Sprintf("  … +%d more active epics", overflow)))
	}

	content := "\n" + summaryLine + "\n"
	for _, l := range barLines {
		content += l + "\n"
	}

	return panel("[s] STATUS", content, fullW)
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
// Shows only ToolSummary lines (human-readable tool use); mail excluded.
// Lines are full-width — no truncation.
func (m Model) renderActivityOverview(colW, budget int) string {
	innerW := colW - 2
	agentW := 12
	prefixW := 2 + 2 + agentW + 1 // "  ↯ " + agent + " "
	lineW := max(8, innerW-prefixW)

	var lines []string
	toolLimit := min(budget, len(m.data.activity))
	for i := len(m.data.activity) - toolLimit; i < len(m.data.activity); i++ {
		e := m.data.activity[i]
		// Wrap long lines instead of truncating
		prefix := fmt.Sprintf("  ↯ %-*s ", agentW, truncate(e.AgentID, agentW))
		wrapped := wordWrap(e.Line, lineW)
		for j, seg := range wrapped {
			if j == 0 {
				lines = append(lines, prefix+seg)
			} else {
				lines = append(lines, strings.Repeat(" ", prefixW)+seg)
			}
		}
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

// wordWrap splits s into segments of at most width runes, breaking on spaces where possible.
func wordWrap(s string, width int) []string {
	if width <= 0 || len(s) == 0 {
		return []string{s}
	}
	var segments []string
	for len(s) > 0 {
		if len(s) <= width {
			segments = append(segments, s)
			break
		}
		// Try to break at last space within width
		cut := width
		if idx := strings.LastIndex(s[:width], " "); idx > 0 {
			cut = idx + 1
		}
		segments = append(segments, strings.TrimRight(s[:cut], " "))
		s = strings.TrimLeft(s[cut:], " ")
	}
	return segments
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


package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderWorktrees() string {
	worktrees := m.filteredWorktrees()

	avail := availableWidth(m.width)
	const numColsDiff = 5
	avail -= numColsDiff * 2
	ws := colWidths(avail, []struct{ pct, min int }{{25, 10}, {25, 10}, {14, 8}, {14, 6}, {0, 6}})
	nameW, branchW, agentW, issueW := ws[0], ws[1], ws[2], ws[3]
	diffW := max(6, avail-nameW-branchW-agentW-issueW)

	cols := []table.Column{
		{Title: "NAME", Width: nameW},
		{Title: "BRANCH", Width: branchW},
		{Title: "AGENT", Width: agentW},
		{Title: "ISSUE", Width: issueW},
		{Title: "DIFF", Width: diffW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(worktrees), vRows)

	rows := make([]table.Row, 0, end-start)
	var replacements [][2]string
	ri := 0
	for i := start; i < end; i++ {
		wt := worktrees[i]
		ds := m.data.DiffStats[wt.Name]
		if ds != nil && ds.FilesChanged > 0 {
			styledDiff := fmt.Sprintf("%df ", ds.FilesChanged) +
				diffAdd.Render(fmt.Sprintf("+%d ", ds.Insertions)) +
				diffDel.Render(fmt.Sprintf("-%d", ds.Deletions))
			ph := cellPlaceholder(ri, lipgloss.Width(styledDiff))
			rows = append(rows, table.Row{
				slugFromWorktree(wt.Name), wt.Branch, wt.Agent, wt.Issue, ph,
			})
			replacements = append(replacements, [2]string{ph, styledDiff})
			ri++
		} else {
			rows = append(rows, table.Row{
				slugFromWorktree(wt.Name), wt.Branch, wt.Agent, wt.Issue, "",
			})
		}
	}

	var content string
	if len(worktrees) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No worktrees — builders create them automatically", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = styledTableView(t, replacements) + "\n"
	}

	title := fmt.Sprintf("[w] WORKTREES (%d) — [Enter] view diff", len(m.data.Worktrees))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[w] WORKTREES (%d/%d) filter: %s", len(worktrees), len(m.data.Worktrees), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

func (m Model) renderDiff() string {
	title := "DIFF"
	if m.selectedWorktreeName != "" {
		title = "DIFF: " + slugFromWorktree(m.selectedWorktreeName)
	}

	if m.diffLoading {
		return panel(title, renderEmpty("Loading diff…", availableWidth(m.width)), panelWidth(m.width))
	}

	if m.diffContent == "" || m.diffContent == "(no diff available)" || m.diffContent == "(no changes)" {
		return panel(title, renderEmpty("No changes", availableWidth(m.width)), panelWidth(m.width))
	}

	lines := splitLines(m.diffContent)

	// Apply horizontal scroll on raw lines before styling so ANSI codes
	// are not split mid-sequence.
	shifted := make([]string, len(lines))
	for i, l := range lines {
		shifted[i] = hshiftLine(l, m.diffHScroll)
	}

	styledLines := make([]string, len(shifted))
	for i, l := range shifted {
		// Use original (pre-shift) line for prefix detection so the diff
		// prefix character (+/-/@@) is always checked even when scrolled.
		orig := lines[i]
		switch {
		case strings.HasPrefix(orig, "diff --git"), strings.HasPrefix(orig, "+++"), strings.HasPrefix(orig, "---"):
			styledLines[i] = diffHeader.Render(l)
		case strings.HasPrefix(orig, "@@"):
			styledLines[i] = diffHunk.Render(l)
		case strings.HasPrefix(orig, "+"):
			styledLines[i] = diffAdd.Render(l)
		case strings.HasPrefix(orig, "-"):
			styledLines[i] = diffDel.Render(l)
		default:
			styledLines[i] = l
		}
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(styledLines, m.diffScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	hScrollInfo := ""
	if m.diffHScroll > 0 {
		hScrollInfo = idleStyle.Render(fmt.Sprintf(" ←%d", m.diffHScroll))
	}

	return panelNoTruncate(title+scrollInfo+hScrollInfo, viewContent+"\n", panelWidth(m.width))
}

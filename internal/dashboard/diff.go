package dashboard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

func (m Model) renderWorktrees() string {
	worktrees := m.filteredWorktrees()

	avail := availableWidth(m.width)
	const numColsDiff = 5
	avail -= numColsDiff * 2
	nameW := proportionalWidth(avail, 25, 10)
	branchW := proportionalWidth(avail, 25, 10)
	agentW := proportionalWidth(avail, 14, 8)
	issueW := proportionalWidth(avail, 14, 6)
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
		return panel(title, renderEmpty(m.spinner.View()+" Loading diff…", availableWidth(m.width)), panelWidth(m.width))
	}

	if m.diffContent == "" || m.diffContent == "(no diff available)" || m.diffContent == "(no changes)" {
		return panel(title, renderEmpty("No changes", availableWidth(m.width)), panelWidth(m.width))
	}

	lines := splitLines(m.diffContent)

	vp := m.diffVP
	vp.StyleLineFunc = func(i int) lipgloss.Style {
		if i >= len(lines) {
			return lipgloss.Style{}
		}
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			return diffHeader
		case strings.HasPrefix(line, "@@"):
			return diffHunk
		case strings.HasPrefix(line, "+"):
			return diffAdd
		case strings.HasPrefix(line, "-"):
			return diffDel
		default:
			return lipgloss.Style{}
		}
	}
	vp.SetContentLines(lines)
	vp.SetYOffset(m.diffYOff)
	vp.SetXOffset(m.diffXOff)
	scrollInfo := vpScrollIndicator(vp)

	hScrollInfo := ""
	if vp.XOffset() > 0 {
		hScrollInfo = idleStyle.Render(fmt.Sprintf(" ←%d", vp.XOffset()))
	}

	return panelNoTruncate(title+scrollInfo+hScrollInfo, vp.View()+"\n", panelWidth(m.width))
}

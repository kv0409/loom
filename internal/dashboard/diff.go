package dashboard

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) renderWorktrees() string {
	worktrees := m.filteredWorktrees()

	avail := availableWidth(m.width)
	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(worktrees), vRows)

	headers := []string{"NAME", "BRANCH", "AGENT", "ISSUE", "DIFF"}

	rows := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		wt := worktrees[i]
		diff := ""
		if ds := m.data.DiffStats[wt.Name]; ds != nil && ds.FilesChanged > 0 {
			diff = fmt.Sprintf("%df ", ds.FilesChanged) +
				diffAdd.Render(fmt.Sprintf("+%d ", ds.Insertions)) +
				diffDel.Render(fmt.Sprintf("-%d", ds.Deletions))
		}
		rows = append(rows, []string{slugFromWorktree(wt.Name), wt.Branch, wt.Agent, wt.Issue, diff})
	}

	var content string
	if len(worktrees) == 0 {
		t := newLGTable(headers, nil, -1, avail, nil)
		content = t.Render() + "\n" + renderEmpty("No worktrees — builders create them automatically", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail, nil)
		content = t.Render() + "\n"
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

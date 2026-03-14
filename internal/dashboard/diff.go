package dashboard

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/karanagi/loom/internal/worktree"
)

func fetchDiff(wtPath string) string {
	base := worktree.DefaultBranch(wtPath)
	cmd := exec.Command("git", "diff", base+"...HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return "(no diff available)"
	}
	if len(out) == 0 {
		return "(no changes)"
	}
	return string(out)
}

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
	for i := start; i < end; i++ {
		wt := worktrees[i]
		ds := m.data.diffStats[wt.Name]
		diffStr := ""
		if ds != nil && ds.FilesChanged > 0 {
			filesStr := fmt.Sprintf("%df ", ds.FilesChanged)
			insStr := diffAdd.Render(fmt.Sprintf("+%d ", ds.Insertions))
			delStr := diffDel.Render(fmt.Sprintf("-%d", ds.Deletions))
			diffStr = filesStr + insStr + delStr
		}
		rows = append(rows, table.Row{
			slugFromWorktree(wt.Name), wt.Branch, wt.Agent, wt.Issue, diffStr,
		})
	}

	var content string
	if len(worktrees) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No worktrees — builders create them automatically", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = t.View() + "\n"
	}

	title := fmt.Sprintf("[w] WORKTREES (%d) — [Enter] view diff", len(m.data.worktrees))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[w] WORKTREES (%d/%d) filter: %s", len(worktrees), len(m.data.worktrees), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

func (m Model) renderDiff() string {
	title := "DIFF"
	if m.selectedWorktree < len(m.data.worktrees) {
		title = "DIFF: " + slugFromWorktree(m.data.worktrees[m.selectedWorktree].Name)
	}

	if m.diffContent == "" || m.diffContent == "(no diff available)" || m.diffContent == "(no changes)" {
		return panel(title, renderEmpty("No changes", availableWidth(m.width)), panelWidth(m.width))
	}

	lines := splitLines(m.diffContent)
	styledLines := make([]string, len(lines))
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff --git"), strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			styledLines[i] = diffHeader.Render(l)
		case strings.HasPrefix(l, "@@"):
			styledLines[i] = diffHunk.Render(l)
		case strings.HasPrefix(l, "+"):
			styledLines[i] = diffAdd.Render(l)
		case strings.HasPrefix(l, "-"):
			styledLines[i] = diffDel.Render(l)
		default:
			styledLines[i] = l
		}
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(styledLines, m.diffScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel(title+scrollInfo, viewContent+"\n", panelWidth(m.width))
}

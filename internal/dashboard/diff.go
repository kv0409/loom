package dashboard

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAdd    = lipgloss.NewStyle().Foreground(colGreen)
	diffDel    = lipgloss.NewStyle().Foreground(colRed)
	diffHunk   = lipgloss.NewStyle().Foreground(colCyan)
	diffHeader = lipgloss.NewStyle().Bold(true).Foreground(colYellow)
)

func fetchDiff(wtPath string) string {
	cmd := exec.Command("git", "diff", "main...HEAD")
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

	// Proportional column widths.
	avail := m.width - 6
	if avail < 40 {
		avail = 40
	}
	nameW := max(10, avail*25/100)
	branchW := max(10, avail*25/100)
	agentW := max(8, avail*14/100)
	issueW := max(6, avail*14/100)
	diffW := max(6, avail-nameW-branchW-agentW-issueW)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%-%ds %%s", nameW, branchW, agentW, issueW)
	content := fmt.Sprintf(fmtStr+"\n", "NAME", "BRANCH", "AGENT", "ISSUE", "DIFF")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"
	content += "\n"
	for i, wt := range worktrees {
		diffStr := ""
		if ds := m.data.diffStats[wt.Name]; ds != nil && ds.FilesChanged > 0 {
			diffStr = fmt.Sprintf("%df +%d -%d", ds.FilesChanged, ds.Insertions, ds.Deletions)
		}
		plain := fmt.Sprintf(fmtStr,
			truncate(slugFromWorktree(wt.Name), nameW), truncate(wt.Branch, branchW), truncate(wt.Agent, agentW), truncate(wt.Issue, issueW), truncate(diffStr, diffW))
		if diffStr != "" {
			// Render line without diff, then append colored diff stats
			base := fmt.Sprintf(fmtStr,
				truncate(slugFromWorktree(wt.Name), nameW), truncate(wt.Branch, branchW), truncate(wt.Agent, agentW), truncate(wt.Issue, issueW), "")
			colored := activeStyle.Render(truncate(diffStr, diffW))
			line := base + colored
			if i == m.cursor {
				line = selectedStyle.Render(plain)
			}
			content += line + "\n"
		} else {
			line := plain
			if i == m.cursor {
				line = selectedStyle.Render(line)
			}
			content += line + "\n"
		}
	}
	if len(worktrees) == 0 {
		content += renderEmpty("No worktrees — builders create them automatically", m.width-6)
	}
	title := fmt.Sprintf("[w] WORKTREES (%d) — [Enter] view diff", len(m.data.worktrees))
	if m.searchQuery != "" {
		title = fmt.Sprintf("[w] WORKTREES (%d/%d) filter: %s", len(worktrees), len(m.data.worktrees), m.searchQuery)
	}
	return panel(title, content, m.width-2)
}

func (m Model) renderDiff() string {
	title := "DIFF"
	if m.selectedWorktree < len(m.data.worktrees) {
		title = "DIFF: " + slugFromWorktree(m.data.worktrees[m.selectedWorktree].Name)
	}

	if m.diffContent == "" || m.diffContent == "(no diff available)" || m.diffContent == "(no changes)" {
		return panel(title, renderEmpty("No changes", m.width-6), m.width-2)
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

	viewH := m.height - 6
	if viewH < 1 {
		viewH = 1
	}
	viewContent, clampedScroll, total := renderViewport(styledLines, m.diffScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel(title+scrollInfo, viewContent+"\n", m.width-2)
}

package dashboard

import (
	"fmt"
	"os/exec"
	"strings"

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

	// Proportional column widths.
	avail := availableWidth(m.width)
	nameW := proportionalWidth(avail, 25, 10)
	branchW := proportionalWidth(avail, 25, 10)
	agentW := proportionalWidth(avail, 14, 8)
	issueW := proportionalWidth(avail, 14, 6)
	diffW := max(6, avail-nameW-branchW-agentW-issueW)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%-%ds %%s", nameW, branchW, agentW, issueW)
	content := fmt.Sprintf(fmtStr+"\n", "NAME", "BRANCH", "AGENT", "ISSUE", "DIFF")
	content += separator(m.width)
	content += "\n"
	for i, wt := range worktrees {
		ds := m.data.diffStats[wt.Name]
		diffStr := ""
		if ds != nil && ds.FilesChanged > 0 {
			diffStr = fmt.Sprintf("%df +%d -%d", ds.FilesChanged, ds.Insertions, ds.Deletions)
		}
		plain := fmt.Sprintf(fmtStr,
			truncate(slugFromWorktree(wt.Name), nameW), truncate(wt.Branch, branchW), truncate(wt.Agent, agentW), truncate(wt.Issue, issueW), truncate(diffStr, diffW))
		if diffStr != "" {
			// Render line without diff, then append semantically colored diff stats
			base := fmt.Sprintf(fmtStr,
				truncate(slugFromWorktree(wt.Name), nameW), truncate(wt.Branch, branchW), truncate(wt.Agent, agentW), truncate(wt.Issue, issueW), "")
			filesStr := fmt.Sprintf("%df ", ds.FilesChanged)
			insStr := fmt.Sprintf("+%d ", ds.Insertions)
			delStr := fmt.Sprintf("-%d", ds.Deletions)
			colored := truncate(filesStr+diffAdd.Render(insStr)+diffDel.Render(delStr), diffW)
			line := base + colored
			if i == m.cursor {
				line = selectedRow(plain)
			}
			content += line + "\n"
		} else {
			line := plain
			if i == m.cursor {
				line = selectedRow(line)
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

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(styledLines, m.diffScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel(title+scrollInfo, viewContent+"\n", m.width-2)
}

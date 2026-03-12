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
	for i, wt := range m.data.worktrees {
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
			} else if i == m.hoverRow {
				line = hoverStyle.Render(plain)
			}
			content += line + "\n"
		} else {
			line := plain
			if i == m.cursor {
				line = selectedStyle.Render(line)
			} else if i == m.hoverRow {
				line = hoverStyle.Render(line)
			}
			content += line + "\n"
		}
	}
	if len(m.data.worktrees) == 0 {
		content += "  No worktrees active. Builders create them automatically.\n"
	}
	return panel(fmt.Sprintf("WORKTREES (%d) — [Enter] view diff", len(m.data.worktrees)), content, m.width-2)
}

func (m Model) renderDiff() string {
	lines := splitLines(m.diffContent)
	viewH := m.height - 5
	if viewH < 1 {
		viewH = 1
	}
	start := m.cursor
	if start > len(lines)-viewH {
		start = len(lines) - viewH
	}
	if start < 0 {
		start = 0
	}
	end := start + viewH
	if end > len(lines) {
		end = len(lines)
	}

	var out string
	for _, l := range lines[start:end] {
		switch {
		case strings.HasPrefix(l, "diff --git"), strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			out += diffHeader.Render(l) + "\n"
		case strings.HasPrefix(l, "@@"):
			out += diffHunk.Render(l) + "\n"
		case strings.HasPrefix(l, "+"):
			out += diffAdd.Render(l) + "\n"
		case strings.HasPrefix(l, "-"):
			out += diffDel.Render(l) + "\n"
		default:
			out += l + "\n"
		}
	}

	title := "DIFF"
	if m.selectedWorktree < len(m.data.worktrees) {
		title = "DIFF: " + slugFromWorktree(m.data.worktrees[m.selectedWorktree].Name)
	}
	return panel(title, out, m.width-2)
}

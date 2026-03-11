package dashboard

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAdd  = lipgloss.NewStyle().Foreground(colGreen)
	diffDel  = lipgloss.NewStyle().Foreground(colRed)
	diffHunk = lipgloss.NewStyle().Foreground(colCyan)
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
	content := fmt.Sprintf("  %-30s %-30s %-15s %s\n", "NAME", "BRANCH", "AGENT", "ISSUE")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"
	for i, wt := range m.data.worktrees {
		line := fmt.Sprintf("  %-30s %-30s %-15s %s",
			truncate(slugFromWorktree(wt.Name), 30), truncate(wt.Branch, 30), wt.Agent, wt.Issue)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		content += line + "\n"
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
		case strings.HasPrefix(l, "+"):
			out += diffAdd.Render(l) + "\n"
		case strings.HasPrefix(l, "-"):
			out += diffDel.Render(l) + "\n"
		case strings.HasPrefix(l, "@@"):
			out += diffHunk.Render(l) + "\n"
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

package dashboard

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/issue"
)

var kanbanColumns = []string{"open", "assigned", "in-progress", "blocked", "review", "done", "cancelled"}

// kanbanBuckets returns issues grouped by status column.
func (m Model) kanbanBuckets() map[string][]*issue.Issue {
	buckets := map[string][]*issue.Issue{}
	for _, iss := range m.data.issues {
		buckets[iss.Status] = append(buckets[iss.Status], iss)
	}
	return buckets
}

func (m *Model) clampKanbanRow() {
	buckets := m.kanbanBuckets()
	col := kanbanColumns[m.kanbanCol]
	n := len(buckets[col])
	if n == 0 {
		m.kanbanRow = 0
	} else if m.kanbanRow >= n {
		m.kanbanRow = n - 1
	}
}

func (m Model) kanbanSelectedIssue() *issue.Issue {
	buckets := m.kanbanBuckets()
	col := kanbanColumns[m.kanbanCol]
	items := buckets[col]
	if m.kanbanRow < len(items) {
		return items[m.kanbanRow]
	}
	return nil
}

func (m Model) renderKanban() string {
	buckets := m.kanbanBuckets()

	minColW := 24
	maxVisibleCols := m.width / minColW
	if maxVisibleCols < 2 {
		maxVisibleCols = 2
	}
	if maxVisibleCols > len(kanbanColumns) {
		maxVisibleCols = len(kanbanColumns)
	}

	startCol := m.kanbanCol - maxVisibleCols/2
	if startCol < 0 {
		startCol = 0
	}
	if startCol+maxVisibleCols > len(kanbanColumns) {
		startCol = len(kanbanColumns) - maxVisibleCols
	}
	visibleCols := kanbanColumns[startCol : startCol+maxVisibleCols]

	colW := m.width/maxVisibleCols - 1
	titleW := colW - 6

	var cols []string
	for i, col := range visibleCols {
		ci := startCol + i
		items := buckets[col]
		content := ""
		for ri, iss := range items {
			line := fmt.Sprintf("%s %s", iss.ID, truncate(iss.Title, titleW))
			if m.view == viewKanban && ci == m.kanbanCol && ri == m.kanbanRow {
				content += "  " + selectedStyle.Render(line) + "\n"
			} else {
				content += "  " + statusStyle(col).Render(line) + "\n"
			}
		}
		if content == "" {
			content = renderEmpty("empty", colW-4)
		}
		cols = append(cols, panel(fmt.Sprintf("%s (%d)", col, len(items)), content, colW))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	var indicator string
	if startCol > 0 {
		indicator += fmt.Sprintf("← %d more  ", startCol)
	}
	endCol := startCol + maxVisibleCols
	if endCol < len(kanbanColumns) {
		indicator += fmt.Sprintf("%d more →", len(kanbanColumns)-endCol)
	}
	if indicator != "" {
		board += "\n" + helpStyle.Render("  "+indicator)
	}

	header := panelIssues.Render(" [b] BOARD ") + "\n"
	return header + board
}

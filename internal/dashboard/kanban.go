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

	colW := m.width/len(kanbanColumns) - 1
	if colW < 20 {
		colW = 20
	}

	var cols []string
	for ci, col := range kanbanColumns {
		items := buckets[col]
		content := ""
		for ri, iss := range items {
			line := fmt.Sprintf("%s %s", iss.ID, truncate(iss.Title, 18))
			if m.view == viewKanban && ci == m.kanbanCol && ri == m.kanbanRow {
				content += "  " + selectedStyle.Render(line) + "\n"
			} else {
				content += "  " + statusStyle(col).Render(line) + "\n"
			}
		}
		if content == "" {
			content = "  (empty)\n"
		}
		cols = append(cols, panel(fmt.Sprintf("%s (%d)", col, len(items)), content, colW))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

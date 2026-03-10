package dashboard

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderKanban() string {
	columns := []string{"open", "assigned", "in-progress", "blocked", "review", "done"}
	buckets := map[string][]string{}
	for _, iss := range m.data.issues {
		s := iss.Status
		if _, ok := buckets[s]; !ok {
			buckets[s] = nil
		}
		line := fmt.Sprintf("%s %s", iss.ID, truncate(iss.Title, 18))
		buckets[s] = append(buckets[s], statusStyle(s).Render(line))
	}

	colW := m.width/len(columns) - 1
	if colW < 20 {
		colW = 20
	}

	var cols []string
	for _, col := range columns {
		content := ""
		for _, line := range buckets[col] {
			content += "  " + line + "\n"
		}
		if content == "" {
			content = "  (empty)\n"
		}
		cols = append(cols, panel(fmt.Sprintf("%s (%d)", col, len(buckets[col])), content, colW))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

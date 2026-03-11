package dashboard

import (
	"fmt"
	"strings"

	"github.com/karanagi/loom/internal/memory"
)

func (m Model) renderMemory() string {
	content := fmt.Sprintf("  %-10s %-12s %-36s %s\n", "ID", "TYPE", "TITLE", "BY")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	for i, e := range m.data.memories {
		line := fmt.Sprintf("  %-10s %-12s %-36s %s",
			e.ID, e.Type, truncate(e.Title, 36), memory.ByField(e))
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("MEMORY (%d entries)", len(m.data.memories)), content, m.width-2)
}

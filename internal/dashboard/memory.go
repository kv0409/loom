package dashboard

import (
	"fmt"
	"strings"

	"github.com/karanagi/loom/internal/memory"
)

func (m Model) renderMemory() string {
	s := headerStyle.Render(fmt.Sprintf("MEMORY (%d entries)", len(m.data.memories))) + "\n\n"
	s += fmt.Sprintf("  %-10s %-12s %-36s %s\n", "ID", "TYPE", "TITLE", "BY")
	s += "  " + strings.Repeat("─", 70) + "\n"

	for i, e := range m.data.memories {
		line := fmt.Sprintf("  %-10s %-12s %-36s %s",
			e.ID, e.Type, truncate(e.Title, 36), memory.ByField(e))
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		s += line + "\n"
	}
	return s
}

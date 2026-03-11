package dashboard

import (
	"fmt"
	"strings"

	"github.com/karanagi/loom/internal/memory"
)

func (m Model) renderMemory() string {
	// Proportional column widths.
	avail := m.width - 6
	if avail < 40 {
		avail = 40
	}
	idW := max(6, avail*12/100)
	typeW := max(8, avail*14/100)
	byW := max(6, avail*14/100)
	titleW := max(10, avail-idW-typeW-byW)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%s", idW, typeW, titleW)
	content := fmt.Sprintf(fmtStr+"\n", "ID", "TYPE", "TITLE", "BY")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	for i, e := range m.data.memories {
		line := fmt.Sprintf(fmtStr,
			truncate(e.ID, idW), truncate(e.Type, typeW), truncate(e.Title, titleW), truncate(memory.ByField(e), byW))
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		} else if i == m.hoverRow {
			line = hoverStyle.Render(line)
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("MEMORY (%d entries)", len(m.data.memories)), content, m.width-2)
}

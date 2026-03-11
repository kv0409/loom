package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderMail() string {
	// Proportional column widths.
	avail := m.width - 6
	if avail < 40 {
		avail = 40
	}
	timeW := max(5, avail*10/100)
	routeW := max(8, avail*18/100)
	typeW := max(5, avail*10/100)
	subjW := max(10, avail-timeW-routeW-typeW)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%s", timeW, routeW, typeW)
	content := fmt.Sprintf(fmtStr+"\n", "TIME", "FROM → TO", "TYPE", "SUBJECT")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	for i, msg := range m.data.messages {
		route := fmt.Sprintf("%s→%s", msg.From, msg.To)
		line := fmt.Sprintf(fmtStr,
			msg.Timestamp.Format("15:04"), truncate(route, routeW), msg.Type, truncate(msg.Subject, subjW))
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		} else if i == m.hoverRow {
			line = hoverStyle.Render(line)
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("MAIL (%d messages, %d unread)", len(m.data.messages), m.data.unread), content, m.width-2)
}

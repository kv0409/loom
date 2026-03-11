package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderMail() string {
	content := fmt.Sprintf("  %-8s %-14s %-8s %s\n", "TIME", "FROM → TO", "TYPE", "SUBJECT")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	for i, msg := range m.data.messages {
		route := fmt.Sprintf("%s→%s", msg.From, msg.To)
		line := fmt.Sprintf("  %-8s %-14s %-8s %s",
			msg.Timestamp.Format("15:04"), truncate(route, 14), msg.Type, truncate(msg.Subject, 35))
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("MAIL (%d messages, %d unread)", len(m.data.messages), m.data.unread), content, m.width-2)
}

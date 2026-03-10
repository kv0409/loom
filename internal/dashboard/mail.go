package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderMail() string {
	s := headerStyle.Render(fmt.Sprintf("MAIL (%d messages, %d unread)", len(m.data.messages), m.data.unread)) + "\n\n"
	s += fmt.Sprintf("  %-8s %-14s %-8s %s\n", "TIME", "FROM → TO", "TYPE", "SUBJECT")
	s += "  " + strings.Repeat("─", 70) + "\n"

	for i, msg := range m.data.messages {
		route := fmt.Sprintf("%s→%s", msg.From, msg.To)
		line := fmt.Sprintf("  %-8s %-14s %-8s %s",
			msg.Timestamp.Format("15:04"), truncate(route, 14), msg.Type, truncate(msg.Subject, 35))
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		s += line + "\n"
	}
	return s
}

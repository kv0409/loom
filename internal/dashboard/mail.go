package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderMail() string {
	messages := m.filteredMessages()

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
	content += "\n"

	if len(messages) == 0 {
		content += renderEmpty("No messages yet", m.width-6)
	}

	visibleRows := m.height - 9
	if visibleRows < 1 {
		visibleRows = 1
	}
	start := m.cursor - visibleRows + 1
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(messages) {
		end = len(messages)
	}

	for i := start; i < end; i++ {
		msg := messages[i]
		route := fmt.Sprintf("%s→%s", msg.From, msg.To)
		line := fmt.Sprintf(fmtStr,
			msg.Timestamp.Format("15:04"), truncate(route, routeW), msg.Type, truncate(msg.Subject, subjW))
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		}
		content += line + "\n"
	}

	title := fmt.Sprintf("MAIL (%d messages, %d unread)", len(m.data.messages), m.data.unread)
	if m.searchQuery != "" {
		title = fmt.Sprintf("MAIL (%d/%d) filter: %s", len(messages), len(m.data.messages), m.searchQuery)
	}
	return panel(title, content, m.width-2)
}

func (m Model) renderMailDetail() string {
	messages := m.filteredMessages()
	if m.cursor >= len(messages) {
		return "No message selected"
	}
	msg := messages[m.cursor]

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s", titleStyle.Render(msg.Subject)))
	lines = append(lines, fmt.Sprintf("  From: %-16s To: %s", msg.From, msg.To))
	lines = append(lines, fmt.Sprintf("  Type: %-16s Time: %s", msg.Type, msg.Timestamp.Format("2006-01-02 15:04:05")))
	if msg.Ref != "" {
		lines = append(lines, fmt.Sprintf("  Ref: %s", msg.Ref))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+headerStyle.Render("BODY"))
	if msg.Body != "" {
		maxW := m.width - 8
		if maxW < 40 {
			maxW = 40
		}
		for _, bl := range strings.Split(msg.Body, "\n") {
			for len(bl) > maxW {
				lines = append(lines, "  "+bl[:maxW])
				bl = bl[maxW:]
			}
			lines = append(lines, "  "+bl)
		}
	} else {
		lines = append(lines, "  (no body)")
	}

	viewH := m.height - 6
	if viewH < 1 {
		viewH = 1
	}
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Mail: "+truncate(msg.Subject, 40)+scrollInfo, viewContent+"\n", m.width-2)
}

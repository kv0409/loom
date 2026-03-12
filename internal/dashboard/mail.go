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

func (m Model) renderMailDetail() string {
	if m.cursor >= len(m.data.messages) {
		return "No message selected"
	}
	msg := m.data.messages[m.cursor]

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

	// Viewport scroll
	viewH := m.height - 5
	if viewH < 1 {
		viewH = 1
	}
	scroll := m.detailScroll
	maxScroll := len(lines) - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + viewH
	if end > len(lines) {
		end = len(lines)
	}

	var s string
	for _, l := range lines[scroll:end] {
		s += l + "\n"
	}

	return panel("Mail: "+truncate(msg.Subject, 40), s, m.width-2)
}

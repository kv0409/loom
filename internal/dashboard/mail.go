package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
)

func (m Model) renderMail() string {
	messages := m.filteredMessages()

	avail := availableWidth(m.width)
	ws := colWidths(avail, []struct{ pct, min int }{{10, 5}, {18, 8}, {10, 5}})
	timeW, routeW, typeW := ws[0], ws[1], ws[2]
	subjW := max(10, avail-timeW-routeW-typeW)

	cols := []table.Column{
		{Title: "TIME", Width: timeW},
		{Title: "FROM → TO", Width: routeW},
		{Title: "TYPE", Width: typeW},
		{Title: "SUBJECT", Width: subjW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(messages), vRows)

	rows := make([]table.Row, 0, end-start)
	for i := start; i < end; i++ {
		msg := messages[i]
		route := fmt.Sprintf("%s→%s", msg.From, msg.To)
		rows = append(rows, table.Row{fmtTime(msg.Timestamp, false), route, msg.Type, msg.Subject})
	}

	var content string
	if len(messages) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No messages yet", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = t.View() + "\n"
	}

	title := fmt.Sprintf("[m] MAIL (%d messages, %d unread)", len(m.data.messages), m.data.unread)
	if m.searchQuery != "" {
		title = fmt.Sprintf("[m] MAIL (%d/%d) filter: %s", len(messages), len(m.data.messages), m.searchQuery)
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
	lines = append(lines, fmt.Sprintf("  Type: %-16s Time: %s", msg.Type, fmtTimeFull(msg.Timestamp)))
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

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Mail: "+truncate(msg.Subject, 40)+scrollInfo, viewContent+"\n", m.width-2)
}

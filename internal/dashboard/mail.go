package dashboard

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

// sortedMessages returns a copy of filteredMessages sorted by priority
// (critical first), then unread first, then newest. This is the single
// source of truth for mail display order — used by render, Enter, and reply.
func (m Model) sortedMessages() []*backend.Message {
	filtered := m.filteredMessages()
	messages := make([]*backend.Message, len(filtered))
	copy(messages, filtered)
	sort.SliceStable(messages, func(i, j int) bool {
		if mailPriorityWeight(messages[i].Priority) != mailPriorityWeight(messages[j].Priority) {
			return mailPriorityWeight(messages[i].Priority) > mailPriorityWeight(messages[j].Priority)
		}
		if messages[i].Read != messages[j].Read {
			return !messages[i].Read
		}
		return messages[i].Timestamp.After(messages[j].Timestamp)
	})
	return messages
}

func (m Model) renderMail() string {
	messages := m.sortedMessages()

	avail := availableWidth(m.width)
	const numCols = 6
	avail -= numCols * 2

	prioW := proportionalWidth(avail, 8, 6)
	fromW := proportionalWidth(avail, 14, 8)
	toW := proportionalWidth(avail, 14, 8)
	typeW := proportionalWidth(avail, 10, 6)
	timeW := proportionalWidth(avail, 10, 7)
	subjW := max(10, avail-prioW-fromW-toW-typeW-timeW)

	cols := []table.Column{
		{Title: "PRIO", Width: prioW},
		{Title: "FROM", Width: fromW},
		{Title: "TO", Width: toW},
		{Title: "TYPE", Width: typeW},
		{Title: "TIME", Width: timeW},
		{Title: "SUBJECT", Width: subjW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(messages), vRows)

	rows := make([]table.Row, 0, end-start)
	var replacements [][2]string
	ri := 0
	for i := start; i < end; i++ {
		msg := messages[i]
		styledPrio := mailPriorityTag(msg.Priority)
		phPrio := cellPlaceholder(ri, lipgloss.Width(styledPrio))

		subj := msg.Subject
		if !msg.Read {
			subj = "● " + subj
		}

		rows = append(rows, table.Row{phPrio, msg.From, msg.To, msg.Type, fmtTime(msg.Timestamp, false), truncate(subj, subjW)})
		replacements = append(replacements, [2]string{phPrio, styledPrio})
		ri++
	}

	var content string
	if len(messages) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No queued messages", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = styledTableView(t, replacements) + "\n"
	}

	var unread int
	for _, msg := range messages {
		if !msg.Read {
			unread++
		}
	}
	title := fmt.Sprintf("[m] MAIL (%d messages, %d unread)", len(m.data.Messages), unread)
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[m] MAIL (%d/%d) filter: %s", len(messages), len(m.data.Messages), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

func mailPriorityWeight(priority string) int {
	switch priority {
	case "critical":
		return 3
	case "normal":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func mailPriorityTag(priority string) string {
	label := priority
	style := idleStyle
	switch priority {
	case "critical":
		style = deadStyle
	case "normal":
		style = reviewStyle
	case "low":
		style = idleStyle
	default:
		label = "unknown"
	}
	return style.Render(label)
}

func (m Model) renderMailDetail() string {
	messages := m.sortedMessages()
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
		maxW := detailContentWidth(m.width)
		lines = append(lines, wrapLines(msg.Body, maxW, "  ")...)
	} else {
		lines = append(lines, "  (no body)")
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Mail: "+truncate(msg.Subject, 40)+scrollInfo, viewContent+"\n", panelWidth(m.width))
}

package dashboard

import (
	"fmt"
	"sort"

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
	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(messages), vRows)

	headers := []string{"PRIO", "FROM", "TO", "TYPE", "TIME", "SUBJECT"}
	rows := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		msg := messages[i]
		styledPrio := mailPriorityTag(msg.Priority)

		subj := msg.Subject
		if !msg.Read {
			subj = "● " + subj
		}

		rows = append(rows, []string{styledPrio, msg.From, msg.To, msg.Type, fmtTime(msg.Timestamp, false), subj})
	}

	var content string
	if len(messages) == 0 {
		t := newLGTable(headers, nil, -1, avail)
		content = t.Render() + "\n" + renderEmpty("No queued messages", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail)
		content = t.Render() + "\n"
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

	vp := m.detailVP
	vp.SetContentLines(lines)
	vp.SetYOffset(m.detailYOff)
	scrollInfo := vpScrollIndicator(vp)

	return panel("Mail: "+truncate(msg.Subject, 40)+scrollInfo, vp.View()+"\n", panelWidth(m.width))
}

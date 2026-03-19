package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
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
		prio := msg.Priority
		if prio == "" {
			prio = "unknown"
		}
		subj := msg.Subject
		if !msg.Read {
			subj = "● " + subj
		}
		rows = append(rows, []string{prio, msg.From, msg.To, msg.Type, fmtTime(msg.Timestamp, false), subj})
	}

	styler := func(row, col int, isSelected bool) lipgloss.Style {
		base := lgTableCellStyle
		if isSelected {
			base = lgTableSelectedStyle
		}
		dataIdx := start + row
		if dataIdx >= len(messages) {
			return base
		}
		if col == 0 { // PRIO
			return base.Foreground(mailPriorityColor(messages[dataIdx].Priority))
		}
		return base
	}

	var content string
	if len(messages) == 0 {
		t := newLGTable(headers, nil, -1, avail, nil)
		content = t.Render() + "\n" + renderEmpty("No queued messages", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail, styler)
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

func (m Model) renderMailDetail() string {
	messages := m.sortedMessages()
	if m.cursor >= len(messages) {
		return "No message selected"
	}
	msg := messages[m.cursor]

	// --- Fixed header: metadata ---
	var header []string
	header = append(header, fmt.Sprintf("  %s", titleStyle.Render(msg.Subject)))
	header = append(header, fmt.Sprintf("  From: %-16s To: %s", msg.From, msg.To))
	header = append(header, fmt.Sprintf("  Type: %-16s Time: %s", msg.Type, fmtTimeFull(msg.Timestamp)))
	if msg.Ref != "" {
		header = append(header, fmt.Sprintf("  Ref: %s", msg.Ref))
	}

	// --- Scrollable body ---
	var body []string
	body = append(body, "  "+headerStyle.Render("BODY"))
	if msg.Body != "" {
		maxW := detailContentWidth(m.width)
		body = append(body, wrapLines(msg.Body, maxW, "  ")...)
	} else {
		body = append(body, "  (no body)")
	}

	headerStr := strings.Join(header, "\n")
	headerLines := strings.Count(headerStr, "\n") + 1
	vpH := scrollViewport(m.height) - headerLines
	if vpH < 1 {
		vpH = 1
	}

	vp := m.detailVP
	vp.SetHeight(vpH)
	vp.SetContentLines(body)
	vp.SetYOffset(m.detailYOff)
	scrollInfo := vpScrollIndicator(vp)

	content := headerStr + "\n" + vp.View()
	return panel("Mail: "+truncate(msg.Subject, panelWidth(m.width)-20)+scrollInfo, content, panelWidth(m.width))
}

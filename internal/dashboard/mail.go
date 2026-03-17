package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/karanagi/loom/internal/dashboard/backend"
)

func (m Model) renderMail() string {
	filtered := m.filteredMessages()
	messages := make([]*backend.Message, len(filtered))
	copy(messages, filtered)
	var unread, critical int
	byType := map[string]int{}
	for _, msg := range messages {
		if !msg.Read {
			unread++
		}
		if msg.Priority == "critical" {
			critical++
		}
		byType[msg.Type]++
	}

	// Sort messages by priority (critical first), then unread first, then newest.
	// Use the same slice for display and cursor navigation so they stay in sync.
	sort.SliceStable(messages, func(i, j int) bool {
		if mailPriorityWeight(messages[i].Priority) != mailPriorityWeight(messages[j].Priority) {
			return mailPriorityWeight(messages[i].Priority) > mailPriorityWeight(messages[j].Priority)
		}
		if messages[i].Read != messages[j].Read {
			return !messages[i].Read
		}
		return messages[i].Timestamp.After(messages[j].Timestamp)
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("  %d unread · %d critical · %d total", unread, critical, len(messages)))
	if len(byType) > 0 {
		var parts []string
		for _, name := range []string{"blocker", "question", "review-request", "completion", "status", "task"} {
			if count := byType[name]; count > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", count, name))
			}
		}
		if len(parts) > 0 {
			lines = append(lines, "  "+strings.Join(parts, idleStyle.Render(" · ")))
		}
	}
	lines = append(lines, "", "  "+headerStyle.Render("INBOX PRESSURE"))
	if len(messages) == 0 {
		lines = append(lines, "  No queued messages.")
	} else if critical > 0 {
		lines = append(lines, deadStyle.Render(fmt.Sprintf("  %d critical message%s should be handled first.", critical, suffix(critical))))
	} else if unread > 0 {
		lines = append(lines, reviewStyle.Render(fmt.Sprintf("  %d unread message%s waiting for a response or acknowledgement.", unread, suffix(unread))))
	} else {
		lines = append(lines, activeStyle.Render("  Inbox is under control."))
	}
	lines = append(lines, "", "  "+headerStyle.Render("NEXT UP"))
	if len(messages) == 0 {
		lines = append(lines, "  No recent mail.")
	} else {
		for idx, msg := range messages[:min(4, len(messages))] {
			prefix := "  "
			if idx == m.cursor {
				prefix = "▸ "
			}
			ref := ""
			if msg.Ref != "" {
				ref = idleStyle.Render(" · " + msg.Ref)
			}
			state := "read"
			if !msg.Read {
				state = "unread"
			}
			line := fmt.Sprintf("%s%s %s → %s · %s · %s%s", prefix, mailPriorityTag(msg.Priority), msg.From, msg.To, msg.Type, state, ref)
			lines = append(lines, line)
			lines = append(lines, fmt.Sprintf("    %s", truncate(msg.Subject, detailContentWidth(m.width)-4)))
		}
	}

	content := strings.Join(lines, "\n") + "\n"

	title := fmt.Sprintf("[m] MAIL (%d messages, %d unread)", len(m.data.Messages), m.data.Unread)
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
			for _, seg := range wordWrap(bl, maxW) {
				lines = append(lines, "  "+seg)
			}
		}
	} else {
		lines = append(lines, "  (no body)")
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Mail: "+truncate(msg.Subject, 40)+scrollInfo, viewContent+"\n", panelWidth(m.width))
}

package dashboard

import (
	"sort"
	"strings"

	"github.com/karanagi/loom/internal/dashboard/backend"
)

// filterChatHistory returns messages between "dashboard" and "orchestrator",
// sorted oldest-first (chronological conversation order).
func filterChatHistory(messages []*backend.Message) []*backend.Message {
	var out []*backend.Message
	for _, m := range messages {
		if (m.From == "dashboard" && m.To == "orchestrator") ||
			(m.From == "orchestrator" && m.To == "dashboard") {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

// renderChatPane renders a bottom panel showing the conversation between
// dashboard and orchestrator, with a text input prompt at the bottom.
func (m Model) renderChatPane() string {
	msgs := filterChatHistory(m.data.Messages)
	paneH := chatPaneHeight(m.height)
	maxW := detailContentWidth(m.width)

	// Build conversation lines.
	var lines []string
	for _, msg := range msgs {
		var prefix, text string
		if msg.From == "dashboard" {
			prefix = chatUserStyle.Render("→ you: ")
			text = msg.Subject
		} else {
			prefix = chatOrchestratorStyle.Render("← orch: ")
			text = msg.Subject
		}
		if msg.Body != "" {
			text += " — " + strings.ReplaceAll(msg.Body, "\n", " ")
		}
		lines = append(lines, prefix+truncate(text, maxW-10))
	}

	if len(lines) == 0 {
		lines = append(lines, emptyMsgStyle.Render("  No conversation yet"))
	}

	// Show only the last paneH-2 lines (reserve 1 for prompt, 1 for spacing).
	historyBudget := paneH - 2
	if historyBudget < 1 {
		historyBudget = 1
	}
	if len(lines) > historyBudget {
		lines = lines[len(lines)-historyBudget:]
	}

	// Append prompt line.
	prompt := chatPromptStyle.Render("❯ ") + m.chatInput()
	lines = append(lines, prompt)

	content := strings.Join(lines, "\n") + "\n"
	return panel("CHAT", content, panelWidth(m.width))
}

// chatInput returns the current chat text input display. When chat input mode
// is not active, it shows a placeholder hint.
func (m Model) chatInput() string {
	return emptyMsgStyle.Render("press C to chat")
}

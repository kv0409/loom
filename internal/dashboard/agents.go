package dashboard

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) renderAgents() string {
	s := headerStyle.Render(fmt.Sprintf("AGENTS (%d)", len(m.data.agents))) + "\n\n"
	s += fmt.Sprintf("  %-16s %-10s %-10s %-22s %-14s %s\n",
		"ID", "ROLE", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT")
	s += "  " + strings.Repeat("─", 90) + "\n"

	for i, a := range m.data.agents {
		wt := "—"
		if a.WorktreeName != "" {
			wt = truncate(a.WorktreeName, 22)
		}
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = truncate(strings.Join(a.AssignedIssues, ","), 14)
		}
		hb := relTime(a.Heartbeat)
		line := fmt.Sprintf("  %-16s %-10s %-10s %-22s %-14s %s",
			a.ID, a.Role, a.Status, wt, issues, hb)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		} else {
			line = statusStyle(a.Status).Render(line)
		}
		s += line + "\n"
	}
	return s
}

func (m Model) renderAgentDetail() string {
	if m.cursor >= len(m.data.agents) {
		return "No agent selected"
	}
	a := m.data.agents[m.cursor]

	s := headerStyle.Render("Agent: "+a.ID) + "\n\n"
	s += fmt.Sprintf("  Role:       %s\n", a.Role)
	s += fmt.Sprintf("  Status:     %s\n", statusStyle(a.Status).Render(a.Status))
	s += fmt.Sprintf("  PID:        %d\n", a.PID)
	s += fmt.Sprintf("  Tmux:       %s\n", a.TmuxTarget)
	s += fmt.Sprintf("  Spawned By: %s\n", a.SpawnedBy)
	s += fmt.Sprintf("  Spawned At: %s\n", a.SpawnedAt.Format("15:04:05"))
	s += fmt.Sprintf("  Heartbeat:  %s\n", relTime(a.Heartbeat))

	if len(a.AssignedIssues) > 0 {
		s += fmt.Sprintf("\n  Issues: %s\n", strings.Join(a.AssignedIssues, ", "))
	}
	if a.WorktreeName != "" {
		s += fmt.Sprintf("  Worktree: %s\n", a.WorktreeName)
	}

	// Show recent mail for this agent
	s += "\n  " + headerStyle.Render("RECENT MAIL") + "\n"
	count := 0
	for _, msg := range m.data.messages {
		if msg.From == a.ID || msg.To == a.ID {
			dir := "←"
			if msg.From == a.ID {
				dir = "→"
			}
			other := msg.From
			if msg.From == a.ID {
				other = msg.To
			}
			s += fmt.Sprintf("  %s %s %s: %s\n", dir, msg.Timestamp.Format("15:04"), other, truncate(msg.Subject, 40))
			count++
			if count >= 5 {
				break
			}
		}
	}
	if count == 0 {
		s += "  (none)\n"
	}

	return s
}

func relTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

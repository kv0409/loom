package dashboard

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) renderAgents() string {
	content := fmt.Sprintf("  %-16s %-10s %-12s %-22s %-14s %s\n",
		"ID", "ROLE", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT")
	content += "  " + strings.Repeat("─", 88) + "\n"

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
		statusCol := fmt.Sprintf("%s %-10s", statusIndicator(a.Status), a.Status)
		line := fmt.Sprintf("  %-16s %-10s %-12s %-22s %-14s %s",
			a.ID, a.Role, statusCol, wt, issues, hb)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		} else {
			line = statusStyle(a.Status).Render(line)
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("AGENTS (%d)", len(m.data.agents)), content, m.width-2)
}

func (m Model) renderAgentDetail() string {
	if m.cursor >= len(m.data.agents) {
		return "No agent selected"
	}
	a := m.data.agents[m.cursor]

	s := fmt.Sprintf("  Role: %-14s Status: %s %-10s Heartbeat: %s\n",
		a.Role, statusIndicator(a.Status), a.Status, relTime(a.Heartbeat))
	s += fmt.Sprintf("  Spawned by: %-10s Spawned at: %-10s PID: %d\n",
		a.SpawnedBy, a.SpawnedAt.Format("15:04:05"), a.PID)
	if a.TmuxTarget != "" {
		s += fmt.Sprintf("  Tmux: %s\n", a.TmuxTarget)
	}

	if len(a.AssignedIssues) > 0 {
		s += "\n  " + headerStyle.Render("ASSIGNED ISSUES") + "\n"
		s += fmt.Sprintf("  └── %s\n", strings.Join(a.AssignedIssues, ", "))
	}
	if a.WorktreeName != "" {
		s += fmt.Sprintf("\n  " + headerStyle.Render("WORKTREE") + ": %s\n", a.WorktreeName)
	}

	// Recent mail
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

	return panel("Agent: "+a.ID, s, m.width-2)
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

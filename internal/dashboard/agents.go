package dashboard

import (
	"fmt"
	"strings"

	"github.com/karanagi/loom/internal/dashboard/backend"
)

func (m Model) renderAgents() string {
	agents := m.filteredAgents()

	avail := availableWidth(m.width)
	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(agents), vRows)

	headers := []string{"ID", "ROLE", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT"}
	rows := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		a := agents[i]
		wt := "—"
		if a.WorktreeName != "" {
			wt = slugFromWorktree(a.WorktreeName)
		}
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = strings.Join(a.AssignedIssues, ",")
		}
		hb := fmtTime(a.Heartbeat, false)
		if a.NudgeCount > 0 {
			hb += fmt.Sprintf(" ↯%d", a.NudgeCount)
		}
		// Tree prefix is hand-rolled because it's embedded in table cells;
		// lipgloss/tree renders a full block and can't produce per-row prefixes.
		prefix := ""
		for oi, oa := range m.data.Agents {
			if oa == a && oi < len(m.data.AgentTree) {
				node := m.data.AgentTree[oi]
				for d := 0; d < node.Depth-1; d++ {
					if d < len(node.Ancestors) && node.Ancestors[d] {
						prefix += "  "
					} else {
						prefix += "│ "
					}
				}
				if node.Depth > 0 {
					if node.IsLast {
						prefix += "└ "
					} else {
						prefix += "├ "
					}
				}
				break
			}
		}

		styledID := prefix + agentPillFor(a.ID, a.ID)
		styledStatus := fmt.Sprintf("%s %s", statusIndicator(a.Status), statusPill(a.Status))
		rows = append(rows, []string{styledID, a.Role, styledStatus, wt, issues, hb})
	}

	var content string
	if len(agents) == 0 {
		t := newLGTable(headers, nil, -1, avail)
		content = t.Render() + "\n" + renderEmpty("No agents running — loom spawn to start", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail)
		content = t.Render() + "\n"
	}

	title := fmt.Sprintf("[a] AGENTS (%d)", len(m.data.Agents))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[a] AGENTS (%d/%d) filter: %s", len(agents), len(m.data.Agents), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

func (m Model) renderAgentDetail() string {
	agents := m.filteredAgents()
	if m.cursor >= len(agents) {
		return "No agent selected"
	}
	a := agents[m.cursor]

	// Build all lines, then apply scroll viewport.
	var lines []string

	lines = append(lines, fmt.Sprintf("  Role: %-14s Status: %s %s Heartbeat: %s",
		a.Role, statusIndicator(a.Status), statusPillStyle(a.Status).Render(a.Status), fmtTime(a.Heartbeat, false)))
	lines = append(lines, fmt.Sprintf("  Spawned by: %-10s Spawned at: %-10s PID: %d",
		a.SpawnedBy, fmtTimeFull(a.SpawnedAt), a.PID))
	modeStr := "ACP"
	if a.Config.Model != "" {
		modeStr += " | Model: " + a.Config.Model
	}
	lines = append(lines, "  "+modeStr)

	if len(a.AssignedIssues) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+headerStyle.Render("ASSIGNED ISSUES"))
		lines = append(lines, fmt.Sprintf("  └── %s", strings.Join(a.AssignedIssues, ", ")))
	}
	if a.WorktreeName != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  "+headerStyle.Render("WORKTREE")+": %s", slugFromWorktree(a.WorktreeName)))
	}

	if a.NudgeCount > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+headerStyle.Render("NUDGES"))
		lines = append(lines, fmt.Sprintf("  Count: %d  Last: %s", a.NudgeCount, fmtTime(a.LastNudge, false)))
	}

	// ACP output — rendered by event type
	lines = append(lines, "")
	lines = append(lines, "  "+headerStyle.Render("RECENT OUTPUT")+" (j/k to scroll)")
	events := m.agentOutputCache
	if len(events) > 0 {
		maxW := detailContentWidth(m.width)
		// Group contiguous token_chunk events into single blocks.
		type group struct {
			kind      backend.ACPKind
			timestamp string
			content   string
			title     string
		}
		var groups []group
		for _, ev := range events {
			if ev.Kind == backend.TokenChunk && len(groups) > 0 && groups[len(groups)-1].kind == backend.TokenChunk {
				groups[len(groups)-1].content += ev.Content
			} else {
				groups = append(groups, group{kind: ev.Kind, timestamp: ev.Timestamp, content: ev.Content, title: ev.Title})
			}
		}
		for i, g := range groups {
			if i > 0 {
				lines = append(lines, "")
			}
			switch g.kind {
			case backend.ToolSummary:
				line := g.title
				if line == "" {
					line = g.content
				}
				line = truncate(line, maxW)
				lines = append(lines, idleStyle.Render("  ⚙ "+line))
			case backend.TokenChunk:
				if g.timestamp != "" {
					lines = append(lines, idleStyle.Render(fmt.Sprintf("  ── %s ──", g.timestamp)))
				}
				lines = append(lines, wrapLines(g.content, maxW, "  ")...)
			default: // CompleteMessage
				if g.timestamp != "" {
					lines = append(lines, idleStyle.Render(fmt.Sprintf("  ── %s ──", g.timestamp)))
				}
				lines = append(lines, wrapLines(g.content, maxW, "  ")...)
			}
		}
	} else {
		if m.agentOutputID == a.ID {
			lines = append(lines, "  "+m.spinner.View()+" loading output...")
		} else {
			lines = append(lines, "  (waiting for output...)")
		}
	}

	// Recent mail
	lines = append(lines, "")
	lines = append(lines, "  "+headerStyle.Render("RECENT MAIL"))
	count := 0
	for _, msg := range m.data.Messages {
		if msg.From == a.ID || msg.To == a.ID {
			dir := "←"
			if msg.From == a.ID {
				dir = "→"
			}
			other := msg.From
			if msg.From == a.ID {
				other = msg.To
			}
			lines = append(lines, fmt.Sprintf("  %s %s %s: %s", dir, fmtTime(msg.Timestamp, false), other, truncate(msg.Subject, 40)))
			count++
			if count >= 5 {
				break
			}
		}
	}
	if count == 0 {
		lines = append(lines, "  No recent activity for this agent.")
	}

	// Apply scroll viewport
	vp := m.detailVP
	vp.SetContentLines(lines)
	vp.SetYOffset(m.detailYOff)
	scrollInfo := vpScrollIndicator(vp)

	return panel("Agent: "+a.ID+" [n]udge"+scrollInfo, vp.View(), panelWidth(m.width))
}

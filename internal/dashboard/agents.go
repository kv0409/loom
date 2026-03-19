package dashboard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

func (m Model) renderAgents() string {
	agents := m.filteredAgents()

	// Compute ID column width from actual tree data.
	idWidth := 16
	for _, a := range agents {
		w := len(a.ID)
		for oi, oa := range m.data.Agents {
			if oa == a && oi < len(m.data.AgentTree) {
				w += m.data.AgentTree[oi].Depth * 2
				break
			}
		}
		if w > idWidth {
			idWidth = w
		}
	}

	avail := availableWidth(m.width)
	const numColsAgents = 6
	avail -= numColsAgents * 2
	idW := min(max(idWidth+2, 16), avail*25/100)
	rem := avail - idW
	roleW := proportionalWidth(rem, 12, 14)
	statusW := statusPillWidth + 4
	issueW := proportionalWidth(rem, 20, 10)
	hbW := 10
	wtW := max(8, rem-roleW-statusW-issueW-hbW)

	cols := []table.Column{
		{Title: "ID", Width: idW},
		{Title: "ROLE", Width: roleW},
		{Title: "STATUS", Width: statusW},
		{Title: "WORKTREE", Width: wtW},
		{Title: "ISSUES", Width: issueW},
		{Title: "HEARTBEAT", Width: hbW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(agents), vRows)

	rows := make([]table.Row, 0, end-start)
	var replacements [][2]string
	ri := 0
	for i := start; i < end; i++ {
		a := agents[i]
		wt := "—"
		if a.WorktreeName != "" {
			wt = truncate(slugFromWorktree(a.WorktreeName), wtW)
		}
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = truncate(strings.Join(a.AssignedIssues, ","), issueW)
		}
		hb := fmtTime(a.Heartbeat, false)
		if a.NudgeCount > 0 {
			hb += fmt.Sprintf(" ↯%d", a.NudgeCount)
		}
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

		truncID := truncate(a.ID, idW-2) // -2 leaves room for agentPill's Padding(0,1)
		styledID := prefix + agentPillFor(truncID, a.ID)
		styledStatus := fmt.Sprintf("%s %s", statusIndicator(a.Status), statusPill(a.Status))
		phID := cellPlaceholder(ri, lipgloss.Width(prefix+agentPillPlain(truncID)))
		phStatus := cellPlaceholder(ri+1, lipgloss.Width(styledStatus))
		rows = append(rows, table.Row{phID, a.Role, phStatus, wt, issues, hb})
		replacements = append(replacements, [2]string{phID, styledID}, [2]string{phStatus, styledStatus})
		ri += 2
	}

	var content string
	if len(agents) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No agents running — loom spawn to start", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = styledTableView(t, replacements) + "\n"
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
			lines = append(lines, "  (loading output...)")
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
	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Agent: "+a.ID+" [n]udge"+scrollInfo, viewContent, panelWidth(m.width))
}

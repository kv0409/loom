package dashboard

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

func (m Model) renderAgents() string {
	agents := m.filteredAgents()

	avail := availableWidth(m.width)
	vRows := visibleRows(m.height, 10)
	start, end := listViewport(m.cursor, len(agents), vRows)

	headers := []string{"ID", "MODEL", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT"}
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

		sg := statusGlyphs[a.Status]
		if sg == "" {
			sg = "●"
		}
		model := a.Config.Model
		if model == "" {
			model = "—"
		}
		rows = append(rows, []string{prefix + a.ID, model, sg + " " + a.Status, wt, issues, hb})
	}

	styler := func(row, col int, isSelected bool) lipgloss.Style {
		base := lgTableCellStyle
		if isSelected {
			base = lgTableSelectedStyle
		}
		dataIdx := start + row
		if dataIdx >= len(agents) {
			return base
		}
		a := agents[dataIdx]
		switch col {
		case 0: // ID — agent color
			return base.Foreground(agentColor(a.ID)).Bold(true)
		case 2: // STATUS — status color
			if c, ok := statusColors[a.Status]; ok {
				return base.Foreground(c)
			}
		}
		return base
	}

	var content string
	if len(agents) == 0 {
		t := newLGTable(headers, nil, -1, avail, nil)
		content = t.Render() + "\n" + renderEmpty("No agents running — loom spawn to start", avail)
	} else {
		t := newLGTable(headers, rows, m.cursor-start, avail, styler)
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

	pw := panelWidth(m.width)
	maxW := detailContentWidth(m.width)

	// --- Fixed header: metadata ---
	headerLines := m.renderAgentHeader(a)

	// --- Scrollable output viewport ---
	outputLines := m.renderAgentOutput(a, maxW)

	// --- Fixed footer: recent mail ---
	footerLines := m.renderAgentFooter(a)

	// Compute viewport height for the output section.
	vpH := agentDetailVPHeight(m.height, len(headerLines), len(footerLines))
	if vpH < 1 {
		vpH = 1
	}

	vp := m.detailVP
	vp.SetHeight(vpH)
	vp.SetContentLines(outputLines)
	vp.SetYOffset(m.detailYOff)
	scrollInfo := vpScrollIndicator(vp)

	// Assemble: header + viewport + footer
	var lines []string
	lines = append(lines, headerLines...)
	lines = append(lines, "") // blank separator before output
	lines = append(lines, "  "+headerStyle.Render("RECENT OUTPUT")+" (j/k scroll, G bottom)"+scrollInfo)
	for _, l := range splitLines(vp.View()) {
		lines = append(lines, l)
	}
	lines = append(lines, "") // blank separator before footer
	lines = append(lines, footerLines...)

	return panel("Agent: "+a.ID+" [n]udge", strings.Join(lines, "\n"), pw)
}

// renderAgentHeader returns the fixed metadata lines for agent detail.
func (m Model) renderAgentHeader(a *backend.Agent) []string {
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
		lines = append(lines, fmt.Sprintf("  Issues: %s", strings.Join(a.AssignedIssues, ", ")))
	}
	if a.WorktreeName != "" {
		lines = append(lines, fmt.Sprintf("  Worktree: %s", slugFromWorktree(a.WorktreeName)))
	}
	if a.NudgeCount > 0 {
		lines = append(lines, fmt.Sprintf("  Nudges: %d  Last: %s", a.NudgeCount, fmtTime(a.LastNudge, false)))
	}
	return lines
}

// renderAgentOutput returns the scrollable output lines for agent detail.
func (m Model) renderAgentOutput(a *backend.Agent, maxW int) []string {
	events := m.agentOutputCache
	if len(events) == 0 {
		if m.agentOutputID == a.ID {
			return []string{"  " + m.spinner.View() + " loading output..."}
		}
		return []string{"  (waiting for output...)"}
	}

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

	var lines []string
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
	return lines
}

// renderAgentFooter returns the fixed recent-mail lines for agent detail.
func (m Model) renderAgentFooter(a *backend.Agent) []string {
	var lines []string
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
			prefix := fmt.Sprintf("  %s %s %s: ", dir, fmtTime(msg.Timestamp, false), other)
			lines = append(lines, prefix+truncate(msg.Subject, detailContentWidth(m.width)-lipgloss.Width(prefix)))
			count++
			if count >= 5 {
				break
			}
		}
	}
	if count == 0 {
		lines = append(lines, "  No recent activity for this agent.")
	}
	return lines
}

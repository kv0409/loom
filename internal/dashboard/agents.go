package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
)

func (m Model) renderAgents() string {
	agents := m.filteredAgents()

	// Compute ID column width from actual tree data.
	idWidth := 16
	for _, a := range agents {
		w := len(a.ID)
		for oi, oa := range m.data.agents {
			if oa == a && oi < len(m.data.agentTree) {
				w += m.data.agentTree[oi].depth * 2
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
	// Compute idW first so the remaining budget is correctly passed to colWidths.
	// Previously idW was computed from the same avail as the other 5 columns,
	// causing the total to exceed avail at typical terminal widths (~80-90 cols),
	// which made the rightmost columns (ISSUES, HEARTBEAT) get clipped or disappear.
	idW := min(max(idWidth+2, 16), avail*25/100)
	ws := colWidths(avail-idW, []struct{ pct, min int }{{10, 6}, {0, statusPillWidth + 4}, {22, 8}, {14, 6}, {0, 10}})
	roleW, statusW, wtW, issueW, hbW := ws[0], ws[1], ws[2], ws[3], ws[4]

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
		for oi, oa := range m.data.agents {
			if oa == a && oi < len(m.data.agentTree) {
				node := m.data.agentTree[oi]
				for d := 0; d < node.depth-1; d++ {
					if d < len(node.ancestors) && node.ancestors[d] {
						prefix += "  "
					} else {
						prefix += "│ "
					}
				}
				if node.depth > 0 {
					if node.isLast {
						prefix += "└ "
					} else {
						prefix += "├ "
					}
				}
				break
			}
		}

		truncID := truncate(a.ID, idW-2) // -2 leaves room for agentPill's Padding(0,1)
		plainID := prefix + agentPillPlain(truncID)
		styledID := prefix + agentPill(truncID)
		plainStatus := statusColPlain(a.Status)
		styledStatus := fmt.Sprintf("%s %s", statusIndicator(a.Status), statusPill(a.Status))
		rows = append(rows, table.Row{plainID, a.Role, plainStatus, wt, issues, hb})
		replacements = append(replacements, [2]string{plainID, styledID}, [2]string{plainStatus, styledStatus})
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

	title := fmt.Sprintf("[a] AGENTS (%d)", len(m.data.agents))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[a] AGENTS (%d/%d) filter: %s", len(agents), len(m.data.agents), m.searchTI.Value())
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
	if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
		modeStr := "ACP"
		if a.Config.Model != "" {
			modeStr += " | Model: " + a.Config.Model
		}
		lines = append(lines, "  "+modeStr)
	} else if a.TmuxTarget != "" {
		lines = append(lines, fmt.Sprintf("  Tmux: %s", a.TmuxTarget))
	}

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
	if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
		lines = append(lines, "")
		lines = append(lines, "  "+headerStyle.Render("RECENT OUTPUT")+" (j/k to scroll)")
		outPath := filepath.Join(m.loomRoot, "agents", a.ID+".output")
		if raw, err := os.ReadFile(outPath); err == nil && strings.TrimSpace(string(raw)) != "" {
			maxW := m.width - 8
			if maxW < 40 {
				maxW = 40
			}
			events := acp.ReadOutputFile(raw)
			// Group contiguous token_chunk events into single blocks.
			type group struct {
				kind      acp.Kind
				timestamp string
				content   string
			}
			var groups []group
			for _, ev := range events {
				if ev.Kind == acp.TokenChunk && len(groups) > 0 && groups[len(groups)-1].kind == acp.TokenChunk {
					groups[len(groups)-1].content += ev.Content
				} else {
					groups = append(groups, group{kind: ev.Kind, timestamp: ev.Timestamp, content: ev.Content})
				}
			}
			for i, g := range groups {
				if i > 0 {
					lines = append(lines, "")
				}
				switch g.kind {
				case acp.ToolSummary:
					line := g.content
					line = truncate(line, maxW)
					lines = append(lines, idleStyle.Render("  ⚙ "+line))
				case acp.TokenChunk:
					if g.timestamp != "" {
						lines = append(lines, idleStyle.Render(fmt.Sprintf("  ── %s ──", g.timestamp)))
					}
					lines = append(lines, wrapEventContent(g.content, maxW))
				default: // CompleteMessage
					if g.timestamp != "" {
						lines = append(lines, idleStyle.Render(fmt.Sprintf("  ── %s ──", g.timestamp)))
					}
					lines = append(lines, wrapEventContent(g.content, maxW))
				}
			}
		} else {
			lines = append(lines, "  (waiting for output...)")
		}
	}

	// Recent mail
	lines = append(lines, "")
	lines = append(lines, "  "+headerStyle.Render("RECENT MAIL"))
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

	hint := ""
	if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
		hint = " [Enter]attach"
	}
	return panel("Agent: "+a.ID+" [n]udge"+hint+scrollInfo, viewContent, panelWidth(m.width))
}

// wrapEventContent word-wraps content into indented lines for the detail view.
func wrapEventContent(content string, maxW int) string {
	var sb strings.Builder
	for i, bodyLine := range strings.Split(content, "\n") {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if bodyLine == "" {
			continue
		}
		first := true
		runes := []rune(bodyLine)
		for len(runes) > maxW {
			cut := maxW
			prefix := string(runes[:cut])
			if sp := strings.LastIndex(prefix, " "); sp > 0 {
				cut = len([]rune(prefix[:sp]))
			}
			if !first {
				sb.WriteByte('\n')
			}
			sb.WriteString("  " + string(runes[:cut]))
			runes = []rune(strings.TrimSpace(string(runes[cut:])))
			first = false
		}
		if len(runes) > 0 {
			if !first {
				sb.WriteByte('\n')
			}
			sb.WriteString("  " + string(runes))
		}
	}
	return sb.String()
}

func sortAgentTree(agents []*agent.Agent) ([]*agent.Agent, []agentTreeNode) {
	if len(agents) == 0 {
		return agents, nil
	}

	// Build children map: parent ID → list of indices
	idSet := map[string]bool{}
	for _, a := range agents {
		idSet[a.ID] = true
	}
	children := map[string][]int{}
	var roots []int
	for i, a := range agents {
		if a.SpawnedBy == "" || !idSet[a.SpawnedBy] {
			roots = append(roots, i)
		} else {
			children[a.SpawnedBy] = append(children[a.SpawnedBy], i)
		}
	}

	sorted := make([]*agent.Agent, 0, len(agents))
	tree := make([]agentTreeNode, 0, len(agents))

	var walk func(idx, depth int, isLast bool, ancestors []bool)
	walk = func(idx, depth int, isLast bool, ancestors []bool) {
		anc := make([]bool, len(ancestors))
		copy(anc, ancestors)
		sorted = append(sorted, agents[idx])
		tree = append(tree, agentTreeNode{depth: depth, isLast: isLast, ancestors: anc})
		kids := children[agents[idx].ID]
		nextAnc := append(anc, isLast)
		for j, kid := range kids {
			walk(kid, depth+1, j == len(kids)-1, nextAnc)
		}
	}

	for i, r := range roots {
		walk(r, 0, i == len(roots)-1, nil)
	}

	// Append any agents not reached (shouldn't happen, but be safe)
	visited := map[int]bool{}
	for _, a := range sorted {
		for i, orig := range agents {
			if orig == a {
				visited[i] = true
				break
			}
		}
	}
	for i, a := range agents {
		if !visited[i] {
			sorted = append(sorted, a)
			tree = append(tree, agentTreeNode{})
		}
	}

	return sorted, tree
}

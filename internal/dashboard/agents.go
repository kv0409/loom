package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/acp"
	"github.com/karanagi/loom/internal/agent"
)

func (m Model) renderAgents() string {
	agents := m.filteredAgents()

	// Compute ID column width from actual tree data.
	idWidth := 16
	for _, a := range agents {
		w := len(a.ID)
		// Find original index for tree data
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

	// Proportional column widths based on terminal width.
	avail := m.width - 6 // 2 indent + 4 inter-column spaces
	if avail < 40 {
		avail = 40
	}
	roleW := max(6, avail*10/100)
	statusW := statusPillWidth + 2
	wtW := max(8, avail*22/100)
	issueW := max(6, avail*14/100)
	hbW := 10 // fixed: "NNNs ago ↯N" fits in 10
	idW := min(max(idWidth+2, 16), avail*25/100)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds", idW, roleW, statusW+2, wtW, issueW, hbW)
	content := fmt.Sprintf(fmtStr+"\n", "ID", "ROLE", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"
	content += "\n"

	if len(agents) == 0 {
		content += renderEmpty("No agents running — loom spawn to start", m.width-6)
	}

	for i, a := range agents {
		wt := "—"
		if a.WorktreeName != "" {
			wt = truncate(slugFromWorktree(a.WorktreeName), wtW)
		}
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = truncate(strings.Join(a.AssignedIssues, ","), issueW)
		}
		hb := relTime(a.Heartbeat)
		if a.NudgeCount > 0 {
			hb += fmt.Sprintf(" ↯%d", a.NudgeCount)
		}
		statusCol := fmt.Sprintf("%s %s", statusIndicator(a.Status), statusPill(a.Status))

		// Build tree prefix — find original index for tree data.
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

		id := prefix + a.ID
		line := fmt.Sprintf(fmtStr, id, a.Role, statusCol, wt, issues, hb)
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		}
		content += line + "\n"
	}

	title := fmt.Sprintf("[a] AGENTS (%d)", len(m.data.agents))
	if m.searchQuery != "" {
		title = fmt.Sprintf("[a] AGENTS (%d/%d) filter: %s", len(agents), len(m.data.agents), m.searchQuery)
	}
	return panel(title, content, m.width-2)
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
		a.Role, statusIndicator(a.Status), statusPillStyle(a.Status).Render(a.Status), relTime(a.Heartbeat)))
	lines = append(lines, fmt.Sprintf("  Spawned by: %-10s Spawned at: %-10s PID: %d",
		a.SpawnedBy, a.SpawnedAt.Format("15:04:05"), a.PID))
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
		lines = append(lines, fmt.Sprintf("  Count: %d  Last: %s", a.NudgeCount, relTime(a.LastNudge)))
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
					if len(line) > maxW {
						line = line[:maxW]
					}
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
			lines = append(lines, fmt.Sprintf("  %s %s %s: %s", dir, msg.Timestamp.Format("15:04"), other, truncate(msg.Subject, 40)))
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
	viewH := detailViewH(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	hint := ""
	if a.Config.KiroMode != "acp" && a.TmuxTarget != "" {
		hint = " [Enter]attach"
	}
	return panel("Agent: "+a.ID+" [n]udge"+hint+scrollInfo, viewContent, m.width-2)
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
		for len(bodyLine) > maxW {
			cut := maxW
			if sp := strings.LastIndex(bodyLine[:cut], " "); sp > 0 {
				cut = sp
			}
			if !first {
				sb.WriteByte('\n')
			}
			sb.WriteString("  " + bodyLine[:cut])
			bodyLine = strings.TrimSpace(bodyLine[cut:])
			first = false
		}
		if bodyLine != "" {
			if !first {
				sb.WriteByte('\n')
			}
			sb.WriteString("  " + bodyLine)
		}
	}
	return sb.String()
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

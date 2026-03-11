package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/agent"
)

func (m Model) renderAgents() string {
	// Compute ID column width from actual tree data.
	idWidth := 16
	for i, a := range m.data.agents {
		w := len(a.ID)
		if i < len(m.data.agentTree) {
			w += m.data.agentTree[i].depth * 2
		}
		if w > idWidth {
			idWidth = w
		}
	}

	fmtStr := fmt.Sprintf("  %%-%ds %%-12s %%-12s %%-22s %%-14s %%s", idWidth)
	content := fmt.Sprintf(fmtStr+"\n", "ID", "ROLE", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	for i, a := range m.data.agents {
		wt := "—"
		if a.WorktreeName != "" {
			wt = truncate(slugFromWorktree(a.WorktreeName), 22)
		}
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = truncate(strings.Join(a.AssignedIssues, ","), 14)
		}
		hb := relTime(a.Heartbeat)
		statusCol := fmt.Sprintf("%s %-10s", statusIndicator(a.Status), a.Status)

		// Build tree prefix (2-char indent per level).
		prefix := ""
		if i < len(m.data.agentTree) {
			node := m.data.agentTree[i]
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
		}

		id := prefix + a.ID
		line := fmt.Sprintf(fmtStr, id, a.Role, statusCol, wt, issues, hb)
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
	if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
		s += "  Mode: ACP\n"
	} else if a.TmuxTarget != "" {
		s += fmt.Sprintf("  Tmux: %s\n", a.TmuxTarget)
	}

	if len(a.AssignedIssues) > 0 {
		s += "\n  " + headerStyle.Render("ASSIGNED ISSUES") + "\n"
		s += fmt.Sprintf("  └── %s\n", strings.Join(a.AssignedIssues, ", "))
	}
	if a.WorktreeName != "" {
		s += fmt.Sprintf("\n  " + headerStyle.Render("WORKTREE") + ": %s\n", slugFromWorktree(a.WorktreeName))
	}

	// ACP output
	if a.Config.KiroMode == "acp" || a.TmuxTarget == "" {
		s += "\n  " + headerStyle.Render("RECENT OUTPUT") + "\n"
		outPath := filepath.Join(m.loomRoot, "agents", a.ID+".output")
		if raw, err := os.ReadFile(outPath); err == nil {
			text := assembleChunksN(string(raw), 500)
			if text == "" {
				s += "  (waiting for output...)\n"
			} else {
				// Word-wrap the assembled text to fit the panel width.
				maxW := m.width - 8
				if maxW < 40 {
					maxW = 40
				}
				for len(text) > 0 {
					end := maxW
					if end > len(text) {
						end = len(text)
					}
					s += "  " + text[:end] + "\n"
					text = text[end:]
				}
			}
		} else {
			s += "  (waiting for output...)\n"
		}
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

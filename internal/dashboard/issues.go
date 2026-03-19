package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

const maxRecentDone = 5

// displayIssues returns active issues followed by up to maxRecentDone done
// issues sorted by most recently updated.
func (m Model) displayIssues() []*backend.Issue {
	var active, done []*backend.Issue
	for _, iss := range m.data.Issues {
		if iss.Status == "done" || iss.Status == "cancelled" {
			done = append(done, iss)
		} else {
			active = append(active, iss)
		}
	}
	sort.Slice(done, func(i, j int) bool { return done[i].UpdatedAt.After(done[j].UpdatedAt) })
	if len(done) > maxRecentDone {
		done = done[:maxRecentDone]
	}
	return append(active, done...)
}

func (m Model) renderIssues() string {
	display := m.filteredIssues()

	activeCount := 0
	for _, iss := range display {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeCount++
		}
	}

	avail := availableWidth(m.width)
	vRows := visibleRows(m.height, 9)
	start, end := issuesViewport(m.cursor, len(display), vRows, activeCount)

	// Split visible rows into active and done segments to insert separator.
	activeEnd := end
	if activeEnd > activeCount {
		activeEnd = activeCount
	}
	doneStart := start
	if doneStart < activeCount {
		doneStart = activeCount
	}

	headers := []string{"ID", "ASSIGNEE", "TITLE"}

	buildRows := func(from, to int) [][]string {
		rows := make([][]string, 0, to-from)
		for i := from; i < to; i++ {
			iss := display[i]
			rows = append(rows, []string{typeGlyph(iss.Type) + " " + iss.ID, iss.Assignee, iss.Title})
		}
		return rows
	}

	buildStyler := func(from int) CellStyler {
		return func(row, col int, isSelected bool) lipgloss.Style {
			base := lgTableCellStyle
			if isSelected {
				base = lgTableSelectedStyle
			}
			dataIdx := from + row
			if col == 0 && dataIdx < len(display) {
				if c, ok := statusColors[display[dataIdx].Status]; ok {
					return base.Foreground(c)
				}
			}
			return base
		}
	}

	var content string
	if len(display) == 0 {
		t := newLGTable(headers, nil, -1, avail, nil)
		content = t.Render() + "\n" + renderEmpty("No issues — loom issue create to add one", avail)
	} else {
		// Active section.
		activeRows := buildRows(start, activeEnd)
		activeCursor, activeSelected := sectionCursor(m.cursor, start, activeEnd)
		sel := -1
		if activeSelected {
			sel = activeCursor
		}
		content = newLGTable(headers, activeRows, sel, avail, buildStyler(start)).Render() + "\n"

		// Done section with separator (headerless — avoids duplicate column headers).
		if doneStart < end {
			content += "\n  " + headerStyle.Render("RECENTLY DONE") + "\n"
			content += separator(m.width)
			doneRows := buildRows(doneStart, end)
			doneCursor, doneSelected := sectionCursor(m.cursor, doneStart, end)
			doneSel := -1
			if doneSelected {
				doneSel = doneCursor
			}
			content += newLGTableHeaderless(doneRows, doneSel, avail, buildStyler(doneStart)).Render() + "\n"
		}
	}

	title := fmt.Sprintf("[i] ISSUES (%d active)", activeCount)
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[i] ISSUES (%d/%d) filter: %s", len(display), len(m.displayIssues()), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

func (m Model) renderIssueDetail() string {
	display := m.filteredIssues()
	if m.cursor >= len(display) {
		return "No issue selected"
	}
	iss := display[m.cursor]
	relatedMemories := m.relatedMemories(iss.ID)
	relatedMessages := m.relatedMessages(iss.ID)
	relatedWorktree := m.relatedWorktree(iss)

	// --- Fixed header: metadata ---
	var header []string
	header = append(header, fmt.Sprintf("  %s", titleStyle.Render(iss.Title)))
	header = append(header, fmt.Sprintf("  Type: %-8s Priority: %-8s Status: %s %s",
		iss.Type, iss.Priority, statusIndicator(iss.Status), statusPillStyle(iss.Status).Render(iss.Status)))
	if iss.Assignee != "" {
		header = append(header, fmt.Sprintf("  Assignee: %s", iss.Assignee))
	}
	header = append(header, "", "  "+headerStyle.Render("NEXT ACTION"))
	header = append(header, fmt.Sprintf("  %s", m.issueNextAction(iss, relatedWorktree, len(relatedMessages))))

	// --- Scrollable body ---
	var body []string
	if iss.Description != "" {
		body = append(body, "  "+headerStyle.Render("DESCRIPTION"))
		body = append(body, wrapLines(iss.Description, detailContentWidth(m.width), "  ")...)
	}
	if iss.Parent != "" {
		body = append(body, fmt.Sprintf("  Parent: %s", iss.Parent))
	}
	if len(iss.DependsOn) > 0 {
		body = append(body, fmt.Sprintf("  Depends: %s", strings.Join(iss.DependsOn, ", ")))
	}

	if len(iss.Dispatch) > 0 {
		body = append(body, "", "  "+headerStyle.Render("DISPATCH"))
		keys := make([]string, 0, len(iss.Dispatch))
		for k := range iss.Dispatch {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			body = append(body, fmt.Sprintf("  %s=%s", k, iss.Dispatch[k]))
		}
	}

	if len(iss.Children) > 0 {
		issueMap := make(map[string]*backend.Issue, len(m.data.Issues))
		for _, ci := range m.data.Issues {
			issueMap[ci.ID] = ci
		}
		body = append(body, "", "  "+headerStyle.Render("CHILDREN"))
		t := tree.New().
			EnumeratorStyle(treeConnectorStyle).
			IndenterStyle(treeConnectorStyle)
		for _, cid := range iss.Children {
			label := cid
			if ci, ok := issueMap[cid]; ok {
				prefix := fmt.Sprintf("%s %s [%s] ", statusIndicator(ci.Status), cid, ci.Status)
				label = prefix + truncate(ci.Title, detailContentWidth(m.width)-lipgloss.Width(prefix))
			}
			t.Child(label)
		}
		for _, line := range splitLines(t.String()) {
			if line != "" {
				body = append(body, "  "+line)
			}
		}
	}

	if relatedWorktree != nil {
		body = append(body, "", "  "+headerStyle.Render("WORKTREE"))
		body = append(body, fmt.Sprintf("  %s", relatedWorktree.Name))
		body = append(body, fmt.Sprintf("  Branch: %s", relatedWorktree.Branch))
		if relatedWorktree.Agent != "" {
			body = append(body, fmt.Sprintf("  Agent: %s", relatedWorktree.Agent))
		}
	}

	if len(relatedMemories) > 0 {
		body = append(body, "", "  "+headerStyle.Render("RELATED MEMORY"))
		for _, entry := range relatedMemories[:min(3, len(relatedMemories))] {
			snippet := m.backend.MemorySnippet(entry)
			if snippet == "" {
				snippet = entry.Title
			}
			body = append(body, fmt.Sprintf("  %s %s", entry.ID, truncate(entry.Title, detailContentWidth(m.width)-len(entry.ID)-4)))
			body = append(body, fmt.Sprintf("    %s", truncate(snippet, detailContentWidth(m.width)-4)))
		}
	}

	if len(relatedMessages) > 0 {
		body = append(body, "", "  "+headerStyle.Render("RELATED MAIL"))
		for _, msg := range relatedMessages[:min(3, len(relatedMessages))] {
			body = append(body, fmt.Sprintf("  %s %s → %s", fmtTime(msg.Timestamp, false), msg.From, msg.To))
			body = append(body, fmt.Sprintf("    %s", truncate(msg.Subject, detailContentWidth(m.width)-4)))
		}
	}

	if len(iss.History) > 0 {
		body = append(body, "", "  "+headerStyle.Render("HISTORY"))
		for _, h := range iss.History {
			detail := ""
			if h.Detail != "" {
				detail = " — " + h.Detail
			}
			body = append(body, fmt.Sprintf("  %s %s %s%s", fmtTime(h.At, false), h.By, h.Action, detail))
		}
	}

	headerStr := strings.Join(header, "\n")
	headerLines := strings.Count(headerStr, "\n") + 1
	vpH := scrollViewport(m.height) - headerLines
	if vpH < 1 {
		vpH = 1
	}

	vp := m.detailVP
	vp.SetHeight(vpH)
	vp.SetContentLines(body)
	vp.SetYOffset(m.detailYOff)
	scrollInfo := vpScrollIndicator(vp)

	content := headerStr + "\n" + vp.View()
	return panel("Issue: "+iss.ID+scrollInfo, content, panelWidth(m.width))
}

func (m Model) relatedMemories(issueID string) []*backend.MemoryEntry {
	var related []*backend.MemoryEntry
	for _, entry := range m.data.Memories {
		for _, affect := range entry.Affects {
			if affect == issueID {
				related = append(related, entry)
				break
			}
		}
	}
	return related
}

func (m Model) relatedMessages(issueID string) []*backend.Message {
	var related []*backend.Message
	for _, msg := range m.data.Messages {
		if msg.Ref == issueID || strings.Contains(msg.Subject, issueID) || strings.Contains(msg.Body, issueID) {
			related = append(related, msg)
		}
	}
	return related
}

func (m Model) relatedWorktree(iss *backend.Issue) *backend.Worktree {
	for _, wt := range m.data.Worktrees {
		if wt.Issue == iss.ID || wt.Name == iss.Worktree || wt.Branch == iss.Worktree {
			return wt
		}
	}
	return nil
}

func (m Model) issueNextAction(iss *backend.Issue, wt *backend.Worktree, relatedMailCount int) string {
	switch iss.Status {
	case "blocked":
		if relatedMailCount > 0 {
			return "Resolve the blocker thread first, then hand the issue back to the active builder."
		}
		return "Clarify the blocker owner and decide whether to reassign or unblock dependencies."
	case "review":
		return "Review the current changeset and either merge it or bounce it back with a concrete request."
	case "assigned":
		return "Confirm the assignee has picked this up and that the task scope is still correct."
	case "in-progress":
		if wt != nil {
			return "Inspect the active worktree and recent activity to decide whether to nudge, review, or wait."
		}
		return "Check recent agent activity and make sure implementation is progressing in the expected worktree."
	case "open":
		return "Assign an owner and break the work down before it disappears into the queue."
	default:
		return "Review the latest context and decide the next owner-facing action."
	}
}

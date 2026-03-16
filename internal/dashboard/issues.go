package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/worktree"
)

const maxRecentDone = 5

// displayIssues returns active issues followed by up to maxRecentDone done
// issues sorted by most recently updated.
func (m Model) displayIssues() []*issue.Issue {
	var active, done []*issue.Issue
	for _, iss := range m.data.issues {
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
	const numColsIssues = 3
	avail -= numColsIssues * 2
	ws := colWidths(avail, []struct{ pct, min int }{{20, 16}, {15, 8}})
	idW, assignW := ws[0], ws[1]
	titleW := max(10, avail-idW-assignW)

	cols := []table.Column{
		{Title: "ID", Width: idW},
		{Title: "ASSIGNEE", Width: assignW},
		{Title: "TITLE", Width: titleW},
	}

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

	ri := 0
	buildRows := func(from, to int) ([]table.Row, [][2]string) {
		rows := make([]table.Row, 0, to-from)
		var replacements [][2]string
		for i := from; i < to; i++ {
			iss := display[i]
			sg := statusGlyphs[iss.Status]
			if sg == "" {
				sg = "●"
			}
			plainID := sg + typeGlyph(iss.Type) + " " + iss.ID
			styledID := statusStyle(iss.Status).Render(plainID)
			ph := cellPlaceholder(ri, lipgloss.Width(styledID))
			rows = append(rows, table.Row{ph, truncate(iss.Assignee, assignW), truncate(iss.Title, titleW)})
			replacements = append(replacements, [2]string{ph, styledID})
			ri++
		}
		return rows, replacements
	}

	var content string
	if len(display) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No issues — loom issue create to add one", avail)
	} else {
		// Active section.
		activeRows, activeRepl := buildRows(start, activeEnd)
		activeCursor := -1
		if m.cursor >= start && m.cursor < activeEnd {
			activeCursor = m.cursor - start
		}
		t := newStyledTable(cols, activeRows, len(activeRows))
		if activeCursor >= 0 {
			t.SetCursor(activeCursor)
		}
		content = styledTableView(t, activeRepl) + "\n"

		// Done section with separator (headerless — avoids duplicate column headers).
		if doneStart < end {
			content += "\n  " + headerStyle.Render("RECENTLY DONE") + "\n"
			content += separator(m.width)
			doneRows, doneRepl := buildRows(doneStart, end)
			doneCursor := -1
			if m.cursor >= doneStart && m.cursor < end {
				doneCursor = m.cursor - doneStart
			}
			dt := newStyledTableHeaderless(cols, doneRows, len(doneRows))
			if doneCursor >= 0 {
				dt.Focus()
				dt.SetCursor(doneCursor)
				dt.SetStyles(table.Styles{
					Header:   lipgloss.NewStyle(),
					Cell:     tableCellStyle,
					Selected: tableSelectedStyle,
				})
			}
			doneView := tableBodyView(dt)
			for _, r := range doneRepl {
				doneView = strings.Replace(doneView, r[0], r[1], 1)
			}
			content += doneView + "\n"
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

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s", titleStyle.Render(iss.Title)))
	lines = append(lines, fmt.Sprintf("  Type: %-8s Priority: %-8s Status: %s %s",
		iss.Type, iss.Priority, statusIndicator(iss.Status), statusPillStyle(iss.Status).Render(iss.Status)))
	if iss.Assignee != "" {
		lines = append(lines, fmt.Sprintf("  Assignee: %s", iss.Assignee))
	}
	lines = append(lines, "", "  "+headerStyle.Render("NEXT ACTION"))
	lines = append(lines, fmt.Sprintf("  %s", m.issueNextAction(iss, relatedWorktree, len(relatedMessages))))

	if iss.Description != "" {
		lines = append(lines, "", "  "+headerStyle.Render("DESCRIPTION"))
		lines = append(lines, fmt.Sprintf("  %s", iss.Description))
	}
	if iss.Parent != "" {
		lines = append(lines, fmt.Sprintf("  Parent: %s", iss.Parent))
	}
	if len(iss.DependsOn) > 0 {
		lines = append(lines, fmt.Sprintf("  Depends: %s", strings.Join(iss.DependsOn, ", ")))
	}

	if len(iss.Dispatch) > 0 {
		lines = append(lines, "", "  "+headerStyle.Render("DISPATCH"))
		keys := make([]string, 0, len(iss.Dispatch))
		for k := range iss.Dispatch {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, fmt.Sprintf("  %s=%s", k, iss.Dispatch[k]))
		}
	}

	if len(iss.Children) > 0 {
		issueMap := make(map[string]*issue.Issue, len(m.data.issues))
		for _, ci := range m.data.issues {
			issueMap[ci.ID] = ci
		}
		lines = append(lines, "", "  "+headerStyle.Render("CHILDREN"))
		for i, cid := range iss.Children {
			label := cid
			if ci, ok := issueMap[cid]; ok {
				label = fmt.Sprintf("%s %s [%s] %s", statusIndicator(ci.Status), cid, ci.Status, truncate(ci.Title, 30))
			}
			prefix := "├──"
			if i == len(iss.Children)-1 {
				prefix = "└──"
			}
			lines = append(lines, fmt.Sprintf("  %s %s", prefix, label))
		}
	}

	if relatedWorktree != nil {
		lines = append(lines, "", "  "+headerStyle.Render("WORKTREE"))
		lines = append(lines, fmt.Sprintf("  %s", relatedWorktree.Name))
		lines = append(lines, fmt.Sprintf("  Branch: %s", relatedWorktree.Branch))
		if relatedWorktree.Agent != "" {
			lines = append(lines, fmt.Sprintf("  Agent: %s", relatedWorktree.Agent))
		}
	}

	if len(relatedMemories) > 0 {
		lines = append(lines, "", "  "+headerStyle.Render("RELATED MEMORY"))
		for _, entry := range relatedMemories[:min(3, len(relatedMemories))] {
			snippet := memory.Snippet(entry)
			if snippet == "" {
				snippet = entry.Title
			}
			lines = append(lines, fmt.Sprintf("  %s %s", entry.ID, truncate(entry.Title, 48)))
			lines = append(lines, fmt.Sprintf("    %s", truncate(snippet, detailContentWidth(m.width)-4)))
		}
	}

	if len(relatedMessages) > 0 {
		lines = append(lines, "", "  "+headerStyle.Render("RELATED MAIL"))
		for _, msg := range relatedMessages[:min(3, len(relatedMessages))] {
			lines = append(lines, fmt.Sprintf("  %s %s → %s", fmtTime(msg.Timestamp, false), msg.From, msg.To))
			lines = append(lines, fmt.Sprintf("    %s", truncate(msg.Subject, detailContentWidth(m.width)-4)))
		}
	}

	if len(iss.History) > 0 {
		lines = append(lines, "", "  "+headerStyle.Render("HISTORY"))
		for _, h := range iss.History {
			detail := ""
			if h.Detail != "" {
				detail = " — " + h.Detail
			}
			lines = append(lines, fmt.Sprintf("  %s %s %s%s", fmtTime(h.At, false), h.By, h.Action, detail))
		}
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Issue: "+iss.ID+scrollInfo, viewContent+"\n", panelWidth(m.width))
}

func (m Model) relatedMemories(issueID string) []*memory.Entry {
	var related []*memory.Entry
	for _, entry := range m.data.memories {
		for _, affect := range entry.Affects {
			if affect == issueID {
				related = append(related, entry)
				break
			}
		}
	}
	return related
}

func (m Model) relatedMessages(issueID string) []*mail.Message {
	var related []*mail.Message
	for _, msg := range m.data.messages {
		if msg.Ref == issueID || strings.Contains(msg.Subject, issueID) || strings.Contains(msg.Body, issueID) {
			related = append(related, msg)
		}
	}
	return related
}

func (m Model) relatedWorktree(iss *issue.Issue) *worktree.Worktree {
	for _, wt := range m.data.worktrees {
		if wt.Issue == iss.ID || wt.Name == iss.Worktree || wt.Branch == iss.Worktree {
			return wt
		}
	}
	return nil
}

func (m Model) issueNextAction(iss *issue.Issue, wt *worktree.Worktree, relatedMailCount int) string {
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

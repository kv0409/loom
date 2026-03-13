package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/karanagi/loom/internal/issue"
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

	buildRows := func(from, to int) []table.Row {
		rows := make([]table.Row, 0, to-from)
		for i := from; i < to; i++ {
			iss := display[i]
			sg := statusGlyphs[iss.Status]
			if sg == "" {
				sg = "●"
			}
			idCell := statusStyle(iss.Status).Render(sg+typeGlyph(iss.Type)+" "+iss.ID)
			rows = append(rows, table.Row{idCell, truncate(iss.Assignee, assignW), truncate(iss.Title, titleW)})
		}
		return rows
	}

	var content string
	if len(display) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No issues — loom issue create to add one", avail)
	} else {
		// Active section.
		activeRows := buildRows(start, activeEnd)
		activeCursor := -1
		if m.cursor >= start && m.cursor < activeEnd {
			activeCursor = m.cursor - start
		}
		t := newStyledTable(cols, activeRows, len(activeRows))
		if activeCursor >= 0 {
			t.SetCursor(activeCursor)
		}
		content = t.View() + "\n"

		// Done section with separator.
		if doneStart < end {
			content += "\n  " + headerStyle.Render("RECENTLY DONE") + "\n"
			content += separator(m.width)
			doneRows := buildRows(doneStart, end)
			doneCursor := -1
			if m.cursor >= doneStart && m.cursor < end {
				doneCursor = m.cursor - doneStart
			}
			dt := newStyledTable(cols, doneRows, len(doneRows))
			if doneCursor >= 0 {
				dt.SetCursor(doneCursor)
			}
			content += dt.View() + "\n"
		}
	}

	title := fmt.Sprintf("[i] ISSUES (%d active)", activeCount)
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[i] ISSUES (%d/%d) filter: %s", len(display), len(m.displayIssues()), m.searchTI.Value())
	}
	return panel(title, content, m.width-2)
}

func (m Model) renderIssueDetail() string {
	display := m.filteredIssues()
	if m.cursor >= len(display) {
		return "No issue selected"
	}
	iss := display[m.cursor]

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s", titleStyle.Render(iss.Title)))
	lines = append(lines, fmt.Sprintf("  Type: %-8s Priority: %-8s Status: %s %s",
		iss.Type, iss.Priority, statusIndicator(iss.Status), statusPillStyle(iss.Status).Render(iss.Status)))
	if iss.Assignee != "" {
		lines = append(lines, fmt.Sprintf("  Assignee: %s", iss.Assignee))
	}

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

	return panel("Issue: "+iss.ID+scrollInfo, viewContent+"\n", m.width-2)
}

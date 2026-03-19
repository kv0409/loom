package dashboard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
)

func (m Model) renderMemory() string {
	memories := m.filteredMemories()

	avail := availableWidth(m.width)
	const numCols = 4
	avail -= numCols * 2

	idW := proportionalWidth(avail, 12, 8)
	typeW := proportionalWidth(avail, 12, 8)
	titleW := proportionalWidth(avail, 36, 12)
	snippetW := max(10, avail-idW-typeW-titleW)

	cols := []table.Column{
		{Title: "ID", Width: idW},
		{Title: "TYPE", Width: typeW},
		{Title: "TITLE", Width: titleW},
		{Title: "DETAIL", Width: snippetW},
	}

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(memories), vRows)

	rows := make([]table.Row, 0, end-start)
	for i := start; i < end; i++ {
		e := memories[i]
		snippet := m.backend.MemorySnippet(e)
		if snippet == "" {
			snippet = e.Title
		}
		rows = append(rows, table.Row{e.ID, e.Type, truncate(e.Title, titleW), truncate(snippet, snippetW)})
	}

	var content string
	if len(memories) == 0 {
		t := newStyledTable(cols, nil, vRows)
		content = t.View() + "\n" + renderEmpty("No memory entries yet", avail)
	} else {
		t := newStyledTable(cols, rows, vRows)
		t.SetCursor(m.cursor - start)
		content = fixSelectedRowBg(t.View()) + "\n"
	}

	title := fmt.Sprintf("[d] MEMORY (%d entries)", len(m.data.Memories))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[d] MEMORY (%d/%d) filter: %s", len(memories), len(m.data.Memories), m.searchTI.Value())
	}
	return panel(title, content, panelWidth(m.width))
}

func (m Model) renderMemoryDetail() string {
	memories := m.filteredMemories()
	if m.cursor >= len(memories) {
		return "No memory entry selected"
	}
	e := memories[m.cursor]

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s", titleStyle.Render(e.Title)))
	lines = append(lines, fmt.Sprintf("  ID: %-12s Type: %-12s By: %s", e.ID, e.Type, m.backend.MemoryByField(e)))
	lines = append(lines, fmt.Sprintf("  Time: %s", fmtTimeFull(e.Timestamp)))

	maxW := detailContentWidth(m.width)
	switch e.Type {
	case "decision":
		if e.Context != "" {
			lines = append(lines, "", "  "+headerStyle.Render("CONTEXT"))
			lines = append(lines, wrapLines(e.Context, maxW, "    ")...)
		}
		if e.Decision != "" {
			lines = append(lines, "", "  "+headerStyle.Render("DECISION"))
			lines = append(lines, wrapLines(e.Decision, maxW, "    ")...)
		}
		if e.Rationale != "" {
			lines = append(lines, "", "  "+headerStyle.Render("RATIONALE"))
			lines = append(lines, wrapLines(e.Rationale, maxW, "    ")...)
		}
		if len(e.Alternatives) > 0 {
			lines = append(lines, "", "  "+headerStyle.Render("ALTERNATIVES"))
			for _, alt := range e.Alternatives {
				lines = append(lines, fmt.Sprintf("    • %s", alt.Option))
				if alt.RejectedBecause != "" {
					lines = append(lines, fmt.Sprintf("      Rejected: %s", alt.RejectedBecause))
				}
			}
		}
	case "discovery":
		if e.Location != "" {
			lines = append(lines, fmt.Sprintf("  Location: %s", e.Location))
		}
		if e.Finding != "" {
			lines = append(lines, "", "  "+headerStyle.Render("FINDING"))
			lines = append(lines, wrapLines(e.Finding, maxW, "    ")...)
		}
		if e.Implications != "" {
			lines = append(lines, "", "  "+headerStyle.Render("IMPLICATIONS"))
			lines = append(lines, wrapLines(e.Implications, maxW, "    ")...)
		}
	case "convention":
		if e.Rule != "" {
			lines = append(lines, "", "  "+headerStyle.Render("RULE"))
			lines = append(lines, wrapLines(e.Rule, maxW, "    ")...)
		}
		if e.AppliesTo != "" {
			lines = append(lines, fmt.Sprintf("  Applies to: %s", e.AppliesTo))
		}
		if len(e.Examples) > 0 {
			lines = append(lines, "", "  "+headerStyle.Render("EXAMPLES"))
			for _, ex := range e.Examples {
				lines = append(lines, fmt.Sprintf("    • %s", ex))
			}
		}
	}

	if len(e.Affects) > 0 {
		lines = append(lines, "", fmt.Sprintf("  Affects: %s", strings.Join(e.Affects, ", ")))
	}
	if len(e.Tags) > 0 {
		lines = append(lines, fmt.Sprintf("  Tags: %s", strings.Join(e.Tags, ", ")))
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Memory: "+e.ID+scrollInfo, viewContent+"\n", panelWidth(m.width))
}



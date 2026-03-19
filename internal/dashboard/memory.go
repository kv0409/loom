package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderMemory() string {
	memories := m.filteredMemories()
	avail := availableWidth(m.width)

	vRows := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(memories), vRows)

	rows := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		e := memories[i]
		snippet := m.backend.MemorySnippet(e)
		if snippet == "" {
			snippet = e.Title
		}
		rows = append(rows, []string{e.ID, e.Type, e.Title, snippet})
	}

	var content string
	if len(memories) == 0 {
		t := newLGTable([]string{"ID", "TYPE", "TITLE", "DETAIL"}, nil, -1, avail)
		content = t.Render() + "\n" + renderEmpty("No memory entries yet", avail)
	} else {
		t := newLGTable([]string{"ID", "TYPE", "TITLE", "DETAIL"}, rows, m.cursor-start, avail)
		content = t.Render() + "\n"
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

	vp := m.detailVP
	vp.SetContentLines(lines)
	vp.SetYOffset(m.detailYOff)
	scrollInfo := vpScrollIndicator(vp)

	return panel("Memory: "+e.ID+scrollInfo, vp.View()+"\n", panelWidth(m.width))
}



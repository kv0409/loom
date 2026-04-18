package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderMemory() string {
	memories := m.filteredMemories()
	avail := availableWidth(m.width)

	vRows := visibleRows(m.height, 10)
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
		t := newLGTable([]string{"ID", "TYPE", "TITLE", "DETAIL"}, nil, -1, avail, nil)
		content = t.Render() + "\n" + renderEmpty("No memory entries yet", avail)
	} else {
		t := newLGTable([]string{"ID", "TYPE", "TITLE", "DETAIL"}, rows, m.cursor-start, avail, nil)
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

	// --- Fixed header: metadata ---
	var header []string
	header = append(header, fmt.Sprintf("  %s", titleStyle.Render(e.Title)))
	header = append(header, fmt.Sprintf("  ID: %-12s Type: %-12s By: %s", e.ID, e.Type, m.backend.MemoryByField(e)))
	header = append(header, fmt.Sprintf("  Time: %s", fmtTimeFull(e.Timestamp)))

	// --- Scrollable body ---
	maxW := detailContentWidth(m.width)
	var body []string
	switch e.Type {
	case "decision":
		if e.Context != "" {
			body = append(body, "  "+headerStyle.Render("CONTEXT"))
			body = append(body, wrapLines(e.Context, maxW, "    ")...)
		}
		if e.Decision != "" {
			body = append(body, "", "  "+headerStyle.Render("DECISION"))
			body = append(body, wrapLines(e.Decision, maxW, "    ")...)
		}
		if e.Rationale != "" {
			body = append(body, "", "  "+headerStyle.Render("RATIONALE"))
			body = append(body, wrapLines(e.Rationale, maxW, "    ")...)
		}
		if len(e.Alternatives) > 0 {
			body = append(body, "", "  "+headerStyle.Render("ALTERNATIVES"))
			for _, alt := range e.Alternatives {
				body = append(body, fmt.Sprintf("    • %s", alt.Option))
				if alt.RejectedBecause != "" {
					body = append(body, fmt.Sprintf("      Rejected: %s", alt.RejectedBecause))
				}
			}
		}
	case "discovery":
		if e.Location != "" {
			body = append(body, fmt.Sprintf("  Location: %s", e.Location))
		}
		if e.Finding != "" {
			body = append(body, "", "  "+headerStyle.Render("FINDING"))
			body = append(body, wrapLines(e.Finding, maxW, "    ")...)
		}
		if e.Implications != "" {
			body = append(body, "", "  "+headerStyle.Render("IMPLICATIONS"))
			body = append(body, wrapLines(e.Implications, maxW, "    ")...)
		}
	case "convention":
		if e.Rule != "" {
			body = append(body, "", "  "+headerStyle.Render("RULE"))
			body = append(body, wrapLines(e.Rule, maxW, "    ")...)
		}
		if e.AppliesTo != "" {
			body = append(body, fmt.Sprintf("  Applies to: %s", e.AppliesTo))
		}
		if len(e.Examples) > 0 {
			body = append(body, "", "  "+headerStyle.Render("EXAMPLES"))
			for _, ex := range e.Examples {
				body = append(body, fmt.Sprintf("    • %s", ex))
			}
		}
	}

	if len(e.Affects) > 0 {
		body = append(body, "", fmt.Sprintf("  Affects: %s", strings.Join(e.Affects, ", ")))
	}
	if len(e.Tags) > 0 {
		body = append(body, fmt.Sprintf("  Tags: %s", strings.Join(e.Tags, ", ")))
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
	return panel("Memory: "+e.ID+scrollInfo, content, panelWidth(m.width))
}



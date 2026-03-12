package dashboard

import (
	"fmt"
	"strings"

	"github.com/karanagi/loom/internal/memory"
)

func (m Model) renderMemory() string {
	memories := m.filteredMemories()

	// Proportional column widths.
	avail := m.width - 6
	if avail < 40 {
		avail = 40
	}
	ws := colWidths(avail, []struct{ pct, min int }{{12, 6}, {14, 8}, {14, 6}})
	idW, typeW, byW := ws[0], ws[1], ws[2]
	titleW := max(10, avail-idW-typeW-byW)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%s", idW, typeW, titleW)
	content := tableHeader([]int{idW, typeW, titleW, byW}, []string{"ID", "TYPE", "TITLE", "BY"}, m.width)

	if len(memories) == 0 {
		content += renderEmpty("No memory entries yet", m.width-6)
	}

	vRows := visibleRows(m.height, 9) // header + tab bar + panel chrome + help bar (2 lines)
	start := m.cursor - vRows + 1
	if start < 0 {
		start = 0
	}
	end := start + vRows
	if end > len(memories) {
		end = len(memories)
	}

	for i := start; i < end; i++ {
		e := memories[i]
		line := fmt.Sprintf(fmtStr,
			truncate(e.ID, idW), truncate(e.Type, typeW), truncate(e.Title, titleW), truncate(memory.ByField(e), byW))
		if i == m.cursor {
			line = selectedRow(line)
		}
		content += line + "\n"
	}

	title := fmt.Sprintf("[d] MEMORY (%d entries)", len(m.data.memories))
	if m.searchQuery != "" {
		title = fmt.Sprintf("[d] MEMORY (%d/%d) filter: %s", len(memories), len(m.data.memories), m.searchQuery)
	}
	return panel(title, content, m.width-2)
}

func (m Model) renderMemoryDetail() string {
	memories := m.filteredMemories()
	if m.cursor >= len(memories) {
		return "No memory entry selected"
	}
	e := memories[m.cursor]

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s", titleStyle.Render(e.Title)))
	lines = append(lines, fmt.Sprintf("  ID: %-12s Type: %-12s By: %s", e.ID, e.Type, memory.ByField(e)))
	lines = append(lines, fmt.Sprintf("  Time: %s", e.Timestamp.Format("2006-01-02 15:04:05")))

	switch e.Type {
	case "decision":
		if e.Context != "" {
			lines = append(lines, "", "  "+headerStyle.Render("CONTEXT"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Context, m.width-8), "\n"), "\n")...)
		}
		if e.Decision != "" {
			lines = append(lines, "", "  "+headerStyle.Render("DECISION"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Decision, m.width-8), "\n"), "\n")...)
		}
		if e.Rationale != "" {
			lines = append(lines, "", "  "+headerStyle.Render("RATIONALE"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Rationale, m.width-8), "\n"), "\n")...)
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
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Finding, m.width-8), "\n"), "\n")...)
		}
		if e.Implications != "" {
			lines = append(lines, "", "  "+headerStyle.Render("IMPLICATIONS"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Implications, m.width-8), "\n"), "\n")...)
		}
	case "convention":
		if e.Rule != "" {
			lines = append(lines, "", "  "+headerStyle.Render("RULE"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Rule, m.width-8), "\n"), "\n")...)
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

	return panel("Memory: "+e.ID+scrollInfo, viewContent+"\n", m.width-2)
}

// wrapField formats a multi-line text field with indentation.
func wrapField(text string, maxW int) string {
	var s string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			s += "\n"
			continue
		}
		for len(line) > maxW {
			cut := maxW
			if sp := strings.LastIndex(line[:cut], " "); sp > 0 {
				cut = sp
			}
			s += "    " + line[:cut] + "\n"
			line = line[cut:]
			line = strings.TrimSpace(line)
		}
		if line != "" {
			s += "    " + line + "\n"
		}
	}
	return s
}

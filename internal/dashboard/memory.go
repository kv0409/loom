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
	idW := max(6, avail*12/100)
	typeW := max(8, avail*14/100)
	byW := max(6, avail*14/100)
	titleW := max(10, avail-idW-typeW-byW)

	fmtStr := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%s", idW, typeW, titleW)
	content := fmt.Sprintf(fmtStr+"\n", "ID", "TYPE", "TITLE", "BY")
	content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	visibleRows := m.height - 8 // header + tab bar + panel chrome + help bar
	if visibleRows < 1 {
		visibleRows = 1
	}
	start := m.cursor - visibleRows + 1
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(memories) {
		end = len(memories)
	}

	for i := start; i < end; i++ {
		e := memories[i]
		line := fmt.Sprintf(fmtStr,
			truncate(e.ID, idW), truncate(e.Type, typeW), truncate(e.Title, titleW), truncate(memory.ByField(e), byW))
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		} else if i == m.hoverRow {
			line = hoverStyle.Render(line)
		}
		content += line + "\n"
	}

	title := fmt.Sprintf("MEMORY (%d entries)", len(m.data.memories))
	if m.searchQuery != "" {
		title = fmt.Sprintf("MEMORY (%d/%d) filter: %s", len(memories), len(m.data.memories), m.searchQuery)
	}
	return panel(title, content, m.width-2)
}

func (m Model) renderMemoryDetail() string {
	memories := m.filteredMemories()
	if m.cursor >= len(memories) {
		return "No memory entry selected"
	}
	e := memories[m.cursor]

	s := fmt.Sprintf("  %s\n", titleStyle.Render(e.Title))
	s += fmt.Sprintf("  ID: %-12s Type: %-12s By: %s\n", e.ID, e.Type, memory.ByField(e))
	s += fmt.Sprintf("  Time: %s\n", e.Timestamp.Format("2006-01-02 15:04:05"))

	switch e.Type {
	case "decision":
		if e.Context != "" {
			s += "\n  " + headerStyle.Render("CONTEXT") + "\n"
			s += wrapField(e.Context, m.width-8)
		}
		if e.Decision != "" {
			s += "\n  " + headerStyle.Render("DECISION") + "\n"
			s += wrapField(e.Decision, m.width-8)
		}
		if e.Rationale != "" {
			s += "\n  " + headerStyle.Render("RATIONALE") + "\n"
			s += wrapField(e.Rationale, m.width-8)
		}
		if len(e.Alternatives) > 0 {
			s += "\n  " + headerStyle.Render("ALTERNATIVES") + "\n"
			for _, alt := range e.Alternatives {
				s += fmt.Sprintf("    • %s\n", alt.Option)
				if alt.RejectedBecause != "" {
					s += fmt.Sprintf("      Rejected: %s\n", alt.RejectedBecause)
				}
			}
		}
	case "discovery":
		if e.Location != "" {
			s += fmt.Sprintf("  Location: %s\n", e.Location)
		}
		if e.Finding != "" {
			s += "\n  " + headerStyle.Render("FINDING") + "\n"
			s += wrapField(e.Finding, m.width-8)
		}
		if e.Implications != "" {
			s += "\n  " + headerStyle.Render("IMPLICATIONS") + "\n"
			s += wrapField(e.Implications, m.width-8)
		}
	case "convention":
		if e.Rule != "" {
			s += "\n  " + headerStyle.Render("RULE") + "\n"
			s += wrapField(e.Rule, m.width-8)
		}
		if e.AppliesTo != "" {
			s += fmt.Sprintf("  Applies to: %s\n", e.AppliesTo)
		}
		if len(e.Examples) > 0 {
			s += "\n  " + headerStyle.Render("EXAMPLES") + "\n"
			for _, ex := range e.Examples {
				s += fmt.Sprintf("    • %s\n", ex)
			}
		}
	}

	if len(e.Affects) > 0 {
		s += fmt.Sprintf("\n  Affects: %s\n", strings.Join(e.Affects, ", "))
	}
	if len(e.Tags) > 0 {
		s += fmt.Sprintf("  Tags: %s\n", strings.Join(e.Tags, ", "))
	}

	return panel("Memory: "+e.ID, s, m.width-2)
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

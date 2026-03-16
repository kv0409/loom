package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
	"github.com/karanagi/loom/internal/memory"
)

func (m Model) renderMemory() string {
	memories := m.filteredMemories()
	counts := map[string]int{}
	for _, e := range memories {
		counts[e.Type]++
	}

	recentDecisions := make([]*memory.Entry, 0)
	for _, e := range memories {
		if e.Type == "decision" {
			recentDecisions = append(recentDecisions, e)
		}
	}
	sort.SliceStable(recentDecisions, func(i, j int) bool {
		return recentDecisions[i].Timestamp.After(recentDecisions[j].Timestamp)
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("  %d decisions · %d discoveries · %d conventions", counts["decision"], counts["discovery"], counts["convention"]))
	lines = append(lines, "", "  "+headerStyle.Render("MEMORY MAP"))
	if len(memories) == 0 {
		lines = append(lines, "  No memory entries yet.")
	} else {
		for _, e := range memories[:min(3, len(memories))] {
			snippet := memory.Snippet(e)
			if snippet == "" {
				snippet = e.Title
			}
			affects := ""
			if len(e.Affects) > 0 {
				affects = idleStyle.Render(" · " + strings.Join(e.Affects, ", "))
			}
			lines = append(lines, fmt.Sprintf("  %s %s%s", e.ID, truncate(e.Title, 36), affects))
			lines = append(lines, fmt.Sprintf("    %s", truncate(snippet, detailContentWidth(m.width)-4)))
		}
	}
	lines = append(lines, "", "  "+headerStyle.Render("RECENT DECISIONS"))
	if len(recentDecisions) == 0 {
		lines = append(lines, "  No recorded decisions yet.")
	} else {
		for idx, e := range recentDecisions[:min(4, len(recentDecisions))] {
			prefix := "  "
			if idx == m.cursor && len(memories) > 0 {
				prefix = "▸ "
			}
			lines = append(lines, fmt.Sprintf("%s%s %s", prefix, e.ID, truncate(e.Title, 42)))
			if e.Decision != "" {
				lines = append(lines, fmt.Sprintf("    %s", truncate(e.Decision, detailContentWidth(m.width)-4)))
			}
		}
	}

	content := strings.Join(lines, "\n") + "\n"

	title := fmt.Sprintf("[d] MEMORY (%d entries)", len(m.data.memories))
	if m.searchTI.Value() != "" {
		title = fmt.Sprintf("[d] MEMORY (%d/%d) filter: %s", len(memories), len(m.data.memories), m.searchTI.Value())
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
	lines = append(lines, fmt.Sprintf("  ID: %-12s Type: %-12s By: %s", e.ID, e.Type, memory.ByField(e)))
	lines = append(lines, fmt.Sprintf("  Time: %s", fmtTimeFull(e.Timestamp)))

	switch e.Type {
	case "decision":
		if e.Context != "" {
			lines = append(lines, "", "  "+headerStyle.Render("CONTEXT"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Context, detailContentWidth(m.width)), "\n"), "\n")...)
		}
		if e.Decision != "" {
			lines = append(lines, "", "  "+headerStyle.Render("DECISION"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Decision, detailContentWidth(m.width)), "\n"), "\n")...)
		}
		if e.Rationale != "" {
			lines = append(lines, "", "  "+headerStyle.Render("RATIONALE"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Rationale, detailContentWidth(m.width)), "\n"), "\n")...)
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
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Finding, detailContentWidth(m.width)), "\n"), "\n")...)
		}
		if e.Implications != "" {
			lines = append(lines, "", "  "+headerStyle.Render("IMPLICATIONS"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Implications, detailContentWidth(m.width)), "\n"), "\n")...)
		}
	case "convention":
		if e.Rule != "" {
			lines = append(lines, "", "  "+headerStyle.Render("RULE"))
			lines = append(lines, strings.Split(strings.TrimRight(wrapField(e.Rule, detailContentWidth(m.width)), "\n"), "\n")...)
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

// wrapField formats a multi-line text field with indentation.
// Uses rune-based slicing to avoid splitting multi-byte UTF-8 characters.
func wrapField(text string, maxW int) string {
	var s string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			s += "\n"
			continue
		}
		runes := []rune(line)
		for len(runes) > maxW {
			cut := maxW
			segment := string(runes[:cut])
			if sp := strings.LastIndex(segment, " "); sp > 0 {
				cut = utf8.RuneCountInString(segment[:sp])
			}
			s += "    " + string(runes[:cut]) + "\n"
			runes = runes[cut:]
			line = strings.TrimSpace(string(runes))
			runes = []rune(line)
		}
		if len(runes) > 0 {
			s += "    " + string(runes) + "\n"
		}
	}
	return s
}

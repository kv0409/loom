package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	// Count active issues for the separator position.
	activeCount := 0
	for _, iss := range display {
		if iss.Status != "done" && iss.Status != "cancelled" {
			activeCount++
		}
	}

	// Column widths: ID is fixed, assignee fits longest name, title gets the rest.
	avail := availableWidth(m.width)
	const idW = 16 // "▶● LOOM-NNN-NN" fits in 16
	assignW := 8
	for _, iss := range display {
		if n := len(iss.Assignee); n > assignW {
			assignW = n
		}
	}
	titleW := avail - idW - assignW - 2 // 2 for spacing
	if titleW < 10 {
		titleW = 10
	}

	header := fmt.Sprintf("  %-*s %-*s %s\n", idW, "ID", assignW, "ASSIGNEE", "TITLE")
	content := header + separator(m.width)
	content += "\n"

	if len(display) == 0 {
		content += renderEmpty("No issues — loom issue create to add one", m.width-6)
	}

	for i, iss := range display {
		if i == activeCount && activeCount < len(display) {
			content += "\n  " + headerStyle.Render("RECENTLY DONE") + "\n"
			content += separator(m.width)
		}

		// Build plain-text id column first so padding is ANSI-unaware.
		// Visual: <statusGlyph><typeGlyph> <ID>
		sg := statusGlyphs[iss.Status]
		if sg == "" {
			sg = "●"
		}
		idPlain := sg + typeGlyph(iss.Type) + " " + iss.ID
		idPadded := idPlain + strings.Repeat(" ", max(0, idW-lipgloss.Width(idPlain)))

		assignPadded := truncate(iss.Assignee, assignW)
		assignPadded += strings.Repeat(" ", max(0, assignW-lipgloss.Width(assignPadded)))

		titleCol := truncate(iss.Title, titleW)

		if i == m.cursor {
			// Style the whole row as selected; prefix with ▸ (no byte-slicing).
			line := "▸ " + idPadded + " " + assignPadded + " " + titleCol
			content += selectedStyle.Render(line) + "\n"
		} else {
			// Colour the id column; plain text for assignee and title.
			line := "  " + statusStyle(iss.Status).Render(idPadded) + " " + assignPadded + " " + titleCol
			content += line + "\n"
		}
	}

	title := fmt.Sprintf("[i] ISSUES (%d active)", activeCount)
	if m.searchQuery != "" {
		title = fmt.Sprintf("[i] ISSUES (%d/%d) filter: %s", len(display), len(m.displayIssues()), m.searchQuery)
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
			lines = append(lines, fmt.Sprintf("  %s %s %s%s", h.At.Format("15:04"), h.By, h.Action, detail))
		}
	}

	viewH := scrollViewport(m.height)
	viewContent, clampedScroll, total := renderViewport(lines, m.detailScroll, viewH)
	scrollInfo := scrollIndicator(clampedScroll, viewH, total)

	return panel("Issue: "+iss.ID+scrollInfo, viewContent+"\n", m.width-2)
}

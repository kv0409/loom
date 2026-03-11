package dashboard

import (
	"fmt"
	"sort"
	"strings"

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
	display := m.displayIssues()

	// Count active issues for the separator position.
	activeCount := 0
	for _, iss := range display {
		if iss.Status != "done" {
			activeCount++
		}
	}

	header := fmt.Sprintf("  %-12s %-8s %-16s %-36s %s\n",
		"ID", "TYPE", "STATUS", "TITLE", "ASSIGNEE")
	content := header + "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"

	for i, iss := range display {
		if i == activeCount && activeCount < len(display) {
			content += "\n  " + headerStyle.Render("RECENTLY DONE") + "\n"
			content += "  " + strings.Repeat("─", max(20, m.width-6)) + "\n"
		}
		statusCol := fmt.Sprintf("%s %-12s", statusIndicator(iss.Status), iss.Status)
		line := fmt.Sprintf("  %-12s %-8s %-16s %-36s %s",
			iss.ID, iss.Type, statusCol, truncate(iss.Title, 36), iss.Assignee)
		if i == m.cursor {
			line = selectedStyle.Render("▸" + line[1:])
		} else {
			line = statusStyle(iss.Status).Render(line)
		}
		content += line + "\n"
	}

	return panel(fmt.Sprintf("ISSUES (%d active)", activeCount), content, m.width-2)
}

func (m Model) renderIssueDetail() string {
	display := m.displayIssues()
	if m.cursor >= len(display) {
		return "No issue selected"
	}
	iss := display[m.cursor]

	s := fmt.Sprintf("  %s\n", titleStyle.Render(iss.Title))
	s += fmt.Sprintf("  Type: %-8s Priority: %-8s Status: %s %s\n",
		iss.Type, iss.Priority, statusIndicator(iss.Status), statusStyle(iss.Status).Render(iss.Status))
	if iss.Assignee != "" {
		s += fmt.Sprintf("  Assignee: %s\n", iss.Assignee)
	}

	if iss.Description != "" {
		s += "\n  " + headerStyle.Render("DESCRIPTION") + "\n"
		s += fmt.Sprintf("  %s\n", iss.Description)
	}
	if iss.Parent != "" {
		s += fmt.Sprintf("  Parent: %s\n", iss.Parent)
	}
	if len(iss.DependsOn) > 0 {
		s += fmt.Sprintf("  Depends: %s\n", strings.Join(iss.DependsOn, ", "))
	}

	if len(iss.Children) > 0 {
		s += "\n  " + headerStyle.Render("CHILDREN") + "\n"
		for _, cid := range iss.Children {
			label := cid
			for _, ci := range m.data.issues {
				if ci.ID == cid {
					label = fmt.Sprintf("%s %s [%s] %s", statusIndicator(ci.Status), cid, ci.Status, truncate(ci.Title, 30))
					break
				}
			}
			s += fmt.Sprintf("  └── %s\n", label)
		}
	}

	if len(iss.History) > 0 {
		s += "\n  " + headerStyle.Render("HISTORY") + "\n"
		limit := min(len(iss.History), 8)
		for _, h := range iss.History[:limit] {
			detail := ""
			if h.Detail != "" {
				detail = " — " + h.Detail
			}
			s += fmt.Sprintf("  %s %s %s%s\n", h.At.Format("15:04"), h.By, h.Action, detail)
		}
	}

	return panel("Issue: "+iss.ID, s, m.width-2)
}

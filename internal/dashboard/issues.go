package dashboard

import (
	"fmt"
	"strings"
)

func (m Model) renderIssues() string {
	s := headerStyle.Render(fmt.Sprintf("ISSUES (%d)", len(m.data.issues))) + "\n\n"
	s += fmt.Sprintf("  %-12s %-8s %-14s %-36s %s\n",
		"ID", "TYPE", "STATUS", "TITLE", "ASSIGNEE")
	s += "  " + strings.Repeat("─", 85) + "\n"

	for i, iss := range m.data.issues {
		line := fmt.Sprintf("  %-12s %-8s %-14s %-36s %s",
			iss.ID, iss.Type, iss.Status, truncate(iss.Title, 36), iss.Assignee)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		} else {
			line = statusStyle(iss.Status).Render(line)
		}
		s += line + "\n"
	}
	return s
}

func (m Model) renderIssueDetail() string {
	if m.cursor >= len(m.data.issues) {
		return "No issue selected"
	}
	iss := m.data.issues[m.cursor]

	s := headerStyle.Render("Issue: "+iss.ID) + "\n\n"
	s += fmt.Sprintf("  Title:    %s\n", iss.Title)
	s += fmt.Sprintf("  Type:     %s\n", iss.Type)
	s += fmt.Sprintf("  Status:   %s\n", statusStyle(iss.Status).Render(iss.Status))
	s += fmt.Sprintf("  Priority: %s\n", iss.Priority)
	if iss.Assignee != "" {
		s += fmt.Sprintf("  Assignee: %s\n", iss.Assignee)
	}
	if iss.Description != "" {
		s += fmt.Sprintf("\n  %s\n", iss.Description)
	}
	if iss.Parent != "" {
		s += fmt.Sprintf("  Parent:   %s\n", iss.Parent)
	}
	if len(iss.DependsOn) > 0 {
		s += fmt.Sprintf("  Depends:  %s\n", strings.Join(iss.DependsOn, ", "))
	}
	if len(iss.Children) > 0 {
		s += "\n  " + headerStyle.Render("CHILDREN") + "\n"
		for _, cid := range iss.Children {
			label := cid
			for _, ci := range m.data.issues {
				if ci.ID == cid {
					label = fmt.Sprintf("%s [%s] %s", cid, ci.Status, truncate(ci.Title, 30))
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
	return s
}

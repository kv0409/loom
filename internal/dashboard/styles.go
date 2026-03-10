package dashboard

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var wtPrefixRe = regexp.MustCompile(`^LOOM-\d+-\d+-`)

func slugFromWorktree(name string) string {
	return wtPrefixRe.ReplaceAllString(name, "")
}

var (
	green  = lipgloss.Color("2")
	yellow = lipgloss.Color("3")
	red    = lipgloss.Color("1")
	cyan   = lipgloss.Color("6")
	gray   = lipgloss.Color("8")
	white  = lipgloss.Color("15")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(white)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(white)
	activeStyle   = lipgloss.NewStyle().Foreground(green)
	blockedStyle  = lipgloss.NewStyle().Foreground(yellow)
	reviewStyle   = lipgloss.NewStyle().Foreground(cyan)
	deadStyle     = lipgloss.NewStyle().Foreground(red)
	idleStyle     = lipgloss.NewStyle().Foreground(gray)
	selectedStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	helpStyle     = lipgloss.NewStyle().Foreground(gray)
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gray)
)

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "active", "in-progress", "assigned", "done":
		return activeStyle
	case "blocked":
		return blockedStyle
	case "review":
		return reviewStyle
	case "dead", "error", "cancelled":
		return deadStyle
	default:
		return idleStyle
	}
}

func statusIndicator(status string) string {
	switch status {
	case "blocked":
		return blockedStyle.Render("⚠")
	case "review":
		return reviewStyle.Render("⚠")
	case "dead", "error", "cancelled":
		return deadStyle.Render("●")
	case "active", "in-progress", "assigned", "done":
		return activeStyle.Render("●")
	default:
		return idleStyle.Render("●")
	}
}

func panel(title string, content string, width int) string {
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	s := borderStyle.Width(innerW).Render(content)
	if title != "" {
		lines := splitLines(s)
		if len(lines) > 0 {
			t := titleStyle.Render(" " + title + " ")
			tLen := lipgloss.Width(t)
			borderColor := lipgloss.NewStyle().Foreground(gray)
			remaining := innerW - tLen - 1
			if remaining < 0 {
				remaining = 0
			}
			lines[0] = borderColor.Render("╭─") + t + borderColor.Render(strings.Repeat("─", remaining)+"╮")
			s = joinLines(lines)
		}
	}
	return s
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

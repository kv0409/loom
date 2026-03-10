package dashboard

import "github.com/charmbracelet/lipgloss"

var (
	green  = lipgloss.Color("2")
	yellow = lipgloss.Color("3")
	red    = lipgloss.Color("1")
	gray   = lipgloss.Color("8")
	white  = lipgloss.Color("15")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(white)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(white)
	activeStyle   = lipgloss.NewStyle().Foreground(green)
	blockedStyle  = lipgloss.NewStyle().Foreground(yellow)
	deadStyle     = lipgloss.NewStyle().Foreground(red)
	idleStyle     = lipgloss.NewStyle().Foreground(gray)
	selectedStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	helpStyle     = lipgloss.NewStyle().Foreground(gray)
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gray)
)

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "active", "in-progress", "done":
		return activeStyle
	case "blocked":
		return blockedStyle
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
	case "dead", "error", "cancelled":
		return deadStyle.Render("●")
	case "active", "in-progress", "done":
		return activeStyle.Render("●")
	default:
		return idleStyle.Render("●")
	}
}

func panel(title string, content string, width int) string {
	s := borderStyle.Width(width - 2).Render(content)
	if title != "" {
		// Overlay title on top border
		t := titleStyle.Render(" " + title + " ")
		lines := splitLines(s)
		if len(lines) > 0 {
			topBorder := lines[0]
			if len(topBorder) > 3 {
				tLen := lipgloss.Width(t)
				if tLen+3 < lipgloss.Width(topBorder) {
					lines[0] = topBorder[:2] + t + topBorder[2+tLen:]
				}
			}
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

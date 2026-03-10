package dashboard

import "github.com/charmbracelet/lipgloss"

var (
	green  = lipgloss.Color("2")
	yellow = lipgloss.Color("3")
	red    = lipgloss.Color("1")
	gray   = lipgloss.Color("8")
	white  = lipgloss.Color("15")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(white)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(white).Underline(true)
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

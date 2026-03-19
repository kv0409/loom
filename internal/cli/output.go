package cli

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
)

// Tokyo Night palette (matches dashboard/styles.go)
var (
	colGreen  = lipgloss.Color("#9ECE6A")
	colYellow = lipgloss.Color("#E0AF68")
	colRed    = lipgloss.Color("#F7768E")
	colGray   = lipgloss.Color("#565F89")
	colBlue   = lipgloss.Color("#7AA2F7")
)

func noColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

func colored(s string, style lipgloss.Style) string {
	if noColor() {
		return s
	}
	return style.Render(s)
}

// PrintSuccess prints a green checkmark line: "✓ msg" or "✓ msg — id"
func PrintSuccess(msg string, id ...string) {
	line := "✓ " + msg
	if len(id) > 0 && id[0] != "" {
		line += colored(" — "+id[0], lipgloss.NewStyle().Foreground(colBlue))
	}
	fmt.Println(colored(line, lipgloss.NewStyle().Foreground(colGreen)))
}

// PrintWarning prints a yellow warning line: "! msg" or "! msg — hint"
func PrintWarning(msg string, hint ...string) {
	line := "! " + msg
	if len(hint) > 0 && hint[0] != "" {
		line += " — " + hint[0]
	}
	fmt.Println(colored(line, lipgloss.NewStyle().Foreground(colYellow)))
}

// PrintError prints a red error line to stderr: "✗ msg" or "✗ msg — hint"
func PrintError(msg string, hint ...string) {
	line := "✗ " + msg
	if len(hint) > 0 && hint[0] != "" {
		line += " — " + hint[0]
	}
	fmt.Fprintln(os.Stderr, colored(line, lipgloss.NewStyle().Foreground(colRed)))
}

// PrintInfo prints dimmed informational text.
func PrintInfo(msg string) {
	fmt.Println(colored(msg, lipgloss.NewStyle().Foreground(colGray)))
}

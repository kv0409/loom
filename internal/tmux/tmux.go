package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func tmuxBin() (string, error) {
	return exec.LookPath("tmux")
}

func run(args ...string) (string, error) {
	bin, err := tmuxBin()
	if err != nil {
		return "", fmt.Errorf("tmux not found: %w", err)
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// SessionExists checks if a tmux session with the given name exists.
func SessionExists(name string) bool {
	_, err := run("has-session", "-t", name)
	return err == nil
}

// CreateSession creates a new detached tmux session.
func CreateSession(name string) error {
	_, err := run("new-session", "-d", "-s", name)
	return err
}

// KillSession kills a tmux session.
func KillSession(name string) error {
	_, err := run("kill-session", "-t", name)
	return err
}

// NewWindow creates a new window in the session, returns the window target (e.g., "loom:3").
func NewWindow(session string, windowName string) (string, error) {
	return run("new-window", "-t", session, "-n", windowName, "-P", "-F", "#{session_name}:#{window_index}")
}

// SplitWindow splits a window into a new pane, returns the pane target (e.g., "loom:3.1").
func SplitWindow(target string) (string, error) {
	return run("split-window", "-t", target, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}")
}

// SendKeys sends keystrokes to a tmux target. Does NOT append Enter automatically.
func SendKeys(target string, keys string) error {
	_, err := run("send-keys", "-t", target, keys)
	return err
}

// KillPane kills a specific pane.
func KillPane(target string) error {
	_, err := run("kill-pane", "-t", target)
	return err
}

// KillWindow kills a specific window.
func KillWindow(target string) error {
	_, err := run("kill-window", "-t", target)
	return err
}

// ListWindows returns window names in a session.
func ListWindows(session string) ([]string, error) {
	out, err := run("list-windows", "-t", session, "-F", "#{window_name}")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ListPanes returns pane targets in a window.
func ListPanes(target string) ([]string, error) {
	out, err := run("list-panes", "-t", target, "-F", "#{session_name}:#{window_index}.#{pane_index}")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// AttachSession attaches to a session, replacing the current process.
func AttachSession(session string) error {
	bin, err := tmuxBin()
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(bin, []string{"tmux", "attach-session", "-t", session}, os.Environ())
}

// SelectPane selects/focuses a specific pane.
func SelectPane(target string) error {
	_, err := run("select-pane", "-t", target)
	return err
}

// CapturePane captures the current content of a pane.
func CapturePane(target string) (string, error) {
	return run("capture-pane", "-t", target, "-p")
}

// RunInPane sends a command to a pane and presses Enter.
func RunInPane(target string, command string) error {
	return SendKeys(target, command+" Enter")
}

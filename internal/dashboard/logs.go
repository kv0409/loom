package dashboard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type logLine struct {
	Category string
	Text     string
}

// logReader tracks file offset for incremental reads.
type logReader struct {
	mu      sync.Mutex
	path    string
	offset  int64
	lines   []logLine
	partial string // incomplete line from last read
}

func newLogReader(loomRoot string) *logReader {
	return &logReader{path: filepath.Join(loomRoot, "logs", "daemon.log")}
}

const maxLogLines = 200

// read returns all accumulated log lines, reading only new bytes since last call.
func (r *logReader) read() []logLine {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(r.path)
	if err != nil {
		return r.lines
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return r.lines
	}

	// Handle truncation/rotation: file shrank → reset
	if info.Size() < r.offset {
		r.offset = 0
		r.lines = nil
		r.partial = ""
	}

	if info.Size() == r.offset {
		return r.lines
	}

	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return r.lines
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		return r.lines
	}
	r.offset += int64(len(buf))

	// Prepend any partial line from previous read
	text := r.partial + string(buf)
	r.partial = ""

	rawLines := strings.Split(text, "\n")

	// Last element may be incomplete if file didn't end with newline
	if len(rawLines) > 0 && !strings.HasSuffix(text, "\n") {
		r.partial = rawLines[len(rawLines)-1]
		rawLines = rawLines[:len(rawLines)-1]
	}

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		cat, t, ok := classifyLogLine(trimmed)
		if !ok {
			continue
		}
		r.lines = append(r.lines, logLine{Category: cat, Text: t})
	}

	// Cap stored lines
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}

	return r.lines
}

// classifyLogLine returns (category, display text, keep).
func classifyLogLine(line string) (string, string, bool) {
	if strings.Contains(line, "[acp-notif]") {
		return "", "", false
	}
	if strings.Contains(line, "[acp] activating agent") ||
		strings.Contains(line, "calling Initialize") ||
		strings.Contains(line, "calling NewSession") ||
		strings.Contains(line, "sending initial task") ||
		strings.Contains(line, "process exited") ||
		strings.Contains(line, "session=") {
		return "lifecycle", line, true
	}
	if strings.Contains(line, "failed") || strings.Contains(line, "Failed") ||
		strings.Contains(line, "error") || strings.Contains(line, "Error") {
		return "error", line, true
	}
	if strings.Contains(line, "[acp-stderr]") {
		return "stderr", line, true
	}
	if strings.Contains(line, "Warning") || strings.Contains(line, "WARNING") {
		return "warn", line, true
	}
	return "", "", false
}

func (m Model) renderLogs() string {
	filter := ""
	categories := []string{"", "lifecycle", "error", "stderr", "warn"}
	if m.logFilter > 0 && m.logFilter < len(categories) {
		filter = categories[m.logFilter]
	}

	filterLabel := "all"
	if filter != "" {
		filterLabel = filter
	}
	header := fmt.Sprintf("  Filter: [%s]  (f to cycle: all → lifecycle → error → stderr → warn)\n", filterLabel)
	header += "  " + strings.Repeat("─", m.width-6) + "\n"

	var lines []logLine
	for _, l := range m.data.logs {
		if filter == "" || l.Category == filter {
			lines = append(lines, l)
		}
	}

	if len(lines) == 0 {
		header += "  (no matching log entries)\n"
		return panel("LOGS", header, m.width-2)
	}

	visible := m.height - 8
	if visible < 5 {
		visible = 5
	}
	start := 0
	if len(lines) > visible {
		start = len(lines) - visible
	}

	content := header
	for i := start; i < len(lines); i++ {
		l := lines[i]
		tag := categoryTag(l.Category)
		content += fmt.Sprintf("  %s %s\n", tag, truncate(l.Text, m.width-16))
	}

	return panel(fmt.Sprintf("LOGS (%d events)", len(lines)), content, m.width-2)
}

func categoryTag(cat string) string {
	switch cat {
	case "error":
		return deadStyle.Render(fmt.Sprintf("%-10s", "ERROR"))
	case "stderr":
		return idleStyle.Render(fmt.Sprintf("%-10s", "STDERR"))
	case "warn":
		return idleStyle.Render(fmt.Sprintf("%-10s", "WARN"))
	default:
		return activeStyle.Render(fmt.Sprintf("%-10s", cat))
	}
}

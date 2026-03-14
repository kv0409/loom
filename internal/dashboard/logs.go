package dashboard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/table"
)

type logLine struct {
	Category string
	Agent    string
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

var agentRe = regexp.MustCompile(`\[acp\]\s+(\S+?):|activating agent (\S+)`)

// read returns all accumulated log lines, reading only new bytes since last call.
// It returns a snapshot copy so the caller (bubbletea Cmd goroutine) never shares
// the underlying array with a concurrent read() call, avoiding a data race.
func (r *logReader) read() []logLine {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(r.path)
	if err != nil {
		return r.snapshot()
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return r.snapshot()
	}

	// Handle truncation/rotation: file shrank → reset
	if info.Size() < r.offset {
		r.offset = 0
		r.lines = nil
		r.partial = ""
	}

	if info.Size() == r.offset {
		return r.snapshot()
	}

	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return r.snapshot()
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		return r.snapshot()
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
		r.lines = append(r.lines, logLine{Category: cat, Agent: extractAgent(trimmed), Text: t})
	}

	// Cap stored lines
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}

	return r.snapshot()
}

// snapshot returns a copy of r.lines. Must be called with r.mu held.
func (r *logReader) snapshot() []logLine {
	cp := make([]logLine, len(r.lines))
	copy(cp, r.lines)
	return cp
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

// extractAgent pulls an agent identifier from a log line.
func extractAgent(line string) string {
	m := agentRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

func (m Model) countLogAgents() int {
	seen := map[string]bool{}
	for _, l := range m.data.logs {
		if l.Agent != "" {
			seen[l.Agent] = true
		}
	}
	return len(seen)
}

func (m Model) renderLogs() string {
	// Collect unique agents from log data
	agentSet := map[string]bool{}
	for _, l := range m.data.logs {
		if l.Agent != "" {
			agentSet[l.Agent] = true
		}
	}
	agents := make([]string, 0, len(agentSet))
	for a := range agentSet {
		agents = append(agents, a)
	}
	sort.Strings(agents)

	filter := ""
	categories := []string{"", "lifecycle", "error", "stderr", "warn"}
	if m.logFilter > 0 && m.logFilter < len(categories) {
		filter = categories[m.logFilter]
	}

	agentFilter := ""
	if m.logAgentFilter > 0 && m.logAgentFilter <= len(agents) {
		agentFilter = agents[m.logAgentFilter-1]
	}

	filterLabel := "all"
	if filter != "" {
		filterLabel = filter
	}
	agentLabel := "all"
	if agentFilter != "" {
		agentLabel = agentFilter
	}
	header := fmt.Sprintf("  Filter: [%s]  Agent: [%s]  (f=category, F=agent)\n", filterLabel, agentLabel)
	header += separator(m.width) + "\n"

	var lines []logLine
	for _, l := range m.data.logs {
		if filter != "" && l.Category != filter {
			continue
		}
		if agentFilter != "" && l.Agent != agentFilter {
			continue
		}
		lines = append(lines, l)
	}

	if len(lines) == 0 {
		header += renderEmpty("No matching log entries", m.width-6)
		return panel("[l] LOGS", header, m.width-2)
	}

	visible := visibleRows(m.height, 9)
	start, end := listViewport(m.cursor, len(lines), visible)

	tagW := 10
	const numColsLogs = 2
	textW := m.width - tagW - 8 - numColsLogs*2
	if textW < 10 {
		textW = 10
	}
	cols := []table.Column{
		{Title: "CAT", Width: tagW},
		{Title: "MESSAGE", Width: textW},
	}
	rows := make([]table.Row, 0, end-start)
	for i := start; i < end; i++ {
		l := lines[i]
		rows = append(rows, table.Row{categoryTag(l.Category), truncate(l.Text, textW)})
	}
	t := newStyledTable(cols, rows, end-start)

	content := header + t.View()
	return panel(fmt.Sprintf("[l] LOGS (%d events)", len(lines)), content, m.width-2)
}

func categoryTag(cat string) string {
	switch cat {
	case "error":
		return deadStyle.Render("ERROR")
	case "stderr":
		return idleStyle.Render("STDERR")
	case "warn":
		return idleStyle.Render("WARN")
	default:
		return activeStyle.Render(cat)
	}
}

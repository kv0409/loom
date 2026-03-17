package backend

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const maxLogLines = 200

var agentRe = regexp.MustCompile(`\[acp\]\s+(\S+?):|activating agent (\S+)`)

// logReader tracks file offset for incremental reads of the daemon log.
type logReader struct {
	mu      sync.Mutex
	path    string
	offset  int64
	lines   []LogLine
	partial string
}

func newLogReader(loomRoot string) *logReader {
	return &logReader{path: filepath.Join(loomRoot, "logs", "daemon.log")}
}

// read returns all accumulated log lines, reading only new bytes since last call.
func (r *logReader) read() []LogLine {
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

	text := r.partial + string(buf)
	r.partial = ""

	rawLines := strings.Split(text, "\n")

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
		r.lines = append(r.lines, LogLine{Category: cat, Agent: extractAgent(trimmed), Text: t})
	}

	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}

	return r.snapshot()
}

// snapshot returns a copy of r.lines. Must be called with r.mu held.
func (r *logReader) snapshot() []LogLine {
	cp := make([]LogLine, len(r.lines))
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

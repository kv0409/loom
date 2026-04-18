package backend

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
		cat, t, ok := ClassifyLogLine(trimmed)
		if !ok {
			continue
		}
		r.lines = append(r.lines, LogLine{Category: cat, Agent: ExtractAgent(trimmed), Text: t})
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

// ClassifyLogLine returns (category, display text, keep).
func ClassifyLogLine(line string) (string, string, bool) {
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

// ExtractAgent pulls an agent identifier from a log line.
func ExtractAgent(line string) string {
	m := agentRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

// maxReadDaemonLogBytes caps how much of daemon.log we load into memory. The
// daemon rotates the file at Config.Limits.LogMaxSizeMB, but old large logs
// or racy growth between rotations can push past that — tail a bounded window
// so UI reads stay cheap.
const maxReadDaemonLogBytes = 2 * 1024 * 1024

// ReadDaemonLog reads the tail of the daemon log file and returns all
// classified lines. For files larger than maxReadDaemonLogBytes only the
// trailing window is parsed (the first partial line is dropped).
func ReadDaemonLog(loomRoot string) []LogLine {
	f, err := os.Open(filepath.Join(loomRoot, "logs", "daemon.log"))
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	size := info.Size()
	partial := false
	if size > maxReadDaemonLogBytes {
		if _, err := f.Seek(size-maxReadDaemonLogBytes, io.SeekStart); err != nil {
			return nil
		}
		partial = true
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	raws := strings.Split(string(data), "\n")
	if partial && len(raws) > 0 {
		raws = raws[1:] // drop likely-truncated first line
	}
	var lines []LogLine
	for _, raw := range raws {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		cat, text, ok := ClassifyLogLine(trimmed)
		if !ok {
			continue
		}
		lines = append(lines, LogLine{Category: cat, Agent: ExtractAgent(trimmed), Text: text})
	}
	return lines
}

// AgentCount pairs an agent ID with its event count.
type AgentCount struct {
	Agent string
	Count int
}

// Summary holds aggregate stats for a set of log lines.
type Summary struct {
	Errors    int
	Warnings  int
	Total     int
	HotAgents []AgentCount
}

// LogSummary computes aggregate stats from classified log lines.
func LogSummary(lines []LogLine) Summary {
	s := Summary{Total: len(lines)}
	counts := map[string]int{}
	for _, l := range lines {
		switch l.Category {
		case "error", "stderr":
			s.Errors++
		case "warn":
			s.Warnings++
		}
		if l.Agent != "" {
			counts[l.Agent]++
		}
	}
	for agent, c := range counts {
		s.HotAgents = append(s.HotAgents, AgentCount{agent, c})
	}
	sort.Slice(s.HotAgents, func(i, j int) bool {
		if s.HotAgents[i].Count != s.HotAgents[j].Count {
			return s.HotAgents[i].Count > s.HotAgents[j].Count
		}
		return s.HotAgents[i].Agent < s.HotAgents[j].Agent
	})
	if len(s.HotAgents) > 3 {
		s.HotAgents = s.HotAgents[:3]
	}
	return s
}

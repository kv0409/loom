package acp

import (
	"encoding/json"
	"strings"
)

// Kind classifies an ACPEvent.
type Kind int

const (
	TokenChunk     Kind = iota // reassembled agent_message_chunk stream
	ToolSummary                // session_update line (tool use / status)
	CompleteMessage            // any other non-empty output line
)

var kindNames = [...]string{"token_chunk", "tool_summary", "message"}

func (k Kind) MarshalJSON() ([]byte, error) {
	if int(k) < len(kindNames) {
		return []byte(`"` + kindNames[k] + `"`), nil
	}
	return []byte(`"unknown"`), nil
}

func (k *Kind) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	for i, name := range kindNames {
		if name == s {
			*k = Kind(i)
			return nil
		}
	}
	*k = CompleteMessage
	return nil
}

// ACPEvent is a parsed, typed unit of ACP output.
type ACPEvent struct {
	Kind      Kind   `json:"kind"`
	Timestamp string `json:"ts,omitempty"`
	Content   string `json:"content"`
}

// ReadOutputFile reads an .output file, trying NDJSON first, falling back to
// legacy text format for files written before the structured output change.
func ReadOutputFile(raw []byte) []ACPEvent {
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	// Probe first non-empty line to detect format.
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if l[0] == '{' {
			return readNDJSON(lines)
		}
		return ParseOutput(string(raw))
	}
	return nil
}

func readNDJSON(lines []string) []ACPEvent {
	var events []ACPEvent
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev ACPEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events
}

// eventKind maps ACP sessionUpdate strings to typed Kind values.
func eventKind(sessionUpdate string) Kind {
	switch sessionUpdate {
	case "agent_message_chunk":
		return TokenChunk
	case "session_update":
		return ToolSummary
	default:
		return CompleteMessage
	}
}

// ParseOutput parses the raw content of an agent .output file into typed events.
// Input lines are expected in the format written by the daemon:
//
//	[HH:MM:SS] [sessionUpdate]<tab>content
//
// agent_message_chunk lines are reassembled into a single TokenChunk event per
// contiguous run. session_update lines become ToolSummary events. All other
// non-empty lines become CompleteMessage events.
func ParseOutput(raw string) []ACPEvent {
	type pending struct {
		ts      string
		builder strings.Builder
	}
	var events []ACPEvent
	var chunk *pending // active reassembly buffer for token chunks

	flushChunk := func() {
		if chunk != nil {
			events = append(events, ACPEvent{Kind: TokenChunk, Timestamp: chunk.ts, Content: chunk.builder.String()})
			chunk = nil
		}
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		// Extract optional [HH:MM:SS] prefix.
		ts, rest := "", line
		if len(line) >= 10 && line[0] == '[' && line[9] == ']' {
			ts = line[1:9]
			rest = strings.TrimLeft(line[10:], " ")
		}

		// Extract [kind] prefix.
		kind, content := "", rest
		if strings.HasPrefix(rest, "[") {
			if end := strings.Index(rest, "]"); end > 0 {
				kind = rest[1:end]
				content = strings.TrimLeft(rest[end+1:], "\t ")
			}
		}

		switch kind {
		case "agent_message_chunk":
			if chunk == nil {
				chunk = &pending{ts: ts}
			}
			chunk.builder.WriteString(content)
		case "session_update":
			flushChunk()
			if content != "" {
				events = append(events, ACPEvent{Kind: ToolSummary, Timestamp: ts, Content: content})
			}
		default:
			flushChunk()
			if content != "" {
				events = append(events, ACPEvent{Kind: CompleteMessage, Timestamp: ts, Content: content})
			}
		}
	}
	flushChunk()
	return events
}

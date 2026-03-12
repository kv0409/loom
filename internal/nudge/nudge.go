// Package nudge defines predefined nudge types for the Loom system.
package nudge

// Type is a predefined nudge signal.
type Type struct {
	Key     string // short identifier (used by CLI)
	Label   string // human-readable label (shown in dashboard menu)
	Message string // text sent to the agent
}

// Types is the ordered list of predefined nudge types.
var Types = []Type{
	{"check-inbox", "Check your inbox", "[LOOM] Nudge: Check your inbox — you have unread mail"},
	{"heartbeat-stale", "Heartbeat stale", "[LOOM] Nudge: Your heartbeat is stale — run loom agent heartbeat"},
	{"child-needs-attention", "Child agent needs attention", "[LOOM] Nudge: One of your child agents needs attention"},
	{"resume-work", "Resume work", "[LOOM] Nudge: Please resume work on your assigned task"},
	{"report-status", "Report status", "[LOOM] Nudge: Please report your current status"},
}

// ByKey returns the nudge type with the given key, or nil if not found.
func ByKey(key string) *Type {
	for i := range Types {
		if Types[i].Key == key {
			return &Types[i]
		}
	}
	return nil
}

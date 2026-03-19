package dashboard

import (
	"testing"
)

func TestAgentColor_KnownRoles(t *testing.T) {
	cases := []struct {
		id   string
		role string
	}{
		{"orchestrator", "orchestrator"},
		{"lead-001", "lead"},
		{"builder-001", "builder"},
		{"reviewer-001", "reviewer"},
		{"explorer-001", "explorer"},
		{"researcher-001", "researcher"},
	}
	for _, tc := range cases {
		c := agentColor(tc.id)
		if c == nil {
			t.Errorf("agentColor(%q) returned nil", tc.id)
		}
	}
}

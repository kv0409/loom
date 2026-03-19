package dashboard

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/karanagi/loom/internal/dashboard/backend"
)

func TestRenderActivity_ColumnWidthOffByOne(t *testing.T) {
	m := Model{width: 80, height: 30}
	// Populate with a few entries so the table renders.
	m.data.Activity = []backend.ActivityEntry{
		{AgentID: "agent-1", Line: "Called execute_bash: go test"},
		{AgentID: "agent-2", Line: "Called fs_read: main.go"},
	}

	rendered := m.renderActivity()
	// Each rendered line must fit within the full terminal width.
	for i, line := range strings.Split(rendered, "\n") {
		w := lipgloss.Width(line)
		if w > m.width {
			t.Errorf("line %d width %d exceeds terminal width %d: %q", i, w, m.width, line)
		}
	}
}

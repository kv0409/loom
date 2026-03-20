package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/karanagi/loom/internal/dashboard/backend"
)

func TestRenderChatPane_ShowsOnlyCurrentOrchestratorSession(t *testing.T) {
	m := testModel(viewOverview)
	now := time.Now()
	m.data.Agents = []*backend.Agent{
		{
			ID:        "orchestrator",
			Role:      "orchestrator",
			SpawnedAt: now.Add(-10 * time.Minute),
		},
	}
	m.data.Messages = []*backend.Message{
		{
			From:      "dashboard",
			To:        "orchestrator",
			Subject:   "old task",
			Timestamp: now.Add(-30 * time.Minute),
		},
		{
			From:      "orchestrator",
			To:        "dashboard",
			Subject:   "old reply",
			Timestamp: now.Add(-20 * time.Minute),
		},
		{
			From:      "dashboard",
			To:        "orchestrator",
			Subject:   "current task",
			Timestamp: now.Add(-5 * time.Minute),
		},
		{
			From:      "orchestrator",
			To:        "dashboard",
			Subject:   "current reply",
			Timestamp: now.Add(-4 * time.Minute),
		},
	}

	got := m.renderChatPane()

	if strings.Contains(got, "old task") || strings.Contains(got, "old reply") {
		t.Fatalf("expected old chat history to be hidden, got:\n%s", got)
	}
	if !strings.Contains(got, "current task") || !strings.Contains(got, "current reply") {
		t.Fatalf("expected current chat history to be shown, got:\n%s", got)
	}
}

func TestRenderChatPane_WithoutOrchestratorFallsBackToFullConversation(t *testing.T) {
	m := testModel(viewOverview)
	now := time.Now()
	m.data.Messages = []*backend.Message{
		{
			From:      "dashboard",
			To:        "orchestrator",
			Subject:   "task",
			Timestamp: now.Add(-2 * time.Minute),
		},
		{
			From:      "orchestrator",
			To:        "dashboard",
			Subject:   "reply",
			Timestamp: now.Add(-time.Minute),
		},
	}

	got := m.renderChatPane()

	if !strings.Contains(got, "task") || !strings.Contains(got, "reply") {
		t.Fatalf("expected chat history to render without orchestrator metadata, got:\n%s", got)
	}
}

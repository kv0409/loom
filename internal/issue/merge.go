package issue

import (
	"fmt"
	"time"
)

// Merge transitions an issue to "done" with merged_at set, recording the merge
// in history. It clears the assignee so status-driven daemon logic no longer
// treats the issue as active. Returns the updated issue.
func Merge(loomRoot, id string) (*Issue, error) {
	iss, err := Load(loomRoot, id)
	if err != nil {
		return nil, err
	}

	if iss.Status == "done" || iss.Status == "cancelled" {
		return nil, fmt.Errorf("cannot merge %s: already in terminal state %q", id, iss.Status)
	}

	now := time.Now()
	iss.MergedAt = &now
	iss.ClosedAt = &now
	iss.CloseReason = "merged"
	iss.History = append(iss.History, HistoryEntry{
		At: now, By: actor(), Action: "merged",
		Detail: iss.Status + " → done",
	})
	iss.Status = "done"
	iss.Assignee = ""

	if err := Save(loomRoot, iss); err != nil {
		return nil, err
	}
	return iss, nil
}

// IsMerged returns true if the issue has been merged (MergedAt is set).
func (iss *Issue) IsMerged() bool {
	return iss.MergedAt != nil
}

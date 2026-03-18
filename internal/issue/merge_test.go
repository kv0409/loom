package issue

import (
	"testing"
)

func TestMerge_SetsStatusDone(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "merge me", CreateOpts{})
	iss.Status = "review"
	iss.Assignee = "builder-001"
	Save(root, iss)

	merged, err := Merge(root, iss.ID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != "done" {
		t.Errorf("expected status done, got %s", merged.Status)
	}
	if merged.MergedAt == nil {
		t.Error("expected MergedAt to be set")
	}
	if merged.ClosedAt == nil {
		t.Error("expected ClosedAt to be set")
	}
	if merged.CloseReason != "merged" {
		t.Errorf("expected close reason 'merged', got %q", merged.CloseReason)
	}
}

func TestMerge_ClearsAssignee(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "assigned merge", CreateOpts{})
	iss.Status = "in-progress"
	iss.Assignee = "builder-001"
	Save(root, iss)

	merged, err := Merge(root, iss.ID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Assignee != "" {
		t.Errorf("expected assignee cleared, got %q", merged.Assignee)
	}
}

func TestMerge_RecordsHistory(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "history merge", CreateOpts{})
	iss.Status = "review"
	Save(root, iss)

	merged, err := Merge(root, iss.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := merged.History[len(merged.History)-1]
	if last.Action != "merged" {
		t.Errorf("expected last history action 'merged', got %q", last.Action)
	}
}

func TestMerge_RejectsTerminalState(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "already done", CreateOpts{})
	iss.Status = "done"
	Save(root, iss)

	_, err := Merge(root, iss.ID)
	if err == nil {
		t.Error("expected error merging already-done issue")
	}
}

func TestMerge_RejectsCancelled(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "cancelled", CreateOpts{})
	iss.Status = "cancelled"
	Save(root, iss)

	_, err := Merge(root, iss.ID)
	if err == nil {
		t.Error("expected error merging cancelled issue")
	}
}

func TestMerge_PersistsToDisk(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "persist merge", CreateOpts{})
	iss.Status = "review"
	Save(root, iss)

	Merge(root, iss.ID)

	reloaded, err := Load(root, iss.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "done" {
		t.Errorf("expected persisted status done, got %s", reloaded.Status)
	}
	if reloaded.MergedAt == nil {
		t.Error("expected persisted MergedAt")
	}
}

func TestMerge_NotVisibleInActiveList(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "merged issue", CreateOpts{})
	iss.Status = "review"
	Save(root, iss)

	Merge(root, iss.ID)

	// Default List excludes done/cancelled.
	active, err := List(root, ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.ID == iss.ID {
			t.Error("merged issue should not appear in active list")
		}
	}
}

func TestIsMerged_Method(t *testing.T) {
	root := setupTestRoot(t)
	iss, _ := Create(root, "check merged", CreateOpts{})
	if iss.IsMerged() {
		t.Error("new issue should not be merged")
	}

	iss.Status = "review"
	Save(root, iss)
	Merge(root, iss.ID)

	reloaded, _ := Load(root, iss.ID)
	if !reloaded.IsMerged() {
		t.Error("merged issue should report IsMerged=true")
	}
}

package issue

import (
	"testing"
)

func TestValidateTransition_ValidTransitions(t *testing.T) {
	valid := []struct{ from, to string }{
		{"open", "assigned"},
		{"assigned", "in-progress"},
		{"in-progress", "review"},
		{"in-progress", "blocked"},
		{"in-progress", "done"},
		{"blocked", "in-progress"},
		{"review", "done"},
		{"review", "in-progress"},
	}
	for _, tc := range valid {
		if err := validateTransition(tc.from, tc.to); err != nil {
			t.Errorf("expected %s → %s to be valid, got error: %v", tc.from, tc.to, err)
		}
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	invalid := []struct{ from, to string }{
		{"open", "done"},
		{"open", "in-progress"},
		{"open", "review"},
		{"open", "blocked"},
		{"assigned", "done"},
		{"assigned", "review"},
		{"assigned", "blocked"},
		{"assigned", "open"},
		{"in-progress", "open"},
		{"in-progress", "assigned"},
		{"blocked", "done"},
		{"blocked", "open"},
		{"blocked", "review"},
		{"review", "open"},
		{"review", "blocked"},
		{"done", "open"},
		{"done", "in-progress"},
	}
	for _, tc := range invalid {
		if err := validateTransition(tc.from, tc.to); err == nil {
			t.Errorf("expected %s → %s to be invalid, but got no error", tc.from, tc.to)
		}
	}
}

func TestValidateTransition_CancelledAlwaysAllowed(t *testing.T) {
	statuses := []string{"open", "assigned", "in-progress", "blocked", "review", "done"}
	for _, from := range statuses {
		if err := validateTransition(from, "cancelled"); err != nil {
			t.Errorf("expected %s → cancelled to be valid, got error: %v", from, err)
		}
	}
}

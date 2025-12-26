package actions

import (
	"testing"
)

func TestValidateAction_UnknownAction(t *testing.T) {
	err := ValidateAction("nonexistent_action", nil)
	if err == nil {
		t.Error("expected error for unknown action")
	}
	if err.Error() != "unknown action 'nonexistent_action'" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateAction_KnownActionWithoutPreflight(t *testing.T) {
	// Actions that don't implement Preflight should pass validation
	// (most actions don't implement it yet during migration)
	err := ValidateAction("download", nil)
	if err != nil {
		t.Errorf("expected nil for action without Preflight, got: %v", err)
	}
}

func TestRegisteredNames(t *testing.T) {
	names := RegisteredNames()

	// Should have actions registered
	if len(names) == 0 {
		t.Error("expected registered actions")
	}

	// Should include known actions
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	expected := []string{"download", "extract", "chmod", "install_binaries"}
	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("expected action '%s' to be registered", exp)
		}
	}

	// Should be sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %s comes after %s", names[i], names[i-1])
		}
	}
}

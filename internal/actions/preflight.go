package actions

import (
	"fmt"
	"sort"
)

// Preflight is implemented by actions that can validate their parameters
// without executing side effects. This enables static validation of recipes.
//
// Actions implementing Preflight should use shared parameter extraction functions
// that are also used by Execute(), ensuring validation and execution cannot drift.
type Preflight interface {
	// Preflight validates that the given parameters would produce a valid
	// action execution. Returns nil if valid, error describing the problem
	// if invalid.
	//
	// CONTRACT: Preflight MUST NOT have side effects (no filesystem, no network).
	// It validates parameter presence, types, and semantic correctness.
	Preflight(params map[string]interface{}) error
}

// ValidateAction checks if an action exists and validates its parameters.
// This is the entry point for semantic validation of action steps.
//
// Returns nil if:
//   - The action exists and implements Preflight, and Preflight returns nil
//   - The action exists but does not implement Preflight (passes by default)
//
// Returns error if:
//   - The action does not exist
//   - The action implements Preflight and Preflight returns an error
func ValidateAction(name string, params map[string]interface{}) error {
	action := Get(name)
	if action == nil {
		return fmt.Errorf("unknown action '%s'", name)
	}

	if pf, ok := action.(Preflight); ok {
		return pf.Preflight(params)
	}

	// Action exists but doesn't implement Preflight - pass validation
	return nil
}

// RegisteredNames returns all registered action names sorted alphabetically.
// This is useful for error messages suggesting valid action names.
func RegisteredNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

package actions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// PreflightResult contains the results of preflight validation.
// Actions can return both errors (fatal) and warnings (non-fatal suggestions).
type PreflightResult struct {
	// Errors are fatal validation failures that would cause execution to fail.
	Errors []string

	// Warnings are non-fatal suggestions for improvement.
	// Examples: missing platform support, deprecated parameters, redundant config.
	Warnings []string
}

// AddError adds an error to the result.
func (r *PreflightResult) AddError(msg string) {
	r.Errors = append(r.Errors, msg)
}

// AddErrorf adds a formatted error to the result.
func (r *PreflightResult) AddErrorf(format string, args ...interface{}) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
}

// AddWarning adds a warning to the result.
func (r *PreflightResult) AddWarning(msg string) {
	r.Warnings = append(r.Warnings, msg)
}

// AddWarningf adds a formatted warning to the result.
func (r *PreflightResult) AddWarningf(format string, args ...interface{}) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// HasErrors returns true if there are any errors.
func (r *PreflightResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any warnings.
func (r *PreflightResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// ToError returns an error if there are any errors, nil otherwise.
// This is a convenience method for callers that only care about errors.
func (r *PreflightResult) ToError() error {
	if r == nil || !r.HasErrors() {
		return nil
	}
	if len(r.Errors) == 1 {
		return fmt.Errorf("%s", r.Errors[0])
	}
	return fmt.Errorf("%s (and %d more errors)", r.Errors[0], len(r.Errors)-1)
}

// Preflight is implemented by actions that can validate their parameters
// without executing side effects. This enables static validation of recipes.
//
// Actions implementing Preflight should use shared parameter extraction functions
// that are also used by Execute(), ensuring validation and execution cannot drift.
type Preflight interface {
	// Preflight validates that the given parameters would produce a valid
	// action execution. Returns a PreflightResult containing any errors
	// (fatal) or warnings (non-fatal suggestions).
	//
	// CONTRACT: Preflight MUST NOT have side effects (no filesystem, no network).
	// It validates parameter presence, types, and semantic correctness.
	Preflight(params map[string]interface{}) *PreflightResult
}

// ValidateAction checks if an action exists and validates its parameters.
// This is the entry point for semantic validation of action steps.
//
// Returns a PreflightResult containing errors and warnings:
//   - If the action does not exist, returns result with error
//   - If the action implements Preflight, returns its PreflightResult
//   - If the action exists but doesn't implement Preflight, returns empty result
func ValidateAction(name string, params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	action := Get(name)
	if action == nil {
		result.AddErrorf("unknown action '%s'", name)
		return result
	}

	if pf, ok := action.(Preflight); ok {
		return pf.Preflight(params)
	}

	// Action exists but doesn't implement Preflight - pass validation
	return result
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

// registryValidator implements recipe.ActionValidator using the action registry.
type registryValidator struct{}

func (v *registryValidator) RegisteredNames() []string {
	return RegisteredNames()
}

func (v *registryValidator) ValidateAction(name string, params map[string]interface{}) *recipe.ActionValidationResult {
	result := ValidateAction(name, params)
	return &recipe.ActionValidationResult{
		Errors:   result.Errors,
		Warnings: result.Warnings,
	}
}

func init() {
	recipe.SetActionValidator(&registryValidator{})
}

// containsPlaceholder checks if a string contains a {placeholder} variable
func containsPlaceholder(s, placeholder string) bool {
	return strings.Contains(s, "{"+placeholder+"}")
}

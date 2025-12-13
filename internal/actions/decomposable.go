package actions

import (
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// Decomposable indicates an action can be broken into primitive steps.
// Composite actions implement this interface to enable plan generation
// with primitive-only steps.
type Decomposable interface {
	// Decompose returns the steps this action expands to.
	// Steps may be primitives or other composites (recursive decomposition).
	// Called during plan generation, not execution.
	Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error)
}

// Step represents a single operation returned by Decompose.
// It may be a primitive (terminal) or another composite (requires further decomposition).
type Step struct {
	Action   string                 // Action name (primitive or composite)
	Params   map[string]interface{} // Fully resolved parameters
	Checksum string                 // For download actions: expected SHA256
	Size     int64                  // For download actions: expected size in bytes
}

// EvalContext provides context during decomposition.
// Used by composite actions to resolve parameters and compute checksums.
type EvalContext struct {
	Version    string            // Resolved version (e.g., "1.29.3")
	VersionTag string            // Original version tag (e.g., "v1.29.3")
	OS         string            // Target OS (runtime.GOOS)
	Arch       string            // Target architecture (runtime.GOARCH)
	Recipe     *recipe.Recipe    // Full recipe (for reference)
	Resolver   *version.Resolver // For API calls (asset resolution, etc.)
}

// primitives is the set of Tier 1 primitive action names.
// These actions execute directly without decomposition.
var primitives = map[string]bool{
	"download":          true,
	"extract":           true,
	"chmod":             true,
	"install_binaries":  true,
	"set_env":           true,
	"set_rpath":         true,
	"link_dependencies": true,
	"install_libraries": true,
}

// IsPrimitive returns true if the action is a Tier 1 primitive.
// Primitives execute directly and cannot be decomposed further.
func IsPrimitive(action string) bool {
	return primitives[action]
}

// RegisterPrimitive adds an action name to the primitive registry.
// This is primarily for testing and extending the primitive set.
func RegisterPrimitive(action string) {
	primitives[action] = true
}

// Primitives returns a copy of all registered primitive action names.
func Primitives() []string {
	result := make([]string, 0, len(primitives))
	for name := range primitives {
		result = append(result, name)
	}
	return result
}

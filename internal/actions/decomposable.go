package actions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

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

// DownloadResult contains the result of a pre-download operation.
type DownloadResult struct {
	AssetPath string // Path to the downloaded file
	Checksum  string // SHA256 checksum (hex encoded)
	Size      int64  // File size in bytes
}

// Downloader provides download functionality for checksum computation during decomposition.
// This interface is satisfied by validate.PreDownloader.
type Downloader interface {
	Download(ctx context.Context, url string) (*DownloadResult, error)
}

// EvalContext provides context during decomposition.
// Used by composite actions to resolve parameters and compute checksums.
type EvalContext struct {
	Context    context.Context   // Context for cancellation
	Version    string            // Resolved version (e.g., "1.29.3")
	VersionTag string            // Original version tag (e.g., "v1.29.3")
	OS         string            // Target OS (runtime.GOOS)
	Arch       string            // Target architecture (runtime.GOARCH)
	Recipe     *recipe.Recipe    // Full recipe (for reference)
	Resolver   *version.Resolver // For API calls (asset resolution, etc.)
	Downloader Downloader        // For downloading files to compute checksums
}

// primitives is the set of primitive action names.
// These actions execute directly without decomposition.
// Includes both core primitives (download, extract, etc.) and
// ecosystem primitives (go_build, cargo_build, etc.).
var primitives = map[string]bool{
	// Core primitives
	"download":          true,
	"extract":           true,
	"chmod":             true,
	"install_binaries":  true,
	"set_env":           true,
	"set_rpath":         true,
	"link_dependencies": true,
	"install_libraries": true,
	// Ecosystem primitives achieve determinism through ecosystem-specific configuration
	"cargo_build":  true,
	"cpan_install": true,
	"gem_exec":     true,
	"go_build":     true,
	"npm_exec":     true,
	"pip_install":  true,
}

// IsPrimitive returns true if the action is a primitive.
// Primitives execute directly and cannot be decomposed further.
func IsPrimitive(action string) bool {
	return primitives[action]
}

// IsDecomposable returns true if the action implements the Decomposable interface.
// This checks the action registry to determine if the action can be decomposed.
func IsDecomposable(action string) bool {
	act := Get(action)
	if act == nil {
		return false
	}
	_, ok := act.(Decomposable)
	return ok
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

// DecomposeToPrimitives recursively decomposes an action until all steps are primitives.
// It handles nested composite actions and detects cycles to prevent infinite recursion.
// Returns an error if the action is neither primitive nor decomposable.
func DecomposeToPrimitives(ctx *EvalContext, action string, params map[string]interface{}) ([]Step, error) {
	visited := make(map[string]bool)
	return decomposeToPrimitivesInternal(ctx, action, params, visited)
}

// decomposeToPrimitivesInternal is the recursive implementation with cycle detection.
func decomposeToPrimitivesInternal(ctx *EvalContext, action string, params map[string]interface{}, visited map[string]bool) ([]Step, error) {
	// Check if action is already a primitive
	if IsPrimitive(action) {
		return []Step{{Action: action, Params: params}}, nil
	}

	// Compute hash for cycle detection
	hash := computeStepHash(action, params)
	if visited[hash] {
		return nil, fmt.Errorf("cycle detected: action %q has already been visited in this decomposition chain", action)
	}
	visited[hash] = true

	// Get the action from registry
	act := Get(action)
	if act == nil {
		return nil, fmt.Errorf("action %q not found in registry", action)
	}

	// Check if it implements Decomposable
	decomposable, ok := act.(Decomposable)
	if !ok {
		return nil, fmt.Errorf("action %q is neither primitive nor decomposable", action)
	}

	// Decompose the action
	steps, err := decomposable.Decompose(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose %q: %w", action, err)
	}

	// Recursively process each resulting step
	var primitives []Step
	for _, step := range steps {
		// Recursive call - step.Action may be composite or primitive
		subPrimitives, err := decomposeToPrimitivesInternal(ctx, step.Action, step.Params, visited)
		if err != nil {
			return nil, err
		}

		// Carry forward checksum/size from the step if present and result is single primitive
		if len(subPrimitives) == 1 && step.Checksum != "" {
			subPrimitives[0].Checksum = step.Checksum
			subPrimitives[0].Size = step.Size
		}

		primitives = append(primitives, subPrimitives...)
	}

	return primitives, nil
}

// computeStepHash computes a hash of action name and params for cycle detection.
func computeStepHash(action string, params map[string]interface{}) string {
	// Marshal params to JSON for consistent hashing
	paramsJSON, _ := json.Marshal(params)
	data := fmt.Sprintf("%s:%s", action, string(paramsJSON))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for shorter hash
}

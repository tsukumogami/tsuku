package actions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

// Cleanup removes the downloaded file and its parent directory.
// This should be called after the download is no longer needed.
func (r *DownloadResult) Cleanup() error {
	if r.AssetPath == "" {
		return nil
	}
	// Remove the parent directory (typically created by the downloader)
	dir := filepath.Dir(r.AssetPath)
	return os.RemoveAll(dir)
}

// Downloader provides download functionality for checksum computation during decomposition.
// This interface is satisfied by validate.PreDownloader.
type Downloader interface {
	Download(ctx context.Context, url string) (*DownloadResult, error)
}

// EvalConstraints holds version constraints extracted from golden files.
// Used during constrained evaluation to pin dependency versions for deterministic output.
type EvalConstraints struct {
	// PipConstraints maps package names to pinned versions.
	// Extracted from locked_requirements in pip_exec steps.
	// Keys are normalized package names (lowercase, hyphens).
	// Used for version lookups by GetPipConstraint.
	PipConstraints map[string]string

	// PipRequirements contains the full locked_requirements string.
	// Extracted from pip_exec steps in golden files.
	// Used during constrained evaluation to preserve hashes.
	PipRequirements string

	// PipHasNativeAddons indicates whether the pip package has native extensions.
	// Extracted from has_native_addons param in pip_exec steps.
	// Used during constrained evaluation to preserve the original detection result.
	PipHasNativeAddons bool

	// GoSum contains the full go.sum content for go_build steps.
	// Extracted from go_sum param in golden files.
	GoSum string

	// GoVersion contains the Go binary version used during decomposition.
	// Extracted from go_version param in go_build steps.
	// Used to pin the go_version parameter during constrained evaluation.
	GoVersion string

	// CargoLock contains the full Cargo.lock content for cargo_install steps.
	// Extracted from lock_data param in golden files.
	CargoLock string

	// NpmLock contains package-lock.json content for npm_install steps.
	// Extracted from package_lock param in golden files.
	NpmLock string

	// GemLock contains Gemfile.lock content for gem_install steps.
	// Extracted from lock_data param in golden files.
	GemLock string

	// CpanMeta contains cpanfile.snapshot content for cpan_install steps.
	// Extracted from snapshot param in golden files.
	CpanMeta string

	// DependencyVersions maps toolchain dependency names to pinned versions.
	// Extracted from dependencies[] in the golden file plan.
	// Used to pin toolchain versions (e.g., python-standalone, go, nodejs)
	// during constrained evaluation for reproducible plan generation.
	DependencyVersions map[string]string
}

// EvalContext provides context during decomposition.
// Used by composite actions to resolve parameters and compute checksums.
type EvalContext struct {
	Context       context.Context   // Context for cancellation
	Version       string            // Resolved version (e.g., "1.29.3")
	VersionTag    string            // Original version tag (e.g., "v1.29.3")
	OS            string            // Target OS (runtime.GOOS)
	Arch          string            // Target architecture (runtime.GOARCH)
	Recipe        *recipe.Recipe    // Full recipe (for reference)
	Resolver      *version.Resolver // For API calls (asset resolution, etc.)
	Downloader    Downloader        // For downloading files to compute checksums
	DownloadCache *DownloadCache    // For caching downloaded files (optional)
	Constraints   *EvalConstraints  // Version constraints for constrained evaluation (optional)
}

// primitives is the set of primitive action names.
// These actions execute directly without decomposition.
// Includes both core primitives (download, extract, etc.) and
// ecosystem primitives (go_build, cargo_build, etc.).
var primitives = map[string]bool{
	// Core primitives - fully deterministic
	"download_file":     true,
	"extract":           true,
	"chmod":             true,
	"install_binaries":  true,
	"set_env":           true,
	"set_rpath":         true,
	"link_dependencies": true,
	"install_libraries": true,
	"apply_patch_file":  true,
	"text_replace":      true,
	"homebrew_relocate": true,
	// Ecosystem primitives - have residual non-determinism
	"cargo_build":        true,
	"cmake_build":        true,
	"configure_make":     true,
	"cpan_install":       true,
	"gem_exec":           true,
	"go_build":           true,
	"install_gem_direct": true, // Direct gem install for bundler self-installation
	"meson_build":        true,
	"nix_realize":        true,
	"npm_exec":           true,
	"pip_exec":           true,
	"pip_install":        true,
	"setup_build_env":    true,
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

// IsDeterministic returns true if the action produces identical results given
// identical inputs. Core primitives are deterministic. Ecosystem primitives
// have residual non-determinism and return false. Unknown actions return
// false for safety.
func IsDeterministic(action string) bool {
	act := Get(action)
	if act == nil {
		// Unknown actions are not deterministic for safety
		return false
	}
	return act.IsDeterministic()
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

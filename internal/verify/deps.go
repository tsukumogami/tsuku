package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// MaxTransitiveDepth is the maximum depth for recursive dependency validation.
// This matches the value used in actions.MaxTransitiveDepth.
const MaxTransitiveDepth = 10

// RecipeLoader provides recipe lookup for dependency validation.
// This interface decouples the verify package from the registry package.
type RecipeLoader interface {
	// LoadRecipe loads a recipe by name, returning nil if not found.
	LoadRecipe(name string) (*recipe.Recipe, error)
}

// ActionLookup is a function that returns an action by name for IsExternallyManagedFor checks.
type ActionLookup func(string) interface{}

// ValidateDependencies performs recursive dependency validation for a binary.
// It validates ABI compatibility, classifies dependencies, and recursively
// validates tsuku-managed dependencies.
//
// Parameters:
//   - binaryPath: absolute path to the binary to validate
//   - state: current installation state containing sonames
//   - loader: recipe loader for checking externally-managed status
//   - actionLookup: function to look up actions for IsExternallyManagedFor
//   - visited: map tracking already-validated binaries (for cycle detection)
//   - recurse: whether to recursively validate dependencies
//   - targetOS: target operating system (runtime.GOOS)
//   - targetArch: target architecture (runtime.GOARCH)
//   - tsukuHome: path to tsuku home directory ($TSUKU_HOME)
//
// Returns a slice of DepResult for each dependency found, or error on failure.
func ValidateDependencies(
	binaryPath string,
	state *install.State,
	loader RecipeLoader,
	actionLookup ActionLookup,
	visited map[string]bool,
	recurse bool,
	targetOS, targetArch string,
	tsukuHome string,
) ([]DepResult, error) {
	// Normalize path for consistent cycle detection
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve path: %w", err)
	}

	// Resolve symlinks for accurate cycle detection
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink resolution fails, use the absolute path
		resolvedPath = absPath
	}

	// Check for cycle (already validated this binary)
	if visited[resolvedPath] {
		return nil, nil // Already validated, skip
	}
	visited[resolvedPath] = true

	// Check depth limit
	if len(visited) > MaxTransitiveDepth {
		return nil, &ValidationError{
			Category: ErrMaxDepthExceeded,
			Path:     binaryPath,
			Message:  fmt.Sprintf("dependency tree exceeds maximum depth of %d", MaxTransitiveDepth),
		}
	}

	// Validate ABI compatibility (PT_INTERP check on Linux)
	if err := ValidateABI(binaryPath); err != nil {
		return nil, err
	}

	// Extract dependencies via header validation
	// Note: ValidateHeader checks if this is a valid shared library
	// For executables, we still want to validate dependencies, so we call it anyway
	headerInfo, err := extractDependencies(binaryPath)
	if err != nil {
		// If we can't parse the binary, it might be a script or static binary
		// Return empty results (no dependencies to validate)
		return []DepResult{}, nil
	}

	// No dependencies means static binary or leaf library
	if len(headerInfo.Dependencies) == 0 {
		return []DepResult{}, nil
	}

	// Build soname index for classification
	index := BuildSonameIndex(state)

	// Extract RPATH entries for path variable expansion
	rpaths, _ := ExtractRpaths(binaryPath)

	// Compute allowed prefix for path validation
	allowedPrefix := filepath.Join(tsukuHome, "tools")

	// Process each dependency
	var results []DepResult
	for _, dep := range headerInfo.Dependencies {
		result := validateSingleDependency(
			dep, binaryPath, rpaths, allowedPrefix,
			state, index, loader, actionLookup,
			visited, recurse,
			targetOS, targetArch, tsukuHome,
		)
		results = append(results, result)
	}

	return results, nil
}

// extractDependencies extracts the dependency list from a binary.
// This is a lighter-weight version of ValidateHeader that doesn't require
// the file to be a shared library (also works for executables).
func extractDependencies(binaryPath string) (*HeaderInfo, error) {
	// For now, use ValidateHeader which already extracts dependencies
	// In the future, we might want a separate function that doesn't require
	// the file to be a shared library
	return ValidateHeader(binaryPath)
}

// validateSingleDependency validates a single dependency and optionally recurses.
func validateSingleDependency(
	dep, binaryPath string,
	rpaths []string, allowedPrefix string,
	state *install.State,
	index *SonameIndex,
	loader RecipeLoader,
	actionLookup ActionLookup,
	visited map[string]bool,
	recurse bool,
	targetOS, targetArch, tsukuHome string,
) DepResult {
	result := DepResult{
		Soname: dep,
	}

	// Try to expand path variables
	// Only apply allowedPrefix check if the dep contains path variables
	prefixForExpansion := ""
	if IsPathVariable(dep) {
		prefixForExpansion = allowedPrefix
	}
	expanded, err := ExpandPathVariables(dep, binaryPath, rpaths, prefixForExpansion)
	if err != nil {
		// Path expansion failed - check if this is a system library by pattern
		if IsSystemLibrary(dep, targetOS) {
			result.Category = DepPureSystem
			result.Status = ValidationPass
			return result
		}

		// Check if it's in the soname index (tsuku-managed)
		category, recipeName, version := ClassifyDependency(dep, index, DefaultRegistry, targetOS)
		if category == DepTsukuManaged {
			result.Category = DepTsukuManaged
			result.Recipe = recipeName
			result.Version = version
			result.Status = ValidationFail
			result.Error = fmt.Sprintf("path expansion failed: %v", err)
			return result
		}

		// Unknown dependency with unexpandable path
		result.Category = DepUnknown
		result.Status = ValidationFail
		result.Error = fmt.Sprintf("cannot expand path: %v", err)
		return result
	}
	result.ResolvedPath = expanded

	// Classify the dependency
	category, recipeName, version := ClassifyDependency(dep, index, DefaultRegistry, targetOS)
	result.Category = category
	result.Recipe = recipeName
	result.Version = version

	// Validate based on category
	switch category {
	case DepPureSystem:
		if err := validateSystemDep(expanded, targetOS); err != nil {
			result.Status = ValidationFail
			result.Error = err.Error()
		} else {
			result.Status = ValidationPass
		}

	case DepTsukuManaged:
		// Refine to EXTERNALLY_MANAGED if recipe uses package manager
		if loader != nil && actionLookup != nil && isExternallyManaged(recipeName, loader, actionLookup, targetOS, targetArch) {
			result.Category = DepExternallyManaged
		}

		// Validate that the library provides the expected soname
		if err := validateTsukuDep(dep, recipeName, version, state); err != nil {
			result.Status = ValidationFail
			result.Error = err.Error()
		} else {
			result.Status = ValidationPass
		}

		// Recurse into tsuku-managed deps (not externally-managed)
		if recurse && result.Status == ValidationPass && result.Category == DepTsukuManaged {
			libPath := resolveLibraryPath(recipeName, version, tsukuHome)
			if libPath != "" {
				transitive, err := ValidateDependencies(
					libPath, state, loader, actionLookup,
					visited, true, targetOS, targetArch, tsukuHome,
				)
				if err != nil {
					result.Status = ValidationFail
					result.Error = fmt.Sprintf("transitive validation failed: %v", err)
				} else {
					result.Transitive = transitive
					// Check if any transitive dep failed
					for _, t := range transitive {
						if t.Status == ValidationFail {
							result.Status = ValidationFail
							result.Error = "transitive dependency failed"
							break
						}
					}
				}
			}
		}

	case DepExternallyManaged:
		// Validate that it provides expected soname, but don't recurse
		if err := validateTsukuDep(dep, recipeName, version, state); err != nil {
			result.Status = ValidationFail
			result.Error = err.Error()
		} else {
			result.Status = ValidationPass
		}

	case DepUnknown:
		result.Status = ValidationFail
		result.Error = "dependency not found in soname index or system patterns"
	}

	return result
}

// validateSystemDep verifies a system library is accessible.
// For pattern-matched system libraries (libc.so.6, libpthread.so.0, etc.), we trust the
// pattern match without requiring a file path. The dynamic linker will resolve these at runtime.
// For non-pattern-matched libraries or libraries with absolute paths, we verify file existence.
func validateSystemDep(libPath string, targetOS string) error {
	// If the library matches a system pattern, trust it without file check.
	// This handles:
	// - macOS dyld cache (system libraries not on disk since macOS 11)
	// - Linux bare sonames (libc.so.6) that the dynamic linker resolves
	if IsSystemLibrary(libPath, targetOS) {
		return nil
	}

	// For absolute paths or non-pattern-matched libs, check file existence
	if filepath.IsAbs(libPath) {
		if _, err := os.Stat(libPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("system library not found: %s", libPath)
			}
			return fmt.Errorf("cannot access system library %s: %v", libPath, err)
		}
	}

	return nil
}

// validateTsukuDep verifies a tsuku-managed library provides the expected soname.
func validateTsukuDep(soname, recipeName, version string, state *install.State) error {
	if state == nil || state.Libs == nil {
		return &ValidationError{
			Category: ErrMissingSoname,
			Message:  fmt.Sprintf("no state available to verify %s", recipeName),
		}
	}

	versions, ok := state.Libs[recipeName]
	if !ok {
		return &ValidationError{
			Category: ErrMissingSoname,
			Message:  fmt.Sprintf("library %s not found in state", recipeName),
		}
	}

	versionState, ok := versions[version]
	if !ok {
		return &ValidationError{
			Category: ErrMissingSoname,
			Message:  fmt.Sprintf("library %s version %s not found in state", recipeName, version),
		}
	}

	// Check if the expected soname is in the library's sonames
	for _, s := range versionState.Sonames {
		if s == soname {
			return nil
		}
	}

	return &ValidationError{
		Category: ErrMissingSoname,
		Message:  fmt.Sprintf("library %s@%s does not provide soname %s (has: %v)", recipeName, version, soname, versionState.Sonames),
	}
}

// isExternallyManaged checks if a recipe uses package manager actions.
func isExternallyManaged(recipeName string, loader RecipeLoader, actionLookup ActionLookup, targetOS, targetArch string) bool {
	if loader == nil || actionLookup == nil {
		return false
	}

	rec, err := loader.LoadRecipe(recipeName)
	if err != nil || rec == nil {
		return false
	}

	// Create a target for the current platform
	target := &platformTarget{os: targetOS, arch: targetArch}

	return rec.IsExternallyManagedFor(target, actionLookup)
}

// platformTarget implements recipe.Matchable for platform matching.
type platformTarget struct {
	os          string
	arch        string
	linuxFamily string
}

func (t *platformTarget) OS() string          { return t.os }
func (t *platformTarget) Arch() string        { return t.arch }
func (t *platformTarget) LinuxFamily() string { return t.linuxFamily }

// resolveLibraryPath returns the path to a library's directory for recursion.
// Libraries are installed under $TSUKU_HOME/tools/<recipe>-<version>/lib/
func resolveLibraryPath(recipeName, version, tsukuHome string) string {
	if recipeName == "" || version == "" || tsukuHome == "" {
		return ""
	}

	// Standard library path
	libDir := filepath.Join(tsukuHome, "tools", fmt.Sprintf("%s-%s", recipeName, version), "lib")

	// Check if the directory exists
	if _, err := os.Stat(libDir); err != nil {
		return ""
	}

	// Return the lib directory - the caller will need to find specific library files
	// For now, we return empty since we can't easily enumerate all library files
	return ""
}

// ValidateDependenciesSimple is a convenience wrapper for ValidateDependencies
// that uses sensible defaults for the current platform.
func ValidateDependenciesSimple(binaryPath string, state *install.State, tsukuHome string) ([]DepResult, error) {
	return ValidateDependencies(
		binaryPath,
		state,
		nil, // No recipe loader
		nil, // No action lookup
		make(map[string]bool),
		true, // Recurse by default
		runtime.GOOS,
		runtime.GOARCH,
		tsukuHome,
	)
}

package verify

// DepCategory represents the classification of a dependency for verification purposes.
type DepCategory int

const (
	// DepPureSystem indicates an inherently OS-provided library (libc, libpthread, etc.).
	// These are expected on any conforming system and are verified accessible but not recursed.
	DepPureSystem DepCategory = iota

	// DepTsukuManaged indicates a library that tsuku builds/manages.
	// These are verified for expected sonames and recursively validated.
	DepTsukuManaged

	// DepExternallyManaged indicates a tsuku recipe that delegates to a package manager (apt, brew).
	// These are verified for expected sonames but not recursed (pkg manager owns internals).
	DepExternallyManaged

	// DepUnknown indicates an unclassified dependency.
	// Pre-GA, this causes verification failure to surface corner cases.
	DepUnknown
)

// String returns a human-readable name for the category.
func (c DepCategory) String() string {
	switch c {
	case DepPureSystem:
		return "PURE_SYSTEM"
	case DepTsukuManaged:
		return "TSUKU_MANAGED"
	case DepExternallyManaged:
		return "EXTERNALLY_MANAGED"
	case DepUnknown:
		return "UNKNOWN"
	default:
		return "INVALID"
	}
}

// ClassifyDependency determines the category of a dependency soname.
// Returns the category, recipe name (if tsuku-managed), and version (if tsuku-managed).
//
// Classification order is critical for correctness:
//  1. Check soname index FIRST - if we have a recipe providing this soname, it's tsuku-managed
//  2. Check system patterns - matches OS-provided library patterns
//  3. Otherwise - UNKNOWN (fails pre-GA to surface corner cases)
//
// This order ensures that "libssl.so.3" is identified as TSUKU_MANAGED when we have
// an openssl recipe installed, rather than potentially matching a system pattern.
//
// Note: Distinguishing between TSUKU_MANAGED and EXTERNALLY_MANAGED requires recipe
// lookup (to check IsExternallyManagedFor), which is handled by the caller in issue #989.
// This function returns TSUKU_MANAGED for all sonames in the index; refinement happens
// at the orchestration layer.
func ClassifyDependency(dep string, index *SonameIndex, registry *SystemLibraryRegistry, targetOS string) (DepCategory, string, string) {
	// 1. Check soname index FIRST
	// This ensures "libssl.so.3" is identified as TSUKU when we have an openssl recipe,
	// rather than potentially matching a system pattern.
	if index != nil {
		if recipe, version, found := index.Lookup(dep); found {
			// Note: Determining if this is TSUKU_MANAGED vs EXTERNALLY_MANAGED
			// requires recipe lookup, which is handled in issue #989.
			// For now, return TSUKU_MANAGED; the orchestration layer will refine.
			return DepTsukuManaged, recipe, version
		}
	}

	// 2. Check system patterns
	if registry != nil && registry.IsSystemLibrary(dep, targetOS) {
		return DepPureSystem, "", ""
	}

	// 3. Unknown - fail pre-GA to surface corner cases
	return DepUnknown, "", ""
}

// ClassifyResult holds the result of dependency classification with additional context.
type ClassifyResult struct {
	// Category is the dependency classification
	Category DepCategory

	// Recipe is the recipe name if Category is DepTsukuManaged or DepExternallyManaged
	Recipe string

	// Version is the installed version if Category is DepTsukuManaged or DepExternallyManaged
	Version string

	// Original is the original dependency string that was classified
	Original string
}

// ClassifyDependencyResult is a convenience wrapper that returns a ClassifyResult.
func ClassifyDependencyResult(dep string, index *SonameIndex, registry *SystemLibraryRegistry, targetOS string) ClassifyResult {
	category, recipe, version := ClassifyDependency(dep, index, registry, targetOS)
	return ClassifyResult{
		Category: category,
		Recipe:   recipe,
		Version:  version,
		Original: dep,
	}
}

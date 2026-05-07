// Package sonameindex builds a (platform, SONAME) -> providing-recipe map
// from library recipes' `outputs` lists.
//
// The index is consumed by the homebrew action's relocate phase: when
// scanning a bottle binary's NEEDED SONAMES, each NEEDED entry that is
// not resolved by the system runtime linker is looked up here to find
// the tsuku library recipe that ships it. See
// docs/designs/DESIGN-tsuku-homebrew-dylib-chaining.md for the broader
// mechanism.
//
// The package is intentionally a leaf: it depends only on
// internal/recipe types, so both internal/executor and
// internal/actions can import it without inverting the existing
// executor -> install dependency direction.
package sonameindex

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// Platform identifies the operating system that owns a SONAME entry.
// Values match the OS strings used elsewhere in the codebase
// ("linux", "darwin").
type Platform string

const (
	// PlatformLinux is the platform key for Linux ELF SONAMES
	// (basename pattern lib*.so[.N...]).
	PlatformLinux Platform = "linux"
	// PlatformDarwin is the platform key for macOS Mach-O dylibs
	// (basename pattern lib*[.N...].dylib).
	PlatformDarwin Platform = "darwin"
)

// Provider describes the recipe that ships a given SONAME on a given
// platform.
type Provider struct {
	// Recipe is the recipe name (the metadata.name field).
	Recipe string
}

// Index is a single-valued map from (platform, SONAME) to the recipe
// that provides it. Build it once at plan-generation time and consult
// it from the SONAME completeness scanner.
type Index struct {
	// entries maps SONAME -> Provider, partitioned by platform.
	entries map[Platform]map[string]Provider
}

// Lookup returns the provider for the given (platform, SONAME) pair
// and reports whether such an entry was found.
func (i *Index) Lookup(platform Platform, soname string) (Provider, bool) {
	if i == nil || i.entries == nil {
		return Provider{}, false
	}
	platformEntries, ok := i.entries[platform]
	if !ok {
		return Provider{}, false
	}
	provider, ok := platformEntries[soname]
	return provider, ok
}

// Size reports the total number of (platform, SONAME) entries in the
// index. Useful for tests and diagnostics.
func (i *Index) Size() int {
	if i == nil {
		return 0
	}
	total := 0
	for _, platformEntries := range i.entries {
		total += len(platformEntries)
	}
	return total
}

// BuildFiltered is Build with a per-recipe skip predicate. It is the
// pragmatic escape hatch for known-collision alternates — for example,
// `libcurl-source` is a source-built variant maintained alongside the
// canonical `libcurl` library recipe, and both ship the same
// `lib/libcurl.so` SONAME. Build itself fails loudly on collisions
// (intentional per the design), so callers that load the entire
// registry into the index need a way to drop the alternate. The
// recommended skip predicate matches recipes whose name ends in
// `-source`; the long-term fix is to give those recipes non-colliding
// outputs (or migrate them to a separate provider type), at which
// point this helper can fold back into Build.
//
// skip is consulted before the parser walks the recipe; recipes for
// which skip returns true contribute no entries to the index.
func BuildFiltered(recipes []*recipe.Recipe, skip func(*recipe.Recipe) bool) (*Index, error) {
	if skip == nil {
		return Build(recipes)
	}
	filtered := make([]*recipe.Recipe, 0, len(recipes))
	for _, r := range recipes {
		if r == nil {
			continue
		}
		if skip(r) {
			continue
		}
		filtered = append(filtered, r)
	}
	return Build(filtered)
}

// IsKnownCollisionAlternate reports whether a recipe name is one of the
// known *-source / *-alternate forms that intentionally ships the same
// SONAME as a canonical sibling recipe. Callers that build the index
// from the live registry should pass this to BuildFiltered to avoid the
// loud-but-expected collision error.
//
// Currently the only entry is `libcurl-source` (a source-built variant
// of `libcurl`). New entries should be added only when the alternate is
// genuinely necessary — the long-term fix is to give such recipes
// non-colliding outputs.
func IsKnownCollisionAlternate(name string) bool {
	return name == "libcurl-source"
}

// Build walks the supplied recipes, parses their `outputs` lists for
// SONAME-shaped paths, and returns a single-valued
// (platform, SONAME) -> Provider map.
//
// Only library-typed recipes contribute entries. Non-library recipes
// are skipped silently — they may legitimately ship dylibs as part of
// a tool install (icons, internal helpers) and are not authoritative
// providers.
//
// Output entries that are not SONAME-shaped (header files, pkg-config
// files, archives, etc.) are skipped without error. The parser is
// loose-on-purpose: the recipe registry is the trust boundary, and
// the intent here is to extract SONAMES, not to validate recipes.
//
// Build fails when two distinct library recipes both claim the same
// (platform, SONAME). Collisions are loud at plan-generation time so
// the auto-include code path stays trivially deterministic.
func Build(recipes []*recipe.Recipe) (*Index, error) {
	idx := &Index{
		entries: map[Platform]map[string]Provider{
			PlatformLinux:  {},
			PlatformDarwin: {},
		},
	}

	// Iterate in deterministic order so collision errors are stable.
	sorted := append([]*recipe.Recipe(nil), recipes...)
	sort.Slice(sorted, func(a, b int) bool {
		return sorted[a].Metadata.Name < sorted[b].Metadata.Name
	})

	for _, r := range sorted {
		if r == nil {
			continue
		}
		if !r.IsLibrary() {
			continue
		}
		if err := indexRecipe(idx, r); err != nil {
			return nil, err
		}
	}

	return idx, nil
}

// indexRecipe extracts SONAME entries from each step's `outputs` list
// and inserts them into the index under the providing recipe.
func indexRecipe(idx *Index, r *recipe.Recipe) error {
	recipeName := r.Metadata.Name
	for _, step := range r.Steps {
		platforms := stepPlatforms(step)
		if len(platforms) == 0 {
			continue
		}
		outputs := stepOutputs(step)
		if len(outputs) == 0 {
			continue
		}
		for _, output := range outputs {
			for _, p := range platforms {
				sonames := parseSonames(p, output)
				for _, soname := range sonames {
					if err := insert(idx, p, soname, Provider{Recipe: recipeName}); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// stepPlatforms returns the platforms a step contributes outputs for,
// derived from its when clause. A step with no when clause (or a
// when clause that does not constrain the OS dimension) contributes
// to both linux and darwin.
func stepPlatforms(step recipe.Step) []Platform {
	if step.When == nil || step.When.IsEmpty() {
		return []Platform{PlatformLinux, PlatformDarwin}
	}
	osList := whenOSList(step.When)
	if len(osList) == 0 {
		return []Platform{PlatformLinux, PlatformDarwin}
	}
	platforms := make([]Platform, 0, len(osList))
	seen := make(map[Platform]bool)
	for _, os := range osList {
		var p Platform
		switch os {
		case "linux":
			p = PlatformLinux
		case "darwin":
			p = PlatformDarwin
		default:
			continue
		}
		if !seen[p] {
			platforms = append(platforms, p)
			seen[p] = true
		}
	}
	return platforms
}

// whenOSList returns the union of the when clause's OS dimension,
// considering both the dedicated OS field and the OS portion of any
// platform tuples. Returns an empty slice when the when clause does
// not constrain the OS dimension at all.
func whenOSList(when *recipe.WhenClause) []string {
	if when == nil {
		return nil
	}
	osList := append([]string(nil), when.OS...)
	for _, tuple := range when.Platform {
		os, _, _ := strings.Cut(tuple, "/")
		if os != "" {
			osList = append(osList, os)
		}
	}
	return osList
}

// stepOutputs returns the step's `outputs` parameter as a slice of
// strings. Non-string entries (e.g., {src=, dest=} mappings) are
// skipped — they are valid in install_binaries but not relevant for
// SONAME indexing, which only cares about library file paths.
func stepOutputs(step recipe.Step) []string {
	raw, ok := step.Params["outputs"]
	if !ok {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// parseSonames returns the SONAME aliases produced by the given
// `outputs` entry on the given platform. Returns nil if the entry is
// not SONAME-shaped (e.g., a header file or an archive).
//
// Validation rules (from the Phase 2 spec in the design):
//   - The path must start with "lib/" and contain no ".." segments.
//   - The basename must start with "lib".
//   - The basename must match the platform's library extension
//     (".so[.N...]" on Linux, "[.N...].dylib" on macOS).
//
// Aliasing (also from the spec): a versioned form contributes both
// itself and each progressively-shorter dotted-numeric form down to
// the unversioned base. For example, "lib/libfoo.so.1.2.3" yields
// ["libfoo.so.1.2.3", "libfoo.so.1.2", "libfoo.so.1", "libfoo.so"].
// Hyphen-versioned forms (libfoo-2.dylib) are not aliased to their
// unhyphenated bases — recipes that need both forms list both forms
// in their outputs.
func parseSonames(platform Platform, output string) []string {
	if !strings.HasPrefix(output, "lib/") {
		return nil
	}
	cleaned := path.Clean(output)
	// path.Clean collapses ".." segments; reject anything that produced
	// a different path (which signals a traversal attempt).
	if cleaned != output {
		return nil
	}
	for _, segment := range strings.Split(output, "/") {
		if segment == ".." {
			return nil
		}
	}
	basename := path.Base(output)
	if !strings.HasPrefix(basename, "lib") {
		return nil
	}

	switch platform {
	case PlatformLinux:
		return linuxSonameAliases(basename)
	case PlatformDarwin:
		return darwinSonameAliases(basename)
	}
	return nil
}

// linuxSonameAliases returns the SONAME chain for a Linux basename.
// Returns nil if the basename is not a SONAME-shaped library file.
func linuxSonameAliases(basename string) []string {
	// Find ".so" in the basename. A SONAME is structured as
	// "<name>.so" or "<name>.so.<digits>(.<digits>)*".
	idx := strings.Index(basename, ".so")
	if idx < 0 {
		return nil
	}
	name := basename[:idx]
	if name == "" {
		return nil
	}
	rest := basename[idx+len(".so"):]
	// Must be either empty or ".<digits>(.<digits>)*".
	if rest != "" {
		if !strings.HasPrefix(rest, ".") {
			// e.g. "libfoo.something" — not a SONAME we recognize.
			return nil
		}
		segments := strings.Split(rest[1:], ".")
		for _, segment := range segments {
			if !isAllDigits(segment) {
				return nil
			}
		}
	}

	// Build the alias chain by progressively stripping trailing
	// ".<digits>" segments off the version portion. The unversioned
	// "<name>.so" form is always included.
	aliases := []string{basename}
	current := basename
	for {
		dot := strings.LastIndex(current, ".")
		if dot < 0 {
			break
		}
		suffix := current[dot+1:]
		if !isAllDigits(suffix) {
			break
		}
		current = current[:dot]
		aliases = append(aliases, current)
	}
	return dedupe(aliases)
}

// darwinSonameAliases returns the SONAME chain for a macOS basename.
// Returns nil if the basename is not a SONAME-shaped dylib.
func darwinSonameAliases(basename string) []string {
	if !strings.HasSuffix(basename, ".dylib") {
		return nil
	}
	stem := strings.TrimSuffix(basename, ".dylib")
	if stem == "" {
		return nil
	}

	// Build the alias chain by progressively stripping trailing
	// ".<digits>" segments from the stem and re-appending ".dylib".
	// The fully-stripped (unversioned) form is always included.
	aliases := []string{basename}
	current := stem
	for {
		dot := strings.LastIndex(current, ".")
		if dot < 0 {
			break
		}
		suffix := current[dot+1:]
		if !isAllDigits(suffix) {
			break
		}
		current = current[:dot]
		aliases = append(aliases, current+".dylib")
	}
	return dedupe(aliases)
}

// insert adds (platform, soname) -> provider to the index. If the
// SONAME is already claimed by a different recipe on this platform,
// returns an error naming both providers so the caller can surface
// the collision at plan-generation time.
func insert(idx *Index, platform Platform, soname string, provider Provider) error {
	platformEntries := idx.entries[platform]
	if existing, ok := platformEntries[soname]; ok {
		if existing.Recipe == provider.Recipe {
			return nil
		}
		return fmt.Errorf(
			"sonameindex: SONAME %q on platform %q is claimed by both recipes %q and %q",
			soname, platform, existing.Recipe, provider.Recipe,
		)
	}
	platformEntries[soname] = provider
	return nil
}

// isAllDigits reports whether s is a non-empty sequence of ASCII
// digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// dedupe returns the input slice with duplicates removed, preserving
// first-occurrence order.
func dedupe(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// knownGapAllowlist is the static set of SONAMES that no current
// tsuku library recipe ships. The SONAME completeness scanner
// (Issue 6) uses IsKnownGap to downgrade the "no provider" log line
// to debug-level for these entries — install logs would otherwise
// grow noisy until the missing library recipes land.
//
// Each entry is removed from the allowlist when the corresponding
// library recipe is authored. The list is intentionally small; new
// entries should be added only after confirming no recipe is in
// flight to provide the SONAME.
var knownGapAllowlist = map[string]bool{
	// libuuid: shipped by util-linux on most distros, no tsuku recipe yet.
	// Bottle binaries that use uuid functions (e.g., wget) need it.
	"libuuid.so.1":    true,
	"libuuid.1.dylib": true,
	// libacl: POSIX ACL helpers, ships with acl on most distros.
	// Coreutils binaries reference it when ACL features are enabled.
	"libacl.so.1":    true,
	"libacl.1.dylib": true,
	// libattr: extended-attribute helpers, ships with attr on most distros.
	// Coreutils binaries reference it when xattr features are enabled.
	"libattr.so.1":    true,
	"libattr.1.dylib": true,
}

// IsKnownGap reports whether the given SONAME is on the static
// allowlist of SONAMES with no current tsuku-recipe coverage. The
// scanner uses this to downgrade "no provider" log lines to
// debug-level so install output stays clean for known-missing
// libraries.
func IsKnownGap(soname string) bool {
	return knownGapAllowlist[soname]
}

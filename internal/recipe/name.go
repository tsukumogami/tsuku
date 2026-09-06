package recipe

import "strings"

// IsValidRecipeName reports whether name is safe to pass to URL or path
// construction functions. Names containing '/', '\', "..", or a null byte
// are rejected to prevent path traversal in local-registry deployments
// and to keep recipe names usable in URL components.
//
// It is NOT the rule for a name arriving from outside the user's control.
// That is ValidateStrictName, an allowlist, and the two are deliberately
// separate -- see its doc comment for why one cannot delegate to the other.
// This helper is a denylist: it accepts "a:b" and "x$(id)y", which the
// allowlist refuses. Both facts matter before you reach for either.
//
// Do not merge them. This is the older and more referenced of the two, so it
// is where someone noticing the duplication is most likely to start, and the
// comment here used to claim it was the single source of truth for a
// well-formed recipe identifier. It was not, and saying so invited exactly
// that merge. The rules are incomparable rather than redundant:
// ValidateStrictName is stricter everywhere except "foo..bar", which this
// helper rejects by substring and the boundary is required to accept.
//
// The remaining callers are backstops beneath that boundary -- distributed
// registry caching (internal/distributed), the binary index rebuild
// (internal/index), and RegistryProvider.recipePath -- where being weaker
// than the boundary is acceptable and traversal is what matters. The recipe
// validator no longer calls this at all; it uses ValidateStrictName.
func IsValidRecipeName(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "/") {
		return false
	}
	if strings.Contains(name, "\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if strings.ContainsRune(name, '\x00') {
		return false
	}
	return true
}

package recipe

import "strings"

// IsValidRecipeName reports whether name is safe to pass to URL or path
// construction functions. Names containing '/', '\', "..", or a null byte
// are rejected to prevent path traversal in local-registry deployments
// and to keep recipe names usable in URL components.
//
// This helper is the single source of truth for "is this string a
// well-formed recipe identifier?" Callers in distributed registry caching
// (internal/distributed), the binary index rebuild (internal/index), and
// the recipe validator (internal/recipe/validator.go) all share this
// definition so a name accepted by one path is accepted by all.
//
// The check is deliberately minimal: it rejects the small set of
// characters that would cause path-traversal or URL-injection bugs.
// Stricter validation (e.g., the `^[a-z0-9._-]+$` pattern enforced on
// runtime_dependencies entries) is layered on top of this in the
// validator.
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

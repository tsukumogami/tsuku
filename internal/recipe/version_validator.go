package recipe

import "sync"

// VersionValidator validates version configuration for recipes.
// This interface is implemented by the version package and registered at init time,
// enabling the recipe package to validate version configuration without importing
// the version package (which would cause a circular import).
type VersionValidator interface {
	// CanResolveVersion returns true if a version provider can be created for this recipe.
	CanResolveVersion(r *Recipe) bool

	// KnownSources returns the list of known version source values.
	KnownSources() []string

	// ValidateVersionConfig performs detailed validation of version configuration.
	// Returns nil if valid, error describing the problem if invalid.
	ValidateVersionConfig(r *Recipe) error
}

var (
	versionValidator   VersionValidator
	versionValidatorMu sync.RWMutex
)

// SetVersionValidator registers the version validator.
// This is called from the version package init() to register the factory-based validator.
func SetVersionValidator(v VersionValidator) {
	versionValidatorMu.Lock()
	defer versionValidatorMu.Unlock()
	versionValidator = v
}

// GetVersionValidator returns the registered validator or nil if none is registered.
func GetVersionValidator() VersionValidator {
	versionValidatorMu.RLock()
	defer versionValidatorMu.RUnlock()
	return versionValidator
}

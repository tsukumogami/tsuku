package version

import (
	"context"
	"fmt"
	"sync"
)

// ResolverFunc is a function that resolves a version from a custom source
type ResolverFunc func(ctx context.Context, r *Resolver) (*VersionInfo, error)

// VersionResolverFunc is a function that resolves a specific version from a custom source
type VersionResolverFunc func(ctx context.Context, r *Resolver, version string) (*VersionInfo, error)

// Registry manages custom version source resolvers
// It provides a pluggable system for adding new version sources without modifying core code
// The registry is thread-safe and can be accessed concurrently
type Registry struct {
	mu               sync.RWMutex
	resolvers        map[string]ResolverFunc
	versionResolvers map[string]VersionResolverFunc
}

// NewRegistry creates a registry with default resolvers
// Default resolvers include nodejs_dist for Node.js and npm packages
// Policy: Only add custom resolvers for multi-use sources; use github_file for single-use cases
func NewRegistry() *Registry {
	return &Registry{
		resolvers: map[string]ResolverFunc{
			"nodejs_dist": func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
				return r.ResolveNodeJS(ctx)
			},
		},
		versionResolvers: map[string]VersionResolverFunc{
			// nodejs_dist doesn't support specific versions yet
			// Add version resolvers here when custom sources need them
		},
	}
}

// Resolve resolves a version using the named source
// Returns ResolverError with ErrTypeUnknownSource if the source is not registered
// This method is thread-safe
func (reg *Registry) Resolve(ctx context.Context, r *Resolver, source string) (*VersionInfo, error) {
	reg.mu.RLock()
	fn, ok := reg.resolvers[source]
	reg.mu.RUnlock()

	if !ok {
		return nil, &ResolverError{
			Type:    ErrTypeUnknownSource,
			Source:  source,
			Message: fmt.Sprintf("no resolver registered for source '%s'", source),
		}
	}
	return fn(ctx, r)
}

// ResolveVersion resolves a specific version using the named source
// Returns ResolverError with ErrTypeUnknownSource if the source is not registered
// Returns ResolverError with ErrTypeNotSupported if the source doesn't support specific versions
// This method is thread-safe
func (reg *Registry) ResolveVersion(ctx context.Context, r *Resolver, source, version string) (*VersionInfo, error) {
	reg.mu.RLock()
	fn, ok := reg.versionResolvers[source]
	reg.mu.RUnlock()

	if !ok {
		// Check if the source exists at all
		reg.mu.RLock()
		_, sourceExists := reg.resolvers[source]
		reg.mu.RUnlock()

		if !sourceExists {
			return nil, &ResolverError{
				Type:    ErrTypeUnknownSource,
				Source:  source,
				Message: fmt.Sprintf("no resolver registered for source '%s'", source),
			}
		}

		// Source exists but doesn't support specific versions
		return nil, &ResolverError{
			Type:    ErrTypeNotSupported,
			Source:  source,
			Message: fmt.Sprintf("source '%s' does not support resolving specific versions", source),
		}
	}
	return fn(ctx, r, version)
}

// Register adds a custom resolver for extensibility and testing
// Returns error if a resolver with the same name is already registered
// This method is thread-safe
func (reg *Registry) Register(source string, fn ResolverFunc) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, exists := reg.resolvers[source]; exists {
		return fmt.Errorf("resolver already registered: %s", source)
	}
	reg.resolvers[source] = fn
	return nil
}

// List returns all registered version source names
// This method is thread-safe
func (reg *Registry) List() []string {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	sources := make([]string, 0, len(reg.resolvers))
	for name := range reg.resolvers {
		sources = append(sources, name)
	}
	return sources
}

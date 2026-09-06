package project

import (
	"context"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/index"
)

// Resolver answers what a project declared, by combining the binary index
// (command -> recipe) with the project config (recipe -> version).
type Resolver struct {
	config *ConfigResult
	lookup autoinstall.LookupFunc

	// declarations maps a bare recipe name to what the configuration declared
	// for it. It is built once, in NewResolver, so the precedence rule has one
	// production site and the two entry points below cannot disagree about it.
	declarations map[string][]ProjectDeclaration
}

// NewResolver creates a ProjectVersionResolver. If config is nil (no
// .tsuku.toml found), the resolver reports no declarations for every command.
func NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver {
	r := &Resolver{config: config, lookup: lookup}
	if config != nil && config.Config != nil {
		r.declarations = buildDeclarations(config.Config.Tools, config.Path)
	}
	return r
}

// DeclarationsFor returns the recipes the project declared among matches, each
// with the version declared for it. Order follows the index's ranking of
// matches; where one recipe carries several declarations, they follow the
// order buildDeclarations states.
//
// The result is empty when the project declares no provider of the command,
// which is how "not project-declared" is reported -- there is no second return
// value that can disagree with the length of the first.
//
// It takes matches rather than a command because the caller has already looked
// the command up. Resolving from the command here would open the index a
// second time for every run that finds a .tsuku.toml.
func (r *Resolver) DeclarationsFor(_ context.Context, matches []index.BinaryMatch) ([]ProjectDeclaration, error) {
	if len(r.declarations) == 0 {
		return nil, nil
	}

	var set []ProjectDeclaration
	seen := make(map[string]bool)
	for _, m := range matches {
		if seen[m.Recipe] {
			continue
		}
		seen[m.Recipe] = true
		set = append(set, r.declarations[m.Recipe]...)
	}
	return set, nil
}

// ProjectVersionFor returns the project-pinned version for a command.
//
// It is the pre-existing entry point, kept so this package still compiles
// against its caller in cmd/tsuku. It reports a version alone, so it cannot
// say which of several declared recipes the version belongs to; DeclarationsFor
// is what replaces it.
func (r *Resolver) ProjectVersionFor(ctx context.Context, command string) (string, bool, error) {
	if r.config == nil {
		return "", false, nil
	}

	matches, err := r.lookup(ctx, command)
	if err != nil {
		return "", false, err
	}

	declared, err := r.DeclarationsFor(ctx, matches)
	if err != nil {
		return "", false, err
	}
	if len(declared) == 0 {
		return "", false, nil
	}
	return declared[0].Version, true, nil
}

// Tools returns the tool map from the underlying config, or nil if no config
// is present. This is used by callers that need to check whether a recipe
// appears in the project config without going through the command lookup path.
func (r *Resolver) Tools() map[string]ToolRequirement {
	if r.config == nil {
		return nil
	}
	return r.config.Config.Tools
}

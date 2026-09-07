package project

import (
	"context"

	"github.com/tsukumogami/tsuku/internal/index"
)

// Resolver answers what a project declared, by combining the binary index
// (command -> recipe) with the project config (recipe -> version).
//
// It does not look anything up itself. The caller has already opened the index
// to find the command's providers, and resolving from the command here would
// open it a second time on every run that finds a .tsuku.toml.
type Resolver struct {
	// declarations maps a bare recipe name to what the configuration declared
	// for it. It is built once, in NewResolver, so the precedence rule has one
	// production site.
	declarations map[string][]ProjectDeclaration
}

// NewResolver creates a Resolver. If config is nil (no .tsuku.toml found), the
// resolver reports no declarations for every command.
func NewResolver(config *ConfigResult) *Resolver {
	r := &Resolver{}
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
func (r *Resolver) DeclarationsFor(_ context.Context, matches []index.BinaryMatch) ([]ProjectDeclaration, error) {
	if len(r.declarations) == 0 {
		return nil, nil
	}

	// A recipe appearing twice in matches would duplicate its declarations.
	// The index cannot produce that -- (command, recipe) is its primary key --
	// but matches arrives from the caller, so the guard is here rather than
	// left to an invariant this package does not own.
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

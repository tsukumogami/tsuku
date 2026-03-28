package project

import (
	"context"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
)

// Resolver maps commands to project-pinned versions by combining the binary
// index (command -> recipe) with the project config (recipe -> version).
type Resolver struct {
	config *ConfigResult
	lookup autoinstall.LookupFunc
}

// NewResolver creates a ProjectVersionResolver. If config is nil (no
// .tsuku.toml found), the resolver returns ("", false, nil) for every command.
func NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver {
	return &Resolver{config: config, lookup: lookup}
}

// ProjectVersionFor returns the project-pinned version for a command.
// It looks up the command in the binary index, then checks if any matching
// recipe is declared in the project config.
func (r *Resolver) ProjectVersionFor(ctx context.Context, command string) (string, bool, error) {
	if r.config == nil {
		return "", false, nil
	}

	matches, err := r.lookup(ctx, command)
	if err != nil {
		return "", false, err
	}

	for _, m := range matches {
		if req, ok := r.config.Config.Tools[m.Recipe]; ok {
			return req.Version, true, nil
		}
	}

	return "", false, nil
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

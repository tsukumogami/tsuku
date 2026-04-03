package project

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
)

// Resolver maps commands to project-pinned versions by combining the binary
// index (command -> recipe) with the project config (recipe -> version).
type Resolver struct {
	config    *ConfigResult
	lookup    autoinstall.LookupFunc
	bareToOrg map[string][]string // bare recipe name -> org-scoped config keys
}

// NewResolver creates a ProjectVersionResolver. If config is nil (no
// .tsuku.toml found), the resolver returns ("", false, nil) for every command.
func NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver {
	r := &Resolver{config: config, lookup: lookup}
	if config != nil && config.Config != nil {
		r.bareToOrg = buildBareToOrgMap(config.Config.Tools)
	}
	return r
}

// buildBareToOrgMap scans config tool keys for org-scoped entries and builds
// a reverse map from bare recipe names to their org-scoped config keys.
func buildBareToOrgMap(tools map[string]ToolRequirement) map[string][]string {
	m := make(map[string][]string)
	for key := range tools {
		_, bare, isOrg, err := SplitOrgKey(key)
		if err != nil || !isOrg {
			continue
		}
		m[bare] = append(m[bare], key)
	}

	// Sort values for deterministic resolution and warn on duplicates.
	for bare, keys := range m {
		sort.Strings(keys)
		if len(keys) > 1 {
			fmt.Fprintf(os.Stderr, "warning: multiple org-scoped tools map to bare name %q: %v\n", bare, keys)
		}
	}

	return m
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
		// Fast path: bare key match (existing behavior)
		if req, ok := r.config.Config.Tools[m.Recipe]; ok {
			return req.Version, true, nil
		}
		// Org-scoped key match via reverse map
		if orgKeys, ok := r.bareToOrg[m.Recipe]; ok {
			for _, orgKey := range orgKeys {
				if req, ok := r.config.Config.Tools[orgKey]; ok {
					return req.Version, true, nil
				}
			}
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

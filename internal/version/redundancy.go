package version

import (
	"fmt"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// RedundantVersion represents a case where a recipe has an explicit [version]
// configuration that matches what would be inferred from its actions.
type RedundantVersion struct {
	// Source is the explicit version source that is redundant
	Source string
	// Action is the action that infers this source
	Action string
	// Message is a human-readable description of the redundancy
	Message string
}

// actionInference maps action names to the version source they infer.
// Note: go_install is NOT in this map because it has no inference strategy.
var actionInference = map[string]string{
	"cargo_install":  "crates_io",
	"pipx_install":   "pypi",
	"npm_install":    "npm",
	"gem_install":    "rubygems",
	"cpan_install":   "metacpan",
	"github_archive": "github_releases",
	"github_file":    "github_releases",
}

// DetectRedundantVersion checks if a recipe has an explicit [version] configuration
// that matches what would be inferred from its actions.
//
// A version source is considered redundant when:
//   - The recipe has an explicit [version] source or github_repo
//   - The recipe uses an action that can infer the same version source
//   - The explicit source matches what would be inferred
//
// Note: go_install is excluded because it has no inference strategy - recipes
// using go_install must specify source="goproxy" explicitly.
func DetectRedundantVersion(r *recipe.Recipe) []RedundantVersion {
	var redundant []RedundantVersion

	// Check for github_repo redundancy with github_archive/github_file
	if r.Version.GitHubRepo != "" {
		for _, step := range r.Steps {
			if step.Action == "github_archive" || step.Action == "github_file" {
				if repo, ok := step.Params["repo"].(string); ok && repo != "" {
					// Check if the github_repo matches the repo param
					if repo == r.Version.GitHubRepo {
						redundant = append(redundant, RedundantVersion{
							Source:  "github_repo=" + r.Version.GitHubRepo,
							Action:  step.Action,
							Message: fmt.Sprintf("[version] github_repo=%q is redundant; %s with repo=%q infers this automatically", r.Version.GitHubRepo, step.Action, repo),
						})
					}
				}
			}
		}
	}

	// Check for source redundancy with inferring actions
	if r.Version.Source == "" {
		return redundant
	}

	for _, step := range r.Steps {
		inferredSource, hasInference := actionInference[step.Action]
		if !hasInference {
			continue
		}

		// Check if the explicit source matches what would be inferred
		if r.Version.Source == inferredSource {
			redundant = append(redundant, RedundantVersion{
				Source:  r.Version.Source,
				Action:  step.Action,
				Message: fmt.Sprintf("[version] source=%q is redundant; %s infers this automatically", r.Version.Source, step.Action),
			})
		}
	}

	return redundant
}

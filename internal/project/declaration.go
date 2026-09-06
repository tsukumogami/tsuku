package project

import (
	"sort"
)

// ProjectDeclaration is one recipe a project declared, with the version it
// declared for it and the configuration that did the declaring.
//
// Recipe is the bare recipe name, which is the form the binary index reports
// and the form every consumer compares against. Version is what the
// configuration said, verbatim: `latest` and prefixes reach the installer
// unchanged, because resolving them stays the installer's job (R20).
//
// ConfigKey and ConfigPath are not decoration. A refusal naming two declared
// providers has to name the key each came from, and the disclosure emitted in
// internal/autoinstall needs the authorizing file's path, which that package
// has no other route to.
type ProjectDeclaration struct {
	Recipe     string // bare recipe name
	Version    string // as declared, verbatim
	ConfigKey  string // the .tsuku.toml key it was declared under
	ConfigPath string // the .tsuku.toml that declared it
}

// buildDeclarations groups a config's tool keys by the bare recipe name they
// reduce to, and applies the precedence rule per recipe. The result maps a
// bare recipe name to every declaration the configuration makes for it.
//
// The grouping inside each bare name is by the recipe a key *denotes*, which
// is the pair of bare name and org-scoped source SplitOrgKey computes:
//
//	"koto"                -> koto, from no source
//	"org-a/koto"          -> koto, from org-a/koto
//	"org-a/registry:koto" -> koto, from org-a/registry
//
// Keying on the denoted recipe rather than on the bare name is the whole point
// of the rule below. Two org-scoped keys whose sources differ denote different
// recipes from different registries; only the index's bare-name representation
// makes them look alike.
//
// Almost always that is one declaration. It is more than one exactly when the
// configuration names two or more org-scoped sources for the same bare name,
// which R2a says must not be collapsed: they reach the ambiguity refusal
// instead of one of them being selected on an ordering the user never
// expressed.
//
// The precedence is stated here rather than left to fall out of iteration
// order. For each bare name:
//
//   - One denoted recipe or fewer: one declaration. The bare key's version if
//     the configuration has a bare key, else the sorted-first key of the one
//     org-scoped source.
//   - Two or more org-scoped sources: one declaration per source, ordered by
//     source. A bare key alongside them is dropped rather than added as a
//     third.
//
// Dropping it rather than adding it follows from the collapse above, not from
// the bare key being uninstallable -- it is perfectly installable, from the
// default loader chain. The collapse says a bare key and an org-scoped key
// reducing to it are the same recipe. With two org sources that holds against
// each of them separately, so the bare key introduces no recipe the set does
// not already carry; all it does is fail to say which of the two it meant.
// The ambiguity to report is the one between the two registries, and adding a
// third entry would put a recipe in the refusal that the user cannot choose
// between the others by naming.
//
// A malformed key -- one SplitOrgKey rejects -- is skipped. It cannot denote a
// recipe, so it cannot be declared, and treating it as a bare name would let
// "../etc/passwd" become a declaration of the recipe "../etc/passwd".
func buildDeclarations(tools map[string]ToolRequirement, configPath string) map[string][]ProjectDeclaration {
	// bare recipe name -> denoted recipe -> the config keys denoting it.
	byRecipe := make(map[string]map[string][]string)
	for key := range tools {
		source, bare, isOrg, err := SplitOrgKey(key)
		if err != nil {
			continue
		}
		if !isOrg {
			source = ""
		}
		if byRecipe[bare] == nil {
			byRecipe[bare] = make(map[string][]string)
		}
		byRecipe[bare][source] = append(byRecipe[bare][source], key)
	}

	declarations := make(map[string][]ProjectDeclaration, len(byRecipe))
	for bare, bySource := range byRecipe {
		orgSources := make([]string, 0, len(bySource))
		for source := range bySource {
			if source != "" {
				orgSources = append(orgSources, source)
			}
		}
		sort.Strings(orgSources)

		_, hasBareKey := bySource[""]

		declare := func(source string) ProjectDeclaration {
			keys := bySource[source]
			sort.Strings(keys)
			key := keys[0]
			return ProjectDeclaration{
				Recipe:     bare,
				Version:    tools[key].Version,
				ConfigKey:  key,
				ConfigPath: configPath,
			}
		}

		if len(orgSources) <= 1 {
			// One recipe however many keys denote it. The bare key wins,
			// which is the precedence that holds today.
			source := ""
			if !hasBareKey {
				source = orgSources[0]
			}
			declarations[bare] = []ProjectDeclaration{declare(source)}
			continue
		}

		set := make([]ProjectDeclaration, 0, len(orgSources))
		for _, source := range orgSources {
			set = append(set, declare(source))
		}
		declarations[bare] = set
	}

	return declarations
}

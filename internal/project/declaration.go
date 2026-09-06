package project

import (
	"sort"
)

// ProjectDeclaration is one recipe a project declared, with the version it
// declared for it and the configuration that did the declaring.
//
// Recipe is the bare recipe name, which is the form the binary index reports
// and the form every consumer compares against. It is a match key and not an
// identity: two declarations in one set can carry the same Recipe, which is
// exactly what a configuration naming two registries for one name produces.
// Anything that has to tell those apart -- a refusal message, a map keyed per
// declaration -- keys on ConfigKey. A message built from Recipe alone reads
// "koto and koto" and reinstates the defect this set exists to fix.
//
// Version is what the configuration said, verbatim: `latest` and prefixes
// reach the installer unchanged, because resolving them stays the installer's
// job (R20).
//
// ConfigKey and ConfigPath are not decoration, though nothing outside this
// package reads them yet. A refusal naming two declared providers will have to
// name the key each came from, and the disclosure internal/autoinstall will
// emit needs the authorizing file's path, which that package has no other
// route to. Both consumers are later units; the fields are here because the
// set is what carries the association to them.
type ProjectDeclaration struct {
	Recipe     string
	Version    string
	ConfigKey  string
	ConfigPath string
}

// buildDeclarations groups a config's tool keys by the bare recipe name they
// reduce to, and applies the precedence rule per recipe. The result maps a
// bare recipe name to every declaration the configuration makes for it.
//
// Inside each bare name, keys are grouped by the recipe they *denote*, which
// is the bare name plus the org-scoped source SplitOrgKey computes. The source
// is the empty string for a bare key, which is why "" is the map key for one
// below:
//
//	"koto"                 -> koto, from no source
//	"org-a/koto"           -> koto, from org-a/koto
//	"org-a/koto@2.0.0"     -> koto, from org-a/koto
//	"org-a/registry:koto"  -> koto, from org-a/registry
//
// Rows two and three denote one recipe, because SplitOrgKey strips an
// `@version` suffix -- but only on the org-scoped branch, so a bare
// "koto@2.0.0" would denote a recipe of that whole name instead. Row four is a
// different recipe from rows two and three despite the matching bare name,
// because its source differs; that is the case the rule below turns on, and it
// is why the grouping keys on the denoted recipe rather than on the bare name.
//
// # The rule
//
// It is stated per bare name rather than left to fall out of iteration order,
// and it turns on how many distinct org-scoped sources the configuration names
// for that bare name -- not on how many recipes it denotes in total:
//
//   - At most one org-scoped source: ONE declaration, however many keys are
//     involved. A bare key's version wins if there is a bare key, else the one
//     source's. This is the ordinary case, and it covers a bare key alone, an
//     org key alone, and the two together. Where several keys denote that one
//     source -- rows two and three above -- the lowest by string order wins,
//     and it is that key the declaration reports as its ConfigKey.
//   - Two or more org-scoped sources: one declaration per source, ordered by
//     source, so the caller sees an ambiguity and refuses. A bare key
//     alongside them is dropped rather than added as a third.
//
// Dropping it rather than adding it follows from the first bullet, not from
// the bare key being uninstallable -- it is perfectly installable, from the
// default loader chain. That bullet treats a bare key and an org-scoped key
// reducing to it as one recipe. With two org sources that holds against each
// of them separately, so the bare key introduces no recipe the set does not
// already carry; all it does is fail to say which of the two it meant. The
// ambiguity to report is the one between the two registries, and a third entry
// would put a recipe in the refusal that the user cannot pick between the
// others by naming.
//
// A malformed key -- one SplitOrgKey rejects -- is skipped. It cannot denote a
// recipe, so it cannot be declared, and treating it as a bare name would let
// "../etc/passwd" become a declaration of the recipe "../etc/passwd".
func buildDeclarations(tools map[string]ToolRequirement, configPath string) map[string][]ProjectDeclaration {
	// bare recipe name -> denoted source -> the winning config key for it.
	//
	// Several keys can denote one recipe, and the winner is the lowest by
	// string order. Taking the minimum here rather than collecting and sorting
	// later is what makes every entry below a single key with no further
	// choice to make.
	byRecipe := make(map[string]map[string]string)
	for key := range tools {
		source, bare, isOrg, err := SplitOrgKey(key)
		if err != nil {
			continue
		}
		if !isOrg {
			source = ""
		}
		// These two statements together are why no byRecipe entry is ever
		// empty, which the second loop relies on and indexes a slice on. Any
		// edit that lets the first run without the second breaks it -- a
		// condition placed between them, or a narrowing of the write guard
		// below, which is what someone tuning within-source precedence touches.
		if byRecipe[bare] == nil {
			byRecipe[bare] = make(map[string]string)
		}
		if won, seen := byRecipe[bare][source]; !seen || key < won {
			byRecipe[bare][source] = key
		}
	}

	declarations := make(map[string][]ProjectDeclaration, len(byRecipe))
	for bare, bySource := range byRecipe {
		// bySource is never empty -- see the loop above, which is where that
		// could be broken. The one-source branch indexes orgSources on it.
		orgSources := make([]string, 0, len(bySource))
		for source := range bySource {
			if source != "" {
				orgSources = append(orgSources, source)
			}
		}
		sort.Strings(orgSources)

		bareKey, hasBareKey := bySource[""]

		declare := func(key string) ProjectDeclaration {
			return ProjectDeclaration{
				Recipe:     bare,
				Version:    tools[key].Version,
				ConfigKey:  key,
				ConfigPath: configPath,
			}
		}

		if len(orgSources) <= 1 {
			// One recipe, however many keys denote it.
			key := bareKey
			if !hasBareKey {
				// bySource is non-empty and holds no bare key, so it holds
				// exactly the one org source this branch admits.
				key = bySource[orgSources[0]]
			}
			declarations[bare] = []ProjectDeclaration{declare(key)}
			continue
		}

		set := make([]ProjectDeclaration, 0, len(orgSources))
		for _, source := range orgSources {
			set = append(set, declare(bySource[source]))
		}
		declarations[bare] = set
	}

	return declarations
}

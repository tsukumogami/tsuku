package batch

import "strings"

// Reconcile rules, in the order they are tried. The rule that matched is
// recorded on each change.
const (
	// RuleName: a recipe has the entry's name.
	RuleName = "name"
	// RuleSource: a recipe installs from the entry's source.
	RuleSource = "source"
	// RuleAlias: the entry's name is one of a recipe's
	// [metadata.satisfies] aliases.
	RuleAlias = "alias"
	// RuleSatisfies: the entry's identifier is listed under the matching
	// ecosystem key of a recipe's [metadata.satisfies].
	RuleSatisfies = "satisfies"
)

// ReconcileChange records one entry marked success because a recipe covers it.
type ReconcileChange struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	FromStatus string `json:"from_status"`
	Recipe     string `json:"recipe"`
	Rule       string `json:"rule"`
}

// ReconcileResult reports what Reconcile did.
type ReconcileResult struct {
	// Recipes is how many recipe identities were compared against.
	Recipes int `json:"recipes"`
	// Open is how many entries were open (not success or excluded) before.
	Open int `json:"open"`
	// Reconciled is how many entries were marked success. Always equal to
	// len(Changes).
	Reconciled int               `json:"reconciled"`
	Changes    []ReconcileChange `json:"changes,omitempty"`
}

// Reconcile marks every open queue entry that an existing recipe already
// covers as success, so the batch never generates a second recipe for a tool
// it has. An entry is open when its status is neither success nor excluded.
//
// An entry is covered when a recipe has its name, installs from its source,
// lists its name as an alias, or lists its identifier under the entry's
// ecosystem in [metadata.satisfies]. cargo: and crates.io: sources are
// treated as the same ecosystem. Installed binary names are deliberately not
// used: unrelated tools share binary names too often for that to identify a
// tool.
//
// A name match whose source differs from the recipe's also sets confidence to
// curated, since the recipe, not the disambiguated source, is what exists.
func Reconcile(queue *UnifiedQueue, recipes []RecipeIdentity) *ReconcileResult {
	res := &ReconcileResult{Recipes: len(recipes)}

	byName := make(map[string]*RecipeIdentity)
	bySource := make(map[string]*RecipeIdentity)
	byAlias := make(map[string]*RecipeIdentity)
	bySatisfies := make(map[string]*RecipeIdentity) // "eco:identifier"
	for i := range recipes {
		r := &recipes[i]
		setFirst(byName, r.Name, r)
		if r.Source != "" {
			setFirst(bySource, NormalizeSource(r.Source), r)
		}
		for key, names := range r.Satisfies {
			for _, n := range names {
				if key == "aliases" {
					setFirst(byAlias, n, r)
				} else {
					setFirst(bySatisfies, normalizeEcosystem(key)+":"+n, r)
				}
			}
		}
	}

	for i := range queue.Entries {
		e := &queue.Entries[i]
		if e.Status == StatusSuccess || e.Status == StatusExcluded {
			continue
		}
		res.Open++

		src := NormalizeSource(e.Source)
		eco, ident, _ := strings.Cut(src, ":")

		var r *RecipeIdentity
		rule := ""
		switch {
		case byName[e.Name] != nil:
			r, rule = byName[e.Name], RuleName
		case bySource[src] != nil:
			r, rule = bySource[src], RuleSource
		case byAlias[e.Name] != nil:
			r, rule = byAlias[e.Name], RuleAlias
		case bySatisfies[eco+":"+ident] != nil:
			r, rule = bySatisfies[eco+":"+ident], RuleSatisfies
		case bySatisfies[eco+":"+e.Name] != nil:
			r, rule = bySatisfies[eco+":"+e.Name], RuleSatisfies
		default:
			continue
		}

		res.Changes = append(res.Changes, ReconcileChange{
			Name:       e.Name,
			Source:     e.Source,
			FromStatus: e.Status,
			Recipe:     r.Name,
			Rule:       rule,
		})
		if rule == RuleName && NormalizeSource(r.Source) != src {
			e.Confidence = ConfidenceCurated
		}
		e.Status = StatusSuccess
	}

	res.Reconciled = len(res.Changes)
	return res
}

// setFirst keeps the first recipe seen for a key, so the outcome doesn't
// depend on which of two claimants is read last.
func setFirst(m map[string]*RecipeIdentity, key string, r *RecipeIdentity) {
	if key == "" {
		return
	}
	if _, ok := m[key]; !ok {
		m[key] = r
	}
}

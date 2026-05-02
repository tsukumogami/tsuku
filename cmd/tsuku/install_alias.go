package main

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/tui"
)

// installPick is bound to the hidden --pick flag. Test-only surface that
// short-circuits the picker for CI: the install command resolves the typed
// multi-satisfier alias to the named recipe without rendering the picker.
//
// Documented as test-only on the flag's help text. The blessed user-facing
// flag for explicit recipe selection is --from <recipe-name>; --pick exists
// because PTY harnesses are flaky in CI.
var installPick string

// resolveMultiSatisfier picks one recipe from the candidates per the truth
// table in PRD-multi-satisfier-picker.md (case C):
//
//   - --pick <recipe>: short-circuit for CI; validate that the named recipe
//     is in the candidate list, return it.
//   - --from <recipe>: same as --pick but blessed as a user-facing flag;
//     the install command's --from parser passes us the recipe name when
//     no colon is present (alias-selection form).
//   - -y / non-TTY: return an ambiguous error so the caller can emit the
//     parallel "Multiple recipes satisfy" structured error and exit 10.
//   - default (interactive TTY): render the arrow-driven picker.
//
// The candidates slice is the alphabetically-sorted output of
// loader.LookupAllSatisfiers.
func resolveMultiSatisfier(alias string, candidates []string) (string, error) {
	// --pick test-only override.
	if installPick != "" {
		if !slices.Contains(candidates, installPick) {
			return "", fmt.Errorf("--pick %q: not a satisfier of alias %q (candidates: %v)",
				installPick, alias, candidates)
		}
		return installPick, nil
	}

	// --from <recipe> user-facing override (no colon means recipe-name
	// form, distinct from the existing <ecosystem>:<source> form which
	// goes through the create pipeline above).
	if installFrom != "" && !containsColon(installFrom) {
		if !slices.Contains(candidates, installFrom) {
			return "", fmt.Errorf("--from %q: not a satisfier of alias %q (candidates: %v)",
				installFrom, alias, candidates)
		}
		return installFrom, nil
	}

	// Non-interactive: error with the candidate list so the user knows
	// what to pass via --from.
	if installYes || !tui.IsAvailable() {
		return "", errAmbiguousAlias
	}

	// Interactive: render the picker.
	choices := buildPickerChoices(candidates)
	prompt := fmt.Sprintf("Multiple recipes satisfy %q. Pick one:", alias)
	idx, err := tui.Pick(prompt, choices)
	if err != nil {
		return "", err
	}
	return candidates[idx], nil
}

// errAmbiguousAlias is the sentinel returned by resolveMultiSatisfier when
// the alias is multi-satisfier but the install command is non-interactive.
// Caller (install.go) checks this and emits the structured error.
var errAmbiguousAlias = fmt.Errorf("multi-satisfier alias requires --from in non-interactive mode")

// buildPickerChoices loads each candidate recipe to grab its description,
// then returns Choice rows for the picker. Recipes that fail to load
// produce a Choice with the recipe name only — the picker still functions,
// the user just sees less context.
func buildPickerChoices(candidates []string) []tui.Choice {
	out := make([]tui.Choice, len(candidates))
	for i, name := range candidates {
		out[i] = tui.Choice{Name: name}
		r, err := loader.GetWithContext(context.Background(), name, recipe.LoaderOptions{})
		if err == nil && r != nil {
			out[i].Description = r.Metadata.Description
		}
	}
	return out
}

// handleAmbiguousAliasError prints the "Multiple recipes satisfy" error
// to stderr (or as JSON when --json is set) and exits with ExitAmbiguous.
// Parallel to handleAmbiguousInstallError for discovery-layer ambiguity.
//
// Format mirrors the existing AmbiguousMatchError so scripts already wired
// for that exit code keep working.
func handleAmbiguousAliasError(alias string, candidates []string) {
	if installJSON {
		matches := make([]ambiguousAliasMatch, len(candidates))
		for i, name := range candidates {
			matches[i] = ambiguousAliasMatch{Recipe: name, From: name}
		}
		resp := ambiguousAliasError{
			Status:     "error",
			Category:   "ambiguous_alias",
			Message:    fmt.Sprintf("Multiple recipes satisfy alias %q. Use --from to specify a recipe.", alias),
			Alias:      alias,
			Candidates: matches,
			ExitCode:   ExitAmbiguous,
		}
		printJSON(resp)
	} else {
		fmt.Fprintf(os.Stderr, "Error: Multiple recipes satisfy alias %q. Use --from to specify a recipe:\n", alias)
		for _, name := range candidates {
			fmt.Fprintf(os.Stderr, "  tsuku install %s --from %s\n", alias, name)
		}
	}
	exitWithCode(ExitAmbiguous)
}

// ambiguousAliasError is the structured JSON shape for multi-satisfier alias
// errors under --json. Field names mirror ambiguousInstallError so consumers
// can switch on `category` to route between the two.
type ambiguousAliasError struct {
	Status     string                `json:"status"`
	Category   string                `json:"category"`
	Message    string                `json:"message"`
	Alias      string                `json:"alias"`
	Candidates []ambiguousAliasMatch `json:"candidates"`
	ExitCode   int                   `json:"exit_code"`
}

// ambiguousAliasMatch is one entry in the JSON candidates list. Recipe is
// the canonical recipe name; From mirrors the same value (the --from flag
// value the user would pass to disambiguate).
type ambiguousAliasMatch struct {
	Recipe string `json:"recipe"`
	From   string `json:"from"`
}

// containsColon reports whether s contains ':' (the separator used by the
// existing --from <ecosystem>:<source> form). When false, --from is being
// used in the new alias-selection (recipe-name) form.
func containsColon(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}

package project

import (
	"context"
	"testing"

	"github.com/tsukumogami/tsuku/internal/index"
)

// resolveOver builds a resolver over tools and runs it against matches. The
// cases here are all single-provider, so the matches are written out rather
// than taken from the fixture -- what they exercise is which configuration key
// denotes a recipe, not which of several providers wins.
func resolveOver(t *testing.T, tools map[string]ToolRequirement, matches []index.BinaryMatch) []ProjectDeclaration {
	t.Helper()
	r := NewResolver(&ConfigResult{Config: &ProjectConfig{Tools: tools}})
	declared, err := r.DeclarationsFor(context.Background(), matches)
	if err != nil {
		t.Fatalf("DeclarationsFor error = %v", err)
	}
	return declared
}

// soleDeclaration fails unless exactly one recipe was declared, and returns it.
func soleDeclaration(t *testing.T, declared []ProjectDeclaration) ProjectDeclaration {
	t.Helper()
	if len(declared) != 1 {
		t.Fatalf("declarations = %v, want exactly one", configKeys(declared))
	}
	return declared[0]
}

func TestResolver_CommandInIndexAndConfig(t *testing.T) {
	declared := resolveOver(t,
		map[string]ToolRequirement{"jq": {Version: "1.7.1"}},
		[]index.BinaryMatch{{Recipe: "jq", Command: "jq"}})

	if got := soleDeclaration(t, declared); got.Version != "1.7.1" {
		t.Errorf("version = %q, want %q", got.Version, "1.7.1")
	}
}

func TestResolver_CommandInIndexButNotConfig(t *testing.T) {
	declared := resolveOver(t,
		map[string]ToolRequirement{"ripgrep": {Version: "14.0.0"}},
		[]index.BinaryMatch{{Recipe: "jq", Command: "jq"}})

	if len(declared) != 0 {
		t.Errorf("declarations = %v, want none: the config declares no provider of jq",
			configKeys(declared))
	}
}

// A command no recipe provides is declared by nothing, however much the
// configuration names. The caller reports that as ErrNoMatch before it ever
// asks about declarations, so an answer here is not what decides the run --
// but a resolver that read its config instead of the matches would answer
// anyway, and this is where that shows.
func TestResolver_CommandNotInIndex(t *testing.T) {
	declared := resolveOver(t,
		map[string]ToolRequirement{"jq": {Version: "1.7.1"}},
		nil)

	if len(declared) != 0 {
		t.Errorf("declarations = %v, want none for a command with no providers",
			configKeys(declared))
	}
}

func TestResolver_OrgScopedKeyMatchesBareName(t *testing.T) {
	declared := resolveOver(t,
		map[string]ToolRequirement{"tsukumogami/koto": {Version: "1.0.0"}},
		[]index.BinaryMatch{{Recipe: "koto", Command: "koto"}})

	got := soleDeclaration(t, declared)
	if got.Recipe != "koto" || got.Version != "1.0.0" {
		t.Errorf("declaration = %+v, want koto at 1.0.0 from the org-scoped key", got)
	}
}

func TestResolver_OrgScopedWithQualifiedName(t *testing.T) {
	declared := resolveOver(t,
		map[string]ToolRequirement{"myorg/registry:mytool": {Version: "2.0.0"}},
		[]index.BinaryMatch{{Recipe: "mytool", Command: "mytool"}})

	got := soleDeclaration(t, declared)
	if got.Recipe != "mytool" || got.Version != "2.0.0" {
		t.Errorf("declaration = %+v, want mytool at 2.0.0 from the qualified key", got)
	}
}

func TestResolver_BareKeyTakesPriorityOverOrgScoped(t *testing.T) {
	declared := resolveOver(t, map[string]ToolRequirement{
		"koto":             {Version: "3.0.0"},
		"tsukumogami/koto": {Version: "1.0.0"},
	}, []index.BinaryMatch{{Recipe: "koto", Command: "koto"}})

	got := soleDeclaration(t, declared)
	if got.Version != "3.0.0" {
		t.Errorf("version = %q, want %q: the bare key outranks the one org-scoped source",
			got.Version, "3.0.0")
	}
}

// Two bare names declared alongside one org-scoped key. Each command sees its
// own declaration and nothing of the other's, which is the property that would
// break if declarations were keyed on anything but the recipe a key denotes.
func TestResolver_OrgScopedMixedWithBareKeys(t *testing.T) {
	tools := map[string]ToolRequirement{
		"node":             {Version: "20"},
		"tsukumogami/koto": {Version: "1.0.0"},
	}

	for _, tc := range []struct {
		recipe  string
		version string
	}{
		{"node", "20"},
		{"koto", "1.0.0"},
	} {
		t.Run(tc.recipe, func(t *testing.T) {
			declared := resolveOver(t, tools,
				[]index.BinaryMatch{{Recipe: tc.recipe, Command: tc.recipe}})
			got := soleDeclaration(t, declared)
			if got.Version != tc.version {
				t.Errorf("version = %q, want %q", got.Version, tc.version)
			}
		})
	}
}

// An empty version still declares the recipe. Declaredness and the version are
// separate answers, and collapsing them -- reporting "not declared" because
// there is no version to report -- is what the set replaced.
func TestResolver_EmptyVersionInConfig(t *testing.T) {
	declared := resolveOver(t,
		map[string]ToolRequirement{"jq": {Version: ""}},
		[]index.BinaryMatch{{Recipe: "jq", Command: "jq"}})

	got := soleDeclaration(t, declared)
	if got.Version != "" {
		t.Errorf("version = %q, want empty (use latest)", got.Version)
	}
}

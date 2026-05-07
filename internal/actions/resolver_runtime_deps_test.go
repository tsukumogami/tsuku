package actions

import (
	"reflect"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestResolveDependencies_PopulatesRuntimeDependencies covers the new
// RuntimeDependencies slice on ResolvedDeps. It must mirror the recipe's
// metadata.runtime_dependencies verbatim (preserving order, with version
// tags stripped) so downstream consumers — wrapper PATH today, RPATH
// chain in homebrew_relocate after the dylib-chaining design lands —
// read the author-declared list directly.
func TestResolveDependencies_PopulatesRuntimeDependencies(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			RuntimeDependencies: []string{"libevent", "utf8proc", "ncurses"},
		},
		Steps: []recipe.Step{
			{Action: "homebrew", Params: map[string]interface{}{"formula": "tmux"}},
		},
	}

	deps := ResolveDependencies(r)

	want := []string{"libevent", "utf8proc", "ncurses"}
	if !reflect.DeepEqual(deps.RuntimeDependencies, want) {
		t.Errorf("RuntimeDependencies = %v, want %v", deps.RuntimeDependencies, want)
	}
}

// TestResolveDependencies_RuntimeDependenciesStripsVersionTag confirms
// version tags ("name@version") are stripped from RuntimeDependencies so
// the slice carries pure recipe names — the form RPATH consumers can pass
// straight through to filepath.Join.
func TestResolveDependencies_RuntimeDependenciesStripsVersionTag(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			RuntimeDependencies: []string{"openssl@3", "zlib"},
		},
	}

	deps := ResolveDependencies(r)

	want := []string{"openssl", "zlib"}
	if !reflect.DeepEqual(deps.RuntimeDependencies, want) {
		t.Errorf("RuntimeDependencies = %v, want %v", deps.RuntimeDependencies, want)
	}
}

// TestResolveDependencies_RuntimeDependenciesEmptyByDefault confirms that
// recipes without metadata.runtime_dependencies leave the slice nil — no
// default population, no surprises for existing recipes.
func TestResolveDependencies_RuntimeDependenciesEmptyByDefault(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{},
		Steps: []recipe.Step{
			{Action: "homebrew", Params: map[string]interface{}{"formula": "ripgrep"}},
		},
	}

	deps := ResolveDependencies(r)

	if deps.RuntimeDependencies != nil {
		t.Errorf("RuntimeDependencies = %v, want nil", deps.RuntimeDependencies)
	}
}

// TestResolveDependencies_RuntimeDependenciesIgnoresExtraField confirms
// extra_runtime_dependencies does NOT contribute to the RuntimeDependencies
// slice. RuntimeDependencies is the strict author-declared list;
// extras (and step-level deps, and action-implicit deps) merge into the
// Runtime map only.
func TestResolveDependencies_RuntimeDependenciesIgnoresExtraField(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			RuntimeDependencies:      []string{"libevent"},
			ExtraRuntimeDependencies: []string{"bash"},
		},
	}

	deps := ResolveDependencies(r)

	want := []string{"libevent"}
	if !reflect.DeepEqual(deps.RuntimeDependencies, want) {
		t.Errorf("RuntimeDependencies = %v, want %v (extras must not leak in)", deps.RuntimeDependencies, want)
	}
}

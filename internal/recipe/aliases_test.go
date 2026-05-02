package recipe

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/tsukumogami/tsuku/internal/registry"
)

var errFakeRecipeNotFound = errors.New("recipe not found (fake)")

// fakeAliasesProvider is a minimal AliasesProvider for tests. It also
// implements RecipeProvider trivially so it can be plugged into a Loader.
type fakeAliasesProvider struct {
	source  RecipeSource
	aliases map[string][]string
}

func (p *fakeAliasesProvider) Get(ctx context.Context, name string) ([]byte, error) {
	return nil, errFakeRecipeNotFound
}

func (p *fakeAliasesProvider) List(ctx context.Context) ([]RecipeInfo, error) {
	return nil, nil
}

func (p *fakeAliasesProvider) Source() RecipeSource {
	return p.source
}

func (p *fakeAliasesProvider) AliasesEntries(ctx context.Context) (map[string][]string, error) {
	return p.aliases, nil
}

func TestAliases_LookupAllSatisfiers_Single(t *testing.T) {
	loader := NewLoader(&fakeAliasesProvider{
		source: SourceEmbedded,
		aliases: map[string][]string{
			"java": {"openjdk"},
		},
	})

	got, ok := loader.LookupAllSatisfiers("java")
	if !ok {
		t.Fatal("LookupAllSatisfiers returned ok=false; want true")
	}
	want := []string{"openjdk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LookupAllSatisfiers(java) = %v, want %v", got, want)
	}
}

func TestAliases_LookupAllSatisfiers_Multi(t *testing.T) {
	loader := NewLoader(&fakeAliasesProvider{
		source: SourceEmbedded,
		aliases: map[string][]string{
			// Insert in non-alphabetical order to verify sort.
			"java": {"temurin", "openjdk", "microsoft-openjdk", "corretto"},
		},
	})

	got, ok := loader.LookupAllSatisfiers("java")
	if !ok {
		t.Fatal("ok=false; want true")
	}
	want := []string{"corretto", "microsoft-openjdk", "openjdk", "temurin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LookupAllSatisfiers(java) = %v, want %v (sorted)", got, want)
	}
}

func TestAliases_LookupAllSatisfiers_Unknown(t *testing.T) {
	loader := NewLoader(&fakeAliasesProvider{
		source:  SourceEmbedded,
		aliases: map[string][]string{"java": {"openjdk"}},
	})

	got, ok := loader.LookupAllSatisfiers("python")
	if ok {
		t.Errorf("LookupAllSatisfiers(python) returned ok=true; want false; got %v", got)
	}
}

func TestAliases_LookupAllSatisfiers_DefensiveCopy(t *testing.T) {
	loader := NewLoader(&fakeAliasesProvider{
		source:  SourceEmbedded,
		aliases: map[string][]string{"java": {"openjdk", "temurin"}},
	})

	got, _ := loader.LookupAllSatisfiers("java")
	got[0] = "MUTATED"

	// Subsequent call should be unaffected by the caller's mutation.
	got2, _ := loader.LookupAllSatisfiers("java")
	if got2[0] == "MUTATED" {
		t.Error("LookupAllSatisfiers leaked the index slice; caller mutation contaminated subsequent reads")
	}
}

func TestAliases_HasMultiSatisfier(t *testing.T) {
	tests := []struct {
		name     string
		aliases  map[string][]string
		query    string
		wantMult bool
	}{
		{
			name:     "no entry",
			aliases:  map[string][]string{"java": {"openjdk"}},
			query:    "python",
			wantMult: false,
		},
		{
			name:     "single satisfier",
			aliases:  map[string][]string{"java": {"openjdk"}},
			query:    "java",
			wantMult: false,
		},
		{
			name:     "two satisfiers",
			aliases:  map[string][]string{"java": {"openjdk", "temurin"}},
			query:    "java",
			wantMult: true,
		},
		{
			name:     "four satisfiers",
			aliases:  map[string][]string{"java": {"openjdk", "temurin", "corretto", "microsoft-openjdk"}},
			query:    "java",
			wantMult: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader(&fakeAliasesProvider{source: SourceEmbedded, aliases: tt.aliases})
			if got := loader.HasMultiSatisfier(tt.query); got != tt.wantMult {
				t.Errorf("HasMultiSatisfier(%q) = %v, want %v", tt.query, got, tt.wantMult)
			}
		})
	}
}

func TestAliases_BuildIsLazy(t *testing.T) {
	calls := 0
	loader := NewLoader(&countingAliasProvider{
		fakeAliasesProvider: fakeAliasesProvider{
			source:  SourceEmbedded,
			aliases: map[string][]string{"java": {"openjdk"}},
		},
		callCount: &calls,
	})

	// First lookup triggers build.
	loader.LookupAllSatisfiers("java")
	if calls != 1 {
		t.Errorf("expected 1 build call after first lookup; got %d", calls)
	}
	// Subsequent lookups reuse cached index.
	loader.LookupAllSatisfiers("python")
	loader.LookupAllSatisfiers("java")
	if calls != 1 {
		t.Errorf("expected 1 build call after multiple lookups; got %d (index should be cached)", calls)
	}
}

type countingAliasProvider struct {
	fakeAliasesProvider
	callCount *int
}

func (p *countingAliasProvider) AliasesEntries(ctx context.Context) (map[string][]string, error) {
	*p.callCount++
	return p.aliases, nil
}

func TestAliases_MergeAcrossProviders(t *testing.T) {
	// Two providers each contributing satisfiers for the same alias
	// should produce a merged, deduplicated, sorted result.
	loader := NewLoader(
		&fakeAliasesProvider{
			source:  SourceEmbedded,
			aliases: map[string][]string{"java": {"temurin", "openjdk"}},
		},
		&fakeAliasesProvider{
			source:  SourceRegistry,
			aliases: map[string][]string{"java": {"corretto", "openjdk"}}, // openjdk is duplicate
		},
	)

	got, ok := loader.LookupAllSatisfiers("java")
	if !ok {
		t.Fatal("ok=false; want true")
	}
	want := []string{"corretto", "openjdk", "temurin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged satisfiers = %v, want %v (deduplicated, sorted)", got, want)
	}
}

func TestAliases_RegistryProvider_FromManifest(t *testing.T) {
	// End-to-end: a synthetic manifest with two recipes claiming the
	// same alias produces both via the AliasesProvider implementation
	// in RegistryProvider, and they show up in the loader's index.
	reg := registry.New(t.TempDir())

	manifest := &registry.Manifest{
		SchemaVersion: 1,
		Recipes: []registry.ManifestRecipe{
			{
				Name: "openjdk",
				Satisfies: map[string][]string{
					"homebrew": {"openjdk"},
					AliasesKey: {"java"},
				},
			},
			{
				Name: "temurin",
				Satisfies: map[string][]string{
					AliasesKey: {"java"},
				},
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := reg.CacheManifest(data); err != nil {
		t.Fatalf("CacheManifest: %v", err)
	}

	loader := newTestLoaderWithRegistry(reg)

	got, ok := loader.LookupAllSatisfiers("java")
	if !ok {
		t.Fatal("ok=false for alias java; want true")
	}
	want := []string{"openjdk", "temurin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LookupAllSatisfiers(java) = %v, want %v", got, want)
	}

	// The 1:1 satisfies index must NOT include the alias entries —
	// only the ecosystem ones (homebrew here).
	if name, ok := loader.LookupSatisfies("java"); ok {
		t.Errorf("LookupSatisfies(java) returned %q; the alias-only entry should not show up in the 1:1 index", name)
	}
	if name, ok := loader.LookupSatisfies("openjdk"); !ok || name != "openjdk" {
		t.Errorf("LookupSatisfies(openjdk) = %q, ok=%v; want openjdk, true (the homebrew ecosystem entry)", name, ok)
	}
}

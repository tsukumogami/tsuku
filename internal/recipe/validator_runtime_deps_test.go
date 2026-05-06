package recipe

import (
	"strings"
	"testing"
)

func TestValidateRecipeWithLoader_MultiSatisfierDepRejected(t *testing.T) {
	// Loader has an alias 'java' claimed by openjdk + temurin (multi-satisfier).
	loader := NewLoader(&fakeAliasesProvider{
		source:  SourceEmbedded,
		aliases: map[string][]string{"java": {"openjdk", "temurin"}},
	})

	// A recipe whose runtime_dependencies list 'java' as a dep.
	r := &Recipe{
		Metadata: MetadataSection{
			Name:                "maven",
			Description:         "test",
			RuntimeDependencies: []string{"java"},
		},
	}

	result := ValidateRecipeWithLoader(r, loader)
	if result.Valid {
		t.Fatal("expected validation to fail for multi-satisfier alias in runtime_dependencies")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "runtime_dependencies") &&
			strings.Contains(e.Message, "java") &&
			strings.Contains(e.Message, "openjdk") &&
			strings.Contains(e.Message, "temurin") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error message naming the alias and both satisfiers; got errors: %+v", result.Errors)
	}
}

func TestValidateRecipeWithLoader_SingleSatisfierDepAccepted(t *testing.T) {
	// Single-satisfier alias is fine — plan generation is deterministic.
	loader := NewLoader(&fakeAliasesProvider{
		source:  SourceEmbedded,
		aliases: map[string][]string{"openjdk": {"openjdk"}},
	})

	r := &Recipe{
		Metadata: MetadataSection{
			Name:                "maven",
			Description:         "test",
			RuntimeDependencies: []string{"openjdk"},
		},
	}

	result := ValidateRecipeWithLoader(r, loader)
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "multi-satisfier") {
			t.Errorf("unexpected multi-satisfier error for single-satisfier dep: %v", e)
		}
	}
}

func TestValidateRecipeWithLoader_DirectRecipeDepAccepted(t *testing.T) {
	// A dep that names a recipe directly (not an alias) doesn't appear
	// in the alias index at all. Should not trigger the check.
	loader := NewLoader(&fakeAliasesProvider{
		source:  SourceEmbedded,
		aliases: map[string][]string{"java": {"openjdk", "temurin"}},
	})

	r := &Recipe{
		Metadata: MetadataSection{
			Name:                "maven",
			Description:         "test",
			RuntimeDependencies: []string{"temurin"}, // direct recipe name, not the alias
		},
	}

	result := ValidateRecipeWithLoader(r, loader)
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "multi-satisfier") {
			t.Errorf("unexpected multi-satisfier error for direct recipe dep: %v", e)
		}
	}
}

func TestValidateRecipeWithLoader_ExtraRuntimeDepsChecked(t *testing.T) {
	// extra_runtime_dependencies is checked the same way as
	// runtime_dependencies.
	loader := NewLoader(&fakeAliasesProvider{
		source:  SourceEmbedded,
		aliases: map[string][]string{"java": {"openjdk", "temurin"}},
	})

	r := &Recipe{
		Metadata: MetadataSection{
			Name:                     "maven",
			Description:              "test",
			ExtraRuntimeDependencies: []string{"java"},
		},
	}

	result := ValidateRecipeWithLoader(r, loader)
	if result.Valid {
		t.Fatal("expected validation to fail for multi-satisfier alias in extra_runtime_dependencies")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "extra_runtime_dependencies") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error to flag extra_runtime_dependencies; got: %+v", result.Errors)
	}
}

func TestValidateRecipeWithLoader_NilLoaderSkipsCheck(t *testing.T) {
	// When ValidateRecipeWithLoader is called with a nil loader (e.g.,
	// older callers), the multi-satisfier check is skipped — basic
	// validation still runs.
	r := &Recipe{
		Metadata: MetadataSection{
			Name:                "maven",
			Description:         "test",
			RuntimeDependencies: []string{"java"},
		},
	}

	result := ValidateRecipeWithLoader(r, nil)
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "multi-satisfier") {
			t.Errorf("nil loader should skip multi-satisfier check; got: %v", e)
		}
	}
}

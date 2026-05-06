package recipe

import (
	"strings"
	"testing"
)

// TestValidateRuntimeDependencyNames_AcceptsValidNames covers the names that
// the strict pattern allows: lowercase ASCII, digits, '.', '_', and '-'.
// All 298 recipes that declare runtime_dependencies today match this shape,
// so any breakage here would surface as a recipe-validation regression.
func TestValidateRuntimeDependencyNames_AcceptsValidNames(t *testing.T) {
	valid := []string{
		"pcre2",
		"openssl",
		"libnghttp2",
		"jpeg-turbo",
		"libatomic_ops",
		"python3.11",
		"foo.bar",
		"a",
		"x1",
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					Name:                "tool",
					Description:         "test",
					RuntimeDependencies: []string{name},
				},
			}
			result := ValidateRecipe(r)
			for _, e := range result.Errors {
				if strings.Contains(e.Field, "runtime_dependencies") {
					t.Fatalf("unexpected error for valid name %q: %v", name, e)
				}
			}
		})
	}
}

// TestValidateRuntimeDependencyNames_RejectsBadPattern enumerates each
// rejection class — non-matching pattern, '..' / '/' / '\\', leading '-',
// null byte, empty entry, duplicate. Each variant gets its own subtest so
// failures point at the specific rule that broke.
func TestValidateRuntimeDependencyNames_RejectsBadPattern(t *testing.T) {
	cases := []struct {
		name    string
		dep     string
		wantSub string
	}{
		{"uppercase", "OpenSSL", "must match"},
		{"space", "open ssl", "must match"},
		{"colon", "name:tag", "must match"},
		{"at_sign", "python@3.11", "must match"},
		{"unicode", "café", "must match"},
		{"path_traversal", "..", "path traversal"},
		{"path_traversal_embedded", "foo..bar", "path traversal"},
		{"slash", "foo/bar", "must not contain '/'"},
		{"backslash", "foo\\bar", "must not contain"},
		{"leading_dash", "-foo", "must not start with '-'"},
		{"null_byte", "foo\x00bar", "null byte"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					Name:                "tool",
					Description:         "test",
					RuntimeDependencies: []string{tc.dep},
				},
			}
			result := ValidateRecipe(r)
			if result.Valid {
				t.Fatalf("expected validation to fail for %q", tc.dep)
			}
			var found bool
			for _, e := range result.Errors {
				if strings.Contains(e.Field, "runtime_dependencies") &&
					strings.Contains(e.Message, tc.wantSub) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q for entry %q; got: %+v", tc.wantSub, tc.dep, result.Errors)
			}
		})
	}
}

// TestValidateRuntimeDependencyNames_RejectsEmptyString covers the empty
// entry rejection rule (separate from "any other invalid character") so a
// failure points at the empty-string check specifically.
func TestValidateRuntimeDependencyNames_RejectsEmptyString(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name:                "tool",
			Description:         "test",
			RuntimeDependencies: []string{"libfoo", "", "libbar"},
		},
	}
	result := ValidateRecipe(r)
	if result.Valid {
		t.Fatal("expected validation to fail when runtime_dependencies contains an empty string")
	}
	var found bool
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "runtime_dependencies[1]") &&
			strings.Contains(e.Message, "must not be empty") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected empty-string error at index 1; got: %+v", result.Errors)
	}
}

// TestValidateRuntimeDependencyNames_RejectsDuplicates covers the duplicate
// entry rejection rule. The validator reports the index of the first
// occurrence in the message so authors can find both copies.
func TestValidateRuntimeDependencyNames_RejectsDuplicates(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name:                "tool",
			Description:         "test",
			RuntimeDependencies: []string{"libfoo", "libbar", "libfoo"},
		},
	}
	result := ValidateRecipe(r)
	if result.Valid {
		t.Fatal("expected validation to fail when runtime_dependencies contains a duplicate")
	}
	var found bool
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "runtime_dependencies[2]") &&
			strings.Contains(e.Message, "duplicates entry at index 0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate error pointing at index 0; got: %+v", result.Errors)
	}
}

// TestValidateRuntimeDependencyNames_AppliesToExtraRuntimeDeps checks that
// the same rules apply to extra_runtime_dependencies — recipes can be
// equally hostile to validators via either field.
func TestValidateRuntimeDependencyNames_AppliesToExtraRuntimeDeps(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name:                     "tool",
			Description:              "test",
			ExtraRuntimeDependencies: []string{"BAD"},
		},
	}
	result := ValidateRecipe(r)
	if result.Valid {
		t.Fatal("expected validation to fail for BAD entry in extra_runtime_dependencies")
	}
	var found bool
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "extra_runtime_dependencies") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error to flag extra_runtime_dependencies; got: %+v", result.Errors)
	}
}

// TestValidateRuntimeDependencyNames_EmptyListIsFine confirms recipes
// without any runtime_dependencies pass the new check (the vast majority
// of recipes today).
func TestValidateRuntimeDependencyNames_EmptyListIsFine(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name:        "tool",
			Description: "test",
		},
	}
	result := ValidateRecipe(r)
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "runtime_dependencies") {
			t.Errorf("unexpected runtime_dependencies error on recipe with empty list: %v", e)
		}
	}
}

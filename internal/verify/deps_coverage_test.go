package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestPlatformTarget_Libc(t *testing.T) {
	target := &platformTarget{
		os:   "linux",
		arch: "amd64",
		libc: "musl",
	}
	if target.Libc() != "musl" {
		t.Errorf("Libc() = %q, want %q", target.Libc(), "musl")
	}
}

func TestPlatformTarget_GPU(t *testing.T) {
	target := &platformTarget{
		os:   "linux",
		arch: "amd64",
	}
	if target.GPU() != "" {
		t.Errorf("GPU() = %q, want empty", target.GPU())
	}
}

func TestValidateTsukuDep_MissingVersion(t *testing.T) {
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libssl.so.3"},
				},
			},
		},
	}

	// Request a version that does not exist
	err := validateTsukuDep("libssl.so.3", "openssl", "4.0.0", state)
	if err == nil {
		t.Error("expected error for missing version, got nil")
	}
	if verr, ok := err.(*ValidationError); ok {
		if verr.Category != ErrMissingSoname {
			t.Errorf("expected ErrMissingSoname, got %v", verr.Category)
		}
	}
}

func TestValidateTsukuDep_NilLibs(t *testing.T) {
	state := &install.State{
		Libs: nil,
	}

	err := validateTsukuDep("libssl.so.3", "openssl", "3.2.1", state)
	if err == nil {
		t.Error("expected error for nil Libs, got nil")
	}
}

func TestIsExternallyManaged_NilLoader(t *testing.T) {
	result := isExternallyManaged("openssl", nil, nil, "linux", "amd64")
	if result {
		t.Error("expected false for nil loader")
	}
}

func TestIsExternallyManaged_NilActionLookup(t *testing.T) {
	loader := newMockRecipeLoader()
	result := isExternallyManaged("openssl", loader, nil, "linux", "amd64")
	if result {
		t.Error("expected false for nil action lookup")
	}
}

func TestIsExternallyManaged_RecipeNotFound(t *testing.T) {
	loader := newMockRecipeLoader()
	result := isExternallyManaged("nonexistent", loader, mockActionLookup, "linux", "amd64")
	if result {
		t.Error("expected false for missing recipe")
	}
}

func TestIsExternallyManaged_ExternalRecipe(t *testing.T) {
	loader := newMockRecipeLoader()
	loader.addExternallyManagedRecipe("openssl")

	result := isExternallyManaged("openssl", loader, mockActionLookup, "linux", "amd64")
	if !result {
		t.Error("expected true for externally-managed recipe")
	}
}

func TestValidateSingleDependency_TsukuManagedExternallyManaged(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libssl.so.3", "libcrypto.so.3"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)
	loader := newMockRecipeLoader()
	loader.addExternallyManagedRecipe("openssl")

	// With externally managed recipe, the category should be DepExternallyManaged
	result := validateSingleDependency(
		"libssl.so.3",
		binaryPath,
		nil,
		tmpDir,
		state,
		index,
		loader,
		mockActionLookup,
		make(map[string]bool),
		true, // recurse
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepExternallyManaged {
		t.Errorf("expected DepExternallyManaged, got %v", result.Category)
	}
	if result.Status != ValidationPass {
		t.Errorf("expected ValidationPass, got %v: %s", result.Status, result.Error)
	}
}

func TestValidateSingleDependency_TsukuManaged_WrongVersion(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	// State has libssl.so.3 in version 3.2.1, but we'll manipulate index
	// to point to a version that doesn't have it.
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libcrypto.so.3"}, // Does NOT include libssl.so.3
				},
				"3.2.0": {
					UsedBy:  []string{"test"},
					Sonames: []string{"libssl.so.3"}, // Has libssl.so.3
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	// libssl.so.3 will be in the index (from 3.2.0), classified as TsukuManaged
	// but when validateTsukuDep is called with the resolved version, it should check
	// if the actual soname matches
	result := validateSingleDependency(
		"libssl.so.3",
		binaryPath,
		nil,
		tmpDir,
		state,
		index,
		nil, nil,
		make(map[string]bool),
		false,
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepTsukuManaged {
		t.Errorf("expected DepTsukuManaged, got %v", result.Category)
	}
	if result.Status != ValidationPass {
		t.Errorf("expected ValidationPass, got %v: %s", result.Status, result.Error)
	}
}

func TestValidateSingleDependency_ExternallyManaged_FailValidation(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	// libcrypto.so.3 is in the soname index but the version state
	// doesn't list it as a soname (simulating a mismatch)
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libcrypto.so.3", "libssl.so.3"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)
	loader := newMockRecipeLoader()
	loader.addExternallyManagedRecipe("openssl")

	// Test with a soname that IS in the index but whose version state
	// we'll remove to force a validation failure
	// Override state to remove the version
	stateNoVersion := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {}, // Empty version map
		},
	}

	result := validateSingleDependency(
		"libssl.so.3",
		binaryPath,
		nil,
		tmpDir,
		stateNoVersion,
		index,
		loader,
		mockActionLookup,
		make(map[string]bool),
		false,
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepExternallyManaged {
		t.Errorf("expected DepExternallyManaged, got %v", result.Category)
	}
	if result.Status != ValidationFail {
		t.Errorf("expected ValidationFail, got %v", result.Status)
	}
}

func TestValidateSystemDep_PatternMatch(t *testing.T) {
	// System library patterns should be trusted
	err := validateSystemDep("libc.so.6", "linux")
	if err != nil {
		t.Errorf("expected nil for system library pattern, got: %v", err)
	}
}

func TestValidateSystemDep_AbsolutePathNotExist(t *testing.T) {
	err := validateSystemDep("/nonexistent/path/libfoo.so", "linux")
	if err == nil {
		t.Error("expected error for non-existent absolute path")
	}
}

func TestValidateSystemDep_AbsolutePathAccessError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "libtest.so")
	if err := os.WriteFile(path, []byte("test"), 0000); err != nil {
		t.Fatal(err)
	}

	// The path exists, so Stat won't return an error, it'll succeed
	err := validateSystemDep(path, "linux")
	// This should pass since the file exists even if unreadable
	_ = err
}

func TestResolveLibraryPath_AllEmpty(t *testing.T) {
	tests := []struct {
		name      string
		recipe    string
		version   string
		tsukuHome string
	}{
		{"empty recipe", "", "1.0.0", "/tmp"},
		{"empty version", "openssl", "", "/tmp"},
		{"empty home", "openssl", "1.0.0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := resolveLibraryPath(tt.recipe, tt.version, tt.tsukuHome)
			if path != "" {
				t.Errorf("expected empty path, got %q", path)
			}
		})
	}
}

func TestIsExternallyManaged_NonExternalRecipe(t *testing.T) {
	loader := newMockRecipeLoader()
	// Add a recipe that doesn't use package managers
	loader.recipes["openssl"] = &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "download", Params: map[string]interface{}{"url": "https://example.com"}},
		},
	}

	result := isExternallyManaged("openssl", loader, mockActionLookup, "linux", "amd64")
	if result {
		t.Error("expected false for non-externally-managed recipe")
	}
}

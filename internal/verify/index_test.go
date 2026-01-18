package verify

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
)

func TestNewSonameIndex(t *testing.T) {
	t.Parallel()

	index := NewSonameIndex()

	if index.SonameToRecipe == nil {
		t.Error("SonameToRecipe should not be nil")
	}
	if index.SonameToVersion == nil {
		t.Error("SonameToVersion should not be nil")
	}
	if index.Size() != 0 {
		t.Errorf("Size() = %d, want 0", index.Size())
	}
}

func TestBuildSonameIndex_NilState(t *testing.T) {
	t.Parallel()

	index := BuildSonameIndex(nil)

	if index == nil {
		t.Fatal("BuildSonameIndex(nil) returned nil")
	}
	if index.Size() != 0 {
		t.Errorf("Size() = %d, want 0", index.Size())
	}
}

func TestBuildSonameIndex_EmptyState(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	index := BuildSonameIndex(state)

	if index == nil {
		t.Fatal("BuildSonameIndex returned nil")
	}
	if index.Size() != 0 {
		t.Errorf("Size() = %d, want 0", index.Size())
	}
}

func TestBuildSonameIndex_SingleLibrary(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					Sonames: []string{"libssl.so.3", "libcrypto.so.3"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	if index.Size() != 2 {
		t.Errorf("Size() = %d, want 2", index.Size())
	}

	// Check libssl.so.3
	recipe, version, found := index.Lookup("libssl.so.3")
	if !found {
		t.Error("libssl.so.3 not found in index")
	}
	if recipe != "openssl" {
		t.Errorf("recipe = %q, want %q", recipe, "openssl")
	}
	if version != "3.2.1" {
		t.Errorf("version = %q, want %q", version, "3.2.1")
	}

	// Check libcrypto.so.3
	recipe, version, found = index.Lookup("libcrypto.so.3")
	if !found {
		t.Error("libcrypto.so.3 not found in index")
	}
	if recipe != "openssl" {
		t.Errorf("recipe = %q, want %q", recipe, "openssl")
	}
	if version != "3.2.1" {
		t.Errorf("version = %q, want %q", version, "3.2.1")
	}
}

func TestBuildSonameIndex_MultipleLibraries(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					Sonames: []string{"libssl.so.3"},
				},
			},
			"zlib": {
				"1.3.1": {
					Sonames: []string{"libz.so.1"},
				},
			},
			"libyaml": {
				"0.2.5": {
					Sonames: []string{"libyaml-0.so.2"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	if index.Size() != 3 {
		t.Errorf("Size() = %d, want 3", index.Size())
	}

	tests := []struct {
		soname  string
		recipe  string
		version string
	}{
		{"libssl.so.3", "openssl", "3.2.1"},
		{"libz.so.1", "zlib", "1.3.1"},
		{"libyaml-0.so.2", "libyaml", "0.2.5"},
	}

	for _, tt := range tests {
		t.Run(tt.soname, func(t *testing.T) {
			recipe, version, found := index.Lookup(tt.soname)
			if !found {
				t.Errorf("%s not found in index", tt.soname)
				return
			}
			if recipe != tt.recipe {
				t.Errorf("recipe = %q, want %q", recipe, tt.recipe)
			}
			if version != tt.version {
				t.Errorf("version = %q, want %q", version, tt.version)
			}
		})
	}
}

func TestBuildSonameIndex_MultipleVersions(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					Sonames: []string{"libssl.so.3"},
				},
				"1.1.1w": {
					Sonames: []string{"libssl.so.1.1"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	if index.Size() != 2 {
		t.Errorf("Size() = %d, want 2", index.Size())
	}

	// Check both versions map correctly
	recipe, version, found := index.Lookup("libssl.so.3")
	if !found {
		t.Error("libssl.so.3 not found in index")
	}
	if recipe != "openssl" || version != "3.2.1" {
		t.Errorf("libssl.so.3 -> (%q, %q), want (openssl, 3.2.1)", recipe, version)
	}

	recipe, version, found = index.Lookup("libssl.so.1.1")
	if !found {
		t.Error("libssl.so.1.1 not found in index")
	}
	if recipe != "openssl" || version != "1.1.1w" {
		t.Errorf("libssl.so.1.1 -> (%q, %q), want (openssl, 1.1.1w)", recipe, version)
	}
}

func TestSonameIndex_Lookup_NotFound(t *testing.T) {
	t.Parallel()

	index := NewSonameIndex()
	index.SonameToRecipe["libssl.so.3"] = "openssl"
	index.SonameToVersion["libssl.so.3"] = "3.2.1"

	recipe, version, found := index.Lookup("libfoo.so.1")
	if found {
		t.Error("Lookup should return false for non-existent soname")
	}
	if recipe != "" || version != "" {
		t.Errorf("Lookup returned (%q, %q), want (\"\", \"\")", recipe, version)
	}
}

func TestSonameIndex_Contains(t *testing.T) {
	t.Parallel()

	index := NewSonameIndex()
	index.SonameToRecipe["libssl.so.3"] = "openssl"

	if !index.Contains("libssl.so.3") {
		t.Error("Contains(libssl.so.3) = false, want true")
	}
	if index.Contains("libfoo.so.1") {
		t.Error("Contains(libfoo.so.1) = true, want false")
	}
}

func TestBuildSonameIndex_EmptySonames(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"somelib": {
				"1.0.0": {
					Sonames: []string{}, // Empty sonames
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	if index.Size() != 0 {
		t.Errorf("Size() = %d, want 0 (empty sonames should not add entries)", index.Size())
	}
}

func TestBuildSonameIndex_NilLibsMap(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: nil,
	}

	index := BuildSonameIndex(state)

	if index == nil {
		t.Fatal("BuildSonameIndex returned nil")
	}
	if index.Size() != 0 {
		t.Errorf("Size() = %d, want 0", index.Size())
	}
}

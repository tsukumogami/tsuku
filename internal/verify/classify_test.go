package verify

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
)

func TestDepCategory_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category DepCategory
		want     string
	}{
		{DepPureSystem, "PURE_SYSTEM"},
		{DepTsukuManaged, "TSUKU_MANAGED"},
		{DepExternallyManaged, "EXTERNALLY_MANAGED"},
		{DepUnknown, "UNKNOWN"},
		{DepCategory(99), "INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.category.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyDependency_InIndex(t *testing.T) {
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
	registry := DefaultRegistry

	tests := []struct {
		name    string
		dep     string
		wantCat DepCategory
		wantRec string
		wantVer string
	}{
		{"libssl.so.3", "libssl.so.3", DepTsukuManaged, "openssl", "3.2.1"},
		{"libcrypto.so.3", "libcrypto.so.3", DepTsukuManaged, "openssl", "3.2.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, recipe, version := ClassifyDependency(tt.dep, index, registry, "linux")
			if cat != tt.wantCat {
				t.Errorf("category = %v, want %v", cat, tt.wantCat)
			}
			if recipe != tt.wantRec {
				t.Errorf("recipe = %q, want %q", recipe, tt.wantRec)
			}
			if version != tt.wantVer {
				t.Errorf("version = %q, want %q", version, tt.wantVer)
			}
		})
	}
}

func TestClassifyDependency_SystemLibrary_Linux(t *testing.T) {
	t.Parallel()

	index := NewSonameIndex()
	registry := DefaultRegistry

	tests := []struct {
		name string
		dep  string
	}{
		{"libc.so.6", "libc.so.6"},
		{"libm.so.6", "libm.so.6"},
		{"libpthread.so.0", "libpthread.so.0"},
		{"libdl.so.2", "libdl.so.2"},
		{"libgcc_s.so.1", "libgcc_s.so.1"},
		{"ld-linux-x86-64.so.2", "ld-linux-x86-64.so.2"},
		{"linux-vdso.so.1", "linux-vdso.so.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, recipe, version := ClassifyDependency(tt.dep, index, registry, "linux")
			if cat != DepPureSystem {
				t.Errorf("category = %v, want %v", cat, DepPureSystem)
			}
			if recipe != "" {
				t.Errorf("recipe = %q, want empty", recipe)
			}
			if version != "" {
				t.Errorf("version = %q, want empty", version)
			}
		})
	}
}

func TestClassifyDependency_SystemLibrary_Darwin(t *testing.T) {
	t.Parallel()

	index := NewSonameIndex()
	registry := DefaultRegistry

	tests := []struct {
		name string
		dep  string
	}{
		{"libSystem", "/usr/lib/libSystem.B.dylib"},
		{"libc++", "/usr/lib/libc++.1.dylib"},
		{"libobjc", "/usr/lib/libobjc.A.dylib"},
		{"CoreFoundation", "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, recipe, version := ClassifyDependency(tt.dep, index, registry, "darwin")
			if cat != DepPureSystem {
				t.Errorf("category = %v, want %v", cat, DepPureSystem)
			}
			if recipe != "" {
				t.Errorf("recipe = %q, want empty", recipe)
			}
			if version != "" {
				t.Errorf("version = %q, want empty", version)
			}
		})
	}
}

func TestClassifyDependency_Unknown(t *testing.T) {
	t.Parallel()

	index := NewSonameIndex()
	registry := DefaultRegistry

	tests := []struct {
		name     string
		dep      string
		targetOS string
	}{
		{"libfoo.so.1 on linux", "libfoo.so.1", "linux"},
		{"libbar.dylib on darwin", "libbar.dylib", "darwin"},
		{"unknown on unknown os", "libfoo.so.1", "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, recipe, version := ClassifyDependency(tt.dep, index, registry, tt.targetOS)
			if cat != DepUnknown {
				t.Errorf("category = %v, want %v", cat, DepUnknown)
			}
			if recipe != "" {
				t.Errorf("recipe = %q, want empty", recipe)
			}
			if version != "" {
				t.Errorf("version = %q, want empty", version)
			}
		})
	}
}

func TestClassifyDependency_IndexPriority(t *testing.T) {
	t.Parallel()

	// Setup: libz is both a system pattern AND installed via tsuku
	// The index should take priority (critical design decision)
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"zlib": {
				"1.3.1": {
					Sonames: []string{"libz.so.1"},
				},
			},
		},
	}
	index := BuildSonameIndex(state)
	registry := DefaultRegistry

	// libz.so.1 matches both index AND /usr/lib/libz pattern on Darwin
	// But index should be checked first
	cat, recipe, version := ClassifyDependency("libz.so.1", index, registry, "linux")

	if cat != DepTsukuManaged {
		t.Errorf("category = %v, want %v (index should take priority over system patterns)", cat, DepTsukuManaged)
	}
	if recipe != "zlib" {
		t.Errorf("recipe = %q, want %q", recipe, "zlib")
	}
	if version != "1.3.1" {
		t.Errorf("version = %q, want %q", version, "1.3.1")
	}
}

func TestClassifyDependency_NilIndex(t *testing.T) {
	t.Parallel()

	registry := DefaultRegistry

	// System library should still be recognized with nil index
	cat, recipe, version := ClassifyDependency("libc.so.6", nil, registry, "linux")
	if cat != DepPureSystem {
		t.Errorf("category = %v, want %v", cat, DepPureSystem)
	}
	if recipe != "" || version != "" {
		t.Errorf("recipe/version should be empty for system lib")
	}

	// Unknown should be returned for non-system with nil index
	cat, _, _ = ClassifyDependency("libfoo.so.1", nil, registry, "linux")
	if cat != DepUnknown {
		t.Errorf("category = %v, want %v", cat, DepUnknown)
	}
}

func TestClassifyDependency_NilRegistry(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					Sonames: []string{"libssl.so.3"},
				},
			},
		},
	}
	index := BuildSonameIndex(state)

	// Index lookup should still work with nil registry
	cat, recipe, version := ClassifyDependency("libssl.so.3", index, nil, "linux")
	if cat != DepTsukuManaged {
		t.Errorf("category = %v, want %v", cat, DepTsukuManaged)
	}
	if recipe != "openssl" || version != "3.2.1" {
		t.Errorf("recipe/version mismatch")
	}

	// Unknown should be returned for non-indexed with nil registry
	cat, _, _ = ClassifyDependency("libc.so.6", index, nil, "linux")
	if cat != DepUnknown {
		t.Errorf("category = %v, want %v (no registry to check system patterns)", cat, DepUnknown)
	}
}

func TestClassifyDependencyResult(t *testing.T) {
	t.Parallel()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					Sonames: []string{"libssl.so.3"},
				},
			},
		},
	}
	index := BuildSonameIndex(state)
	registry := DefaultRegistry

	result := ClassifyDependencyResult("libssl.so.3", index, registry, "linux")

	if result.Category != DepTsukuManaged {
		t.Errorf("Category = %v, want %v", result.Category, DepTsukuManaged)
	}
	if result.Recipe != "openssl" {
		t.Errorf("Recipe = %q, want %q", result.Recipe, "openssl")
	}
	if result.Version != "3.2.1" {
		t.Errorf("Version = %q, want %q", result.Version, "3.2.1")
	}
	if result.Original != "libssl.so.3" {
		t.Errorf("Original = %q, want %q", result.Original, "libssl.so.3")
	}
}

func TestClassifyDependency_PathVariables(t *testing.T) {
	t.Parallel()

	// Path variables like $ORIGIN are treated as system patterns
	// because they need expansion before final classification
	index := NewSonameIndex()
	registry := DefaultRegistry

	tests := []struct {
		name string
		dep  string
	}{
		{"$ORIGIN", "$ORIGIN/../lib/libfoo.so"},
		{"${ORIGIN}", "${ORIGIN}/libfoo.so"},
		{"@rpath", "@rpath/libfoo.dylib"},
		{"@loader_path", "@loader_path/../lib/libfoo.dylib"},
		{"@executable_path", "@executable_path/libfoo.dylib"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Path variables are recognized by SystemLibraryRegistry
			cat, _, _ := ClassifyDependency(tt.dep, index, registry, "linux")
			if cat != DepPureSystem {
				t.Errorf("category = %v, want %v (path variables should be recognized)", cat, DepPureSystem)
			}
		})
	}
}

func TestErrUnknownDependency_Value(t *testing.T) {
	t.Parallel()

	// Verify the explicit value per design decision #2
	if ErrUnknownDependency != 11 {
		t.Errorf("ErrUnknownDependency = %d, want 11 (explicit value per design)", ErrUnknownDependency)
	}
}

func TestErrUnknownDependency_String(t *testing.T) {
	t.Parallel()

	got := ErrUnknownDependency.String()
	want := "unknown dependency"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

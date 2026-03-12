package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestDeduplicateLibraries(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two real files
	file1 := filepath.Join(tmpDir, "libfoo.so.1")
	file2 := filepath.Join(tmpDir, "libbar.so.1")
	if err := os.WriteFile(file1, []byte("lib1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("lib2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink to file1
	symlink := filepath.Join(tmpDir, "libfoo.so")
	if err := os.Symlink(file1, symlink); err != nil {
		t.Fatal(err)
	}

	// Deduplicate: file1 and symlink should resolve to same file
	result, err := deduplicateLibraries([]string{file1, symlink, file2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 unique files, got %d: %v", len(result), result)
	}
}

func TestDeduplicateLibraries_BrokenSymlinks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a broken symlink
	broken := filepath.Join(tmpDir, "broken.so")
	if err := os.Symlink("/nonexistent/target", broken); err != nil {
		t.Fatal(err)
	}

	// Create a valid file
	valid := filepath.Join(tmpDir, "valid.so")
	if err := os.WriteFile(valid, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Broken symlinks should be skipped
	result, err := deduplicateLibraries([]string{broken, valid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 file (broken skipped), got %d", len(result))
	}
}

func TestDeduplicateLibraries_Empty(t *testing.T) {
	result, err := deduplicateLibraries(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestIsSharedLibraryPath_MoreCases(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Standard shared objects
		{"/usr/lib/libz.so", true},
		{"/usr/lib/libz.so.1", true},
		{"/usr/lib/libz.so.1.2.11", true},

		// macOS dynamic libraries
		{"/usr/lib/libz.dylib", true},
		{"/usr/local/lib/libiconv.dylib", true},

		// Not shared libraries
		{"/usr/lib/libz.a", false},
		{"/usr/lib/libz.o", false},
		{"/usr/bin/tool", false},
		{"/usr/lib/libz.so.bak", false},
		{"/usr/lib/libz.soname", false},
		{"/usr/include/header.h", false},

		// Edge cases
		{"/usr/lib/libz.so.", true}, // Trailing dot passes version check (empty suffix after dot)
		{"libfoo.so.1.2.3", true},
		{"libfoo.dylib", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isSharedLibraryPath(tt.path)
			if got != tt.want {
				t.Errorf("isSharedLibraryPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCheckExternalLibrary_EmptyPackages(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "zlib",
			Type: "library",
		},
		Steps: []recipe.Step{
			{
				Action: "apk_install",
				Params: map[string]interface{}{}, // No packages key
			},
		},
	}

	target := platform.NewTarget("linux/amd64", "alpine", "musl", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for empty packages, got %v", info)
	}
}

func TestCheckExternalLibrary_WithWhenMismatch(t *testing.T) {
	// Create a recipe with a "when" condition that doesn't match the target
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "zlib",
			Type: "library",
		},
		Steps: []recipe.Step{
			{
				Action: "apk_install",
				When:   &recipe.WhenClause{OS: []string{"darwin"}}, // Won't match linux target
				Params: map[string]interface{}{"packages": []string{"zlib-dev"}},
			},
		},
	}

	target := platform.NewTarget("linux/amd64", "alpine", "musl", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for when mismatch, got %v", info)
	}
}

func TestGetPackagesFromParams_UnsupportedType(t *testing.T) {
	params := map[string]interface{}{
		"packages": 42, // Not a slice
	}

	result := getPackagesFromParams(params)
	if result != nil {
		t.Errorf("expected nil for unsupported type, got %v", result)
	}
}

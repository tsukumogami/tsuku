package verify

import (
	"testing"

	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestGetPackagesFromParams(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   []string
	}{
		{
			name:   "nil params",
			params: nil,
			want:   nil,
		},
		{
			name:   "no packages key",
			params: map[string]interface{}{"other": "value"},
			want:   nil,
		},
		{
			name:   "string slice",
			params: map[string]interface{}{"packages": []string{"zlib-dev", "yaml-dev"}},
			want:   []string{"zlib-dev", "yaml-dev"},
		},
		{
			name:   "interface slice",
			params: map[string]interface{}{"packages": []interface{}{"zlib-dev", "yaml-dev"}},
			want:   []string{"zlib-dev", "yaml-dev"},
		},
		{
			name:   "mixed interface slice",
			params: map[string]interface{}{"packages": []interface{}{"valid", 123, "also-valid"}},
			want:   []string{"valid", "also-valid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPackagesFromParams(tt.params)
			if len(got) != len(tt.want) {
				t.Errorf("getPackagesFromParams() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getPackagesFromParams()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsSharedLibraryPath(t *testing.T) {
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
		{"/usr/bin/zlib", false},
		{"/usr/include/header.h", false},

		// Edge cases
		{"/usr/lib/libz.so.", true}, // Trailing dot passes version check (empty suffix after dot)
		{"libfoo.so.1.2.3", true},
		{"libfoo.dylib", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := isSharedLibraryPath(tt.path); got != tt.want {
				t.Errorf("isSharedLibraryPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPackageManagerActions(t *testing.T) {
	// Verify all expected package manager actions are registered
	expected := map[string]string{
		"apk_install":    "alpine",
		"apt_install":    "debian",
		"dnf_install":    "rhel",
		"pacman_install": "arch",
		"zypper_install": "suse",
	}

	for action, family := range expected {
		if got, ok := packageManagerActions[action]; !ok {
			t.Errorf("packageManagerActions missing %q", action)
		} else if got != family {
			t.Errorf("packageManagerActions[%q] = %q, want %q", action, got, family)
		}
	}
}

func TestCheckExternalLibrary_NonLibrary(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "some-tool",
			Type: "tool",
		},
	}

	target := platform.NewTarget("linux/amd64", "alpine", "musl", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("CheckExternalLibrary() error = %v", err)
	}
	if info != nil {
		t.Errorf("CheckExternalLibrary() = %v, want nil for non-library", info)
	}
}

func TestCheckExternalLibrary_NoMatchingSteps(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "zlib",
			Type: "library",
		},
		Steps: []recipe.Step{
			{
				Action: "homebrew",
				Params: map[string]interface{}{"formula": "zlib"},
			},
		},
	}

	target := platform.NewTarget("linux/amd64", "alpine", "musl", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("CheckExternalLibrary() error = %v", err)
	}
	if info != nil {
		t.Errorf("CheckExternalLibrary() = %v, want nil for no matching steps", info)
	}
}

func TestCheckExternalLibrary_WrongFamily(t *testing.T) {
	// Recipe has apk_install but we're on debian
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "zlib",
			Type: "library",
		},
		Steps: []recipe.Step{
			{
				Action: "apk_install",
				Params: map[string]interface{}{"packages": []string{"zlib-dev"}},
			},
		},
	}

	// Target is debian, not alpine
	target := platform.NewTarget("linux/amd64", "debian", "glibc", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("CheckExternalLibrary() error = %v", err)
	}
	if info != nil {
		t.Errorf("CheckExternalLibrary() = %v, want nil for wrong family", info)
	}
}

func TestCheckExternalLibrary_NonPackageManagerAction(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "openssl",
			Type: "library",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{"url": "https://example.com/openssl.tar.gz"},
			},
		},
	}

	target := platform.NewTarget("linux/amd64", "debian", "glibc", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil for non-package-manager action")
	}
}

func TestCheckExternalLibrary_ReachesPackageCheck(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "zlib",
			Type: "library",
		},
		Steps: []recipe.Step{
			{
				Action: "apt_install",
				Params: map[string]interface{}{"packages": []string{"zlib1g-dev"}},
			},
		},
	}

	// Target is debian, matching apt_install's family
	target := platform.NewTarget("linux/amd64", "debian", "glibc", "")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// On CI without zlib1g-dev installed, allPackagesInstalled returns false -> nil
	// This exercises the allPackagesInstalled call path
	if info != nil {
		t.Logf("library found: %+v", info)
	}
}

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

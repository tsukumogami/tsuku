package verify

import (
	"testing"

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
		{"/usr/lib/libz.so", true},
		{"/usr/lib/libz.so.1", true},
		{"/usr/lib/libz.so.1.2.11", true},
		{"/usr/lib/libz.dylib", true},
		{"/usr/lib/libz.a", false},
		{"/usr/lib/libz.o", false},
		{"/usr/lib/libz.so.bak", false},
		{"/usr/lib/libz.soname", false},
		{"/usr/bin/zlib", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
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

	target := platform.NewTarget("linux/amd64", "alpine", "musl")
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

	target := platform.NewTarget("linux/amd64", "alpine", "musl")
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
	target := platform.NewTarget("linux/amd64", "debian", "glibc")
	info, err := CheckExternalLibrary(r, target)
	if err != nil {
		t.Errorf("CheckExternalLibrary() error = %v", err)
	}
	if info != nil {
		t.Errorf("CheckExternalLibrary() = %v, want nil for wrong family", info)
	}
}

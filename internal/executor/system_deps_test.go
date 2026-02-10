package executor

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestExtractSystemPackages(t *testing.T) {
	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		target   platform.Target
		expected []string
	}{
		{
			name: "alpine packages",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "apk_install",
						Params: map[string]any{
							"packages": []any{"zlib-dev", "openssl-dev"},
						},
						When: &recipe.WhenClause{
							OS:   []string{"linux"},
							Libc: []string{"musl"},
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "alpine", "musl"),
			expected: []string{"zlib-dev", "openssl-dev"},
		},
		{
			name: "debian packages",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "apt_install",
						Params: map[string]any{
							"packages": []any{"libssl-dev", "libz-dev"},
						},
						When: &recipe.WhenClause{
							OS:          []string{"linux"},
							LinuxFamily: "debian",
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "debian", "glibc"),
			expected: []string{"libssl-dev", "libz-dev"},
		},
		{
			name: "filtered by platform",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "apk_install",
						Params: map[string]any{
							"packages": []any{"alpine-pkg"},
						},
						When: &recipe.WhenClause{
							Libc: []string{"musl"},
						},
					},
					{
						Action: "apt_install",
						Params: map[string]any{
							"packages": []any{"debian-pkg"},
						},
						When: &recipe.WhenClause{
							LinuxFamily: "debian",
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "alpine", "musl"),
			expected: []string{"alpine-pkg"},
		},
		{
			name: "deduplicates packages",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "apk_install",
						Params: map[string]any{
							"packages": []any{"gcc", "make"},
						},
					},
					{
						Action: "apk_install",
						Params: map[string]any{
							"packages": []any{"gcc", "bash"},
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "alpine", "musl"),
			expected: []string{"gcc", "make", "bash"},
		},
		{
			name: "no system packages",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "download",
						Params: map[string]any{
							"url": "https://example.com/file.tar.gz",
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "alpine", "musl"),
			expected: nil,
		},
		{
			name: "empty recipe",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{},
			},
			target:   platform.NewTarget("linux/amd64", "alpine", "musl"),
			expected: nil,
		},
		{
			name: "dnf packages for rhel",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "dnf_install",
						Params: map[string]any{
							"packages": []any{"openssl-devel"},
						},
						When: &recipe.WhenClause{
							LinuxFamily: "rhel",
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "rhel", "glibc"),
			expected: []string{"openssl-devel"},
		},
		{
			name: "pacman packages for arch",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "pacman_install",
						Params: map[string]any{
							"packages": []any{"openssl"},
						},
						When: &recipe.WhenClause{
							LinuxFamily: "arch",
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "arch", "glibc"),
			expected: []string{"openssl"},
		},
		{
			name: "zypper packages for suse",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "zypper_install",
						Params: map[string]any{
							"packages": []any{"libopenssl-devel"},
						},
						When: &recipe.WhenClause{
							LinuxFamily: "suse",
						},
					},
				},
			},
			target:   platform.NewTarget("linux/amd64", "suse", "glibc"),
			expected: []string{"libopenssl-devel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSystemPackages(tt.recipe, tt.target)

			if len(got) != len(tt.expected) {
				t.Errorf("ExtractSystemPackages() got %v, want %v", got, tt.expected)
				return
			}

			for i, pkg := range got {
				if pkg != tt.expected[i] {
					t.Errorf("ExtractSystemPackages()[%d] = %q, want %q", i, pkg, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractSystemPackagesFromSteps(t *testing.T) {
	tests := []struct {
		name     string
		steps    []recipe.Step
		expected []string
	}{
		{
			name: "extracts from multiple step types",
			steps: []recipe.Step{
				{
					Action: "apk_install",
					Params: map[string]any{"packages": []any{"pkg1"}},
				},
				{
					Action: "apt_install",
					Params: map[string]any{"packages": []any{"pkg2"}},
				},
			},
			expected: []string{"pkg1", "pkg2"},
		},
		{
			name: "ignores non-system actions",
			steps: []recipe.Step{
				{
					Action: "download",
					Params: map[string]any{"url": "https://example.com"},
				},
				{
					Action: "apk_install",
					Params: map[string]any{"packages": []any{"real-pkg"}},
				},
			},
			expected: []string{"real-pkg"},
		},
		{
			name:     "empty steps",
			steps:    []recipe.Step{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSystemPackagesFromSteps(tt.steps)

			if len(got) != len(tt.expected) {
				t.Errorf("ExtractSystemPackagesFromSteps() got %v, want %v", got, tt.expected)
				return
			}

			for i, pkg := range got {
				if pkg != tt.expected[i] {
					t.Errorf("ExtractSystemPackagesFromSteps()[%d] = %q, want %q", i, pkg, tt.expected[i])
				}
			}
		})
	}
}

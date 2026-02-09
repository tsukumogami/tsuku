package main

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestExtractSystemPackages(t *testing.T) {
	tests := []struct {
		name         string
		steps        []recipe.Step
		target       platform.Target
		wantPackages []string
	}{
		{
			name: "extracts apk packages for alpine",
			steps: []recipe.Step{
				{Action: "apk_install", Params: map[string]interface{}{"packages": []interface{}{"zlib-dev"}}},
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"zlib1g-dev"}}},
			},
			target:       platform.NewTarget("linux/amd64", "alpine", "musl"),
			wantPackages: []string{"zlib-dev"},
		},
		{
			name: "extracts apt packages for debian",
			steps: []recipe.Step{
				{Action: "apk_install", Params: map[string]interface{}{"packages": []interface{}{"zlib-dev"}}},
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"zlib1g-dev"}}},
			},
			target:       platform.NewTarget("linux/amd64", "debian", "glibc"),
			wantPackages: []string{"zlib1g-dev"},
		},
		{
			name: "extracts multiple packages",
			steps: []recipe.Step{
				{Action: "apk_install", Params: map[string]interface{}{"packages": []interface{}{"zlib-dev", "yaml-dev"}}},
			},
			target:       platform.NewTarget("linux/amd64", "alpine", "musl"),
			wantPackages: []string{"zlib-dev", "yaml-dev"},
		},
		{
			name: "returns empty for non-matching platform",
			steps: []recipe.Step{
				{Action: "apk_install", Params: map[string]interface{}{"packages": []interface{}{"zlib-dev"}}},
			},
			target:       platform.NewTarget("darwin/arm64", "", ""),
			wantPackages: nil,
		},
		{
			name: "ignores non-system actions",
			steps: []recipe.Step{
				{Action: "download"},
				{Action: "extract"},
				{Action: "apk_install", Params: map[string]interface{}{"packages": []interface{}{"zlib-dev"}}},
				{Action: "install_binaries"},
			},
			target:       platform.NewTarget("linux/amd64", "alpine", "musl"),
			wantPackages: []string{"zlib-dev"},
		},
		{
			name: "handles step without packages param",
			steps: []recipe.Step{
				{Action: "apk_install", Params: map[string]interface{}{}},
			},
			target:       platform.NewTarget("linux/amd64", "alpine", "musl"),
			wantPackages: nil,
		},
		{
			name: "extracts brew_install packages for darwin",
			steps: []recipe.Step{
				{Action: "brew_install", Params: map[string]interface{}{"packages": []interface{}{"openssl"}}},
			},
			target:       platform.NewTarget("darwin/arm64", "", ""),
			wantPackages: []string{"openssl"},
		},
		{
			name: "respects when clause with libc filter",
			steps: []recipe.Step{
				{
					Action: "apk_install",
					Params: map[string]interface{}{"packages": []interface{}{"zlib-dev"}},
					When:   &recipe.WhenClause{OS: []string{"linux"}, Libc: []string{"musl"}},
				},
				{
					Action: "homebrew",
					Params: map[string]interface{}{"formula": "zlib"},
					When:   &recipe.WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
				},
			},
			target:       platform.NewTarget("linux/amd64", "alpine", "musl"),
			wantPackages: []string{"zlib-dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{Steps: tt.steps}
			got := extractSystemPackages(r, tt.target)

			if len(got) != len(tt.wantPackages) {
				t.Errorf("extractSystemPackages() returned %d packages, want %d", len(got), len(tt.wantPackages))
				t.Errorf("got: %v, want: %v", got, tt.wantPackages)
				return
			}

			for i, pkg := range tt.wantPackages {
				if got[i] != pkg {
					t.Errorf("extractSystemPackages()[%d] = %q, want %q", i, got[i], pkg)
				}
			}
		})
	}
}

func TestBuildTargetFromFlags(t *testing.T) {
	tests := []struct {
		name       string
		family     string
		wantOS     string
		wantFamily string
		wantLibc   string
	}{
		{
			name:       "alpine family",
			family:     "alpine",
			wantOS:     "linux",
			wantFamily: "alpine",
			wantLibc:   "musl",
		},
		{
			name:       "debian family",
			family:     "debian",
			wantOS:     "linux",
			wantFamily: "debian",
			wantLibc:   "glibc",
		},
		{
			name:       "rhel family",
			family:     "rhel",
			wantOS:     "linux",
			wantFamily: "rhel",
			wantLibc:   "glibc",
		},
		{
			name:       "arch family",
			family:     "arch",
			wantOS:     "linux",
			wantFamily: "arch",
			wantLibc:   "glibc",
		},
		{
			name:       "suse family",
			family:     "suse",
			wantOS:     "linux",
			wantFamily: "suse",
			wantLibc:   "glibc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTargetFromFlags(tt.family)

			if got.OS() != tt.wantOS {
				t.Errorf("buildTargetFromFlags(%q).OS() = %q, want %q", tt.family, got.OS(), tt.wantOS)
			}
			if got.LinuxFamily() != tt.wantFamily {
				t.Errorf("buildTargetFromFlags(%q).LinuxFamily() = %q, want %q", tt.family, got.LinuxFamily(), tt.wantFamily)
			}
			if got.Libc() != tt.wantLibc {
				t.Errorf("buildTargetFromFlags(%q).Libc() = %q, want %q", tt.family, got.Libc(), tt.wantLibc)
			}
		})
	}
}

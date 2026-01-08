package main

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestHasSystemDeps(t *testing.T) {
	tests := []struct {
		name  string
		steps []recipe.Step
		want  bool
	}{
		{
			name:  "no steps",
			steps: nil,
			want:  false,
		},
		{
			name: "only download steps",
			steps: []recipe.Step{
				{Action: "download"},
				{Action: "extract"},
				{Action: "install_binaries"},
			},
			want: false,
		},
		{
			name: "has apt_install",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"docker-ce"}}},
			},
			want: true,
		},
		{
			name: "has brew_install",
			steps: []recipe.Step{
				{Action: "brew_install", Params: map[string]interface{}{"formula": "docker"}},
			},
			want: true,
		},
		{
			name: "has require_command",
			steps: []recipe.Step{
				{Action: "require_command", Params: map[string]interface{}{"command": "docker"}},
			},
			want: true,
		},
		{
			name: "has manual",
			steps: []recipe.Step{
				{Action: "manual", Params: map[string]interface{}{"message": "Install manually"}},
			},
			want: true,
		},
		{
			name: "mixed steps with system dep",
			steps: []recipe.Step{
				{Action: "download"},
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"build-essential"}}},
				{Action: "install_binaries"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{Steps: tt.steps}
			got := hasSystemDeps(r)
			if got != tt.want {
				t.Errorf("hasSystemDeps() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSystemDepsForTarget(t *testing.T) {
	tests := []struct {
		name       string
		steps      []recipe.Step
		target     platform.Target
		wantCount  int
		wantAction string
	}{
		{
			name: "filters apt_install for debian",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"docker-ce"}}},
				{Action: "brew_install", Params: map[string]interface{}{"formula": "docker"}},
			},
			target:     platform.NewTarget("linux/amd64", "debian"),
			wantCount:  1,
			wantAction: "apt_install",
		},
		{
			name: "filters brew_install for darwin",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"docker-ce"}}},
				{Action: "brew_install", Params: map[string]interface{}{"formula": "docker"}},
			},
			target:     platform.NewTarget("darwin/arm64", ""),
			wantCount:  1,
			wantAction: "brew_install",
		},
		{
			name: "no system deps returns empty",
			steps: []recipe.Step{
				{Action: "download"},
				{Action: "extract"},
			},
			target:    platform.NewTarget("linux/amd64", "debian"),
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{Steps: tt.steps}
			got := getSystemDepsForTarget(r, tt.target)
			if len(got) != tt.wantCount {
				t.Errorf("getSystemDepsForTarget() returned %d steps, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 && got[0].Action != tt.wantAction {
				t.Errorf("first action = %q, want %q", got[0].Action, tt.wantAction)
			}
		})
	}
}

func TestGetTargetDisplayName(t *testing.T) {
	tests := []struct {
		name   string
		target platform.Target
		want   string
	}{
		{
			name:   "darwin returns macOS",
			target: platform.NewTarget("darwin/arm64", ""),
			want:   "macOS",
		},
		{
			name:   "debian family",
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   "Ubuntu/Debian",
		},
		{
			name:   "rhel family",
			target: platform.NewTarget("linux/amd64", "rhel"),
			want:   "Fedora/RHEL/CentOS",
		},
		{
			name:   "arch family",
			target: platform.NewTarget("linux/amd64", "arch"),
			want:   "Arch Linux",
		},
		{
			name:   "alpine family",
			target: platform.NewTarget("linux/amd64", "alpine"),
			want:   "Alpine Linux",
		},
		{
			name:   "suse family",
			target: platform.NewTarget("linux/amd64", "suse"),
			want:   "openSUSE/SLES",
		},
		{
			name:   "unknown family returns raw value",
			target: platform.NewTarget("linux/amd64", "gentoo"),
			want:   "gentoo",
		},
		{
			name:   "linux without family returns os",
			target: platform.NewTarget("linux/amd64", ""),
			want:   "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTargetDisplayName(tt.target)
			if got != tt.want {
				t.Errorf("getTargetDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTarget(t *testing.T) {
	// Note: This test depends on runtime.GOOS, so we primarily test the override behavior

	tests := []struct {
		name           string
		familyOverride string
		wantFamily     string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:           "valid debian override",
			familyOverride: "debian",
			wantFamily:     "debian",
			wantErr:        false,
		},
		{
			name:           "valid rhel override",
			familyOverride: "rhel",
			wantFamily:     "rhel",
			wantErr:        false,
		},
		{
			name:           "valid arch override",
			familyOverride: "arch",
			wantFamily:     "arch",
			wantErr:        false,
		},
		{
			name:           "valid alpine override",
			familyOverride: "alpine",
			wantFamily:     "alpine",
			wantErr:        false,
		},
		{
			name:           "valid suse override",
			familyOverride: "suse",
			wantFamily:     "suse",
			wantErr:        false,
		},
		{
			name:           "invalid override",
			familyOverride: "gentoo",
			wantErr:        true,
			wantErrContain: "invalid target-family",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTarget(tt.familyOverride)

			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveTarget() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.wantErrContain != "" && !containsString(err.Error(), tt.wantErrContain) {
					t.Errorf("resolveTarget() error = %v, want error containing %q", err, tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("resolveTarget() unexpected error = %v", err)
				return
			}

			if tt.familyOverride != "" && got.LinuxFamily() != tt.wantFamily {
				t.Errorf("resolveTarget() LinuxFamily() = %q, want %q", got.LinuxFamily(), tt.wantFamily)
			}
		})
	}
}

// containsString and containsSubstring are defined in create_test.go

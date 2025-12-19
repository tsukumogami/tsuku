package actions

import (
	"testing"
)

func TestActionDependencies_EcosystemActions(t *testing.T) {
	t.Parallel()
	// Ecosystem actions should have both install-time and runtime dependencies
	tests := []struct {
		action          string
		wantInstallTime []string
		wantRuntime     []string
	}{
		{"npm_install", []string{"nodejs"}, []string{"nodejs"}},
		{"pipx_install", []string{"python-standalone"}, []string{"python-standalone"}},
		{"gem_install", []string{"ruby"}, []string{"ruby"}},
		{"cpan_install", []string{"perl"}, []string{"perl"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.InstallTime, tt.wantInstallTime) {
				t.Errorf("InstallTime = %v, want %v", deps.InstallTime, tt.wantInstallTime)
			}
			if !slicesEqual(deps.Runtime, tt.wantRuntime) {
				t.Errorf("Runtime = %v, want %v", deps.Runtime, tt.wantRuntime)
			}
		})
	}
}

func TestActionDependencies_BuildActions(t *testing.T) {
	t.Parallel()
	// Build actions should have install-time deps for build tools
	tests := []struct {
		action          string
		wantInstallTime []string
	}{
		{"configure_make", []string{"make", "zig", "pkg-config"}},
		{"cmake_build", []string{"cmake", "make", "zig", "pkg-config"}},
		{"meson_build", []string{"meson", "make", "zig", "pkg-config"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.InstallTime, tt.wantInstallTime) {
				t.Errorf("InstallTime = %v, want %v", deps.InstallTime, tt.wantInstallTime)
			}
			if deps.Runtime != nil {
				t.Errorf("Runtime = %v, want nil", deps.Runtime)
			}
		})
	}
}

func TestActionDependencies_CompiledBinaryActions(t *testing.T) {
	t.Parallel()
	// Compiled binary actions should have install-time deps but no runtime deps
	tests := []struct {
		action          string
		wantInstallTime []string
	}{
		{"go_install", []string{"go"}},
		{"cargo_install", []string{"rust"}},
		{"nix_install", []string{"nix-portable"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.InstallTime, tt.wantInstallTime) {
				t.Errorf("InstallTime = %v, want %v", deps.InstallTime, tt.wantInstallTime)
			}
			if deps.Runtime != nil {
				t.Errorf("Runtime = %v, want nil", deps.Runtime)
			}
		})
	}
}

func TestActionDependencies_NoDependencyActions(t *testing.T) {
	t.Parallel()
	// These actions should have no dependencies
	actions := []string{
		// Download/extract
		"download",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"run_command",
		// System package managers
		"apt_install",
		"yum_install",
		"brew_install",
		// Composites
		"download_archive",
		"github_archive",
		"github_file",
	}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			deps := GetActionDeps(action)

			if deps.InstallTime != nil {
				t.Errorf("InstallTime = %v, want nil", deps.InstallTime)
			}
			if deps.Runtime != nil {
				t.Errorf("Runtime = %v, want nil", deps.Runtime)
			}
		})
	}
}

func TestGetActionDeps_UnknownAction(t *testing.T) {
	t.Parallel()
	deps := GetActionDeps("nonexistent_action")

	if deps.InstallTime != nil {
		t.Errorf("InstallTime = %v, want nil for unknown action", deps.InstallTime)
	}
	if deps.Runtime != nil {
		t.Errorf("Runtime = %v, want nil for unknown action", deps.Runtime)
	}
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

package actions

import (
	"strings"
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
		{"pipx_install", []string{"pipx"}, []string{"python"}},
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
		"hashicorp_release",
		"homebrew_bottle",
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

func TestActionDependencies_AllRegisteredActionsHaveEntries(t *testing.T) {
	t.Parallel()
	// Verify that every registered action has an entry in ActionDependencies
	// Skip test_ prefixed actions as those are registered by other tests
	for name := range registry {
		if strings.HasPrefix(name, "test_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if _, ok := ActionDependencies[name]; !ok {
				t.Errorf("action %q is registered but has no entry in ActionDependencies", name)
			}
		})
	}
}

func TestActionDependencies_AllEntriesAreRegisteredActions(t *testing.T) {
	t.Parallel()
	// Verify that every entry in ActionDependencies corresponds to a registered action
	for name := range ActionDependencies {
		t.Run(name, func(t *testing.T) {
			if Get(name) == nil {
				t.Errorf("ActionDependencies has entry for %q but action is not registered", name)
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

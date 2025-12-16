package actions

import (
	"testing"
)

func TestRegister_And_Get(t *testing.T) {
	// Test that core actions are registered
	coreActions := []string{
		"download",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"run_command",
		"apt_install",
		"yum_install",
		"brew_install",
		"npm_install",
		"pipx_install",
		"cargo_install",
		"gem_install",
		"nix_install",
		"download_archive",
		"github_archive",
		"github_file",
		"homebrew",
	}

	for _, name := range coreActions {
		t.Run(name, func(t *testing.T) {
			action := Get(name)
			if action == nil {
				t.Errorf("Get(%q) = nil, want registered action", name)
			}
			if action != nil && action.Name() != name {
				t.Errorf("Get(%q).Name() = %q, want %q", name, action.Name(), name)
			}
		})
	}
}

func TestGet_NonExistent(t *testing.T) {
	action := Get("nonexistent_action")
	if action != nil {
		t.Errorf("Get(nonexistent_action) = %v, want nil", action)
	}
}

// mockAction implements Action for testing Register
type mockAction struct {
	BaseAction
	name string
}

func (m *mockAction) Name() string {
	return m.name
}

func (m *mockAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	return nil
}

func TestRegister_CustomAction(t *testing.T) {
	// Register a custom action
	customAction := &mockAction{name: "test_custom_action"}
	Register(customAction)

	// Verify it can be retrieved
	retrieved := Get("test_custom_action")
	if retrieved == nil {
		t.Error("Get(test_custom_action) = nil after Register")
	}
	if retrieved != nil && retrieved.Name() != "test_custom_action" {
		t.Errorf("Retrieved action name = %q, want %q", retrieved.Name(), "test_custom_action")
	}
}

func TestBaseAction_RequiresNetwork(t *testing.T) {
	// BaseAction should return false by default
	var ba BaseAction
	if ba.RequiresNetwork() {
		t.Error("BaseAction.RequiresNetwork() = true, want false")
	}
}

func TestNetworkValidator_AllActions(t *testing.T) {
	// Actions that should require network access
	networkRequired := map[string]bool{
		// Ecosystem primitives
		"cargo_build":   true,
		"cargo_install": true,
		"go_build":      true,
		"go_install":    true,
		"cpan_install":  true,
		"npm_install":   true,
		"npm_exec":      true, // Downloads packages from npm registry
		"pip_install":   true,
		"pipx_install":  true,
		"gem_install":   true,
		// System package managers
		"apt_install":  true,
		"yum_install":  true,
		"brew_install": true,
		// Nix actions
		"nix_install": true,
		"nix_realize": true,
		// Arbitrary commands (conservative)
		"run_command": true,
	}

	// Actions that should NOT require network (work offline with cached content)
	networkNotRequired := []string{
		"download",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"set_rpath",
		"install_libraries",
		"link_dependencies",
		"configure_make",
		"cmake_build",
		"apply_patch_file",
		"text_replace",
		// Composite actions (inherit from BaseAction)
		"download_archive",
		"github_archive",
		"github_file",
		"homebrew",
		"homebrew_source",
	}

	// Test actions that require network
	for name, expected := range networkRequired {
		t.Run(name+"_requires_network", func(t *testing.T) {
			action := Get(name)
			if action == nil {
				t.Skipf("Action %q not registered", name)
			}
			nv, ok := action.(NetworkValidator)
			if !ok {
				t.Errorf("Action %q does not implement NetworkValidator", name)
				return
			}
			if got := nv.RequiresNetwork(); got != expected {
				t.Errorf("%s.RequiresNetwork() = %v, want %v", name, got, expected)
			}
		})
	}

	// Test actions that don't require network
	for _, name := range networkNotRequired {
		t.Run(name+"_no_network", func(t *testing.T) {
			action := Get(name)
			if action == nil {
				t.Skipf("Action %q not registered", name)
			}
			nv, ok := action.(NetworkValidator)
			if !ok {
				// Action doesn't implement NetworkValidator, that's fine
				// (defaults to no network via BaseAction)
				return
			}
			if got := nv.RequiresNetwork(); got {
				t.Errorf("%s.RequiresNetwork() = true, want false", name)
			}
		})
	}
}

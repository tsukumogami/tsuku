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
		"hashicorp_release",
		"homebrew_bottle",
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

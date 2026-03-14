package actions

import (
	"testing"
)

// -- install_gem_direct.go: Dependencies, IsDeterministic, RequiresNetwork --

func TestInstallGemDirectAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := InstallGemDirectAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "ruby" {
		t.Errorf("Dependencies().InstallTime = %v, want [ruby]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "ruby" {
		t.Errorf("Dependencies().Runtime = %v, want [ruby]", deps.Runtime)
	}
}

func TestInstallGemDirectAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := InstallGemDirectAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

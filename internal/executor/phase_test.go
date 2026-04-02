package executor

import (
	"testing"
)

func TestStepPhase_DefaultsToInstall(t *testing.T) {
	step := ResolvedStep{Action: "download_file"}
	if phase := StepPhase(step); phase != "install" {
		t.Errorf("expected empty phase to default to 'install', got %q", phase)
	}
}

func TestStepPhase_PreservesExplicitPhase(t *testing.T) {
	step := ResolvedStep{Action: "install_shell_init", Phase: "post-install"}
	if phase := StepPhase(step); phase != "post-install" {
		t.Errorf("expected phase 'post-install', got %q", phase)
	}
}

func TestPhaseFiltering(t *testing.T) {
	steps := []ResolvedStep{
		{Action: "download_file", Phase: ""},
		{Action: "extract", Phase: ""},
		{Action: "install_binaries", Phase: "install"},
		{Action: "install_shell_init", Phase: "post-install"},
	}

	t.Run("install phase includes empty and explicit install", func(t *testing.T) {
		var installSteps []ResolvedStep
		for _, s := range steps {
			if StepPhase(s) == "install" {
				installSteps = append(installSteps, s)
			}
		}
		if len(installSteps) != 3 {
			t.Errorf("expected 3 install steps, got %d", len(installSteps))
		}
	})

	t.Run("post-install phase", func(t *testing.T) {
		var postSteps []ResolvedStep
		for _, s := range steps {
			if StepPhase(s) == "post-install" {
				postSteps = append(postSteps, s)
			}
		}
		if len(postSteps) != 1 {
			t.Errorf("expected 1 post-install step, got %d", len(postSteps))
		}
		if postSteps[0].Action != "install_shell_init" {
			t.Errorf("expected install_shell_init, got %s", postSteps[0].Action)
		}
	})

	t.Run("unknown phase returns empty", func(t *testing.T) {
		var preRemoveSteps []ResolvedStep
		for _, s := range steps {
			if StepPhase(s) == "pre-remove" {
				preRemoveSteps = append(preRemoveSteps, s)
			}
		}
		if len(preRemoveSteps) != 0 {
			t.Errorf("expected 0 pre-remove steps, got %d", len(preRemoveSteps))
		}
	})
}

func TestResolvedStepPhaseJSON(t *testing.T) {
	// Verify that Phase field is set correctly in ResolvedStep
	step := ResolvedStep{
		Action: "install_shell_init",
		Phase:  "post-install",
		Params: map[string]interface{}{
			"source_file": "init.sh",
			"target":      "niwa",
		},
	}
	if step.Phase != "post-install" {
		t.Errorf("expected phase post-install, got %s", step.Phase)
	}
}

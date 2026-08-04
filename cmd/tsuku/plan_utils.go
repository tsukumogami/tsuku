package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
)

// loadPlanFromSource reads a plan from file path or stdin.
// If path is "-", reads from stdin.
func loadPlanFromSource(path string) (*executor.InstallationPlan, error) {
	return loadPlanFromSourceWithReader(path, os.Stdin)
}

// loadPlanFromSourceWithReader is the internal implementation that accepts a custom
// stdin reader for testing.
func loadPlanFromSourceWithReader(path string, stdin io.Reader) (*executor.InstallationPlan, error) {
	var reader io.Reader
	if path == "-" {
		reader = stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open plan file: %w", err)
		}
		defer f.Close()
		reader = f
	}

	var plan executor.InstallationPlan
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&plan); err != nil {
		if path == "-" {
			return nil, fmt.Errorf("failed to parse plan from stdin: %w\nHint: Save plan to a file first for debugging", err)
		}
		return nil, fmt.Errorf("failed to parse plan from %s: %w", path, err)
	}

	warnIfExternalPlanIncomplete(&plan)

	return &plan, nil
}

// carriesFieldTheLossyConversionDropped reports whether the plan holds any of
// the six fields the pre-#2484 storage conversion destroyed: the dependency
// tree, the verify block, the recipe type, the binary list, the Linux family,
// and the per-step phase.
//
// The conversion dropped all six unconditionally, so any one of them being
// present proves the plan did not come through it. The implication runs one way
// only -- absence proves nothing, because a tool can legitimately have none of
// them -- which is why this suppresses a warning and never raises one.
func carriesFieldTheLossyConversionDropped(plan *executor.InstallationPlan) bool {
	if len(plan.Dependencies) > 0 || plan.Verify != nil || plan.RecipeType != "" ||
		len(plan.Binaries) > 0 || plan.Platform.LinuxFamily != "" {
		return true
	}
	for _, step := range plan.Steps {
		if step.Phase != "" {
			return true
		}
	}
	return false
}

// warnIfExternalPlanIncomplete tells the user when a plan file may have been
// written by the storage conversion that dropped fields.
//
// On this path there is no recipe to fall back on, so a plan missing its
// dependency tree and verify block installs without either and nothing else
// would say so. Both current writers mark a complete plan: plan generation
// stamps the marker, so `tsuku eval` output carries it, and `tsuku plan export`
// writes the stored record, which has carried it since the conversion was
// fixed.
//
// A missing marker is not enough on its own, because it also describes every
// plan file written by a tsuku older than the marker -- including `tsuku eval`
// output, which was never lossy and is a first-class input here. So the marker
// check is paired with a structural one: a file carrying any field the lossy
// conversion destroyed did not come through it, whatever its marker says. All
// 110 golden plans are in that group, as is any old eval file for a tool with
// dependencies or a verify block. What is left -- no marker and none of the six
// fields -- is the population that is either a pre-#2484 export or a plan with
// genuinely nothing to declare, and those two are the pair the marker exists to
// separate. Nothing can tell them apart, so they are warned about together.
//
// It warns rather than refuses. `tsuku plan export` is documented for offline
// and air-gapped installation, and a user holding a file on a disconnected
// machine cannot re-export -- that needs the tool installed under a current
// tsuku, which is the thing they are trying to do. Refusing would turn a
// degraded install into no install for the people least able to recover.
func warnIfExternalPlanIncomplete(plan *executor.InstallationPlan) {
	if plan == nil || plan.StorageVersion >= install.PlanStorageVersion {
		return
	}
	if carriesFieldTheLossyConversionDropped(plan) {
		return
	}
	fmt.Fprintf(os.Stderr, "Warning: this plan file carries no marker saying whether it is complete, and it\n")
	fmt.Fprintf(os.Stderr, "declares no recipe type, no dependencies and no verification. It may have been\n")
	fmt.Fprintf(os.Stderr, "exported before those fields were stored, in which case installing from it skips\n")
	fmt.Fprintf(os.Stderr, "them silently. To produce a plan that says either way:\n")
	fmt.Fprintf(os.Stderr, "  tsuku eval %s\n\n", plan.Tool)
}

// validateExternalPlan performs validation specific to externally-provided plans.
// Structural validation (format version, primitives, checksums, platform) is handled
// by executor.ExecutePlan, so this function only checks external-plan-specific concerns.
func validateExternalPlan(plan *executor.InstallationPlan, toolName string) error {
	// Check tool name if provided on command line (external-plan-specific)
	if toolName != "" && toolName != plan.Tool {
		return fmt.Errorf("plan is for tool '%s', but '%s' was specified",
			plan.Tool, toolName)
	}

	return nil
}

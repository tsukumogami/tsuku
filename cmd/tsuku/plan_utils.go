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

// warnIfExternalPlanIncomplete tells the user when a plan file was written
// before the storage conversion carried every field.
//
// Such a file has no dependency tree, no verify block and no recipe type, and
// on this path there is no recipe to fall back on -- so it installs without its
// dependencies and without verification, and nothing else would say so. Both
// writers mark a complete plan: plan generation stamps the marker, so `tsuku
// eval` output carries it, and `tsuku plan export` writes the stored record,
// which has carried it since the conversion was fixed. An unmarked file is one
// written by an older tsuku.
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
	fmt.Fprintf(os.Stderr, "Warning: this plan file was written before the plan format carried every field.\n")
	fmt.Fprintf(os.Stderr, "It may omit dependencies, verification, and recipe type, and installing from it\n")
	fmt.Fprintf(os.Stderr, "would skip them silently. To produce a current plan:\n")
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

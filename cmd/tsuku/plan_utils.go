package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tsukumogami/tsuku/internal/executor"
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

	return &plan, nil
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

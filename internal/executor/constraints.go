package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tsukumogami/tsuku/internal/actions"
)

// ExtractConstraints parses a golden file (plan JSON) and extracts version constraints.
// The constraints can be used during constrained evaluation to produce deterministic output.
//
// For pip_exec steps, it extracts package versions from locked_requirements.
// Other ecosystems (go, cargo, npm, gem, cpan) will be implemented in subsequent issues.
func ExtractConstraints(planPath string) (*actions.EvalConstraints, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan InstallationPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	return ExtractConstraintsFromPlan(&plan)
}

// ExtractConstraintsFromPlan extracts constraints from an already-parsed plan.
// This is useful when the plan is already loaded in memory.
func ExtractConstraintsFromPlan(plan *InstallationPlan) (*actions.EvalConstraints, error) {
	constraints := &actions.EvalConstraints{
		PipConstraints: make(map[string]string),
	}

	// Extract from main steps
	extractPipConstraintsFromSteps(plan.Steps, constraints)

	// Extract from dependencies
	for _, dep := range plan.Dependencies {
		extractConstraintsFromDependency(&dep, constraints)
	}

	return constraints, nil
}

// extractConstraintsFromDependency extracts constraints from a dependency plan recursively.
func extractConstraintsFromDependency(dep *DependencyPlan, constraints *actions.EvalConstraints) {
	extractPipConstraintsFromSteps(dep.Steps, constraints)

	for _, nestedDep := range dep.Dependencies {
		extractConstraintsFromDependency(&nestedDep, constraints)
	}
}

// extractPipConstraintsFromSteps extracts pip constraints from pip_exec steps.
func extractPipConstraintsFromSteps(steps []ResolvedStep, constraints *actions.EvalConstraints) {
	for _, step := range steps {
		if step.Action != "pip_exec" {
			continue
		}

		lockedReqs, ok := step.Params["locked_requirements"].(string)
		if !ok || lockedReqs == "" {
			continue
		}

		// Parse locked_requirements and add to constraints
		parsed := ParsePipRequirements(lockedReqs)
		for pkg, ver := range parsed {
			constraints.PipConstraints[pkg] = ver
		}
	}
}

// ParsePipRequirements parses a pip requirements string and extracts package versions.
// The format is: "package==version \\\n    --hash=sha256:hash\n"
// Returns a map of normalized package names to versions.
func ParsePipRequirements(requirements string) map[string]string {
	result := make(map[string]string)

	// Match package==version patterns
	// Handles continuation lines with backslash
	// Example: "black==26.1a1 \\\n    --hash=sha256:..."
	pattern := regexp.MustCompile(`(?m)^([a-zA-Z0-9][-a-zA-Z0-9._]*)==([^ \\\n]+)`)
	matches := pattern.FindAllStringSubmatch(requirements, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			pkg := normalizePackageName(match[1])
			ver := match[2]
			result[pkg] = ver
		}
	}

	return result
}

// normalizePackageName normalizes a Python package name to lowercase with hyphens.
// PEP 503: Package names are case-insensitive and underscores/dots are equivalent to hyphens.
func normalizePackageName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return name
}

// HasPipConstraints returns true if the constraints contain pip package versions.
func HasPipConstraints(constraints *actions.EvalConstraints) bool {
	return constraints != nil && len(constraints.PipConstraints) > 0
}

// GetPipConstraint returns the version constraint for a package, if any.
// The package name is normalized before lookup.
func GetPipConstraint(constraints *actions.EvalConstraints, packageName string) (string, bool) {
	if constraints == nil || len(constraints.PipConstraints) == 0 {
		return "", false
	}
	ver, ok := constraints.PipConstraints[normalizePackageName(packageName)]
	return ver, ok
}

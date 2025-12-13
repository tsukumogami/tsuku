package executor

import "github.com/tsukumogami/tsuku/internal/install"

// ToStoragePlan converts an InstallationPlan to install.Plan for storage in state.json.
// This conversion preserves all metadata including checksums, deterministic flags, and platform info.
func ToStoragePlan(plan *InstallationPlan) *install.Plan {
	if plan == nil {
		return nil
	}

	// Convert steps
	steps := make([]install.PlanStep, len(plan.Steps))
	for i, s := range plan.Steps {
		steps[i] = install.PlanStep{
			Action:        s.Action,
			Params:        s.Params,
			Evaluable:     s.Evaluable,
			Deterministic: s.Deterministic,
			URL:           s.URL,
			Checksum:      s.Checksum,
			Size:          s.Size,
		}
	}

	return &install.Plan{
		FormatVersion: plan.FormatVersion,
		Tool:          plan.Tool,
		Version:       plan.Version,
		Platform: install.PlanPlatform{
			OS:   plan.Platform.OS,
			Arch: plan.Platform.Arch,
		},
		GeneratedAt:   plan.GeneratedAt,
		RecipeHash:    plan.RecipeHash,
		RecipeSource:  plan.RecipeSource,
		Deterministic: plan.Deterministic,
		Steps:         steps,
	}
}

// FromStoragePlan converts an install.Plan back to InstallationPlan for execution.
// This enables re-execution of cached plans from state.json.
func FromStoragePlan(plan *install.Plan) *InstallationPlan {
	if plan == nil {
		return nil
	}

	// Convert steps
	steps := make([]ResolvedStep, len(plan.Steps))
	for i, s := range plan.Steps {
		steps[i] = ResolvedStep{
			Action:        s.Action,
			Params:        s.Params,
			Evaluable:     s.Evaluable,
			Deterministic: s.Deterministic,
			URL:           s.URL,
			Checksum:      s.Checksum,
			Size:          s.Size,
		}
	}

	return &InstallationPlan{
		FormatVersion: plan.FormatVersion,
		Tool:          plan.Tool,
		Version:       plan.Version,
		Platform: Platform{
			OS:   plan.Platform.OS,
			Arch: plan.Platform.Arch,
		},
		GeneratedAt:   plan.GeneratedAt,
		RecipeHash:    plan.RecipeHash,
		RecipeSource:  plan.RecipeSource,
		Deterministic: plan.Deterministic,
		Steps:         steps,
	}
}

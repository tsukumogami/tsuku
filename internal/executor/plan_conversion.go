package executor

import "github.com/tsukumogami/tsuku/internal/install"

// ToStoragePlan converts an InstallationPlan to install.Plan for storage in state.json.
//
// The conversion is lossless: every field of InstallationPlan reaches the stored
// record, including the dependency tree, the verify block, and the per-step phases.
// TestPlanConversionCarriesEveryField enforces that, so a field added to the plan
// types and forgotten here fails a test rather than shipping.
func ToStoragePlan(plan *InstallationPlan) *install.Plan {
	if plan == nil {
		return nil
	}

	return &install.Plan{
		StorageVersion: install.PlanStorageVersion,
		FormatVersion:  plan.FormatVersion,
		Tool:           plan.Tool,
		Version:        plan.Version,
		Platform: install.PlanPlatform{
			OS:          plan.Platform.OS,
			Arch:        plan.Platform.Arch,
			LinuxFamily: plan.Platform.LinuxFamily,
		},
		GeneratedAt:   plan.GeneratedAt,
		RecipeSource:  plan.RecipeSource,
		Deterministic: plan.Deterministic,
		Steps:         stepsToStorage(plan.Steps),
		Dependencies:  depsToStorage(plan.Dependencies),
		Verify:        verifyToStorage(plan.Verify),
		RecipeType:    plan.RecipeType,
		Binaries:      plan.Binaries,
	}
}

// FromStoragePlan converts an install.Plan back to InstallationPlan for execution.
// This enables re-execution of cached plans from state.json.
//
// StorageVersion is deliberately not consulted here. What a record written by an
// older conversion means is a question for the code reading it -- the plan cache
// regenerates, plan show and plan export warn -- not for a converter with nowhere
// to put the answer.
func FromStoragePlan(plan *install.Plan) *InstallationPlan {
	if plan == nil {
		return nil
	}

	return &InstallationPlan{
		FormatVersion: plan.FormatVersion,
		Tool:          plan.Tool,
		Version:       plan.Version,
		Platform: Platform{
			OS:          plan.Platform.OS,
			Arch:        plan.Platform.Arch,
			LinuxFamily: plan.Platform.LinuxFamily,
		},
		GeneratedAt:   plan.GeneratedAt,
		RecipeSource:  plan.RecipeSource,
		Deterministic: plan.Deterministic,
		Steps:         stepsFromStorage(plan.Steps),
		Dependencies:  depsFromStorage(plan.Dependencies),
		Verify:        verifyFromStorage(plan.Verify),
		RecipeType:    plan.RecipeType,
		Binaries:      plan.Binaries,
	}
}

func stepsToStorage(steps []ResolvedStep) []install.PlanStep {
	if steps == nil {
		return nil
	}
	out := make([]install.PlanStep, len(steps))
	for i, s := range steps {
		out[i] = install.PlanStep{
			Action:        s.Action,
			Phase:         s.Phase,
			Params:        s.Params,
			Evaluable:     s.Evaluable,
			Deterministic: s.Deterministic,
			URL:           s.URL,
			Checksum:      s.Checksum,
			Size:          s.Size,
		}
	}
	return out
}

func stepsFromStorage(steps []install.PlanStep) []ResolvedStep {
	if steps == nil {
		return nil
	}
	out := make([]ResolvedStep, len(steps))
	for i, s := range steps {
		out[i] = ResolvedStep{
			Action:        s.Action,
			Phase:         s.Phase,
			Params:        s.Params,
			Evaluable:     s.Evaluable,
			Deterministic: s.Deterministic,
			URL:           s.URL,
			Checksum:      s.Checksum,
			Size:          s.Size,
		}
	}
	return out
}

// depsToStorage converts the dependency tree, recursing to whatever depth the
// plan carries.
func depsToStorage(deps []DependencyPlan) []install.PlanDependency {
	if deps == nil {
		return nil
	}
	out := make([]install.PlanDependency, len(deps))
	for i, d := range deps {
		out[i] = install.PlanDependency{
			Tool:         d.Tool,
			Version:      d.Version,
			Dependencies: depsToStorage(d.Dependencies),
			Steps:        stepsToStorage(d.Steps),
			Verify:       verifyToStorage(d.Verify),
			RecipeType:   d.RecipeType,
		}
	}
	return out
}

func depsFromStorage(deps []install.PlanDependency) []DependencyPlan {
	if deps == nil {
		return nil
	}
	out := make([]DependencyPlan, len(deps))
	for i, d := range deps {
		out[i] = DependencyPlan{
			Tool:         d.Tool,
			Version:      d.Version,
			Dependencies: depsFromStorage(d.Dependencies),
			Steps:        stepsFromStorage(d.Steps),
			Verify:       verifyFromStorage(d.Verify),
			RecipeType:   d.RecipeType,
		}
	}
	return out
}

func verifyToStorage(v *PlanVerify) *install.PlanVerify {
	if v == nil {
		return nil
	}
	out := &install.PlanVerify{
		Command:  v.Command,
		Pattern:  v.Pattern,
		Patterns: v.Patterns,
		ExitCode: v.ExitCode,
	}
	if v.Additional != nil {
		out.Additional = make([]install.PlanAdditionalVerify, len(v.Additional))
		for i, a := range v.Additional {
			out.Additional[i] = install.PlanAdditionalVerify{Command: a.Command, Pattern: a.Pattern}
		}
	}
	return out
}

func verifyFromStorage(v *install.PlanVerify) *PlanVerify {
	if v == nil {
		return nil
	}
	out := &PlanVerify{
		Command:  v.Command,
		Pattern:  v.Pattern,
		Patterns: v.Patterns,
		ExitCode: v.ExitCode,
	}
	if v.Additional != nil {
		out.Additional = make([]PlanAdditionalVerify, len(v.Additional))
		for i, a := range v.Additional {
			out.Additional[i] = PlanAdditionalVerify{Command: a.Command, Pattern: a.Pattern}
		}
	}
	return out
}

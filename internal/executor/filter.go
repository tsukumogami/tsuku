package executor

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// FilterStepsByTarget filters recipe steps based on a target platform and linux family.
// Returns only steps that match the target according to two-stage filtering:
//
// Stage 1: Check action's implicit constraint (if any) against target.
// Stage 2: Check step's explicit when clause (if any) against target platform.
//
// A step is included only if both stages pass. Actions without implicit constraints
// (non-SystemAction or SystemAction returning nil) pass stage 1 automatically.
// Steps without explicit when clauses pass stage 2 automatically.
func FilterStepsByTarget(steps []recipe.Step, target platform.Target) []recipe.Step {
	var result []recipe.Step
	for _, step := range steps {
		if !stepMatchesTarget(step, target) {
			continue
		}
		result = append(result, step)
	}
	return result
}

// stepMatchesTarget checks if a single step matches the given target.
// Returns true if both the action's implicit constraint and the step's
// explicit when clause are satisfied.
func stepMatchesTarget(step recipe.Step, target platform.Target) bool {
	// Stage 1: Check action's implicit constraint
	action := actions.Get(step.Action)
	if sysAction, ok := action.(actions.SystemAction); ok {
		if constraint := sysAction.ImplicitConstraint(); constraint != nil {
			if !constraint.MatchesTarget(target) {
				return false
			}
		}
	}
	// Actions without implicit constraint (non-SystemAction) pass stage 1

	// Stage 2: Check explicit when clause
	if step.When != nil && !step.When.Matches(target) {
		return false
	}
	// Steps without when clause pass stage 2

	return true
}

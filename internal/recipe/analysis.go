package recipe

import "fmt"

// ConstraintLookup returns the implicit constraint for an action by name.
// Returns nil if the action has no implicit constraint (runs anywhere).
// Returns (nil, false) if the action is unknown (validation error).
type ConstraintLookup func(actionName string) (constraint *Constraint, known bool)

// ComputeAnalysis computes a step's effective constraint and family variation status.
// It combines three sources of information:
//  1. Implicit constraint from the action type (via lookup)
//  2. Explicit constraint from the step's when clause
//  3. Interpolation detection for {{linux_family}} in step parameters
//
// Returns error if:
//   - Action is unknown (lookup returns known=false)
//   - Constraint conflicts detected (via MergeWhenClause)
func ComputeAnalysis(action string, when *WhenClause, params map[string]interface{},
	lookup ConstraintLookup) (*StepAnalysis, error) {

	// Validate action exists and get its implicit constraint
	implicit, known := lookup(action)
	if !known {
		return nil, fmt.Errorf("unknown action %q", action)
	}

	// Start with implicit constraint (may be nil for unconstrained actions)
	var constraint *Constraint
	if implicit != nil {
		constraint = implicit.Clone()
	}

	// Merge with explicit when clause (validates conflicts)
	if when != nil {
		merged, err := MergeWhenClause(constraint, when)
		if err != nil {
			return nil, err
		}
		constraint = merged
	}

	// Detect interpolated variables in params
	vars := detectInterpolatedVars(params)
	familyVarying := vars["linux_family"]

	return &StepAnalysis{
		Constraint:    constraint,
		FamilyVarying: familyVarying,
	}, nil
}

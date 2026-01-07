## Goal

Introduce platform constraint types (`Constraint`, `StepAnalysis`) and interpolation detection (`detectInterpolatedVars`) as the foundation for Linux family-aware golden file support.

## Context

This issue implements the core type system from [DESIGN-golden-family-support.md](../docs/DESIGN-golden-family-support.md) Phase 1. These types enable step-level platform constraint representation and variation detection, which downstream issues will use to extend `WhenClause` and compute step analysis.

The `Constraint` type answers "where can this step run?" while `StepAnalysis` combines constraint with variation detection (whether output differs by family). The `detectInterpolatedVars` function scans step parameters for `{{variable}}` patterns that cause platform-varying output.

## Acceptance Criteria

### Constraint Type

Add to `internal/recipe/types.go`:

```go
// Constraint represents platform requirements for a step.
// Answers: "where can this step run?"
// nil constraint means unconstrained (runs anywhere).
type Constraint struct {
    OS          string // e.g., "linux", "darwin", or empty (any)
    Arch        string // e.g., "amd64", "arm64", or empty (any)
    LinuxFamily string // e.g., "debian", or empty (any linux)
}

// Clone returns a copy of the constraint, or an empty constraint if c is nil.
// Nil-safe: can be called on a nil receiver (idiomatic Go pattern).
func (c *Constraint) Clone() *Constraint

// Validate returns an error if the constraint contains invalid combinations.
// Invalid state: LinuxFamily set when OS is not "linux" (or empty).
func (c *Constraint) Validate() error
```

### StepAnalysis Type

Add to `internal/recipe/types.go`:

```go
// StepAnalysis combines constraint with variation detection.
// Stored on Step after construction (pre-computed at load time).
type StepAnalysis struct {
    Constraint    *Constraint // nil means unconstrained (runs anywhere)
    FamilyVarying bool        // true if step uses {{linux_family}} interpolation
}
```

### Interpolation Detection

Add to `internal/recipe/types.go`:

```go
// Known interpolation variables that affect platform variance
var knownVars = []string{"linux_family", "os", "arch"}

// detectInterpolatedVars scans for {{var}} patterns in any string value.
// Returns a map of variable names found (e.g., {"linux_family": true}).
// Generalized to support future variables like {{arch}}.
func detectInterpolatedVars(v interface{}) map[string]bool
```

The function must:
- Recursively scan `string`, `map[string]interface{}`, and `[]interface{}` values
- Detect `{{varname}}` patterns for all variables in `knownVars`
- Return a map with keys for each detected variable

### Behavior

1. `Constraint.Clone()`:
   - Returns `&Constraint{}` when receiver is nil
   - Returns a new `Constraint` with copied field values otherwise

2. `Constraint.Validate()`:
   - Returns `nil` for nil receiver
   - Returns `nil` for empty constraint
   - Returns `nil` when `LinuxFamily` is set and `OS` is empty (family implies linux)
   - Returns `nil` when `LinuxFamily` is set and `OS` is "linux"
   - Returns error when `LinuxFamily` is set and `OS` is not "linux" (e.g., darwin+debian is invalid)

3. `detectInterpolatedVars()`:
   - Returns empty map for nil input
   - Returns empty map for values without interpolation
   - Detects `{{linux_family}}`, `{{os}}`, `{{arch}}` in strings
   - Recurses into nested maps and slices
   - Does not detect unknown variables (only those in `knownVars`)

## Dependencies

None - this is the first issue in the implementation sequence.

## Downstream Dependencies

The following issues depend on this implementation:

- **Issue #823** (feat(recipe): extend WhenClause with family and arch constraints): Uses `Constraint` type for merge semantics
- **Issue #824** (feat(recipe): add step analysis computation logic): Uses `StepAnalysis`, `Constraint`, and `detectInterpolatedVars`

---
status: Current
problem: Recipe steps cannot express OR conditions across multiple platform tuples without duplicating entire steps.
decision: Implement a structured WhenClause type to replace map[string]string, supporting platform arrays with proper type safety.
rationale: With only 2 recipes currently using when clauses, the breaking change is minimal. The structured type provides type safety, clean APIs, and a strong foundation for future filtering conditions like environment variables.
---

# Platform Tuple Support in `when` Clauses

## Status

Current

## Context and Problem Statement

Currently, step-level `when` clauses support independent OS and architecture filtering via `when.os` and `when.arch` fields. This was sufficient for early use cases, but creates precision gaps when recipes need to express "only execute on specific OS/architecture combinations."

### Current Limitation

The existing `when` clause correctly supports single platform filtering via AND logic:

```toml
[[steps]]
action = "some_action"
when = { os = "darwin", arch = "arm64" }
# Executes on: darwin/arm64 only ✓
```

The `shouldExecuteForPlatform()` function in `internal/executor/plan_generator.go:211-234` correctly uses AND logic - if both `os` and `arch` are specified, both must match.

**The real limitation:** Cannot express OR conditions across multiple platform tuples without duplicating entire steps:

```toml
# Current: Must duplicate the step
[[steps]]
action = "apply_patch"
file = "fix-m1.patch"
when = { os = "darwin", arch = "arm64" }

[[steps]]
action = "apply_patch"
file = "fix-m1.patch"  # Same patch, duplicated step
when = { os = "linux", arch = "amd64" }

# Desired: Express as single step with OR logic
[[steps]]
action = "apply_patch"
file = "fix-m1.patch"
when = { platform = ["darwin/arm64", "linux/amd64"] }
```

### Real-World Use Case

Some build steps are platform-specific at the tuple level:

```toml
# Desired: Apply this patch only on Apple Silicon Macs
[[steps]]
action = "apply_patch"
when = { platform = ["darwin/arm64"] }

# Desired: Run different configure flags for each platform
[[steps]]
action = "run_command"
command = "./configure --enable-optimizations"
when = { platform = ["linux/amd64", "linux/arm64"] }

[[steps]]
action = "run_command"
command = "./configure --enable-optimizations --with-universal-archs=universal2"
when = { platform = ["darwin/arm64", "darwin/amd64"] }
```

### Why This Matters Now

1. **Consistency with install_guide**: PR #689 added platform tuple support to `install_guide` with hierarchical fallback. The `when` clause should support the same precision for consistency.

2. **Conditional build steps**: As recipes grow more sophisticated (source builds with platform-specific patches, conditional dependencies), the need for tuple-level precision increases.

3. **Documentation exists**: Issue #686 originally proposed both features together. Users may expect `when` clause tuple support based on the merged install_guide documentation.

### Scope

**In scope:**
- Platform tuple format in `when.platform` field (e.g., `["darwin/arm64", "linux/amd64"]`)
- Update `shouldExecuteForPlatform()` to handle platform tuples
- Validation that `when.platform` values exist in recipe's supported platforms
- Documentation for `when` clause usage
- Test coverage for platform tuple matching logic

**Out of scope:**
- Changes to `install_guide` (already implemented in #689)
- Changes to recipe-level platform constraints (`supported_os`, `supported_arch`)
- Hierarchical fallback (not applicable to when clauses - they're exact match filters)
- Runtime package manager detection (already supported via `when.package_manager`)

## Decision Drivers

- **Consistency**: Match the platform tuple format used in `install_guide` (`os/arch`)
- **Backwards compatibility**: Existing `when.os` and `when.arch` fields must continue to work
- **Validation**: Catch invalid platform tuples at recipe load time, not execution time
- **Clarity**: The TOML syntax should be obvious and self-documenting
- **Simplicity**: Avoid introducing complex boolean logic or hierarchical fallback (when is a filter, not a lookup)

## Upstream Design Reference

This design implements the `when` clause portion of platform tuple support originally scoped in issue #686. The `install_guide` portion was implemented in PR #689 (docs/platform-tuple-support.md).

**Relevant upstream context:**
- Platform tuple format: `os/arch` (e.g., "darwin/arm64", "linux/amd64")
- TOML syntax: Slash-containing keys require quoting
- Validation approach: Check against `Recipe.GetSupportedPlatforms()`
- Implementation pattern: Extend existing logic rather than replace

## Implementation Context

### Existing Patterns from PR #689

**Platform tuple validation** (internal/recipe/platform.go:267-382):
- `GetSupportedPlatforms()` returns all supported platform tuples after applying constraints
- Validation splits tuple keys by `/` to detect tuple vs OS-only format
- Uses `containsString()` helper for membership checks

**TOML unmarshaling** (internal/recipe/types.go:186-206):
- Custom `UnmarshalTOML()` on Step struct handles `when` field
- `when` is stored as `map[string]string` (not `map[string]interface{}`)
- Current implementation only handles string values, not arrays

**Runtime filtering** (internal/executor/plan_generator.go:210-234):
- `shouldExecuteForPlatform()` checks when conditions at plan generation time
- Uses exact string match for `when["os"]` and `when["arch"]`
- Empty when map means "execute on all platforms"

### Conventions to Follow

1. **Validate at load time**: Follow the pattern in `ValidateStepsAgainstPlatforms()`
   - Add validation for `when.platform` array elements
   - Check against `Recipe.GetSupportedPlatforms()`

2. **Use shared helpers**: Reuse `containsString()` for platform membership checks

3. **TOML parsing**: Extend `Step.UnmarshalTOML()` to handle array values
   - Current: `when` maps string keys to string values
   - New: Support `when["platform"]` as string containing comma-separated values or actual array

4. **Test coverage**: Follow testing patterns from `platform_test.go` and `plan_generator_test.go`
   - Unit tests for validation logic
   - Unit tests for filtering logic
   - Edge cases: empty array, invalid tuples, mixed formats

### Anti-patterns to Avoid

- Don't introduce hierarchical fallback for when clauses (that's for install_guide only)
- Don't silently skip invalid platforms - fail validation at load time
- Don't change the `Step.When` type signature if possible (breaks existing code)

## Considered Options

### Option 1: Add `when.platform` as Array Field

Introduce a new `platform` key in the `when` map that accepts an array of platform tuples:

```toml
[[steps]]
action = "apply_patch"
when = { platform = ["darwin/arm64", "linux/amd64"] }
```

**Implementation:**
- Extend `Step.UnmarshalTOML()` to handle `when["platform"]` as array of strings
- Modify `shouldExecuteForPlatform()` to check if current platform is in the array
- Add validation in `ValidateStepsAgainstPlatforms()` for platform array elements
- Store platform array as comma-separated string in `Step.When["platform"]` to preserve map[string]string type

**Pros:**
- **Clean TOML syntax**: Array format is self-documenting and matches user expectations
- **Backwards compatible**: Existing `when.os` and `when.arch` fields unchanged
- **Consistent with install_guide**: Uses same `os/arch` tuple format
- **Easy to validate**: Can check each tuple against `Recipe.GetSupportedPlatforms()`
- **No type signature change**: Internal storage as CSV string preserves existing code

**Cons:**
- **CSV string storage is a hack**: Storing array as comma-separated string is inelegant
- **TOML parsing complexity**: Need special handling for array → string conversion
- **Potential ambiguity**: What if a platform tuple contained a comma? (unlikely but possible)

### Option 2: Replace when with Structured Type

Change `Step.When` from `map[string]string` to a structured type:

```go
type WhenClause struct {
    OS             string
    Arch           string
    Platform       []string
    PackageManager string
}
```

```toml
[[steps]]
action = "apply_patch"
when = { platform = ["darwin/arm64"] }
```

**Implementation:**
- Define new `WhenClause` struct with proper types
- Update all code that reads `Step.When` to use struct fields
- Validation is type-safe (arrays are arrays)

**Pros:**
- **Type safety**: No CSV hacks, arrays are actual arrays
- **Clean implementation**: Proper Go idioms, no workarounds
- **Future extensibility**: Easy to add new when conditions as fields

**Cons:**
- **Breaking change**: All code reading `Step.When` must be updated
- **Broader scope**: Affects many parts of codebase beyond platform tuples
- **Migration risk**: Harder to validate that all usages are updated correctly
- **Not minimal**: Solves more than the immediate problem

### Option 3: Separate `when_platform` Field on Step

Add a dedicated `when_platform` array field at the Step level (sibling to `when`):

```toml
[[steps]]
action = "apply_patch"
when_platform = ["darwin/arm64", "linux/amd64"]
```

**Implementation:**
- Add `WhenPlatform []string` field to `Step` struct
- Extend `shouldExecuteForPlatform()` to check both `when` and `WhenPlatform`
- Validation checks `WhenPlatform` array elements

**Pros:**
- **No CSV hack**: Arrays stored as actual arrays
- **Minimal breaking changes**: `Step.When` type unchanged
- **Clear semantics**: Separate field makes intent obvious

**Cons:**
- **Inconsistent with existing pattern**: Other when conditions use the `when` map
- **Two ways to filter**: Having both `when = { os = "..." }` and `when_platform = [...]` is confusing
- **Potential conflicts**: What if both `when.os` and `when_platform` are specified?

### Evaluation Against Decision Drivers

| Option | Consistency | Backwards Compat | Validation | Clarity | Simplicity |
|--------|-------------|------------------|------------|---------|------------|
| Option 1 (when.platform array) | Good (matches install_guide format) | Excellent (no breaking changes) | Good (validate at load time) | Good (array is clear) | Fair (CSV hack is inelegant) |
| Option 2 (structured type) | Excellent (proper types) | Poor (breaking change) | Excellent (type-safe) | Excellent (clean code) | Poor (broader scope) |
| Option 3 (when_platform field) | Fair (new pattern) | Good (minimal changes) | Good (validate arrays) | Fair (two filter mechanisms) | Good (straightforward) |

### Current Usage

Examining the recipe registry reveals minimal `when` clause usage:
- **2 recipes total**: gcc-libs.toml and nodejs.toml
- **Usage pattern**: Simple OS-only filters (`when = { os = "linux" }`)
- **No complex conditions**: No arch filters, no package_manager filters
- **Migration cost**: Converting 2 recipes is trivial

This low usage makes breaking changes significantly more attractive than initially assumed.

## Decision Outcome

**Chosen: Option 2 (Structured WhenClause Type)**

### Rationale

With only 2 recipes currently using `when` clauses, the breaking change concern for Option 2 is minimal. The structured type approach provides:

1. **Type safety**: Compile-time guarantees prevent bugs from malformed when clauses
2. **Clean foundation**: No CSV hacks or parsing workarounds
3. **Future extensibility**: Adding new filter types (env vars, runtime checks) is straightforward
4. **Better developer experience**: IDE autocomplete, clear API surface

The migration cost is acceptable:
- Convert 2 recipes (gcc-libs.toml, nodejs.toml) from `[steps.when]` table to new format
- Update recipe validation to support transition period (accept both formats with deprecation warning)
- Provide clear migration guide in release notes

### Implementation Summary

**New type definition:**
```go
type WhenClause struct {
    Platform       []string // Platform tuples: ["darwin/arm64", "linux/amd64"]
    OS             []string // OS-only: ["darwin", "linux"] (any arch)
    PackageManager string   // Runtime check (brew, apt, etc.)
}
```

**TOML syntax:**
```toml
# Precise platform targeting
[[steps]]
action = "apply_patch"
when = { platform = ["darwin/arm64", "linux/amd64"] }

# OS-level targeting (any arch)
[[steps]]
action = "install_deps"
when = { os = ["linux"] }
```

**Matching semantics:**
- **Between steps (additive):** Each step's `when` evaluates independently; all matching steps execute
- **Within a clause (OR logic):** `platform = ["darwin/arm64", "linux/amd64"]` matches if current platform is in the array
- **Between fields (validation error):** Cannot specify both `platform` and `os` in same clause (ambiguous semantics)

**Empty array semantics:**
- `platform = []` or `os = []` means "match no platforms" (step never executes)
- Missing `when` field means "match all platforms"

**Migration:**
- 2 existing recipes (gcc-libs.toml, nodejs.toml) use `when = { os = "linux" }` (single string)
- Will be updated to `when = { os = ["linux"] }` (array syntax) in this PR
- No backwards compatibility needed (all recipes in this repo)

### Consequences

**Positive:**
- Clean, maintainable codebase without technical debt
- Type-safe APIs prevent entire class of bugs
- Additive matching semantics align with ecosystem precedents (Cargo, Homebrew)
- Symmetric with install_guide (platform tuples + OS-only)
- Future when clause enhancements (environment variables, runtime conditions) have clear extension path
- Aligns with Go idioms (structured types over string maps)

**Negative:**
- Breaking change (2 recipes need migration: gcc-libs.toml, nodejs.toml)
- Change from single string to array: `os = "linux"` → `os = ["linux"]`

**Mitigation:**
- Migrate 2 recipes in same PR (trivial change)
- No external ecosystem to worry about (all recipes in this repo)

## Solution Architecture

### Overview

The solution introduces a structured `WhenClause` type to replace the current `map[string]string` representation of step filtering conditions. This change touches three layers:

1. **Data layer**: `Step.When` type changes from `map[string]string` to `*WhenClause`
2. **Unmarshaling layer**: TOML parsing in `Step.UnmarshalTOML()` constructs `WhenClause` from TOML input
3. **Execution layer**: `shouldExecuteForPlatform()` checks `WhenClause.Platform` array for matches

### Components

**Modified files:**
```
internal/recipe/types.go
  ├─ WhenClause struct (new)
  ├─ Step.When type change (map[string]string → *WhenClause)
  └─ Step.UnmarshalTOML() updates

internal/executor/plan_generator.go
  └─ shouldExecuteForPlatform() signature change

internal/recipe/platform.go
  └─ ValidateStepsAgainstPlatforms() add when.platform validation

internal/recipe/platform_test.go
  └─ Add test cases for when.platform validation

internal/executor/plan_generator_test.go
  └─ Update shouldExecuteForPlatform tests
```

### Key Interfaces

**WhenClause struct:**
```go
// WhenClause represents platform and runtime filtering conditions for a step
type WhenClause struct {
	// Platform specifies platform tuples where the step should execute
	// Example: ["darwin/arm64", "linux/amd64"]
	// Mutually exclusive with OS field
	Platform []string `toml:"platform,omitempty"`

	// OS specifies operating systems where the step should execute (any arch)
	// Example: ["darwin", "linux"]
	// Mutually exclusive with Platform field
	// Symmetric with install_guide OS-only keys
	OS []string `toml:"os,omitempty"`

	// PackageManager specifies required package manager (brew, apt, etc.)
	// This is a runtime check, not validated against supported platforms
	PackageManager string `toml:"package_manager,omitempty"`
}

// IsEmpty returns true if no filtering conditions are set
func (w *WhenClause) IsEmpty() bool {
	return w == nil ||
		(len(w.Platform) == 0 && len(w.OS) == 0 && w.PackageManager == "")
}

// Matches returns true if the clause matches the given platform
func (w *WhenClause) Matches(os, arch string) bool {
	if w.IsEmpty() {
		return true // No conditions = match all platforms
	}

	// Check platform tuples (exact match required)
	if len(w.Platform) > 0 {
		tuple := fmt.Sprintf("%s/%s", os, arch)
		for _, p := range w.Platform {
			if p == tuple {
				return true
			}
		}
		return false // Platform specified but didn't match
	}

	// Check OS-only (any arch on this OS)
	if len(w.OS) > 0 {
		for _, o := range w.OS {
			if o == os {
				return true
			}
		}
		return false // OS specified but didn't match
	}

	// No platform/OS filtering, only package_manager (not evaluated here)
	return true
}
```

**Step struct change:**
```go
type Step struct {
	Action      string
	When        *WhenClause               // Changed from map[string]string
	Note        string
	Description string
	Params      map[string]interface{}
}
```

**shouldExecuteForPlatform signature:**
```go
// Before
func shouldExecuteForPlatform(when map[string]string, targetOS, targetArch string) bool

// After
func shouldExecuteForPlatform(when *WhenClause, targetOS, targetArch string) bool {
	return when.Matches(targetOS, targetArch)
}
```

### Data Flow

**1. Recipe loading (TOML → Go struct):**
```
Recipe TOML file
  └─> BurntSushi/toml parser
       └─> Step.UnmarshalTOML()
            ├─ Extract "when" map from TOML
            ├─ Check for platform, os, arch, package_manager keys
            ├─ Validate mutual exclusivity (platform vs os/arch)
            └─ Construct WhenClause struct
                 └─> Step.When = &WhenClause{...}
```

**2. Validation (load time):**
```
ValidateStepsAgainstPlatforms()
  └─> For each step with when clause:
       ├─ Check mutual exclusivity (error if both Platform and OS present)
       ├─ If when.Platform:
       │   └─> For each tuple:
       │        ├─ Check format (contains "/")
       │        └─ Validate against Recipe.GetSupportedPlatforms()
       └─ If when.OS:
            └─> For each os:
                 └─ Validate against supported OS set
```

**3. Plan generation (runtime filtering):**
```
GeneratePlan()
  └─> For each step in recipe.Steps:
       ├─ shouldExecuteForPlatform(step.When, targetOS, targetArch)
       │   └─> step.When.Matches(targetOS, targetArch)
       │        ├─ If Platform non-empty: check if current tuple in array
       │        ├─ If OS non-empty: check if current os in array
       │        └─ If empty: match all platforms
       └─ If matches: include in plan

Note: All matching steps execute (additive semantics)
```

## Implementation Approach

### Phase 1: Core Type Definition

**Deliverables:**
- Define `WhenClause` struct in `internal/recipe/types.go`
- Implement `IsEmpty()` and `Matches()` methods
- Add unit tests for `Matches()` logic

**Dependencies:** None

**Test coverage:**
- `Matches()` with platform tuples (exact match, no match, first/last in array)
- `Matches()` with OS arrays (match any arch, no match)
- `Matches()` with empty clause (match all)
- `Matches()` with empty platform/OS arrays (match none - critical edge case)
- `Matches()` with package_manager only (should match all platforms)

### Phase 2: TOML Unmarshaling

**Deliverables:**
- Update `Step.When` field type from `map[string]string` to `*WhenClause`
- Modify `Step.UnmarshalTOML()` to construct `WhenClause` from TOML
- Add validation for mutually exclusive fields (platform vs os/arch)
- Add deprecation warnings for legacy OS/Arch fields

**Dependencies:** Phase 1

**Error cases:**
- Both `platform` and `os` specified → validation error (mutually exclusive)
- Invalid platform tuple format (no `/`) → validation error
- Array type mismatch (TOML provides non-string array elements) → unmarshaling error

### Phase 3: Execution Layer Updates

**Deliverables:**
- Change `shouldExecuteForPlatform()` signature to accept `*WhenClause`
- Update all call sites in `plan_generator.go`
- Update existing tests in `plan_generator_test.go`

**Dependencies:** Phase 2

**Test updates:**
- Update `TestShouldExecuteForPlatform` to use `WhenClause` structs
- Add new test cases for platform tuple filtering

### Phase 4: Validation Layer

**Deliverables:**
- Extend `ValidateStepsAgainstPlatforms()` to validate `when.Platform` and `when.OS` arrays
- Add mutual exclusivity check (error if both platform and os specified)
- Add test coverage in `platform_test.go`

**Dependencies:** Phase 3

**Validation rules:**
- Platform tuples must exist in `Recipe.GetSupportedPlatforms()`
- OS values must exist in supported OS set
- Platform and OS are mutually exclusive (validation error if both present)
- Empty arrays are valid (explicit "match nothing")

### Phase 5: Recipe Migration

**Deliverables:**
- Update gcc-libs.toml: `when = { os = "linux" }` → `when = { os = ["linux"] }`
- Update nodejs.toml: `when = { os = "linux" }` → `when = { os = ["linux"] }`
- Verify all recipe validation passes

**Dependencies:** Phase 4

**Migration changes:**
- Only syntax change: single string → array syntax
- Semantics unchanged: still matches linux on any arch

### Phase 5.5: Integration Testing

**Deliverables:**
- End-to-end test: TOML → Load → Validate → GeneratePlan → Verify correct steps included
- Test with platform tuples, legacy OS/Arch, and mixed scenarios
- Verify deprecation warnings appear for legacy usage
- Test error messages for validation failures

**Dependencies:** Phase 5

**Test scenarios:**
- Recipe with `when = { platform = ["darwin/arm64"] }` executed on darwin/arm64 (match) and linux/amd64 (no match)
- Recipe with `when = { os = ["linux"] }` executed on linux/amd64 and linux/arm64 (both match)
- Recipe with both `platform` and `os` fields (validation error)
- Recipe with invalid tuple format `darwin-arm64` (validation error)
- Multiple steps with overlapping when clauses (verify additive behavior)

### Phase 6: Documentation

**Deliverables:**
- Update `docs/platform-tuple-support.md` with `when` clause examples
- Add migration guide for recipe authors
- Update GUIDE-actions-and-primitives.md with when clause usage

**Dependencies:** Phase 5.5

## Security Considerations

### Download Verification

**Not applicable** - This feature does not download external artifacts. Platform tuple support in `when` clauses is purely a recipe filtering mechanism that determines which steps execute for a given platform. No network operations, no downloads.

### Execution Isolation

**Impact: Low** - This feature affects step filtering logic but does not change execution isolation.

**Analysis:**
- The `when` clause determines which steps are included in the execution plan
- Maliciously crafted `when` conditions cannot bypass execution isolation
- Platform filtering happens at plan generation time (before execution)
- No new privileges or file system access required

**Risks:**
1. **Incorrect platform matching could skip security-critical steps**: If validation logic has bugs, a step might not execute on the intended platform (e.g., a security patch skipped on darwin/arm64).

**Mitigations:**
- Comprehensive unit tests for `Matches()` logic (Phase 1)
- Validation at load time catches invalid platform tuples (Phase 4)
- Existing `ValidateStepsAgainstPlatforms()` ensures all supported platforms are covered for critical steps like `require_system`

**Residual risk:** Recipe authors could intentionally exclude platforms from security-sensitive steps, but this is not a new risk (already possible with OS/Arch filtering).

### Supply Chain Risks

**Not applicable** - This feature does not introduce new supply chain vectors. Recipe TOML files are already the trust boundary; platform tuple syntax is just a new way to express existing filtering logic.

**Existing supply chain model:**
- Recipes come from the tsuku registry (curated) or user-provided paths
- Recipe validation happens at load time
- No change to trust model or provenance verification

### User Data Exposure

**Not applicable** - This feature does not access or transmit user data.

**Analysis:**
- Platform tuple matching uses only `runtime.GOOS` and `runtime.GOARCH` (non-sensitive, already used)
- No new data collection, logging, or telemetry
- Deprecation warnings for legacy OS/Arch fields are logged locally (no external transmission)

### Additional Considerations

**Recipe validation bypass:**
- **Risk**: Malformed TOML could bypass validation if UnmarshalTOML() has bugs
- **Mitigation**: BurntSushi/toml is a well-tested library; custom unmarshaling is minimal and tested
- **Residual**: Type safety from structured `WhenClause` reduces unmarshaling bugs vs. map[string]string

**Denial of service:**
- **Risk**: Extremely large platform arrays could slow validation
- **Mitigation**: Validation only checks against `Recipe.GetSupportedPlatforms()` (small set: 4 tuples max currently)
- **Residual**: Negligible - platform list is bounded by supported_os × supported_arch

### Summary

Platform tuple support in `when` clauses introduces **minimal security risk**. The feature is purely deterministic filtering logic with no external I/O, no new privileges, and no user data access. The primary security consideration is correctness of platform matching (to avoid skipping intended steps), which is mitigated through comprehensive testing and load-time validation.


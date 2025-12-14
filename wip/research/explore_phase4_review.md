# Design Review: Centralize Validation Logic

## Executive Summary

This review assesses the problem statement and options analysis for centralizing validation logic in tsuku. Overall, the design document presents a **well-scoped problem with fair options analysis**, though there are areas for improvement in specificity, missing alternatives, and unstated assumptions.

**Key Findings:**
1. Problem statement is mostly specific but could better define success criteria
2. Missing a hybrid option that combines static maps with structured metadata
3. Pros/cons are generally fair but understate migration complexity for Option 1B
4. Several unstated assumptions about network requirements need to be made explicit
5. No options appear to be strawmen - all are legitimate approaches

## 1. Problem Statement Specificity

### Strengths

The problem statement excels at:

1. **Concrete examples**: Provides actual code snippets (detectRequiredBuildTools switch statement)
2. **Current architecture diagram**: Shows the decision tree builders use
3. **Enumerated problems**: Five specific issues numbered and explained
4. **Clear scope boundaries**: Explicitly states what's in and out of scope

### Weaknesses

**Missing success criteria**: The problem statement doesn't define what "centralized" means operationally. Consider adding:

```
Success looks like:
- A user can run `tsuku validate recipe.toml` without builder context
- Validation requirements are determinable from recipe/plan alone
- Adding a new action requires updating only one location for build tools
- Network requirements are explicit in plan or derivable from action metadata
```

**Vague "derived from recipe/plan"**: The statement "requirements should be derivable from recipe/plan content alone" appears in Decision Drivers but isn't clarified. Does this mean:
- At plan generation time (acceptable)?
- By static analysis of recipe TOML (more restrictive)?
- By introspecting a pre-generated plan (most flexible)?

**Recommendation**: Add a "Evaluation Criteria" section defining what makes a solution successful.

## 2. Missing Alternatives

### Option 1: Action Metadata Storage

The design presents three options but misses a compelling fourth:

**Option 1D: Structured Metadata Registry** (MISSING)

Combine the extensibility of static maps with structured organization:

```go
type ActionMetadata struct {
    Network    bool
    BuildTools []string
    // Future: Runtime ResourceHints, etc.
}

var ActionMetadata = map[string]ActionMetadata{
    "configure_make": {
        Network:    false,
        BuildTools: []string{"autoconf", "automake", "libtool", "pkg-config"},
    },
    "cargo_build": {
        Network:    true,
        BuildTools: []string{"curl"},
    },
}
```

**Why this matters:**
- Follows the existing ActionDependencies pattern (consistency)
- Consolidates network + build tools in one place (vs separate maps in Option 1A)
- Easier to validate completeness (single map to check)
- Simpler to extend (add fields to struct vs new maps)
- No interface changes (unlike Option 1B)

This is a meaningful middle ground between Option 1A (scattered maps) and Option 1C (registration refactor).

### Option 2: Surfacing Requirements

Options 2A/2B/2C are comprehensive, but the document doesn't consider:

**Option 2D: Derive on-demand** (MISSING)

Don't store validation requirements in the plan at all. Instead:

```go
func DeriveValidationConfig(plan *InstallationPlan) ValidationConfig {
    // Compute from plan.Steps using action metadata
}
```

**Pros:**
- No plan format changes
- Works with existing plans
- Requirements always up-to-date with action metadata

**Cons:**
- Requires action metadata to be available at validation time
- Can't validate plans where action registry is unavailable

This is worth considering given the backwards compatibility emphasis.

## 3. Pros/Cons Fairness and Completeness

### Option 1A: Static Registry Maps

**Fair assessment**: Yes, but incomplete.

**Missing cons:**
- "Must enumerate all actions upfront" - Unlike interfaces, you won't get compile errors for missing entries
- "Testing burden" - Need tests to ensure all registered actions have metadata entries (though the codebase already has this pattern for ActionDependencies)

**Missing pros:**
- "Proven pattern" - The codebase already uses this for ActionDependencies successfully
- "Easy to audit" - Single map provides clear view of all requirements

### Option 1B: Interface Methods on Actions

**Unfair characterization**: The cons significantly understate the migration effort.

**Con: "Requires modifying all 49 actions"**
- The document says there are 49 actions, but also shows only 33 action struct files exist
- More importantly: The existing Action interface only has Name() and Execute()
- This would require EVERY action to implement RequiresNetwork() and BuildTools()
- Most would return zero values, creating noise

**Better framing:**
```
Cons:
- Requires modifying 33 action implementations (possibly more with composites)
- Breaking change requires careful versioning
- Most actions return defaults (false, nil) - boilerplate overhead
- Actions loaded from plugins would need interface updates
```

**Missing pro:**
- "Type safety" - Compiler ensures metadata is provided

### Option 1C: Action Struct with Metadata Fields

**Fair assessment**: Yes.

**Could add:**
- Pro: "Consolidates existing scattered maps (deterministicActions, primitives, ActionDependencies)"
- Con: "All-or-nothing migration" - Can't gradually adopt like Option 1A

### Option 2A: Plan-Level Aggregate Fields

**Fair assessment**: Yes.

**Missing consideration:**
- "Plan format versioning" - This is a breaking change to InstallationPlan schema
- Pro: "Can cache expensive computation" - If determining network requirements is costly, doing it once during plan generation is valuable

### Option 2B: Per-Step Metadata in ResolvedStep

**Misleading con**: "Larger plan JSON" is stated as a con, but no evidence it matters.

**Better framing:**
```
Cons:
- Larger plan JSON (estimated +20 bytes per step: {"requires_network":false,"build_tools":[]})
- Validator must aggregate across steps (simple reduce operation)
- Plan format change requires version bump
```

**Missing pro:**
- "Enables selective network access" - Could theoretically run some steps offline, others online

### Option 2C: Separate ValidationRequirements Struct

**Fair assessment**: Yes.

**Could strengthen:**
- Pro: "No plan format version bump needed"
- Con: "Coupling between validator and action registry" - Validator needs access to action metadata maps

## 4. Unstated Assumptions

### Critical Assumptions

**Assumption 1: Network requirements are binary per action**

The design assumes actions either "require network" or don't, without nuance:
- cargo_build: Always needs network? What about vendored dependencies?
- go_build: Could use go.sum with vendored modules
- npm_exec: Might not need network if node_modules committed

**Implication**: Option 1A's `ActionNetworkRequirements = map[string]bool` may be too simplistic. Consider:

```go
type NetworkRequirement int
const (
    NetworkNone = iota
    NetworkOptional
    NetworkRequired
)
```

**Assumption 2: Build tools map 1:1 to actions**

The design assumes build tools are derivable purely from action name:
- `configure_make` → autoconf, automake, libtool, pkg-config

But what if parameters matter? For example:
- cmake_build with generator=Ninja → needs ninja-build
- cmake_build with generator=Make → doesn't need ninja-build

**Implication**: `ActionBuildTools = map[string][]string` may need to become a function:

```go
func GetActionBuildTools(action string, params map[string]interface{}) []string
```

**Assumption 3: Validation requirements are uniform across platforms**

The document shows `detectRequiredBuildTools()` has special logic for Darwin:

```go
if os, hasOS := step.When["os"]; hasOS && os == "darwin" {
    continue  // Skip macOS-only steps
}
```

But the design doesn't address:
- Do build tools vary by platform? (apt packages on Linux, brew on macOS)
- Should ActionBuildTools contain platform-specific mappings?

**Implication**: Metadata structure may need platform awareness:

```go
type ActionBuildTools struct {
    Linux  []string  // apt packages
    Darwin []string  // brew packages
}
```

**Assumption 4: All ecosystem actions require network**

The design states ecosystem actions (cargo_build, go_build, npm_exec) need network. However:
- The eval+plan architecture already pre-downloads some assets
- Some ecosystem actions might work offline if dependencies are cached
- cpan_install (now a primitive, #449) might have hybrid requirements

**Implication**: Network requirements may not be action-intrinsic but context-dependent.

**Assumption 5: BuildTools are apt package names**

The design shows `[]string{"autoconf", "automake"}` but doesn't state:
- Are these Ubuntu package names specifically?
- Do they work on debian:bookworm-slim?
- What about other distros or package managers?

**Implication**: Either document the assumption or make package manager explicit:

```go
type BuildToolRequirement struct {
    Apt    []string
    Apk    []string  // For Alpine
    // Future: Brew, etc.
}
```

### Minor Assumptions

**Assumption 6: detectRequiredBuildTools logic is correct**

The design takes the existing switch statement as authoritative, but doesn't verify:
- Are these the minimal required packages?
- Are all necessary packages included?
- Do the packages exist in both Ubuntu 22.04 and Debian bookworm?

**Assumption 7: Validation must work offline**

The eval+plan architecture is described as "offline validation," but:
- Source builds currently use `Network: host`
- Is offline validation a hard requirement, or is "reproducible validation" sufficient?

**Recommendation**: The "Out of scope" section says "Removing network access from ecosystem actions" is out of scope, which conflicts with the goal of offline validation.

## 5. Strawman Detection

### Are Any Options Designed to Fail?

**No.** All six options represent legitimate architectural choices:

**Option 1A (Static Maps)**: This is actually the EXISTING pattern (ActionDependencies). Not a strawman.

**Option 1B (Interface Methods)**: This is idiomatic Go for metadata. The cons are real (breaking change, boilerplate), not artificially inflated.

**Option 1C (Action Struct)**: This is a reasonable middle ground. The "intermediate complexity" con is fair.

**Option 2A (Plan Aggregate)**: This is the simplest approach for validators. The "loses granularity" con is real but may not matter in practice.

**Option 2B (Per-Step Metadata)**: This provides maximum flexibility. The "larger JSON" con is measurable.

**Option 2C (Separate Struct)**: Clean separation is a valid design principle. The "indirection" con is real.

### Potential Bias Detection

The document appears to favor **Option 1A + Option 2C** based on:
1. Option 1A is described first with "Follows existing pattern" as first pro
2. Option 2C emphasizes "clean separation" which aligns with "Single source of truth" driver
3. The "Conventions to Follow" section states: "Use static registry maps for action metadata"

However, this isn't a strawman situation - it's an articulated preference based on codebase conventions. The other options are fairly presented.

## 6. Additional Considerations

### Missing from Evaluation Matrix

The evaluation matrix (Decision Drivers table) doesn't assess:

1. **Testing complexity**: How easy is it to write tests?
   - Option 1B: Need mock implementations
   - Option 1A: Simple map lookups

2. **Plugin compatibility**: If tsuku supports action plugins in the future:
   - Option 1B: Plugins must implement interface
   - Option 1A: Plugins must register in map

3. **Documentation clarity**: Which approach is easier to explain to contributors?
   - Option 1A: "Add entry to map"
   - Option 1B: "Implement these methods"

4. **Migration path**: For existing recipes/plans:
   - Option 2A/2B: Requires plan regeneration
   - Option 2C: Works with existing plans

### Interaction Between Decisions

The document treats Decision 1 and Decision 2 as independent, but there are dependencies:

**If choosing Option 1B (Interface Methods):**
- Option 2B becomes natural (step can query its action)
- Option 2C becomes harder (need action registry)

**If choosing Option 1A (Static Maps):**
- Option 2A/2B require importing action package in executor
- Option 2C decouples validator from action package

**Recommendation**: Add a "Decision Interaction" section or combined options matrix.

## 7. Recommended Improvements

### Problem Statement

1. **Add success criteria section** defining what "centralized" means operationally
2. **Clarify "derivable from recipe/plan"** - specify at what point (parse time, plan gen, validation)
3. **Add constraint on backwards compatibility** - Must existing plans still validate?

### Options Analysis

4. **Add Option 1D: Structured Metadata Registry** as middle ground between 1A and 1C
5. **Add Option 2D: Derive on-demand** to consider no-plan-change approach
6. **Strengthen Option 1B cons** to reflect true migration effort (33+ files, interface change)
7. **Add platform assumptions** to Option 1A build tools discussion
8. **Quantify "larger plan JSON"** in Option 2B con (estimate bytes per step)

### Assumptions to Surface

9. **Document network requirement nuance** - Not binary, may be conditional
10. **Clarify build tool scope** - apt packages only, or multi-platform?
11. **Address parameter-dependent requirements** - Some actions need different tools based on params
12. **State platform validation strategy** - Linux-only? Multi-platform?

### Evaluation

13. **Add testing complexity** to decision drivers
14. **Add migration path** row to evaluation matrix
15. **Add combined options** section showing interaction between Decisions 1 and 2

## 8. Answers to Specific Questions

### 1. Is the problem statement specific enough to evaluate solutions against?

**Mostly yes, with gaps.**

**Strengths:**
- Concrete examples (detectRequiredBuildTools switch)
- Enumerated problems (5 specific issues)
- Clear scope boundaries

**Gaps:**
- No success criteria or evaluation metrics
- Vague "derivable from recipe/plan" - timing unclear
- Missing constraint on backwards compatibility

**Rating: 7/10** - Specific enough for initial evaluation but needs success criteria.

### 2. Are there missing alternatives we should consider?

**Yes, two significant omissions:**

**Option 1D: Structured Metadata Registry**
```go
type ActionMetadata struct {
    Network    bool
    BuildTools []string
}
var ActionMetadata = map[string]ActionMetadata{...}
```

This combines Option 1A's non-invasiveness with better organization than separate maps.

**Option 2D: Derive on-demand**
```go
func DeriveValidationConfig(plan *InstallationPlan) ValidationConfig
```

This avoids plan format changes entirely by computing requirements at validation time.

Both are legitimate alternatives that fit the decision drivers.

### 3. Are the pros/cons for each option fair and complete?

**Mostly fair, with three issues:**

**Issue 1: Option 1B understates migration complexity**
- Con says "Requires modifying all 49 actions"
- Should emphasize: Breaking interface change, 33+ files, mostly boilerplate
- Should add testing complexity

**Issue 2: Option 2B overstates "larger JSON" con without quantification**
- No evidence plan size matters
- Could estimate: ~20 bytes per step
- Typical plan has <10 steps = ~200 bytes total

**Issue 3: Missing pros for Option 1A**
- Doesn't mention this is the EXISTING pattern (ActionDependencies)
- Proven in production
- Easy to audit

**Rating: 7/10** - Fair intent but needs more precision on migration costs and plan size impact.

### 4. Are there unstated assumptions that need to be explicit?

**Yes, five critical assumptions:**

1. **Network requirements are binary** - But vendored deps, cached modules could make it conditional
2. **Build tools are action-intrinsic** - But cmake generator parameter affects tools needed
3. **Build tools are apt packages** - But distro compatibility not verified
4. **Validation is Linux-only** - Platform-specific build tools not addressed
5. **Offline validation is possible** - Conflicts with ecosystem actions needing network

**Most critical to surface:**
- Network requirements may be conditional on parameters or cache state
- Build tools may vary by platform and action parameters
- "Offline validation" conflicts with "ecosystem actions need network"

These assumptions affect whether static maps (Option 1A) are sufficient or if functions/conditional logic is needed.

### 5. Is any option a strawman (designed to fail)?

**No. All options are legitimate.**

**Evidence:**
- Option 1A is the EXISTING pattern (ActionDependencies) - clearly not a strawman
- Option 1B is idiomatic Go (interface-based metadata) - valid approach
- Option 1C is a reasonable consolidation strategy
- Options 2A/2B/2C represent common architectural patterns (aggregate, per-item, computed)

**Potential bias detected:**
- Document favors Option 1A + 2C based on "Conventions to Follow"
- But this is an articulated preference, not hidden strawman construction

The evaluation matrix shows honest trade-offs. No option is unfairly disadvantaged.

## 9. Overall Assessment

### Strengths

1. **Well-scoped problem** with clear boundaries (in scope / out of scope)
2. **Concrete examples** from actual codebase (detectRequiredBuildTools)
3. **Fair option presentation** - no strawmen detected
4. **Consistent structure** - all options use same template
5. **Acknowledges uncertainties** section shows intellectual honesty

### Weaknesses

1. **Missing success criteria** - Can't fully evaluate without defining "success"
2. **Incomplete options** - Missing hybrid approaches (1D, 2D)
3. **Understated complexity** - Option 1B migration cost not fully explored
4. **Unstated assumptions** - Network/build requirements may not be simple maps
5. **Missing interaction analysis** - Decisions 1 and 2 aren't fully independent

### Recommendation

**The design is 80% ready for decision-making.**

**Before proceeding:**
1. Add success criteria section
2. Consider Option 1D (structured metadata registry) as alternative
3. Surface network requirement assumptions (binary vs conditional)
4. Quantify Option 2B plan size impact (bytes per step)
5. Add combined options recommendation (e.g., "1A + 2C" or "1D + 2A")

**After improvements, this will be a solid foundation for implementation.**

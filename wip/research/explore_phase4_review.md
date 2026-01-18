# Design Review: Embedded Recipe List Generation

## Review Context

Reviewing `docs/designs/DESIGN-embedded-recipe-list.md` as Stage 0 prerequisite for recipe registry separation.

**Key context:**
- Issue #644 (composite action dependency aggregation) is **CLOSED** - the TODO comments in `homebrew.go` and `homebrew_relocate.go` are stale
- The `resolver.go` infrastructure already handles transitive dependencies via `ResolveTransitive()` and `aggregatePrimitiveDeps()`
- 171 recipes exist currently
- Estimate of 15-20 embedded recipes

---

## Question 1: Is the problem statement specific enough?

**Assessment: Mostly adequate, but could be more precise**

The problem statement identifies three risks (incomplete coverage, stale documentation, no CI validation) but doesn't quantify success criteria. Questions that remain unclear:

1. **What counts as "complete"?** The statement says we need to capture "ALL action dependencies including transitive recipe dependencies" but doesn't define the scope precisely:
   - Install-time dependencies only? (current focus)
   - Runtime dependencies too?
   - EvalTime dependencies for decomposable actions?

2. **What is the validation baseline?** How do we know the generated list is correct? The design mentions "validate against known gaps (issue #644)" but #644 is already closed.

3. **Platform scope**: The design mentions "platform-specific dependency handling (e.g., patchelf is Linux-only)" but doesn't specify if we need one list or per-platform lists.

**Recommendation:** Add explicit success criteria:
- "The generated list must include all recipes returned by any action's `Dependencies()` method"
- "The list must include transitive recipe dependencies from recipe metadata (e.g., ruby -> libyaml)"
- Specify whether output is one combined list or per-platform lists

---

## Question 2: Are there missing alternatives?

**Assessment: Yes, one significant alternative is missing**

### Missing Option 1D: Leverage Existing Resolver Infrastructure

The design proposes building a new analysis tool, but the resolver.go already has the machinery:

```go
// ResolveDependenciesForPlatform already handles:
// - Action Dependencies() collection
// - Platform-specific deps (LinuxInstallTime, DarwinInstallTime)
// - Step-level overrides
// - aggregatePrimitiveDeps() for composite actions
```

**Alternative approach:**
1. Create a test/script that iterates all registered actions
2. For each action, call `GetActionDeps()` to extract direct dependencies
3. For each dependency, load the recipe and call `ResolveDependencies()` recursively
4. Union all results

This leverages proven code rather than duplicating dependency extraction logic.

**Pros:**
- Single source of truth (uses same code paths as runtime)
- Already handles composite action aggregation (#644 is resolved)
- No new parsing/extraction logic to maintain
- Type-safe, catches interface changes at compile time

**Cons:**
- Requires building the binary to run analysis
- Harder to run as standalone script

### Missing consideration: Test recipe handling

The design doesn't mention how to handle `testdata/recipes/` (from DESIGN-recipe-registry-separation.md Stage 3). Integration test recipes like `netlify-cli.toml` are embedded for CI but aren't action dependencies. Should they appear in EMBEDDED_RECIPES.md with a different justification?

---

## Question 3: Are the pros/cons fair and complete?

**Assessment: Generally fair, with some gaps**

### Option 1A (Go Program) - Missing cons:
- **Version sync risk**: If someone updates action code but forgets to run the generator, the list drifts. This is the same problem as manual maintenance, just automated.
- **Build dependency**: Must build the binary before CI can validate. Creates a chicken-and-egg for CI workflows.

### Option 1B (Go Test) - Undervalued:
The "somewhat unconventional" con is overstated. This pattern is common:
- `go generate` routinely calls test-like code
- Many projects use `TestMain` for code generation
- The `GENERATE_EMBEDDED=1` pattern is well-understood

### Option 1C (Shell + go run) - Speed concern overstated:
The "slower (compiles on every run)" con depends on module caching. With warm cache, `go run` is fast enough for CI (< 2 seconds for a single-file program).

### Option 2A (Markdown only) - Missing pro:
- **Human auditability**: PRs that change EMBEDDED_RECIPES.md are reviewable. Reviewers can see exactly which recipes are embedded and why.

### Option 3A (Regenerate and Compare) - Missing detail:
The non-determinism concern is valid but solvable. The design should note:
- Avoid timestamps in output
- Sort recipe lists alphabetically
- Use deterministic ordering for dependency graphs

---

## Question 4: Are there unstated assumptions?

**Assessment: Yes, several important assumptions need to be explicit**

### Assumption 1: Issue #644 is still relevant
The design says "Issue #644 (composite action dependency aggregation) has been resolved" but then references "known gaps (#644)" in the implementation approach. The TODO comments in `homebrew.go` (line 32) and `homebrew_relocate.go` (line 22) are stale - they reference #644 but the issue is closed.

**Reality:** The `aggregatePrimitiveDeps()` function in `resolver.go` (lines 250-267) already handles this. The design should clarify that #644's fix is already deployed.

### Assumption 2: Transitive dependencies are limited to recipe metadata
The design correctly identifies that `ruby` depends on `libyaml` via recipe metadata. But it doesn't address:

- **Build-time transitive deps**: cmake's recipe might use actions that declare dependencies. Are those captured?
- **Circular dependency handling**: What if recipe A depends on tool B whose action depends on recipe A? The resolver handles this but it's not mentioned.

### Assumption 3: One embedded list for all platforms
The design doesn't specify whether EMBEDDED_RECIPES.md should show:
- One combined list (all platforms merged)
- Separate sections per platform
- A platform column in the table

Since `patchelf` is Linux-only, the answer matters for accuracy.

### Assumption 4: EvalTime dependencies can be ignored
Some decomposable actions have `EvalTime` dependencies (e.g., `cargo_install` needs `rust` at eval time to generate Cargo.lock). These are technically needed during plan generation, not installation. The design should clarify if EvalTime deps are in scope.

### Assumption 5: Version constraints are always "latest"
The resolver uses `"latest"` as the default version for action dependencies. The embedded list design doesn't discuss version constraints - it assumes we just need recipe names.

---

## Question 5: Is any option a strawman?

**Assessment: No obvious strawmen, but Option 2C is weak**

### Option 2C (Markdown as Source of Truth with Validation Script)
This option is described as "Fragile parsing" and "Format changes break validation" - which is true. However, the evaluation table shows it equal to 2A for "Simplicity" (both "Good"), which seems inconsistent with the "fragile" description.

The option isn't a strawman per se, but the description makes it obviously inferior. A fairer treatment would note that regex parsing of well-structured markdown tables is actually quite reliable if the format is well-defined.

### Balance check:
The options appear genuinely considered. Option 1B (Go Test) gets reasonable treatment despite being "unconventional." The evaluation matrices show nuanced trade-offs rather than one obvious winner.

---

## Additional Observations

### The existing infrastructure is underutilized

Reading `resolver.go`, I found that the dependency resolution infrastructure is more complete than the design suggests:

1. **`aggregatePrimitiveDeps()`** already handles composite action dependency aggregation
2. **`ResolveDependenciesForPlatform()`** handles platform-specific deps
3. **`ResolveTransitive()`** handles recipe-level transitive deps with cycle detection

The design's "Implementation Context" section lists these functions but doesn't leverage them fully in the options analysis.

### The 15-20 estimate seems low

Based on the action Dependencies() implementations I reviewed:
- **Language toolchains**: golang, rust, nodejs, python-standalone, ruby, perl (6)
- **Ruby's transitive**: libyaml (1)
- **Build tools**: make, zig, pkg-config, cmake, meson, ninja (6)
- **Linux-specific**: patchelf (1)

That's 14 direct action dependencies. Adding transitives:
- cmake might have deps
- ruby -> libyaml
- Any homebrew bottles used by embedded recipes

The 15-20 range seems plausible but tight. The tooling should be conservative and include a safety margin.

### CI validation is the key deliverable

Of all the design goals, "CI validation script (verify-embedded-recipes.sh)" is the most important. Without it, the list will drift regardless of how it's generated. The design should prioritize this more explicitly in the decision criteria.

---

## Summary Recommendations

1. **Update #644 references**: Remove stale references since the issue is closed and the fix is deployed

2. **Add Option 1D**: Consider leveraging existing resolver infrastructure rather than building parallel extraction logic

3. **Clarify platform handling**: Specify if output is combined or per-platform

4. **Define EvalTime scope**: Clarify whether EvalTime dependencies should be included

5. **Add success criteria**: Define measurable completeness criteria for validation

6. **Strengthen 2A over 2C**: Either improve 2C's description or more clearly differentiate them in the evaluation

7. **Address test recipes**: Clarify how testdata/recipes/ relates to EMBEDDED_RECIPES.md

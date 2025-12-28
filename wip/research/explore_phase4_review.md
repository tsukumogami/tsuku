# Phase 4: Options Review

## Problem Statement Assessment

**Clarity: STRONG**
The problem statement clearly articulates the current friction: automation tooling needs static recipe metadata without network calls or plan generation. The distinction between what exists (`info`, `eval`, `plan`) and what's missing is well-defined.

**Specificity: STRONG**
Four concrete motivating scenarios are provided:
1. Testing frameworks generating platform-specific golden plans
2. CI pipelines querying platform support pre-installation
3. Documentation tooling extracting recipe info for website
4. Development workflows validating recipe changes pre-commit

Each scenario has clear characteristics (needs static data, no network, no execution).

**Scope Definition: EXCELLENT**
The in-scope/out-of-scope boundaries are explicit and well-justified:
- IN: Static metadata, structured JSON, platform detection, version provider config
- OUT: Version resolution (→ eval), installation state (→ info), plan generation (→ plan/eval)

Clear delineation from existing commands prevents feature overlap.

**Gap: Missing Non-Goal**
Should explicitly state that this is NOT for validating recipe correctness beyond parsing. The scope says "Recipe validation beyond what the parser provides" is out of scope, but doesn't clarify that users shouldn't expect validation feedback (e.g., "this recipe is invalid because...").

## Decision Drivers Assessment

**Alignment with Problem: STRONG**
All six drivers directly support the stated problem:
- "No network dependency" → enables offline CI/dev workflows
- "No execution" → separates static introspection from dynamic resolution
- "Programmatic automation" → serves the automation tooling use case
- "Completeness" → avoids partial data requiring fallback to TOML parsing
- "Symmetry" → reduces learning curve (follows install/eval patterns)
- "Discoverability" → helps users choose between info vs metadata

**Completeness: GOOD, with one gap**
Missing driver: **Performance/Efficiency** - The problem statement mentions that `eval` is "unnecessarily expensive" for static queries, but there's no driver about command speed/resource usage. This matters for CI integration where hundreds of metadata queries might run.

**Recommendation:** Add driver: "Efficiency: Should be fast enough for bulk queries in CI (hundreds of recipes without noticeable delay)"

## Options Analysis

### Decision 1: Command Interface

**Missing Alternatives: YES - One critical option missing**

**Option 1C: Tool Name with OPTIONAL --recipe override**
`tsuku metadata <tool>` where tool name defaults to registry but `--recipe` overrides the source.

Example: `tsuku metadata go --recipe ./recipes/go.toml`

**Pros:**
- More intuitive than mutual exclusivity (tool name is always the subject)
- Matches `npm view <package>` semantics (package name + optional registry override)
- Enables "compare registry vs local": `tsuku metadata go` vs `tsuku metadata go --recipe ./custom.toml`

**Cons:**
- Breaks symmetry with `eval` command (which uses mutual exclusivity)
- Tool name becomes redundant when using --recipe (must match recipe's tool name)
- Could be confusing if --recipe tool name doesn't match argument

**Analysis of Existing Options:**

**Option 1A (Tool Name Only):**
- Fairly presented, but con "Breaks symmetry with install and eval" is overstated
- `install --recipe` exists for testing installation flow, but metadata doesn't need full symmetry
- Con "Golden plan testing needs uncommitted recipes" is the killer argument → makes 1A non-viable

**Option 1B (Mutually Exclusive):**
- Pros/cons are fair and complete
- Not a strawman - this is the strongest option given eval precedent
- Pro "Symmetric with eval" could be stronger: "Reduces cognitive load for users who already know eval"

**Is either a strawman? NO**
- 1A is weak but represents the "simplest thing that could work" baseline
- Neither option is artificially weakened to make the other look better

### Decision 2: Output Format

**Missing Alternatives: NO**
The two fundamental options are covered (JSON-only vs dual format).

**Analysis of Existing Options:**

**Option 2A (JSON Only):**
- Pros are fair but miss a key point: "Aligns with decision driver 'Programmatic automation' as primary use case"
- Con "Inconsistent with existing commands" is valid but could note that `eval` defaults to JSON (no human-readable mode), showing precedent for automation-first commands
- Con "Less friendly for exploratory use" assumes exploration is a goal, but problem statement emphasizes automation

**Option 2B (Dual Format):**
- Pros/cons are balanced and fair
- Con "Human-readable format needs design" understates the complexity - what does human-readable metadata look like? Nested constraints, action lists, platform tuples are all hard to format readably
- Missing con: "Default to human-readable violates principle of least surprise for automation-first command" (users piping to jq would need to remember --json)

**Is either a strawman? BORDERLINE for 2B**
- The human-readable format design complexity is real but understated
- However, it's not deliberately weakened - maintaining two formats IS a genuine cost
- The inconsistency argument for 2A is a real concern (surprises users familiar with info/versions)

**Recommendation:** Add option 2C: "Default to JSON with opt-in --human flag" to address the "automation-first but not human-hostile" middle ground.

### Decision 3: Output Schema Granularity

**Missing Alternatives: YES - One option worth considering**

**Option 3D: JSON output + jq examples in documentation**
Always output full schema but provide cookbook of common jq queries in `tsuku metadata --help` or docs.

Example:
```bash
# Get supported platforms only
tsuku metadata go | jq '.supported_platforms'

# Check if specific platform is supported
tsuku metadata go | jq '.supported_platforms[] | select(. == "linux/amd64")'
```

**Pros:**
- Zero implementation complexity (just documentation)
- Teaches users jq (widely useful skill)
- No flag proliferation or field name maintenance
- Full flexibility (users can query anything)

**Cons:**
- Requires jq to be installed (not always available in minimal CI containers)
- Harder to discover than built-in flags
- Longer command invocations

**Analysis of Existing Options:**

**Option 3A (Full Dump Only):**
- Pros/cons are fair
- Con "Wastes bandwidth" is weak - recipe metadata is <10KB, not a real concern
- Missing pro: "Future-proof - new recipe fields automatically included in output"
- Missing pro: "Matches recipe.Recipe struct directly, no translation layer"

**Option 3B (Optional Field Selection):**
- Cons accurately identify the complexity (field selector syntax, nested paths, maintenance)
- Con "Unclear field names" is critical - dot notation? JSONPath? TOML keys?
- Missing con: "Requires documentation of all selectable fields, which becomes stale"
- This option has real design challenges that aren't fully explored

**Option 3C (Predefined Query Types):**
- Fairly presented but con "Flags proliferate" is the killer
- Missing con: "Partial output modes complicate testing (need fixtures for each mode)"
- Not a strawman but clearly weaker than 3A or 3B

**Is any option a strawman? YES - 3C appears designed to fail**
- "Need to predict which queries are common" + "Flags proliferate" + "Less flexible" = no clear upside
- The "self-documenting" pro is weak (--help text achieves same thing)
- This feels like a compromise position that satisfies neither simplicity nor flexibility

**Unstated Assumption:** Users know jq or are willing to learn it. If this is false, 3A becomes significantly weaker.

### Decision 4: Platform Information Representation

**Missing Alternatives: YES - One option worth considering**

**Option 4D: Raw constraints with helper function**
Output raw constraints but also include a top-level `platform_check(os, arch)` field that's a boolean or explanation.

Wait, that doesn't work in JSON (can't embed functions). Strike that.

**Actual missing option:**

**Option 4E: Raw constraints + explicit unsupported list**
Include `supported_platforms` (computed) AND `unsupported_platforms` (from recipe) as separate arrays, plus raw constraints for completeness.

Actually, that's just a variant of 4C. No genuinely missing options here.

**Analysis of Existing Options:**

**Option 4A (Raw Constraints Only):**
- Con "Requires users to understand complementary hybrid model" is the strongest argument against
- Con "Automation scripts must reimplement platform matching logic" is critical - this violates DRY principle (tsuku already has the logic in `GetSupportedPlatforms()`)
- Missing pro: "Exactly matches TOML schema - no semantic interpretation"
- Not a strawman but clearly inferior for automation use case

**Option 4B (Computed List Only):**
- Pro "Matches GetSupportedPlatforms() method that already exists" is strong - reuses existing code
- Con "Output schema diverges from recipe TOML structure" is weak - JSON output shouldn't be constrained by TOML structure if it improves usability
- Missing con: "Loses information about WHY a platform is unsupported (can't see unsupported_platforms exceptions)"
- This is the most automation-friendly option

**Option 4C (Both Raw and Computed):**
- Pros/cons are fair
- Con "Potentially confusing" is legitimate but addressable with clear docs
- Missing pro: "Enables migration path if we deprecate raw constraints in future API versions"
- Missing pro: "Users can validate our platform logic by comparing raw constraints to computed list"

**Is any option a strawman? NO**
- All three represent genuine trade-offs between simplicity, completeness, and usability
- 4A serves a "purity" use case (minimally processed data)
- 4B serves automation directly
- 4C serves both at cost of redundancy

**Unstated Assumption:** Platform matching logic is deterministic and bug-free. If there are edge cases where `GetSupportedPlatforms()` produces surprising results, users with 4B have no way to debug it.

## Cross-Decision Interactions

### Decision 1 × Decision 2: Recipe Source Affects Output Format

**Interaction:** If using --recipe flag (1B), human-readable output (2B) becomes less useful because local recipe files are likely being tested programmatically, not read by humans for exploration.

**Implication:** Could argue for JSON-only mode when --recipe is used, human-readable mode allowed only for registry tool names. But this creates inconsistency.

**Recommendation:** Note this interaction in the document. If choosing 1B + 2B, keep output format consistent regardless of recipe source.

### Decision 2 × Decision 3: Output Format Determines Field Selection Value

**Interaction:** If JSON-only (2A), field selection (3B/3C) becomes more valuable because users can't quickly scan human-readable output. If dual format (2B), human-readable output provides built-in "field selection" via visual layout.

**Implication:**
- (2A + 3A) forces users to jq for every query → high friction
- (2A + 3B) reduces jq dependency → better UX
- (2B + 3A) is fine because human mode shows curated view

**Recommendation:** If choosing 2A (JSON-only), lean toward 3B/3C (field selection) to compensate. If choosing 2B (dual format), 3A (full dump) is more acceptable.

### Decision 3 × Decision 4: Field Selection Complexity Depends on Platform Schema

**Interaction:** If offering field selection (3B), having both raw constraints AND computed platforms (4C) doubles the number of platform-related fields users must understand.

**Implication:**
- (3B + 4C) creates cognitive load: "Do I query 'platforms' or 'supported_os' and 'supported_arch'?"
- (3B + 4B) is simpler: single computed field for platform queries
- (3A + 4C) is fine: full dump doesn't require field naming decisions

**Recommendation:** If choosing 3B (field selection), lean toward 4B (computed only) for simplicity. If choosing 3A (full dump), 4C (both) is acceptable.

### Decision 1 × Decision 4: Recipe Source Affects Platform Validation

**Interaction:** With --recipe flag (1B), users may be testing recipes before platforms are finalized. Computed platform list (4B/4C) runs validation logic that might reject the recipe, while raw constraints (4A) would pass through.

**Implication:** If `GetSupportedPlatforms()` returns empty list or errors for malformed constraints, metadata command behavior differs:
- 4A: outputs malformed constraints as-is (garbage in, garbage out)
- 4B: errors or outputs empty array (fails fast)
- 4C: could output raw constraints even if computation fails (best of both)

**Recommendation:** Note this edge case. If choosing 4B with 1B, document behavior when platform constraints are malformed. Consider 4C to provide debugging path.

## Unstated Assumptions

### Assumption 1: Recipe Schema Stability
**Implicit assumption:** Recipe TOML schema is stable enough that outputting full metadata won't expose internal fields that might change.

**Why it matters:** If recipe struct has fields like `_internal_cache` or `parser_version`, full dump (3A) would expose them. Over time, users might depend on these, creating breaking changes when we evolve the schema.

**Recommendation:** Explicitly state whether metadata output is a stable API surface. If yes, define which fields are public vs internal. If no, add disclaimer that schema may change between tsuku versions.

### Assumption 2: JSON as Universal Format
**Implicit assumption:** Target audience (CI pipelines, testing frameworks, documentation tooling) can all consume JSON easily.

**Why it matters:** Some environments (Windows batch scripts, simple shell scripts) have poor JSON parsing support. If a motivating use case struggles with JSON, the decision drivers may be incomplete.

**Counter-evidence:** The golden plan testing use case (from problem statement) already works with JSON from `tsuku eval`, so this assumption seems safe for the stated use cases.

**Recommendation:** Add sentence confirming that all motivating scenarios can consume JSON (already proven by eval usage).

### Assumption 3: GetSupportedPlatforms() Correctness
**Implicit assumption:** The existing `GetSupportedPlatforms()` method correctly implements platform matching logic and doesn't have edge case bugs.

**Why it matters:** If choosing 4B (computed list only), users have no way to verify or debug platform logic. If the method has bugs, metadata output will be wrong with no recourse.

**Evidence:** Decision 4 mentions this method "already exists" but doesn't discuss its maturity or test coverage.

**Recommendation:** If choosing 4B or 4C, note in implementation section that platform computation should have comprehensive tests. Consider adding a decision driver: "Correctness: Platform support logic must be well-tested since metadata will be authoritative source."

### Assumption 4: Metadata Command Won't Need Version Context
**Implicit assumption:** All useful static metadata can be extracted without knowing which version the user is interested in.

**Why it matters:** Some recipes might have version-specific platform support (e.g., Go 1.20+ supports linux/riscv64, earlier versions don't). If static metadata can't capture this, users might get incorrect platform lists.

**Counter-evidence:** Problem statement explicitly excludes version resolution from scope. But this could be a future gap.

**Recommendation:** Add to "Out of Scope" section: "Version-specific metadata variations (recipes are version-agnostic)". Note this as a known limitation.

### Assumption 5: Recipe Metadata Fits In Memory
**Implicit assumption:** Full recipe dump (3A) won't be so large that it causes performance issues.

**Reality check:** Current recipes are <5KB TOML → probably <10KB JSON. Even 1000 recipes = 10MB, which is fine.

**Recommendation:** No action needed, but could add note to decision 3A: "Recipe metadata is small enough (<10KB per recipe) that full dumps are not a performance concern."

### Assumption 6: Users Will Read Documentation
**Implicit assumption:** If choosing mutually exclusive args (1B), users will read docs or error messages to understand `--recipe` vs tool name distinction.

**Why it matters:** Mutual exclusivity is a source of friction. Users might try `tsuku metadata go --recipe go.toml` and not understand why it fails.

**Mitigation:** Decision 1B acknowledges "more complex argument validation" but doesn't discuss error message quality.

**Recommendation:** If choosing 1B, add implementation note: "Error message should explain mutual exclusivity clearly: 'Cannot specify both tool name and --recipe flag. Use either: tsuku metadata <tool> OR tsuku metadata --recipe <path>'."

## Recommendations

### Critical Issues

1. **Add missing decision driver: Efficiency/Performance**
   - Current drivers don't address speed requirements for bulk CI usage
   - Suggest: "Efficiency: Fast enough for bulk queries (100s of recipes) in CI without noticeable delay (<1s total)"

2. **Clarify validation scope boundary**
   - Out-of-scope mentions "recipe validation beyond parser" but unclear if command validates constraints
   - Add explicit non-goal: "Does NOT validate recipe correctness or flag errors beyond TOML parsing"

3. **Document GetSupportedPlatforms() maturity**
   - Decision 4B/4C rely on this method but no discussion of test coverage or known edge cases
   - Add implementation note: "Platform computation via GetSupportedPlatforms() must have comprehensive test coverage before this command is considered stable"

### Major Enhancements

4. **Consider adding Option 1C (tool name + optional --recipe override)**
   - May be more intuitive than mutual exclusivity
   - Enables comparison use case: registry vs local
   - Trade-off: breaks symmetry with eval

5. **Add Option 2C (default JSON with opt-in --human)**
   - Addresses automation-first use case while supporting exploration
   - Avoids forcing --json flag in CI scripts
   - Trade-off: inconsistent with info/versions defaults

6. **Add Option 3D (full dump + jq examples in docs)**
   - Zero implementation cost alternative to field selection
   - Acknowledges jq dependency assumption
   - Trade-off: requires jq installation

7. **Expand cross-decision interactions section**
   - Decisions 2×3 (output format affects field selection value)
   - Decisions 3×4 (field selection complexity depends on platform schema)
   - Decisions 1×4 (recipe source affects platform validation on malformed constraints)

### Minor Improvements

8. **Make unstated assumptions explicit**
   - Recipe schema stability and API surface guarantees
   - JSON consumption capability in target environments (mostly validated by eval usage)
   - Version-agnostic metadata as known limitation
   - Error message quality for mutual exclusivity (if choosing 1B)

9. **Strengthen decision driver alignment**
   - Link "No network dependency" explicitly to CI/offline scenarios
   - Link "Completeness" to avoiding fallback TOML parsing

10. **Address potential strawman in Decision 3C**
    - Option 3C (predefined queries) has no clear winning scenario
    - Either remove it or strengthen pros (e.g., "enables flag-based discovery without docs")

11. **Add future-proofing note to Decision 3A**
    - Pro: "New recipe fields automatically included in output"
    - Pro: "Matches recipe.Recipe struct directly, no translation layer"

12. **Clarify Decision 4B trade-off**
    - Con should note information loss: "Cannot see WHY platform is unsupported (unsupported_platforms exceptions not visible)"

### Document Structure

13. **No major structural issues**
    - Problem statement, drivers, implementation context, and options are well-organized
    - Scope boundaries are clear
    - Similar implementations section provides good context

### Overall Assessment

**Strengths:**
- Problem statement is specific with clear motivating scenarios
- Scope boundaries cleanly separate from existing commands
- Implementation context shows strong understanding of codebase patterns
- Options are generally well-researched and fairly presented

**Weaknesses:**
- Missing performance/efficiency decision driver despite problem statement mentioning "unnecessarily expensive"
- Some cross-decision interactions not explored (especially 2×3, 3×4)
- GetSupportedPlatforms() reliability assumed but not validated
- Unstated assumptions about schema stability and API guarantees

**Verdict:**
This is a strong design document that's ready for decision-making with minor revisions. The options analysis is mostly fair (one potential strawman in 3C). Adding the missing alternatives and cross-decision interactions would strengthen it further, but current state is sufficient to evaluate trade-offs.

**Priority Actions Before Deciding:**
1. Add efficiency/performance driver
2. Document GetSupportedPlatforms() test coverage
3. Clarify recipe validation scope
4. Expand cross-decision interactions (especially if considering 3B + 4C combination)

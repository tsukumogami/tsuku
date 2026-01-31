# Design Review: Batch Multi-Platform Validation

## Executive Summary

The design document addresses a real and critical gap: batch-generated recipes claim all-platform support but are only validated on Linux x86_64. However, the problem statement conflates two distinct issues that deserve separate treatment, and the options analysis contains significant blind spots around existing infrastructure and PR-time validation.

**Key Findings:**
1. Problem statement mixes two separate concerns that should be addressed independently
2. Missing a critical Option 4 that builds on existing test-changed-recipes.yml
3. Option 2 (Plan-Only) is a strawman with unfairly dismissive cons
4. The PR-time golden gap is overstated given existing validation
5. Unstated assumption: batch recipes can't leverage existing PR validation patterns

**Recommendation:** Split into two designs and add a fourth option that extends existing PR validation.

---

## 1. Problem Statement Analysis

### What's Actually Two Problems

The problem statement combines:

**Problem A: Batch recipes claim all-platform support without multi-platform validation**
- Scope: batch-generate.yml workflow
- Impact: Batch-merged recipes may break on claimed platforms
- Context: Recipes are auto-merged without human review

**Problem B: New registry recipes have no R2 golden baseline at PR time**
- Scope: All registry recipes (batch and manual)
- Impact: validate-golden-execution.yml skips new recipes
- Context: R2 files only exist post-merge

These problems have different:
- **Root causes**: (A) is about batch automation scope; (B) is about storage migration timing
- **Solutions**: (A) needs platform validation jobs; (B) needs PR-time plan generation
- **Stakeholders**: (A) affects batch pipeline reliability; (B) affects all recipe contributors

### Is It Specific Enough?

**For Problem A:** Yes, but over-constrained. The statement assumes validation must happen "in batch-generate.yml" but doesn't consider whether extending test-changed-recipes.yml could achieve the same goal.

**For Problem B:** No. The problem says "new registry recipes merge without any plan-level validation" but this is misleading:
- test-changed-recipes.yml already does `tsuku install --force --recipe <path>`, which internally generates and validates a plan before installing
- The missing validation is plan **comparison** (diff against baseline), not plan **generation**

### What's Missing

The problem statement doesn't acknowledge that:
1. test-changed-recipes.yml already validates on Linux + macOS for PRs (lines 149-236)
2. Plan generation is inherently validated during install (you can't install without generating a valid plan first)
3. The R2 gap only affects **diff-based validation**, not **execution validation**

### Recommended Reformulation

**Problem A (Batch Multi-Platform Validation):**
> Batch-generated recipes in batch-generate.yml are validated only on Linux x86_64 glibc but claim all-platform support by default. Recipes that pass Linux validation get auto-merged without testing on macOS, ARM64, or musl, so users on those platforms can encounter broken installs. This is a batch-specific gap because manually-submitted recipes in PRs are already validated on Linux and macOS via test-changed-recipes.yml.

**Problem B (PR-Time Golden Comparison for New Recipes):**
> New registry recipes can't perform golden diff validation at PR time because R2 baselines only exist after merge. However, these recipes already undergo execution validation via test-changed-recipes.yml (which generates and validates plans internally), and golden file generation happens via publish-golden-to-r2.yml post-merge. The gap is purely diff-based regression detection, not fundamental plan correctness.

---

## 2. Missing Alternatives

### Critical Omission: Option 4 - Extend test-changed-recipes.yml

The design completely ignores that test-changed-recipes.yml already does multi-platform validation for PR changes. Why not use the same pattern for batch PRs?

**Option 4: Batch PR Triggers test-changed-recipes.yml**

The batch merge job creates a PR. That PR includes changed recipes. test-changed-recipes.yml triggers on recipe changes. It already validates on Linux (matrix per recipe) and macOS (aggregated).

**What needs to change:**
1. Batch merge job creates the PR (already planned)
2. test-changed-recipes.yml triggers on that PR (already works via `paths: recipes/**/*.toml`)
3. Merge only happens after test-changed-recipes.yml passes (standard PR checks requirement)

**Pros:**
- Zero new infrastructure (reuses existing workflow)
- Consistent validation between batch and manual PRs
- Already handles Linux matrix + macOS aggregation
- Already has execution-exclusions.json integration
- No macOS budget increase (same validation either way)
- Platform constraints can be written in merge job based on test-changed-recipes.yml results

**Cons:**
- Batch PR validation is slower (waits for full test-changed-recipes.yml run)
- Doesn't validate before creating the PR (but neither does Option 1 - it validates within the workflow then creates PR)
- Can't selectively exclude recipes mid-batch based on platform results (but this is actually a feature - failing recipes should block the batch PR for review)

### Why Was This Omitted?

The design treats batch-generate.yml as a standalone pipeline that must be self-contained. But once it creates a PR, that PR enters the standard PR validation flow. The implicit assumption is "we can't rely on PR CI because we want to auto-merge immediately", but that's a policy choice, not a technical constraint.

### Other Missing Alternatives

**Option 5: Hybrid - Linux in Batch, Full Matrix in PR**

Batch workflow validates only Linux (cheap, fast feedback). PR includes all passing recipes. test-changed-recipes.yml validates the PR on full matrix before auto-merge.

**Pros:**
- Fast batch cycle (Linux-only)
- Full coverage before merge (via PR CI)
- Clear separation: batch = generation, PR = validation

**Cons:**
- Recipes that fail macOS block the entire batch PR (but this is good - batch should be atomic)

---

## 3. Pros/Cons Fairness Analysis

### Option 1: Platform Matrix Jobs in Batch Workflow

**Stated Cons:**
- "Adds 4 jobs to the workflow (longer wall-clock time)" - Fair
- "macOS jobs cost 10x (budget pressure on large batches)" - Fair, but mitigated by progressive strategy
- "ARM64 runners may have availability issues" - Speculative; no evidence provided
- "Merge job logic for deriving constraints adds complexity" - Fair

**Missing Cons:**
- Duplicates validation that test-changed-recipes.yml would do anyway on the PR
- Tight coupling between batch orchestration and platform validation
- Harder to maintain two separate multi-platform validation systems

**Missing Pros:**
- Validates before creating PR, so bad recipes never enter PR queue
- Platform results available in workflow artifacts for analysis

### Option 2: Plan-Only Validation (No Install)

**Stated Cons:**
- "Doesn't catch runtime failures" - Fair
- "URL existence doesn't guarantee the archive contains what we expect" - True but overstated
- "Can't detect post-extraction issues" - Fair
- "Plan generation already works for most recipes; the failures are in execution" - **This is speculative without data**

**Missing Pros:**
- Could be combined with Option 1: plan validation on all platforms, install validation only on passing subset
- URL validation catches 404s, wrong architecture downloads, missing platform variants
- Plan generation validates template substitution, platform detection, download URL construction
- Extremely cheap for high-volume batch validation (pre-filter before expensive installs)

**Why It's a Strawman:**

The design dismisses Option 2 by saying "the failures are in execution" but provides no data. In reality, many failures in batch generation are:
- URL pattern errors (wrong GitHub release asset names)
- Architecture mapping mistakes (arm64 vs aarch64 vs ARM64)
- Missing platform variants in os_mapping
- Template variable typos

These are all caught by plan generation + URL HEAD checks. Only a subset of failures require actual execution (binary won't run, missing shared libs, wrong binary format).

**Better Framing:**

Option 2 shouldn't be "plan-only validation" but "tiered validation: plan generation on all platforms, execution on subset". This is complementary to Option 1, not competing.

### Option 3: Validate in Container Matrix on Linux

**Stated Cons:**
- "QEMU emulation is slow and sometimes unreliable" - Fair
- "Can't validate macOS binaries at all" - Critical flaw
- "Marking everything Linux-only defeats the purpose" - Fair
- "Most Homebrew bottles have macOS variants that would go untested" - Fair

**This is genuinely a weak option.** The cons are fair and the option is correctly rejected.

---

## 4. Unstated Assumptions

### Assumption 1: Batch Workflow Must Be Self-Contained

**Stated where:** Implicit in Job Architecture diagram (Jobs 1-4 all within batch-generate.yml)

**Impact:** Prevents considering PR-based validation (Option 4)

**Challenge:** Once batch creates a PR, that PR is subject to standard PR checks. Why not leverage this?

**Counter-evidence:** The design states "test-changed-recipes.yml already does install testing on Linux+macOS for PRs" but doesn't consider reusing this for batch PRs.

### Assumption 2: Platform Constraints Must Be Written Before PR Creation

**Stated where:** "The merge job writes `supported_os`/`unsupported_platforms` fields for partial-coverage recipes before PR creation" (line 80)

**Impact:** Requires Job 4 to aggregate platform results and modify recipe TOML files before creating PR

**Challenge:** What if platform constraints are written based on test-changed-recipes.yml results instead? The PR CI validates all recipes, fails on unsupported platforms, and merge job updates recipes that had partial failures.

**Why This Matters:** Writing constraints before PR creation means the batch workflow must duplicate all the platform detection logic that test-changed-recipes.yml already has for Linux-only detection (lines 98-110).

### Assumption 3: Auto-Merge Must Happen Immediately

**Stated where:** "Auto-merge with security gates (no `run_command`)" throughout decision rationale

**Impact:** Drives the need for complete validation within batch workflow (can't wait for PR CI)

**Challenge:** GitHub's auto-merge feature can wait for required checks. If test-changed-recipes.yml is required, auto-merge triggers after it passes. No time penalty vs in-workflow validation.

**Why It's Assumed:** The batch design doc (DESIGN-batch-recipe-generation.md lines 246-283) presents auto-merge as "single PR per batch" but doesn't require immediate merge.

### Assumption 4: PR-Time Plan Generation Adds No Value

**Stated where:** "Whether the PR-time plan generation validation adds meaningful signal beyond what test-changed-recipes already provides" (uncertainties, line 152)

**Impact:** Undermines Problem B's importance

**Challenge:** This is confused. test-changed-recipes.yml validates execution (download + install + verify). It does NOT validate plan **diff** against baseline. These are different:
- Execution validation: Does the recipe install successfully?
- Plan diff validation: Did code changes affect plan output unexpectedly?

Plan diff catches regressions in the planner, version resolver, and template engine that might not cause install failures but change behavior.

**Reality:** PR-time plan generation (without diff) is already happening via install. PR-time plan diff is impossible without baseline (R2 gap). These should not be conflated.

### Assumption 5: Golden Baseline Gap Is Critical

**Stated where:** "This means new recipes merge without any plan-level validation, and the first real validation only happens in the nightly run" (line 29)

**Impact:** Elevates Problem B to same priority as Problem A

**Challenge:** What does "plan-level validation" mean here?
- If it means "plan generates successfully": test-changed-recipes.yml already does this via install
- If it means "plan diff against baseline": this is regression detection, not correctness validation

**Reality:** New recipes merge with execution validation but without diff-based regression detection. The nightly run validates against the R2 baseline that was just generated post-merge, which catches regressions in main but not issues in the new recipe itself.

### Assumption 6: Batch Recipes Can't Use Existing Patterns

**Stated where:** Implicit (never discusses reusing test-changed-recipes.yml for batch PRs)

**Impact:** Leads to designing parallel validation infrastructure

**Challenge:** Batch recipes are just recipes. Once in a PR, why should they be validated differently than manually-contributed recipes?

**Counter:** Batch recipes are auto-generated at scale, so manual review isn't feasible. But validation infrastructure can still be shared.

---

## 5. Strawman Detection

### Option 2 Shows Strawman Characteristics

**Definition:** A strawman is an option designed to fail, presented to make the preferred option look better.

**Evidence for Option 2 as strawman:**

1. **Framed as either/or**: "Plan-Only Validation (No Install)" implies install validation is excluded, but it could be tiered (plans first, install second)

2. **Dismissive language**: "URL existence doesn't guarantee the archive contains what we expect" - true but incomplete. URL validation catches many real failures (404s, wrong architecture downloads)

3. **Unsupported claim**: "Plan generation already works for most recipes; the failures are in execution" - no data provided. This is the crux of the dismissal but it's purely speculative.

4. **Missing hybrid consideration**: Why not plan validation on all platforms + install validation on passing subset? This would combine Option 2's cost savings with Option 1's runtime validation.

5. **Evaluation table score**: Option 2 scores "Poor" on three drivers and "Good" on two. This pattern (majority poor) is suspicious when the option has real merit for pre-filtering.

**Why It Matters:**

If plan generation + URL validation can filter out 60% of failures at 1% of the cost, it's worth doing as a pre-filter before install validation. The design dismisses this without analysis.

**What Would Make It Fair:**

- Provide data on failure modes (plan generation errors vs execution errors)
- Acknowledge plan validation as a complement (tiered), not competitor (replacement)
- Evaluate "Plan validation on 5 platforms + install on 2 platforms" as a hybrid option

### Option 3 Is Legitimately Weak

Option 3 (Container Matrix) is correctly identified as inferior. The cons are fair and the rejection is justified. Not a strawman.

---

## 6. Evaluation Matrix Gaps

### Table (lines 138-145) Missing Key Dimensions

**Missing Driver: Consistency with Existing CI**

| Driver | Option 1 | Option 2 | Option 3 | Option 4 (missing) |
|--------|----------|----------|----------|-------------------|
| Consistency with test-changed-recipes.yml | Poor (duplicates) | N/A | Poor | Good (reuses) |

**Missing Driver: Maintainability**

| Driver | Option 1 | Option 2 | Option 3 | Option 4 (missing) |
|--------|----------|----------|----------|-------------------|
| Maintainability | Fair (two parallel systems) | Good (simple) | Poor (QEMU fragility) | Good (single system) |

**Missing Driver: Fast Feedback for Recipe Authors**

| Driver | Option 1 | Option 2 | Option 3 | Option 4 (missing) |
|--------|----------|----------|----------|-------------------|
| Fast feedback | Good (batch cycle) | Good (batch cycle) | Fair (batch cycle) | Fair (waits for PR CI) |

**Why These Matter:**

- Consistency: The design emphasizes "CLI boundary" as important (line 574 in batch doc) but ignores "validation boundary" consistency across batch and manual PRs
- Maintainability: Two parallel multi-platform validation systems (batch-generate.yml Jobs 3-4 AND test-changed-recipes.yml) doubles maintenance burden
- Fast feedback: Option 4's slower feedback is a real tradeoff worth explicitly evaluating

### "Catches Real Failures" Is Vague

The evaluation table says Option 1 is "Good (actual installs)" and Option 2 is "Poor (plans only)". But what percentage of real failures are caught by plan generation vs install execution?

**More Precise Framing:**

| Failure Type | Caught by Plan Gen | Caught by Install | Prevalence |
|--------------|-------------------|------------------|------------|
| URL 404 / wrong arch | Yes (URL HEAD) | Yes | High (common in new recipes) |
| Template variable typo | Yes | Yes | Medium |
| Missing platform variant | Yes | No (platform not tested) | Medium |
| Binary wrong format | No | Yes | Low (rare with good URL validation) |
| Missing shared libs | No | Yes | Medium (glibc vs musl) |
| Binary doesn't execute | No | Yes | Low (rare in deterministic recipes) |

**Without This Data:** The "catches real failures" comparison is hand-wavy.

---

## 7. Problem B (PR-Time Golden Baseline) Deep Dive

### The Gap Is Overstated

**What the design claims:**
> "New recipes merge without any plan-level validation, and the first real validation only happens in the nightly run" (line 29)

**What actually happens:**

1. **PR time**: test-changed-recipes.yml runs `tsuku install --force --recipe <path>` (line 179)
2. **Install internals**: This calls planner, generates plan, validates schema, downloads artifacts, extracts, installs
3. **Result**: Plan generation is validated (recipe produces valid plan)

**What's NOT happening:**

1. **Plan diff validation**: Comparing generated plan against R2 baseline
2. **Regression detection**: Catching unintended plan changes from code modifications

**The Real Gap:**

New recipes have execution validation but no regression detection. This is a smaller gap than "no plan-level validation" suggests.

### Proposed Solution Misses the Point

The design proposes:
> "Add a step to validate-golden-recipes.yml that generates plans (without comparing to a baseline) for new registry recipes" (line 80)

**But:**
- Plan generation without comparison is already happening via test-changed-recipes.yml install
- Adding another plan generation step (without comparison) duplicates existing validation
- The unsolved problem is **what to compare against** for new recipes (no baseline exists)

**Better Solution for Problem B:**

1. **Accept the gap**: New recipes can't have diff validation until post-merge baseline exists
2. **Mitigate**: validate-golden-execution.yml generates plans at PR time and **saves them as artifacts** (not compared)
3. **Post-merge**: publish-golden-to-r2.yml compares new R2 files against PR artifacts to catch generation drift
4. **Nightly**: Full validation against R2 catches regressions in main

**Why This Is Better:**
- Acknowledges that PR-time diff is impossible without baseline (no pretending)
- Captures PR-time plan for post-merge comparison (catches drift)
- Doesn't duplicate test-changed-recipes.yml execution validation

---

## 8. Decision Drivers Assessment

### "macOS CI budget" (line 46)

**Good driver.** Well-quantified (1000 min/week, 10x cost). Directly impacts feasibility.

**But:** Missing analysis of how test-changed-recipes.yml already consumes this budget. If batch PRs trigger test-changed-recipes.yml (Option 4), there's no additional macOS cost - just shifted timing.

### "Progressive savings" (line 47)

**Good driver.** "Most failures are platform-independent" is a reasonable heuristic.

**But:** Unquantified. What percentage? The batch doc references "85-90% Homebrew deterministic success rate" (line 336) but doesn't break down failure modes by platform-dependence.

**Data needed:**
- Of Linux validation failures, what % would also fail on macOS?
- Of Linux validation passes, what % fail on macOS?

Without this, "progressive saves ~80%" (line 86) is speculative.

### "Partial coverage acceptable" (line 48)

**Good driver.** Better to merge Linux-only recipes than discard them.

**But:** This assumes recipes claim all-platform support by default and need pruning. An alternative is claiming only validated platforms (conservative) and expanding later (additive).

**Tradeoff not discussed:**
- **Optimistic (current)**: Claim all, prune failures → users may hit unsupported platforms
- **Conservative (alternative)**: Claim only validated, expand incrementally → users see fewer recipes initially

### "No golden baseline for new recipes" (line 49)

**Weak driver.** As discussed in section 7, this conflates execution validation (already happening) with diff validation (impossible without baseline).

**Should be reframed:**
- "New recipes lack regression detection baseline at PR time, but execution validation covers correctness"

### "CLI boundary" (line 50)

**Good driver.** Batch pipeline should exercise user code paths.

**But:** This same principle should apply to validation. If test-changed-recipes.yml validates manual PRs, shouldn't batch PRs use the same validation? Selective application of principle.

---

## 9. Existing Infrastructure Analysis

### test-changed-recipes.yml Already Does Multi-Platform

**What it does:**
- Detects changed recipes (line 30-147)
- Validates each on Linux in matrix (line 150-180)
- Validates macOS-compatible recipes aggregated (line 182-236)
- Skips library recipes, system deps, execution-excluded (lines 78-96)
- Detects Linux-only recipes and skips macOS (lines 98-110)

**Why this matters:**

The design proposes building Jobs 3-4 in batch-generate.yml to do multi-platform validation. But test-changed-recipes.yml already has this infrastructure for PRs.

**Integration path:**
1. Batch merge job creates PR with passing recipes
2. PR triggers test-changed-recipes.yml (already configured for `recipes/**/*.toml`)
3. test-changed-recipes.yml validates Linux + macOS
4. Auto-merge waits for test-changed-recipes.yml to pass
5. On pass: PR merges
6. On fail: PR requires review (could be platform constraint issue, could be recipe bug)

**Cost comparison:**
- **Option 1**: Validate in batch workflow, create PR, merge immediately
  - macOS cost: X minutes during batch run
- **Option 4**: Create PR, validate via test-changed-recipes.yml, auto-merge after pass
  - macOS cost: X minutes during PR CI
  - Total cost: Same X minutes, different timing

**Timing comparison:**
- **Option 1**: Batch workflow completes → PR created → immediate merge → ~10 min slower batch (macOS validation)
- **Option 4**: Batch workflow completes → PR created → test-changed-recipes.yml runs → auto-merge → ~10 min slower PR lifecycle

**Why Option 1 might still be preferred:**
- Faster batch cycle (no waiting for PR CI)
- Recipe failures are caught before PR creation (cleaner PR queue)
- Platform results are available in batch artifacts for analysis

**But this should be explicit tradeoff, not unstated assumption.**

### validate-golden-execution.yml Already Does R2 Integration

**What it does:**
- R2 health check (lines 18-47)
- Downloads golden files from R2 for registry recipes (via TSUKU_GOLDEN_SOURCE=r2)
- Validates execution (install --plan) against R2 baseline
- Skips when R2 unavailable or golden files don't exist

**What it doesn't do:**
- Generate plans for new recipes at PR time (design proposes adding this)

**Why the proposed addition is weak:**

The design says:
> "Add a step to validate-golden-recipes.yml that generates plans (without comparing to a baseline) for new registry recipes" (line 80)

But validate-golden-recipes.yml is about **comparing** generated plans to baselines (line 202: `validate-golden.sh`). Adding generation-without-comparison violates the workflow's purpose.

**Better home for this:**

If you want to generate plans at PR time for new recipes:
1. Add to test-changed-recipes.yml (alongside install testing)
2. Or add a new workflow: generate-pr-plans.yml
3. Don't overload validate-golden-recipes.yml with non-validation tasks

---

## 10. Recommendations

### Recommendation 1: Split Into Two Designs

**Design A: Batch Multi-Platform Validation**
- Focus: Adding platform validation to batch-generate.yml OR reusing test-changed-recipes.yml for batch PRs
- Options: Current Option 1 (in-workflow) vs new Option 4 (PR CI) vs hybrid
- Decision drivers: Cost, speed, consistency, maintainability

**Design B: PR-Time Plan Capture for New Registry Recipes**
- Focus: Generating and saving PR-time plans for post-merge drift detection
- Options: Extend test-changed-recipes.yml vs new workflow vs accept gap
- Decision drivers: Regression detection value, artifact storage, CI complexity

**Why split:**
- Different problems with different stakeholders
- Problem B is much lower priority (regression detection vs correctness validation)
- Solutions are independent (can do A without B, or B without A)
- Combining them creates false coupling

### Recommendation 2: Add Option 4 to Design A

**Option 4: Batch PR Triggers test-changed-recipes.yml**

Batch merge job creates PR with passing recipes. test-changed-recipes.yml validates on Linux + macOS. Auto-merge waits for PR checks.

**Evaluate against:**
- Speed (slower PR lifecycle but same total cost)
- Consistency (reuses existing validation for manual and batch PRs)
- Maintainability (single validation system)
- Flexibility (platform failures require manual review, which is good for batch quality)

### Recommendation 3: Reframe Option 2

**From:** "Plan-Only Validation (No Install)"

**To:** "Tiered Validation (Plan + URL Check as Pre-Filter)"

**Hybrid approach:**
1. Generate plans on all 5 platforms (cheap, catches URL/template errors)
2. Validate URLs with HEAD requests (catches 404s, wrong arch downloads)
3. Install validation only on subset (Linux x86_64 + macOS arm64)

**Why:**
- Leverages cheap validation for expensive pre-filtering
- Catches majority of failures at 1% of cost
- Progressive within a tier (plan all platforms → install subset)

### Recommendation 4: Provide Data on Failure Modes

**Before finalizing decision, gather:**
1. Historical failure data from test-changed-recipes.yml (Linux vs macOS failure rates)
2. Batch generation failure categories from existing test runs
3. Estimated percentage of failures caught by plan generation vs install execution

**Use this to:**
- Validate "progressive saves ~80%" claim
- Determine if tiered validation (plan all, install subset) is worthwhile
- Quantify Option 2's "plan generation already works for most recipes" dismissal

### Recommendation 5: Clarify Problem B's Scope

**Current (vague):**
> "New recipes merge without any plan-level validation"

**Clearer:**
> "New registry recipes merge without diff-based regression detection because R2 baselines don't exist at PR time. However, plan correctness is validated via test-changed-recipes.yml execution testing."

**Then ask:**
- Is diff-based regression detection critical for new recipes? (No baseline to regress from)
- Or is this only needed for **changes** to existing recipes? (Existing baseline in R2)

**If only needed for changes:**
- Problem B is solved (validate-golden-execution.yml already does this)
- The "gap" is not a gap, just a limitation (can't detect regression without baseline)

### Recommendation 6: Consider Consistency as Primary Driver

**The design optimizes for:**
- Cost (macOS budget)
- Speed (batch cycle time)
- Coverage (multi-platform validation)

**Missing driver:**
- Consistency (batch and manual PRs use same validation)

**Why consistency matters:**
1. Single system to maintain (test-changed-recipes.yml)
2. Same validation experience for contributors (manual or batch)
3. Less code duplication (platform detection, exclusions, matrix building)
4. Easier to reason about ("recipes in PRs are validated by test-changed-recipes.yml, period")

**Tradeoff:**
- Slower batch cycle (waits for PR CI)
- Less control over batch-specific validation logic

**Worth explicit evaluation.**

---

## 11. Summary of Gaps

| Issue | Impact | Severity |
|-------|--------|----------|
| Two problems conflated | Mixes critical (Problem A) with minor (Problem B) | High |
| Missing Option 4 (PR CI reuse) | Ignores existing infrastructure | High |
| Option 2 strawman | Dismisses cost-effective pre-filtering | Medium |
| No failure mode data | "Progressive saves 80%" is speculative | Medium |
| Problem B overstated | "No plan-level validation" is misleading | Medium |
| Consistency driver missing | Doesn't evaluate single vs dual validation systems | Medium |
| Unstated batch self-containment assumption | Precludes PR-based validation | High |

---

## 12. Verdict by Question

### 1. Is the problem statement specific enough to evaluate solutions against?

**Problem A (multi-platform batch validation):** Yes, but over-constrained by assuming batch-workflow-only validation.

**Problem B (PR-time golden baseline):** No. Conflates execution validation (happening) with diff validation (impossible without baseline).

### 2. Are there missing alternatives we should consider?

**Yes.** Critical omission: Option 4 (batch PR triggers test-changed-recipes.yml) leverages existing infrastructure.

Also missing: Tiered validation (plan generation as pre-filter before install validation).

### 3. Are the pros/cons for each option fair and complete?

**Mostly fair for Option 1.** Missing cons around duplication with test-changed-recipes.yml.

**Unfair for Option 2.** Dismissive framing and unsupported claims. Missing hybrid consideration.

**Fair for Option 3.** Correctly identified as weak.

### 4. Are there unstated assumptions that need to be explicit?

**Yes, six major ones:**
1. Batch workflow must be self-contained (precludes PR CI reuse)
2. Platform constraints must be written before PR creation (couples constraint derivation to batch workflow)
3. Auto-merge must happen immediately (ignores GitHub's auto-merge with required checks)
4. PR-time plan generation adds no value (undervalues regression detection)
5. Golden baseline gap is critical (overstates Problem B)
6. Batch recipes can't use existing patterns (ignores test-changed-recipes.yml for batch PRs)

### 5. Is any option a strawman (designed to fail)?

**Yes.** Option 2 shows strawman characteristics:
- Framed as either/or (plan vs install) instead of tiered (plan then install)
- Dismissive language without data ("failures are in execution")
- Missing hybrid consideration (plan pre-filter + install validation)

**Recommendation:** Reframe as "Tiered Validation" and evaluate hybrid approach.

---

## Appendix: Batch Design Doc Context

### Jobs 3-4 Are Already Specified

From DESIGN-batch-recipe-generation.md lines 410-447:

**Job 3: validate-platforms**
- For each Linux x86_64-passing recipe
- Test on linux-arm64, linux-musl, darwin-arm64, darwin-x86_64
- Platform jobs NEVER modify recipe files (line 432)
- Output: per-recipe, per-platform pass/fail artifacts

**Job 4: merge**
- Collect full result matrix from all platform jobs
- For partial-coverage recipes: derive platform constraints, write to recipe TOML (lines 436-440)
- Exclude recipes with run_command
- Create PR with batch_id in commit message

**This design doc implements Jobs 3-4.** The question is whether this is the right implementation or if Option 4 (PR CI reuse) is better.

### Progressive Validation Is Already Decided

From DESIGN-batch-recipe-generation.md Decision 2A (lines 201-213):

Progressive validation (Linux first, macOS on pass) was chosen over:
- Full matrix (too expensive)
- Linux-only with periodic sweeps (macOS failures discovered too late)

**This design inherits that decision.** But the batch doc's Decision 2A doesn't consider Option 4 (PR CI reuse) either.

### The Real Question

Is the batch design doc's Job 3-4 architecture the right approach, or should batch PRs leverage test-changed-recipes.yml instead?

**This design doc doesn't ask that question.**

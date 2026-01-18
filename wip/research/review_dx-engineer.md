# DX Review: Recipe Registry Separation Design

**Reviewer Role:** Developer Experience (DX) Engineer  
**Focus Areas:** Contributor workflow, local development, debugging, documentation clarity  
**Date:** 2026-01-18

---

## Executive Summary

This design introduces significant friction into the contributor experience through split mental models, hidden dependencies, and incomplete debugging tools. While the decision to separate critical from community recipes is sound, the implementation creates a labyrinth for contributors to navigate.

**Top 3-5 Risks:**
1. **Silent recipe loss on first install** - New contributors won't detect that community recipes are unavailable offline
2. **Cryptic split-directory mental model** - Contributors must internalize "critical = bootstrap, community = registry" but it's never explicitly modeled in the codebase  
3. **Nightly validation introduces 24-hour blind spots** - Community recipe breakage from code changes goes undetected until next morning
4. **testdata/recipes duplication maintenance burden** - Duplicating integration test recipes creates two sources of truth that will inevitably drift
5. **Unfinished critical recipes analysis** - Estimate is 15-20 but "needs validation via actual dependency graph analysis" leaves critical path undefined

---

## Detailed Analysis

### 1. Contributor Complexity: Adding a New Recipe

**Current workflow (simplified):**
1. `git clone` repo
2. `go build -o tsuku ./cmd/tsuku`
3. `./tsuku install <tool>` - works immediately
4. Create recipe in `internal/recipe/recipes/{letter}/{tool}.toml`
5. Tests run automatically

**Proposed workflow (mental model required):**
1. `git clone` repo
2. Must decide: Is this recipe "critical" or "community"?
   - If critical: `internal/recipe/recipes/{letter}/{tool}.toml` (embedded)
   - If community: `recipes/{letter}/{tool}.toml` (registry fetch)
3. For community recipes, contributor discovers at test time that their recipe isn't available locally - requires understanding that recipes/ is fetched from registry
4. After merge, wait 24 hours for nightly validation to catch any issues

**DX Problems:**

- **Implicit categorization rule**: The design says "location-based" but never explains *when* a recipe should be critical vs community in contributor-friendly terms. The answer (lines 69-94) requires reading through action code and dependency graphs.
  
- **No visual cue in directory structure**: Unlike `internal/recipe/recipes/` which screams "embedded," `recipes/` doesn't clearly signal "fetched from registry." A contributor might add a recipe to `recipes/` and be shocked when it's not available during `go test`.

- **Sandbox testing complexity**: The CONTRIBUTING guide mentions `--sandbox` flag but doesn't explain that in the proposed split, community recipes might fail sandbox tests if they fail to fetch from GitHub.

**Concrete scenario:** New contributor adds `ruff` recipe to `recipes/r/ruff.toml`. Runs `go test ./...` locally - passes (recipes loaded from directory). Creates PR. CI fails because recipe isn't in `internal/recipe/recipes/`. Contributor is confused: "But my local test passed!"

### 2. Local Development: Testing Without R2 Access

**Gap: Community recipes can't be tested locally without network access**

The design assumes community recipes are fetched from GitHub registry (lines 584-587). For a contributor without network access (airplane, poor connectivity, enterprise proxy), this is broken.

```
User requests recipe "fzf" (community)
    ↓
Loader checks local, embedded → miss
    ↓
Fetches from GitHub registry → NETWORK ERROR
```

**Mitigations mentioned:**
- Local override in `$TSUKU_HOME/recipes/` (lines 805-806) - but requires manual steps beyond just running tests
- Cache fallback - but only works if recipe was previously fetched

**Missing from design:**
- How does `go test ./...` work for community recipes? Do tests skip network-dependent recipes?
- Can contributors run `go test` offline at all?
- What about contributors in corporations with proxy-blocking GitHub raw URLs?

**DX Risk:** Contributors can't reliably test community recipes locally. This violates the "no system dependencies" philosophy - now you need "GitHub internet access."

### 3. Debugging: When a Recipe Breaks

**Scenario 1: Community recipe fails to fetch**
```
$ ./tsuku install fzf
Error: failed to fetch recipe from registry: DNS timeout
```
Contributor's first question: "Is the recipe missing? Is GitHub down? Is my network broken?"

The error message doesn't distinguish between:
- Recipe doesn't exist in registry
- GitHub is down
- Network is unavailable
- Rate limit exceeded (ErrTypeRateLimit is defined but different from connection error)

**Debugging workflow gap:** When a community recipe is broken (e.g., download URL changed), how does a contributor diagnose it?

1. Recipe works locally (cached)
2. PR passes local tests
3. Nightly fails (24 hours later)
4. Issue created with vague "broken recipes" list
5. Contributor must manually check each recipe's download URLs

No tooling is proposed to:
- Compare recipe against golden file to spot differences
- Validate recipe downloads without installation
- Dry-run recipe generation to catch parse errors

**Contrast with critical recipes:** Critical recipes are validated on every code change (line 634), so breakage is caught immediately. Community recipes are validated only when:
- The recipe file itself changes
- Nightly happens to run
- A code change happens to affect plan generation

**Missing:** A `tsuku validate <recipe>` command that checks:
- TOML parses correctly
- Download URLs are reachable
- Recipe golden file matches current generation
- All references resolve

### 4. Mental Model: Critical/Community/Testdata Split

The design introduces **three recipe locations** (lines 541-544):
1. `internal/recipe/recipes/` - Critical (embedded)
2. `recipes/` - Community (fetched)
3. `testdata/recipes/` - Integration tests (embedded)

**Cognitive load for contributors:**

- Why are there three locations?
- How do I know which one to use?
- If I want to move a recipe from community to critical, what breaks?
- Why are some recipes in testdata/ duplicated from recipes/?

**Unclear mental model:**

The design says "location-based categorization is simplest" (line 439) but there's nothing simple about explaining why `testdata/recipes/netlify-cli.toml` exists when `recipes/n/netlify-cli.toml` might also exist.

The rationale (lines 451-455) explains it's for "feature coverage recipes that aren't action dependencies" - but a contributor looking at the code won't see this explanation. They'll see files duplicated across three directories.

**Documentation burden:** CONTRIBUTING.md will need extensive new sections:
- "Recipe categorization: How to choose critical vs community"
- "testdata/recipes/: When to create test-only variants"
- "Recipe relocation: Moving between categories"

None of this is written yet (lines 751-752 say "requires separate tactical design").

### 5. Critical Gaps

#### Gap 5.1: Critical Recipe Analysis Incomplete

Lines 69-94 list action dependencies but:
- Missing: Recipes that depend on `nix-portable` (currently special-cased, line 90)
- Missing: Recipes that depend on actions that themselves have dependencies
- Known limitation: Dependencies() infrastructure has gaps (line 92, issue #644)
- Result: "Estimated 15-20 (needs validation)" - **undefined scope**

**DX Impact:** When a contributor adds a new action or modifies action dependencies, there's no automated way to validate that critical recipes are correctly categorized. Manual review is required but no checklist exists.

#### Gap 5.2: Nightly Validation Creates Uncertainty

Line 639-643 says:
- Nightly runs validate community recipes
- Creates GitHub issue on failure

**Problems:**
- "Creates GitHub issue" - singular. What if 10 recipes fail? Will there be 10 issues or 1 mega-issue?
- No SLA. If nightly fails at 2am UTC, when is the issue triaged?
- No rollback path. If nightly reveals a broken recipe, how does the community know not to use it?
- No contributor notification. How does the recipe maintainer know their recipe broke?

**Better approach:** Run nightly validation as *scheduled CI check* not an issue creator.

#### Gap 5.3: Cache Policy is Vague

Lines 739-749 define cache implementation but:
- TTL is configurable but no guidance on what it should be
- "Cache until clear" semantics aren't tested
- No way for users to know if a cached recipe is stale
- Missing: What happens if user has version X cached, then pushes Y? Does cache priority win?

#### Gap 5.4: Registry Availability Failure Mode

Lines 422-427 mention uncertainties:
- "When GitHub is unavailable, what should happen?"
- Three options listed but none chosen
- Currently (line 109-110 in registry.go): Network errors wrapped but not gracefully degraded

**Test case missing:** What happens when:
1. Contributor has `go build` locally (no embedded recipes)
2. Runs `go test` with no network
3. Tries to load a community recipe

This will fail, but the error message won't explain why.

#### Gap 5.5: Golden File Migration Path Undefined

Lines 680-682 say:
- Community golden files go to R2
- "Requires separate tactical design"

But:
- How are existing community golden files migrated?
- What happens to git history of golden files?
- How do contributors access R2 during local development? (Gap 5.2 repeated)
- What if R2 is down during CI? (Fallback behavior mentioned line 491 but not specified)

---

## Specific Concerns

### Concern A: Embed Directive Fragility

The current embed directive (line 13, embedded.go):
```go
//go:embed recipes/*/*.toml
var embeddedRecipes embed.FS
```

After separation, the stage 3 proposal (lines 702-707) adds:
```go
//go:embed recipes/*/*.toml
//go:embed ../../../testdata/recipes/*.toml
var embeddedRecipes embed.FS
```

**Problems:**
1. **Relative path fragility**: `../../../testdata/recipes/` is fragile to refactoring. If someone moves embedded.go or recipes/, this silently fails to embed.
2. **No validation at build time**: If testdata/recipes/ doesn't exist or path is wrong, `go build` still succeeds but recipes are missing.
3. **Directory structure hidden**: Contributors won't understand that testdata/recipes are embedded. They'll think they're only used in tests.

**Recommendation:** Add a build time check that verifies:
- All critical recipes are in `internal/recipe/recipes/`
- No recipes are in both `internal/recipe/recipes/` and `recipes/`
- All testdata/recipes are listed in embed directive

### Concern B: Validation Workflow Complexity

The design creates **4 different validation workflows** (lines 610-637):
1. test-changed-recipes.yml - Recipe changes
2. validate-golden-recipes.yml - Golden file presence
3. validate-golden-code.yml - Code changes (critical only)
4. validate-golden-execution.yml - Execution
5. (New) nightly-community-validation.yml

A contributor PR touching both a recipe and code will trigger:
- Workflow 1 (changed recipe)
- Workflow 2 (golden files)
- Workflow 3 (code change, but only critical recipes)
- Workflow 4 (golden files changed)

**Debugging when CI fails:** Which workflow failed? Why? The PR page becomes unreadable with 5 parallel workflows.

**Better approach:** Consolidate into 2 workflows:
- "Fast track": Recipe-only changes (plan + execution)
- "Full validation": Code changes (all critical recipes)

### Concern C: "Stage 7" R2 Implementation Undefined

The design punts R2 implementation to a separate design (lines 762-776) but:
- Blocks full validation of community recipes on achieving Stage 7
- Contributors can't test golden file storage locally
- Authentication between GitHub Actions and R2 is undefined (line 770 mentions "OIDC or API tokens" but no choice made)
- Cost monitoring mentioned (line 774) but not budgeted

**DX Impact:** Stages 1-6 complete but community recipe validation stays broken until Stage 7. Nightly validation runs but golden files don't upload, making validation useless.

### Concern D: Docker Development Not Mentioned

CONTRIBUTING.md mentions Docker development (lines 90-91 in CONTRIBUTING) but the design document has no mention of how Docker development workflow changes. 

If a contributor uses `./docker-dev.sh shell`:
1. Are community recipes available in container?
2. Does container have GitHub access to fetch recipes?
3. Does container share cache with host?

Unclear.

---

## Positive Aspects (for balance)

1. **Existing loader infrastructure works**: The priority chain (lines 73-115 in loader.go) already supports the split. No radical changes needed.

2. **Location-based is better than metadata**: Explicit directory structure is better than adding a `critical = true` field that contributors forget.

3. **Nightly validation catches drift**: Even with 24-hour delay, detecting that community recipes break from external factors (URL changes) is valuable.

4. **testdata/recipes isolation is good**: Test recipes are explicitly separate from production, preventing accidental test-only recipes from shipping.

---

## Recommendations for Improvement

### Immediate (Design Phase)

1. **Define critical recipes list explicitly**: Complete the dependency analysis (lines 94). Create a `CRITICAL_RECIPES.md` that lists all 15-20 critical recipes with rationale. Include in CONTRIBUTING.

2. **Add recipe validation command**: Propose `tsuku validate <recipe>` that checks:
   - TOML parses
   - Download URLs are reachable
   - Golden file matches generation
   - All dependencies are satisfied

3. **Clarify cache behavior**: Specify:
   - Default TTL (suggest 7 days, not 24 hours)
   - Behavior when cache is stale but network unavailable
   - How `tsuku update-registry` works
   - What `--force` flag does

4. **Define nightly failure handling**: Specify:
   - How many broken recipes trigger alert?
   - Who gets notified?
   - Rollback procedure if main branch is broken

### During Implementation (Stage 1-4)

1. **Add build-time validation**: Verify:
   - Embed directive paths are correct
   - No recipes exist in multiple locations
   - Critical recipes don't reference community recipes

2. **Update CONTRIBUTING.md with decision trees**:
   - "Should my recipe be critical?" flowchart
   - "My community recipe test passes locally but fails in CI" troubleshooting
   - "My recipe disappeared after merge" debugging guide

3. **Add `tsuku doctor` command** that reports:
   - How many recipes are cached
   - Cache age for each recipe
   - Network connectivity to registry
   - Which recipes are critical vs community

4. **Create DEBUG_RECIPES.md** for contributors:
   - Common recipe failures and causes
   - How to debug registry fetch errors
   - How to inspect generated plans
   - How to test golden files locally

### Post-Implementation (Stage 5+)

1. **Implement R2 early**: Don't leave Stage 7 as undefined "future work." Community recipe validation is broken without it.

2. **Add recipe deprecation workflow**: When moving a recipe from community to critical, or removing it, have a documented process.

3. **Implement cache poisoning detection**: Add hash verification for cached recipes. If a recipe changes unexpectedly, alert the user.

---

## Critical Path Dependencies

The design currently has a gap in the critical path:

```
Stage 1-4 (recipe migration, CI setup, testdata)
    ↓
Nightly validation works but R2 missing
    ↓ (blocked)
Stage 7 (R2 implementation)
    ↓
Full validation with golden files
```

**Recommendation:** Move Stage 7 earlier. Define R2 structure in this design, implement auth in Stage 4.

---

## Conclusion

This design separates critical and community recipes well conceptually, but the implementation exposes a **layered complexity** that will frustrate contributors:

1. **Conceptual**: Where does my recipe go?
2. **Operational**: How do I test it locally?
3. **Debugging**: Why did it fail in CI but work locally?
4. **Infrastructure**: What's the R2 bucket structure?

The design document reads like it was written for architects, not contributors. Every gap should be filled with "contributor onboarding" in mind.

**Estimated contributor confusion cost:** 2-3 hours per new recipe contributor in first month.

---

## Appendix: Test Coverage Map

| Scenario | Tested | Coverage | Risk |
|----------|--------|----------|------|
| Add critical recipe, code works | Yes | PR workflow | Low |
| Add community recipe, code works | Partial | Only if network available | **High** |
| Modify critical recipe | Yes | Plan + Execution | Low |
| Modify community recipe | Yes | Plan + Execution | Low |
| Code change breaks critical recipe | Yes | CI validates immediately | Low |
| Code change breaks community recipe | No | Nightly (24h delay) | **High** |
| Offline development with community recipes | No | Not possible | **High** |
| Validate recipe without installation | No | No tool provided | **Medium** |
| Cache is stale but network down | No | Undefined behavior | **Medium** |
| R2 is down during CI | No | Fallback undefined | **High** |

**Highest risk areas:** Offline development, R2 availability, code change to community recipe detection.


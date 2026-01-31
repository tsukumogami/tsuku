# Architecture Review: DESIGN-batch-platform-validation.md

**Reviewer**: Architecture Analysis
**Date**: 2026-01-31
**Design Status**: Proposed

## Executive Summary

The design is well-structured with clear component boundaries and data flow. Implementation is feasible with existing tooling. Critical gaps identified:

1. **Skipped platform handling** in merge job is underspecified
2. **Platform constraint algorithm** lacks edge case coverage
3. **Retry logic** needs timeout and total duration bounds
4. **Artifact naming collision** risk when platforms are skipped

The implementation phases are correctly sequenced. Recommendations below address all gaps.

---

## 1. Architecture Clarity Assessment

### 1.1 Component Boundaries

**CLEAR**: The design separates concerns cleanly:
- Generation job: validates on single platform, produces candidate recipes
- Platform jobs: validate on specific environments, produce pass/fail results
- Merge job: aggregates results, derives constraints, creates PR

Each component has a single responsibility and well-defined inputs/outputs.

### 1.2 Data Flow

**CLEAR**: Artifact flow is explicit:
```
generation → passing-recipes → platform jobs
platform jobs → validation-results-<platform>.json → merge job
merge job → constrained recipes + failure JSONL → PR
```

**GAP**: What happens if a platform job fails catastrophically (runner crash, workflow error) vs. recipes failing validation? The merge job needs to distinguish between:
- Platform job didn't run (skipped)
- Platform job ran but produced no artifact (crash)
- Platform job ran and produced empty results (no recipes tested)

**RECOMMENDATION**: Add a metadata field to result artifacts:
```json
{
  "schema_version": "1.0",
  "platform": "linux-glibc-arm64",
  "job_status": "completed",
  "recipe_count": 15,
  "results": [...]
}
```

This lets the merge job detect missing vs. empty artifacts.

### 1.3 Integration Points

**CLEAR**: The design integrates with existing patterns:
- Uses established JSONL failure format with `schema_version`
- Follows `{os}-{libc}-{arch}` environment naming
- Shells out to `tsuku install` CLI (same as orchestrator)
- Reuses platform constraint fields from `internal/recipe/platform.go`

**VERIFIED**: Cross-referenced with `internal/batch/orchestrator.go` exit code handling. The platform jobs correctly map exit codes (0=pass, 5=retry, other=fail).

---

## 2. Missing Components and Interfaces

### 2.1 Platform Job Script

**MISSING**: The design describes what platform jobs do but doesn't specify the implementation mechanism.

**OPTIONS**:
1. Bash script embedded in workflow YAML (existing pattern in orchestrator)
2. Go program in `cmd/tsuku-validate-platform/` (better testability)
3. Extend orchestrator with a platform-validation subcommand

**RECOMMENDATION**: Option 3 (extend orchestrator). The orchestrator already has:
- Recipe iteration logic
- Exit code classification
- Retry with exponential backoff
- JSONL output

Add a new mode: `tsuku batch platform-validate --recipes-dir <path> --platform <id> --output <json>`. This reuses existing code and is testable in unit tests.

### 2.2 Merge Job Constraint Writer

**MISSING**: No specification for how the merge job writes constraints to TOML files.

**REQUIREMENTS**:
- Parse existing recipe TOML
- Add/update `supported_os`, `supported_arch`, `supported_libc`, `unsupported_platforms`
- Preserve existing fields and formatting

**RECOMMENDATION**: Use `internal/recipe` package's existing TOML parser. Add a method:
```go
func (r *Recipe) WriteConstraints(constraints PlatformConstraints) error
```

This ensures consistency with how recipes are read and avoids TOML parsing bugs.

### 2.3 Skipped Platform Metadata

**GAP**: When a platform job is skipped (via skip flags), the merge job needs to know:
1. That the platform was intentionally skipped (not a failure)
2. Which platforms were skipped (for logging and failure JSONL)

**RECOMMENDATION**: Add a workflow artifact `skipped-platforms.json` uploaded by a pre-merge step:
```json
{
  "skipped": ["linux-musl-x86_64", "darwin-arm64", "darwin-x86_64"],
  "reason": "skip_macos=true, skip_musl=true"
}
```

The merge job reads this and excludes skipped platforms from constraint derivation.

### 2.4 Execution Exclusions Integration

**GAP**: `test-changed-recipes.yml` has `execution-exclusions.json` for recipes that can't be tested (e.g., require Docker daemon, need AWS credentials). Platform validation jobs need the same mechanism.

**RECOMMENDATION**: Reuse the same exclusions file. Platform jobs download it and skip excluded recipes, logging them as `"status": "excluded"` in results.

---

## 3. Implementation Phase Sequencing

### 3.1 Phase Order

**CORRECT**: The three phases are properly ordered:
1. Platform validation jobs (#1254) - infrastructure first
2. Merge job with constraint writing (#1256) - depends on results
3. PR CI validation (existing) - defense-in-depth

Each phase builds on the previous. No dependency inversions.

### 3.2 Testability

**GAP**: Phase 1 and 2 are hard to test end-to-end without triggering the full batch workflow.

**RECOMMENDATION**: Add a `workflow_dispatch` trigger to `batch-generate.yml` with inputs:
```yaml
inputs:
  test_mode:
    description: 'Run with synthetic test recipes'
    type: boolean
    default: false
  test_recipe_count:
    description: 'Number of test recipes to generate'
    type: number
    default: 5
```

In test mode, the generation job creates minimal synthetic recipes instead of querying ecosystems. This allows testing platform validation and merge logic in isolation.

### 3.3 Rollback Strategy

**MISSING**: If platform validation introduces a critical bug (e.g., merge job writes invalid TOML), how do we roll back?

**RECOMMENDATION**: Add a feature flag in the workflow:
```yaml
env:
  ENABLE_PLATFORM_VALIDATION: true
```

When disabled, the workflow skips platform jobs and merge job uses generation results only (pre-implementation behavior). This provides a quick rollback path.

---

## 4. Platform Constraint Derivation Algorithm

### 4.1 Algorithm Specification

**INCOMPLETE**: The design gives three examples but doesn't specify the full algorithm. Edge cases:

**Case 1**: Passed all tested platforms, but some platforms were skipped
```
Passed: linux-glibc-x86_64, linux-glibc-arm64
Skipped: linux-musl-x86_64, darwin-arm64, darwin-x86_64
```

**Expected behavior**: Write no constraints (default is all-platform support). Don't claim support for untested platforms.

**Current spec**: Silent on this. Could write `supported_os = ["linux"]` incorrectly.

**Case 2**: Passed only the generation platform
```
Passed: linux-glibc-x86_64
Failed: linux-glibc-arm64, linux-musl-x86_64, darwin-arm64, darwin-x86_64
```

**Expected behavior**: `supported_os = ["linux"], supported_arch = ["x86_64"], supported_libc = ["glibc"]`

**Current spec**: Says "still include with restrictive constraints" but doesn't specify what constraints.

**Case 3**: Passed no platforms (generation platform also failed somehow)
```
Failed: linux-glibc-x86_64, linux-glibc-arm64, linux-musl-x86_64, darwin-arm64, darwin-x86_64
```

**Expected behavior**: Exclude from PR entirely.

**Current spec**: Silent. The merge job section says "If passed some platforms" but doesn't handle zero platforms.

### 4.2 Proposed Algorithm

```
Input: platform_results = map[platform_id]bool (pass/fail)
       skipped_platforms = set[platform_id]

1. Filter out skipped platforms from platform_results

2. If no platforms passed:
   → Exclude recipe from PR
   → Log to failure JSONL with category "all_platforms_failed"

3. If all tested platforms passed:
   → No constraints needed (default all-platform)
   → Include in PR as-is

4. If some platforms passed (partial coverage):
   a. Extract dimensions from passed platforms:
      - passed_os = unique OS values
      - passed_arch = unique arch values
      - passed_libc = unique libc values (Linux only)

   b. Check if failures align to a single dimension:
      - If all passed platforms share same OS and all failed platforms have different OS:
        → supported_os = [passed_os]
      - If all passed platforms share same libc and all failed platforms have different libc:
        → supported_libc = [passed_libc]
      - Otherwise:
        → unsupported_platforms = [list of failed platform_ids]

   c. Apply constraints to recipe TOML
   d. Include in PR

5. If recipe has run_command action:
   → Exclude from PR regardless of validation results
   → Log to failure JSONL with category "requires_manual_review"
```

**RECOMMENDATION**: Add this pseudocode to the design doc under "Platform Constraint Derivation".

### 4.3 Constraint Priority

**AMBIGUITY**: When multiple constraint types could express the same restriction, which takes precedence?

Example:
```
Passed: linux-glibc-x86_64, linux-glibc-arm64
Failed: linux-musl-x86_64, darwin-arm64, darwin-x86_64
```

Could write:
- `supported_os = ["linux"], supported_libc = ["glibc"]` (two constraints)
- `unsupported_platforms = ["linux/musl/x86_64", "darwin/arm64", "darwin/x86_64"]` (three exclusions)

**RECOMMENDATION**: Prefer the first (fewer, broader constraints). Add to design:
> Constraint priority: supported_os > supported_libc > supported_arch > unsupported_platforms

---

## 5. Merge Job Skipped Platform Handling

### 5.1 Current Specification

From lines 247-257:
> The merge job runs after all platform jobs complete (including skipped ones). It:
> 1. Downloads all result artifacts from platform jobs

**PROBLEM**: If a platform job is skipped (via `if: !inputs.skip_macos`), it doesn't run at all. GitHub Actions doesn't create artifacts for skipped jobs. The merge job's artifact download will fail if it expects artifacts that don't exist.

### 5.2 Skip Flag Semantics

The design defines three skip flags:
- `skip_arm64`: skips `validate-linux-arm64`
- `skip_musl`: skips `validate-linux-musl`
- `skip_macos`: skips both macOS jobs

**USE CASE**: Budget-constrained runs skip macOS to save CI minutes.

**CONSTRAINT**: The merge job must still produce valid results when some platforms are skipped.

### 5.3 Solutions

**Option A: Conditional Artifact Download**

Merge job uses `continue-on-error: true` for artifact downloads and handles missing artifacts gracefully:
```yaml
- name: Download platform results
  uses: actions/download-artifact@v4
  with:
    pattern: validation-results-*
  continue-on-error: true

- name: Build result matrix
  run: |
    # Check which platforms have results
    # Only include platforms with artifacts in constraint derivation
```

**Option B: Explicit Skipped Platform Artifact**

Pre-merge step uploads a `skipped-platforms.json` artifact listing which platforms were skipped. Merge job reads this and excludes skipped platforms from constraint derivation.

**Option C: Always Upload Empty Results**

Platform jobs run unconditionally but skip validation when skip flag is true, uploading an empty result artifact:
```json
{
  "schema_version": "1.0",
  "platform": "darwin-arm64",
  "job_status": "skipped",
  "recipe_count": 0,
  "results": []
}
```

### 5.4 Recommendation

**Option B** (explicit skipped platform artifact) is cleanest because:
- Merge job gets complete information (which platforms ran, which were skipped)
- No special error handling for missing artifacts
- Skipped platforms can be logged in failure JSONL metadata

Implementation:
```yaml
merge:
  runs-after: [generate, validate-linux-arm64, validate-linux-musl, validate-darwin-arm64, validate-darwin-x86_64]
  steps:
    - name: Record skipped platforms
      run: |
        echo '{"skipped": [' > skipped.json
        if [[ "${{ inputs.skip_arm64 }}" == "true" ]]; then
          echo '"linux-glibc-arm64",' >> skipped.json
        fi
        if [[ "${{ inputs.skip_musl }}" == "true" ]]; then
          echo '"linux-musl-x86_64",' >> skipped.json
        fi
        if [[ "${{ inputs.skip_macos }}" == "true" ]]; then
          echo '"darwin-arm64", "darwin-x86_64",' >> skipped.json
        fi
        echo ']}' >> skipped.json
        sed -i 's/,]/]/' skipped.json  # Remove trailing comma

    - name: Upload skipped platforms
      uses: actions/upload-artifact@v4
      with:
        name: skipped-platforms
        path: skipped.json
```

---

## 6. Retry Logic Specification

### 6.1 Current Spec

From lines 222-223:
> 5 (ExitNetwork): retry up to 3 times with exponential backoff (2s, 4s, 8s)

**GOOD**: This matches the orchestrator's retry pattern for network errors.

### 6.2 Gaps

**Timeout**: No timeout specified for individual install attempts. If `tsuku install` hangs (stuck download, infinite loop), the platform job could run until the 6-hour GitHub Actions job timeout.

**Total Duration**: No bound on total retry time. With 3 retries at 2s, 4s, 8s, a single recipe could take 14s+ just for backoff. For 100 recipes, this is 23 minutes of pure wait time if everything fails network.

**Partial Download Resume**: The design doesn't say whether retries resume partial downloads or start from scratch.

### 6.3 Recommendations

**Add to design**:
> **Retry Configuration:**
> - Maximum attempts: 3 per recipe
> - Backoff sequence: 2s, 4s, 8s
> - Per-attempt timeout: 5 minutes (via `timeout` command)
> - Total retry budget: 15 minutes across all recipes (fail-fast if exceeded)
> - Partial downloads are not resumed (tsuku cleans up and re-downloads)

This prevents runaway retry cycles while still handling transient network failures.

---

## 7. Artifact Naming and Collision

### 7.1 Current Naming

Result artifacts use platform ID in the name: `validation-results-<platform>.json`

Examples:
- `validation-results-linux-glibc-arm64.json`
- `validation-results-darwin-arm64.json`

**GOOD**: Platform ID ensures unique names across jobs.

### 7.2 Collision Risk

If multiple workflow runs execute concurrently (e.g., manual triggers), GitHub Actions scopes artifacts to the workflow run ID. No collision risk.

**VERIFIED**: GitHub Actions artifact names are scoped to `workflow_run_id`, not global.

### 7.3 Artifact Retention

**GAP**: No artifact retention policy specified. By default, GitHub retains artifacts for 90 days.

**RECOMMENDATION**: Add to design:
> Platform validation artifacts are retained for 7 days (same as generation artifacts). This provides sufficient time for failure analysis without consuming storage.

Implementation:
```yaml
- name: Upload validation results
  uses: actions/upload-artifact@v4
  with:
    name: validation-results-${{ env.PLATFORM_ID }}
    path: validation-results.json
    retention-days: 7
```

---

## 8. Error Handling Gaps

### 8.1 Platform Job Failure Modes

**Scenario 1**: Runner is terminated mid-job (spot instance preemption, GitHub service issue)

**Current behavior**: Job fails, no artifact uploaded.

**Impact**: Merge job downloads fail (see section 5).

**Mitigation**: Option B from section 5.3 (skipped-platforms.json) also handles failures if merge job treats "missing artifact" as "platform unavailable".

**Scenario 2**: `tsuku install` crashes with unexpected exit code (e.g., segfault = 139)

**Current behavior**: Classified as fail, logged with exit code 139.

**Impact**: Recipe marked as failed on that platform.

**Mitigation**: Acceptable. Crashes are failures. No special handling needed.

### 8.2 Merge Job Failure Modes

**Scenario 1**: Merge job can't parse a platform result artifact (malformed JSON)

**Current behavior**: Undefined.

**Recommendation**: Add JSON schema validation:
```go
type ValidationResult struct {
    SchemaVersion string `json:"schema_version"`
    Platform      string `json:"platform"`
    JobStatus     string `json:"job_status"`
    RecipeCount   int    `json:"recipe_count"`
    Results       []RecipeResult `json:"results"`
}

func ParseValidationResults(data []byte) (*ValidationResult, error) {
    var result ValidationResult
    if err := json.Unmarshal(data, &result); err != nil {
        return nil, fmt.Errorf("invalid JSON: %w", err)
    }
    if result.SchemaVersion != "1.0" {
        return nil, fmt.Errorf("unsupported schema version: %s", result.SchemaVersion)
    }
    return &result, nil
}
```

On parse failure, log to `$GITHUB_STEP_SUMMARY` and exclude that platform's results (treat as unavailable).

**Scenario 2**: Merge job can't write constraints to a recipe TOML (disk full, permission denied)

**Current behavior**: Undefined.

**Recommendation**: Fail the merge job. Invalid TOML in the PR would break user installs. Better to fail noisily than create a broken PR.

### 8.3 Constraint Derivation Edge Cases

**Case**: All platforms failed except the generation platform, but generation platform passed

**Example**:
```
Passed: linux-glibc-x86_64
Failed: linux-glibc-arm64, linux-musl-x86_64, darwin-arm64, darwin-x86_64
```

**Expected**: Include with restrictive constraints (see section 4.1 Case 2).

**Risk**: If constraint algorithm is buggy, could write invalid combinations like `supported_os = ["linux"], supported_arch = ["arm64"]` (no platforms match both).

**Mitigation**: Add validation step in merge job:
```go
func ValidateConstraints(constraints PlatformConstraints) error {
    // Check that at least one platform matches the constraints
    platforms := []string{"linux-glibc-x86_64", "linux-glibc-arm64", "linux-musl-x86_64", "darwin-arm64", "darwin-x86_64"}
    for _, p := range platforms {
        if constraints.Matches(p) {
            return nil
        }
    }
    return fmt.Errorf("constraints exclude all platforms")
}
```

---

## 9. Performance Considerations

### 9.1 Wall-Clock Time

**Parallelism**: Platform jobs run in parallel. Total wall-clock is max(platform job durations) + merge job duration.

**Estimates** (from design):
- Linux jobs: ~5 minutes for 25 recipes
- macOS jobs: ~10 minutes for 25 recipes (slower runners)
- Merge job: ~2 minutes (artifact download + constraint writing + PR creation)

**Total**: ~12 minutes for a 25-recipe batch (macOS is bottleneck).

**Scaling**: For a 100-recipe batch:
- Linux jobs: ~20 minutes
- macOS jobs: ~40 minutes
- Merge job: ~5 minutes (more recipes to process)

**Total**: ~45 minutes for a 100-recipe batch.

**Concern**: If macOS runners are under heavy load, queue time could add significant delay.

**Mitigation**: Skip flags allow falling back to Linux-only validation when macOS availability is low.

### 9.2 Network Bandwidth

**Download volume**: Each platform job downloads all passing recipes (artifacts from generation job) plus the `tsuku` binary for that platform.

For 25 recipes at ~10MB each (typical Homebrew bottle size):
- Recipe artifacts: 250MB
- `tsuku` binary: ~20MB

**Total per platform**: ~270MB

**All platforms**: 270MB × 4 = 1.08GB

GitHub Actions bandwidth limits: 10GB/month for private repos (not applicable; tsuku is public).

**Concern**: Minimal. Well within limits.

### 9.3 Artifact Storage

**Generation artifacts**: Recipe TOMLs (~2KB each)
**Platform artifacts**: JSON results (~1KB per recipe)

For 100 recipes:
- Generation: 200KB
- Platform results: 100KB × 5 platforms = 500KB

**Total**: ~700KB per batch run.

At 7-day retention and 4 runs per week: 700KB × 4 = 2.8MB.

**Concern**: Negligible.

---

## 10. Maintainability and Extensibility

### 10.1 Adding New Platforms

**Scenario**: Add `freebsd-x86_64` support.

**Required changes**:
1. Add new platform job to workflow
2. Add FreeBSD runner or container
3. Update constraint derivation algorithm to recognize FreeBSD OS
4. Add platform to result matrix in merge job

**Complexity**: Moderate. The design is extensible but not trivial to extend.

**Recommendation**: Extract platform list to a configuration file:
```yaml
platforms:
  - id: linux-glibc-x86_64
    runner: ubuntu-latest
    skip_flag: null
  - id: linux-glibc-arm64
    runner: ubuntu-24.04-arm
    skip_flag: skip_arm64
  - id: linux-musl-x86_64
    runner: ubuntu-latest
    container: alpine:latest
    skip_flag: skip_musl
  - id: darwin-arm64
    runner: macos-14
    skip_flag: skip_macos
  - id: darwin-x86_64
    runner: macos-13
    skip_flag: skip_macos
```

Use a matrix strategy to generate jobs:
```yaml
jobs:
  platform-validate:
    strategy:
      matrix:
        include: ${{ fromJson(file('.github/platform-matrix.json')) }}
    runs-on: ${{ matrix.runner }}
    container: ${{ matrix.container }}
    if: ${{ !inputs[matrix.skip_flag] || matrix.skip_flag == 'null' }}
```

This makes adding platforms a data change, not a code change.

### 10.2 Two Validation Systems

**Concern**: The design acknowledges maintaining two systems (batch platform validation + PR CI `test-changed-recipes.yml`) is a burden.

**Current justification**: "The batch pipeline has fundamentally different needs from PR CI: it must produce per-platform results for constraint writing."

**Future risk**: Changes to validation logic (e.g., new exit code, new exclusion pattern) must be synchronized across both systems.

**Mitigation**: Extract common validation logic to a reusable action:
```yaml
# .github/actions/validate-recipe-platform/action.yml
name: Validate Recipe on Platform
inputs:
  recipe_path:
    required: true
  platform_id:
    required: true
outputs:
  status:
    description: 'pass, fail, or excluded'
  exit_code:
    description: 'tsuku install exit code'
runs:
  using: composite
  steps:
    - name: Check exclusions
      # ...
    - name: Run tsuku install
      # ...
    - name: Classify result
      # ...
```

Both batch workflow and `test-changed-recipes.yml` use this action. Changes to validation logic are centralized.

---

## 11. Integration with Existing Workflows

### 11.1 test-changed-recipes.yml Interaction

**Current behavior**: Triggers on PR when `recipes/**/*.toml` changes.

**After this design**: Batch PRs will include modified recipes, triggering `test-changed-recipes.yml`.

**Overlap**: Both workflows validate on Linux x86_64 and macOS.

**Benefit**: Defense-in-depth. If batch validation has a bug, PR CI catches it.

**Cost**: Duplicate testing. A 25-recipe batch runs validation twice (batch workflow + PR CI).

**Recommendation**: Add a PR label `skip-validation` that disables `test-changed-recipes.yml` when batch PR is created. The batch workflow has already validated, so PR CI is redundant for batch PRs.

### 11.2 publish-golden-to-r2.yml Interaction

**Current behavior**: Runs post-merge when recipes change on main. Generates golden files on 3 platforms and uploads to R2.

**After this design**: Batch PRs include recipes that already passed platform validation.

**Question**: Should `publish-golden-to-r2.yml` skip recipes that have batch-generated golden files?

**Answer**: No. The golden file workflow validates the final merged recipe on the main branch. Batch validation happens pre-merge. If the merge process introduces a bug (e.g., constraint written incorrectly), the golden workflow catches it.

**Recommendation**: Keep `publish-golden-to-r2.yml` unchanged. It serves a different purpose (post-merge regression baseline).

### 11.3 Preflight Job Dependency

From line 317:
> Dependencies: #1252 (preflight job provides recipe list)

**Question**: How does the preflight job provide the recipe list to platform jobs?

**Current spec**: The preflight job (from #1252) validates queue state and determines which recipes to generate. The generation job uses this list.

**Gap**: Platform jobs download the "passing-recipes" artifact from the generation job, not the preflight job.

**Clarification needed**: The dependency is indirect:
1. Preflight determines what to generate
2. Generation creates recipes and produces passing-recipes artifact
3. Platform jobs download passing-recipes

**Recommendation**: Update the design to clarify this is an indirect dependency:
> Dependencies: #1252 (preflight job determines batch scope; platform jobs depend on generation job output, not preflight directly)

---

## 12. Testing Strategy

### 12.1 Unit Testing

**Platform validation script**: If implemented as orchestrator subcommand (section 2.1), test with:
- Mock recipe files (valid, invalid, network-failing)
- Mock `tsuku install` (stub exit codes)
- Verify retry logic, JSON output format

**Merge job constraint derivation**: Test with:
- All platforms pass
- Partial coverage (OS-level, libc-level, arch-level)
- Only generation platform passes
- No platforms pass
- Skipped platforms mixed with passes/fails

### 12.2 Integration Testing

**End-to-end batch run**: Trigger workflow with `test_mode: true` (section 3.2) to:
- Generate synthetic recipes
- Validate on all platforms
- Merge with constraint writing
- Verify PR content

**Platform-specific validation**: For each platform job, manually verify:
- `tsuku install` succeeds on a known-good recipe
- `tsuku install` fails on a known-bad recipe
- Retry logic triggers on network error (ExitNetwork=5)

### 12.3 Regression Testing

**Constraint accuracy**: After merging a batch PR, verify:
- Recipes with `supported_os = ["linux"]` fail on macOS with "platform not supported" error
- Recipes with no constraints install successfully on all platforms

**Failure JSONL**: Parse batch failure JSONL and verify:
- Platform failures are categorized correctly
- Exit codes are logged
- Retry counts are recorded

---

## 13. Documentation Gaps

### 13.1 Operator Runbook

**Missing**: How to diagnose and recover from common failure modes:
- All macOS jobs fail due to runner unavailability
- Merge job writes invalid constraints
- Platform job runs out of disk space mid-batch

**Recommendation**: Add a "Troubleshooting" section to the design with:
- How to inspect platform validation artifacts
- How to manually re-run a failed platform job
- How to exclude a recipe from a batch run
- How to roll back platform validation (feature flag)

### 13.2 Constraint Semantics

**Ambiguity**: What does `supported_os = ["linux"]` mean for a recipe with `os_mapping = ["darwin"]`?

**Expected behavior**: The planner should detect the conflict and reject the recipe as invalid.

**Current spec**: Silent on this edge case.

**Recommendation**: Add validation in merge job:
```go
func ValidateRecipeConstraints(recipe *Recipe) error {
    // Check that constraints are compatible with mappings
    if len(recipe.SupportedOS) > 0 {
        for _, os := range recipe.OSMapping {
            if !contains(recipe.SupportedOS, os) {
                return fmt.Errorf("os_mapping includes %s but supported_os does not", os)
            }
        }
    }
    return nil
}
```

---

## 14. Recommendations Summary

### Critical (Blocking Implementation)

1. **Specify skipped platform handling** (section 5): Use Option B (skipped-platforms.json artifact)
2. **Formalize constraint algorithm** (section 4.2): Add pseudocode to design
3. **Define platform job implementation** (section 2.1): Extend orchestrator with subcommand

### High Priority (Implement in Phase 1)

4. **Add retry timeouts** (section 6.3): 5 minutes per attempt, 15 minutes total budget
5. **Validate constraint consistency** (section 13.2): Check constraints match mappings
6. **Add test mode** (section 3.2): Synthetic recipes for end-to-end testing

### Medium Priority (Implement in Phase 2)

7. **Extract common validation action** (section 10.2): Reuse between batch and PR CI
8. **Add result artifact metadata** (section 1.2): Job status, recipe count
9. **Handle execution exclusions** (section 2.4): Reuse execution-exclusions.json

### Low Priority (Post-Launch)

10. **Platform matrix configuration file** (section 10.1): Data-driven platform list
11. **Skip PR CI for batch PRs** (section 11.1): Add skip-validation label
12. **Operator runbook** (section 13.1): Troubleshooting guide

---

## 15. Conclusion

The design is architecturally sound with clear component boundaries and correct phase sequencing. The main gaps are in edge case handling (skipped platforms, constraint algorithm) and operational concerns (retry timeouts, error recovery).

All identified gaps have concrete solutions. None are fundamental design flaws. Implementation is feasible once the recommendations are addressed.

**Overall assessment**: Ready to implement with the critical recommendations incorporated.

# DESIGN: Golden Plan Testing

## Status

Proposed

## Context and Problem Statement

Tsuku's test suite relies heavily on integration tests that execute full installation workflows. These tests download files from external services (GitHub releases, npm registry, PyPI, crates.io, RubyGems, Homebrew bottles), perform actual installations, and verify the results. While comprehensive, this approach has significant drawbacks:

1. **Non-determinism**: Every test run makes network calls to resolve versions and download artifacts. External service availability, rate limiting, and content changes cause unpredictable failures.

2. **Slow execution**: Builder tests (cargo, pypi, npm, gem) take 30+ minutes each and are relegated to nightly schedules rather than PR validation. The main test suite takes 15+ minutes per matrix combination.

3. **Limited platform coverage**: Only 4 tools are tested on macOS versus 10 on Linux. arm64 Linux has no coverage due to Homebrew limitations.

4. **Flakiness without recourse**: When tests fail due to transient network issues or external service problems, the only option is manual re-run. There is no retry mechanism for rate limit errors or transient failures.

The `tsuku eval` command now supports generating deterministic installation plans from local recipe files (`--recipe`) with cross-platform targeting (`--os`, `--arch`). This creates an opportunity to test plan generation separately from plan execution, enabling:

- Fast, deterministic testing of the planning phase
- Cross-platform plan validation without requiring those platforms
- Separation of "what will be installed" from "can we install it"

### Scope

**In scope:**
- Using golden files to test installation plan generation
- Testing plan structure, step sequences, template expansion
- Cross-platform plan validation (all 4 platform combinations)
- Recipe validation at plan time

**Out of scope:**
- Replacing execution tests entirely (some execution testing remains valuable)
- Mocking external services (adds complexity with diminishing returns)
- Version resolution testing (inherently dynamic)

## Decision Drivers

1. **Determinism**: Tests should produce the same result given the same inputs, regardless of network conditions or external service state.

2. **Speed**: Tests should complete in seconds, not minutes, to enable running on every PR.

3. **Coverage breadth**: Testing should cover all 4 platform combinations (linux/darwin x amd64/arm64) without requiring those platforms.

4. **Maintainability**: Golden files must be easy to update when recipes change intentionally.

5. **Confidence**: Tests should catch real regressions in plan generation logic, not just structural changes.

## Considered Options

### Option 1: Full Golden File Testing

Store complete installation plans as golden files. Tests generate plans from recipes and compare the entire JSON output against stored expectations.

**Approach:**
- Generate plans with `tsuku eval --recipe <recipe> --os <os> --arch <arch>`
- Store complete plan JSON in `testdata/golden/plans/<tool>-<os>-<arch>.json`
- Tests regenerate plans and compare byte-for-byte (after masking unstable fields)

**Pros:**
- Maximum coverage - any change in plan output is detected
- Simple conceptually - compare actual vs expected
- Catches subtle bugs in template expansion, step ordering, dependency resolution

**Cons:**
- High maintenance burden - any intentional recipe change requires golden file updates
- Brittle to format changes - adding new fields breaks all golden files
- Large test fixtures - complete plans can be 100+ KB each
- **Checksum volatility (critical)**: Plan generation requires downloading files to compute checksums. If upstream files change (new releases, republished artifacts), golden file checksums become stale. This is the fundamental tension: checksums are a security feature we want to test, but they make tests non-deterministic.

### Option 2: Structural Assertion Testing

Instead of comparing entire plans, assert specific structural properties: field presence, step sequences, URL patterns, and dependency relationships.

**Approach:**
- Generate plans programmatically in tests
- Assert structural invariants rather than exact values
- Use pattern matching for URLs (contains version, platform variables)

**Pros:**
- Resilient to format evolution - new fields don't break tests
- Focused on behavior - tests what matters rather than everything
- Smaller test code - no external fixture files to manage
- Handles checksum changes gracefully

**Cons:**
- May miss subtle regressions that structural checks don't cover ("unknown unknowns" problem)
- Requires writing specific assertions for each behavior
- Test code is more complex than simple comparison
- Must explicitly enumerate all behaviors worth testing - implicit coverage is lost

### Option 3: Hybrid Approach - Selective Golden Files with Structural Assertions

Use golden files for a small set of representative recipes where complete plan validation is valuable, combined with structural assertions for specific behaviors.

**Approach:**
- Golden files for 3-5 reference recipes (one per major recipe pattern)
- Structural tests for specific behaviors (platform filtering, dependency ordering, URL expansion)
- Scripts to regenerate golden files when intentional changes occur

**Pros:**
- Balance of coverage and maintainability
- Golden files catch unexpected changes in representative cases
- Structural tests cover specific behaviors without brittleness
- Update burden is bounded to a small set of golden files

**Cons:**
- More complex test organization
- Must decide which recipes merit golden files
- Two testing patterns to maintain

## Decision Outcome

**Chosen: Option 3 - Hybrid Approach**

This option provides the best balance of coverage, maintainability, and confidence. The bounded set of golden files catches broad regressions while structural assertions cover specific behaviors without excessive brittleness.

### Rationale

- **Determinism**: Both golden files and structural tests are fully deterministic when using `--recipe` with local files and pinned versions.
- **Speed**: Plan generation takes milliseconds; the test suite will complete in seconds.
- **Coverage breadth**: Cross-platform plans can be generated and validated without those platforms.
- **Maintainability**: Only 3-5 golden files need updating when recipes change, not hundreds.
- **Confidence**: The combination catches both broad regressions (golden files) and specific behavior bugs (structural tests).

### Trade-offs Accepted

- Golden files require manual updates when representative recipes change intentionally
- Structural tests require explicit assertions for each behavior we want to verify
- Some corner cases may only be caught by one testing approach, not both

## Solution Architecture

### Overview

The testing infrastructure adds three components:

1. **Golden file fixtures** in `testdata/golden/plans/` for reference recipes
2. **Plan comparison utilities** in `internal/testutil/` for deterministic comparison
3. **Structural test helpers** for common plan assertions

### Directory Structure

```
testdata/
├── recipes/                    # Existing test recipes
│   ├── tool-a.toml
│   ├── fzf.toml                # New: reference recipe
│   ├── terraform.toml          # New: reference recipe
│   ├── sqlite-source.toml      # Existing: dependency test
│   └── ...
├── golden/
│   └── plans/                  # New: golden plan files
│       ├── fzf-0.46.0-linux-amd64.json
│       ├── fzf-0.46.0-linux-arm64.json
│       ├── fzf-0.46.0-darwin-amd64.json
│       ├── fzf-0.46.0-darwin-arm64.json
│       └── ...
└── states/                     # Existing state fixtures
```

### Reference Recipe Selection

Golden files will be created for recipes representing distinct patterns. Selection criteria:

1. **Pattern coverage**: Each golden file should test a distinct recipe pattern
2. **Version stability**: Recipes should use versions unlikely to be re-released
3. **Low churn**: Avoid recipes that change frequently in the registry
4. **Meaningful complexity**: Include at least one recipe with dependencies

| Recipe | Pattern | Why Selected |
|--------|---------|--------------|
| `fzf` | GitHub archive download | Clean GitHub archive pattern; tests download_file, extract, chmod, install_binaries. Stable releases. |
| `terraform` | Direct URL with checksum URL | Tests version template expansion in URLs and checksum URL resolution. |
| `sqlite-source` | Library with dependencies | Tests dependency tree generation (readline -> ncurses). Already exists in testdata. |

### Checksum Handling Strategy

Checksums present a fundamental tension: they are a security feature we want to test (plans must have valid checksums), but computing real checksums requires network access and makes tests non-deterministic.

**Solution: Deterministic mock downloader using URL hashing.**

For golden file tests, we use a mock downloader that generates deterministic checksums from URLs:

```go
// internal/testutil/mock_downloader.go - deterministic mock for testing
type DeterministicMockDownloader struct{}

func (d *DeterministicMockDownloader) Download(ctx context.Context, url string) (*actions.DownloadResult, error) {
    // Hash URL to generate deterministic checksum
    hash := sha256.Sum256([]byte(url))
    return &actions.DownloadResult{
        AssetPath: filepath.Join(os.TempDir(), "mock-download"),
        Checksum:  hex.EncodeToString(hash[:]),
        Size:      int64(len(url) * 100), // Deterministic size
    }, nil
}
```

This approach is simpler than fixture-based mocking because:
- **No fixture files needed**: Checksums are derived from URLs
- **Deterministic**: Same URL = same checksum every time
- **Testing checksum presence**: Plans still have the Checksum field populated
- **Offline testing**: No network access required
- **Fast execution**: No actual downloads

For structural tests, checksums are simply asserted to be non-empty rather than checking specific values.

**Note**: The mock downloader is internal to the test package and must not be exported for use outside tests.

### Plan Comparison

Plans are compared with unstable fields masked:

```go
// internal/testutil/golden.go
type GoldenComparison struct {
    IgnoreFields []string // e.g., "generated_at"
    IgnoreChecksums bool  // For tests where checksums may change
}

func ComparePlanToGolden(t *testing.T, actual *executor.InstallationPlan, goldenPath string, opts GoldenComparison) {
    // Load golden file
    // Mask unstable fields in both actual and expected
    // Compare and report differences
}
```

Fields always masked:
- `generated_at` - timestamp changes each run
- `recipe_source` - absolute path differs between machines

Fields optionally masked:
- `checksum`, `size` - if source files change upstream

### Structural Test Helpers

```go
// internal/testutil/plan_assertions.go

// AssertStepSequence verifies actions appear in expected order
func AssertStepSequence(t *testing.T, plan *executor.InstallationPlan, expected []string)

// AssertPlatformFiltering verifies steps are filtered correctly
func AssertPlatformFiltering(t *testing.T, plan *executor.InstallationPlan, os, arch string)

// AssertURLContains verifies URL template expansion
func AssertURLContains(t *testing.T, plan *executor.InstallationPlan, stepIndex int, substrings ...string)

// AssertDependencyOrder verifies dependencies are sorted alphabetically
func AssertDependencyOrder(t *testing.T, plan *executor.InstallationPlan)

// AssertPlanInvariants verifies properties that must hold for all valid plans
func AssertPlanInvariants(t *testing.T, plan *executor.InstallationPlan)
// - All download_file steps have non-empty Checksum
// - All steps are primitive actions (no composites)
// - FormatVersion is current
// - Platform matches generation parameters
```

### Test Organization

```go
// internal/executor/plan_golden_test.go

func TestPlanGolden_FZF(t *testing.T) {
    platforms := []struct{ os, arch string }{
        {"linux", "amd64"}, {"linux", "arm64"},
        {"darwin", "amd64"}, {"darwin", "arm64"},
    }
    for _, p := range platforms {
        t.Run(fmt.Sprintf("%s-%s", p.os, p.arch), func(t *testing.T) {
            plan := generatePlanFromRecipe(t, "testdata/recipes/fzf.toml", p.os, p.arch, "0.46.0")
            goldenPath := fmt.Sprintf("testdata/golden/plans/fzf-0.46.0-%s-%s.json", p.os, p.arch)
            testutil.ComparePlanToGolden(t, plan, goldenPath, testutil.GoldenComparison{
                IgnoreFields: []string{"generated_at", "recipe_source"},
            })
        })
    }
}

// internal/executor/plan_structural_test.go

func TestPlanStructure_StepSequence(t *testing.T) {
    plan := generatePlanFromRecipe(t, "testdata/recipes/simple-binary.toml", "linux", "amd64", "1.0.0")
    testutil.AssertStepSequence(t, plan, []string{
        "download_file", "extract", "chmod", "install_binaries",
    })
}

func TestPlanStructure_PlatformFiltering(t *testing.T) {
    // Recipe has linux-only and darwin-only steps
    recipe := "testdata/recipes/platform-conditional.toml"

    linuxPlan := generatePlanFromRecipe(t, recipe, "linux", "amd64", "1.0.0")
    testutil.AssertPlatformFiltering(t, linuxPlan, "linux", "amd64")

    darwinPlan := generatePlanFromRecipe(t, recipe, "darwin", "arm64", "1.0.0")
    testutil.AssertPlatformFiltering(t, darwinPlan, "darwin", "arm64")
}
```

### Golden File Update Workflow

```bash
# Regenerate all golden files (run when recipes change intentionally)
go test -v ./internal/executor -run TestPlanGolden -update

# The -update flag writes actual output as new golden file
```

Implementation in test:

```go
var updateGolden = flag.Bool("update", false, "update golden files")

func TestPlanGolden_FZF(t *testing.T) {
    // ... generate plan ...
    if *updateGolden {
        writeGoldenFile(t, goldenPath, plan)
        return
    }
    testutil.ComparePlanToGolden(t, plan, goldenPath, opts)
}
```

## Implementation Approach

### Phase 1: Test Infrastructure

Add core testing utilities:

1. `internal/testutil/golden.go` - Golden file comparison with field masking
2. `internal/testutil/plan_assertions.go` - Structural assertion helpers
3. `internal/testutil/mock_downloader.go` - Deterministic mock downloader using URL hashing

### Phase 2: Reference Recipes and Golden File Tests

Create reference recipes and implement golden file tests together (tightly coupled):

1. Add `fzf.toml` to `testdata/recipes/` (copy from embedded registry with pinned version)
2. Add `terraform.toml` to `testdata/recipes/`
3. Use existing `sqlite-source.toml` in `testdata/recipes/` for dependency testing
4. Implement `internal/executor/plan_golden_test.go` with `-update` flag
5. Run with `-update` to generate initial golden files for all 4 platforms
6. Commit test and golden files together
7. Document golden file update process

### Phase 3: Structural Tests

Add structural assertion tests:

1. Step sequence verification
2. Platform filtering tests
3. URL template expansion tests
4. Dependency ordering tests
5. Decomposition tests (composite -> primitive actions)
6. Plan invariant tests (checksums present, primitives only, format version)

### Phase 4: CI Integration

Update CI workflows:

1. Add golden file tests to `test.yml`
2. Ensure `-short` mode skips network-dependent tests but runs golden tests
3. Consider making golden tests a required check

## Security Considerations

### Download Verification

**Not applicable to this feature.** Golden plan testing validates plan generation logic but does not download or execute any files. The plans contain checksums that would be verified during actual execution, but the tests themselves do not perform downloads.

### Execution Isolation

**Not applicable to this feature.** Golden plan tests generate JSON output and compare strings. No binaries are executed, no files are written outside the test directory, and no elevated permissions are required.

### Supply Chain Risks

**Minimal impact.** The test recipes in `testdata/recipes/` are copies of registry recipes. If a registry recipe were compromised, the test fixture would need to be updated manually. This is actually a benefit: golden files act as a snapshot of expected behavior, making unexpected changes visible in code review.

The golden files themselves contain URLs and checksums but do not download anything. A malicious golden file could only cause test failures, not actual compromise.

### User Data Exposure

**Not applicable to this feature.** Golden plan tests read recipe files and write JSON comparison output. No user data is accessed, no network requests are made, and no data is transmitted externally.

## Consequences

### Positive

- **Faster feedback loop**: Plan generation tests complete in seconds, enabling them to run on every PR
- **Deterministic tests**: No more failures due to GitHub rate limiting or external service outages
- **Cross-platform confidence**: Can validate darwin/arm64 plans on linux/amd64 CI runners
- **Regression detection**: Golden files catch unexpected changes in plan generation
- **Documentation value**: Golden files serve as examples of expected plan output

### Negative

- **Maintenance overhead**: Golden files must be updated when recipes change intentionally
- **Test fixture management**: More files to track in version control
- **Two testing patterns**: Developers must understand both golden and structural approaches
- **Checksum staleness**: If upstream binaries change (rare but possible), checksums in golden files become stale

### Mitigations

- **Automated updates**: The `-update` flag makes golden file regeneration a single command
- **Clear ownership**: Document which recipes have golden files and why
- **Bounded scope**: Limit golden files to 3-5 representative recipes
- **CI notification**: Tests fail clearly when golden files are stale, with instructions to update

## Plan Validity and Forward Compatibility

Golden plan tests validate that plans are generated correctly, but do not validate that plans can be executed. This is by design - execution involves network access, platform dependencies, and is non-deterministic.

### How Plan Validity Is Maintained

**1. Plan Format Version (`format_version`)**

Plans include a format version that must match the executor's expected version:

```go
const PlanFormatVersion = 3

func ValidatePlan(plan *InstallationPlan) error {
    if plan.FormatVersion != PlanFormatVersion {
        return fmt.Errorf("unsupported plan format version: %d", plan.FormatVersion)
    }
    // ...
}
```

When plan format changes:
- `PlanFormatVersion` is incremented
- Golden files with old format versions fail validation tests
- The `-update` flag regenerates all golden files with the new format
- This is a deliberate breaking change that requires review

**2. Structural Invariant Tests**

`AssertPlanInvariants()` enforces properties that must hold for all valid plans:
- All `download_file` steps have non-empty `Checksum`
- All steps are primitive actions (no composite actions)
- Platform matches generation parameters
- `FormatVersion` is current

These tests catch regressions where generated plans would fail execution validation.

**3. Integration Test Coverage**

The existing integration tests (`test.yml`, `scheduled-tests.yml`) continue to run actual installations. These tests validate end-to-end execution and catch issues that golden tests cannot:
- Network failures
- Binary compatibility
- Verification command success

Golden tests complement but do not replace integration tests.

**4. CI Workflow for Plan Validity**

When tsuku code changes affect plan generation:

1. **Golden tests fail** - detects the change
2. **Developer reviews diff** - ensures change is intentional
3. **Developer runs `-update`** - regenerates golden files
4. **PR includes updated golden files** - change is visible in review
5. **Integration tests run** - validates execution still works

### What Happens When Tsuku Changes

| Change Type | Golden Test Behavior | Action Required |
|-------------|---------------------|-----------------|
| New plan field added | Tests pass (extra fields ignored) | None (backwards compatible) |
| Plan field removed | Tests fail (expected field missing) | Run `-update` |
| Step order changes | Tests fail (sequence mismatch) | Run `-update`, verify intentional |
| New action type | Tests pass (structural tests cover) | Add structural test if needed |
| Format version bump | Tests fail (validation error) | Run `-update`, update all golden files |
| URL template change | Tests fail (URL mismatch) | Run `-update`, verify intentional |

### Out of Scope

Golden plan tests do not validate:
- That downloaded files match checksums (execution validation)
- That installed binaries work (verification command)
- Platform-specific execution behavior (requires actual platform)

These are validated by integration tests, not golden tests

# Maintainer Review: Issue #2132

**Issue**: test: cover near-75% packages (executor, validate, builders, userconfig)
**Focus**: maintainability (clarity, readability, duplication)

## Findings

### 1. BLOCKING: Table-driven test that ignores its own `expected` field

**File**: `internal/executor/executor_test.go:270-352` (`TestShouldExecute`)

This test defines a table with `expected bool` fields for each case, then **does not use them** in the assertion loop. Instead, it dispatches by string-matching on `tt.name`:

```go
if tt.name == "empty when - always execute" || tt.name == "nil when - always execute" {
    if result != tt.expected { ... }
}
if tt.name == "non-matching OS" {
    if result != false { ... }
}
```

The `"matching OS"` case (`expected: true`) is never asserted at all -- if `shouldExecute` returned `false`, the test would silently pass. The `"matching arch"` case is also never checked. The next developer will see the table, trust that all rows are verified, and miss that two positive cases are completely unchecked.

**Fix**: Replace the name-dispatched `if` blocks with a single `if result != tt.expected` assertion. For the platform-dependent cases ("matching OS" hardcodes `linux`), either skip them with `runtime.GOOS` checks or use the actual runtime value in the `when` clause (the "matching arch" case already does this correctly).

### 2. BLOCKING: Grammar error in test assertion bakes wrong expectation into the test name

**File**: `internal/builders/errors_test.go:242-264` (`TestRateLimitError_ShortRetryAfter` and `TestGitHubRateLimitError_ShortRetryAfter`)

Both tests assert:
```go
if !strings.Contains(suggestion, "1 minutes") {
```

The production code in `errors.go:24-27` formats this as `"%d minutes"` with an integer, so for a 30-second `RetryAfter` the output is literally `"1 minutes"` (grammatically incorrect). The test faithfully asserts this, but its name says "ShortRetryAfter" without signaling that it's testing a floor/clamp behavior -- the next developer will read "1 minutes" in the assertion and wonder whether the test is wrong or the production code is wrong. The real problem: the test is documenting a grammar bug as expected behavior.

This is borderline advisory/blocking. The misread risk: the next person who sees "1 minutes" in a test will either (a) "fix" the test to say "1 minute" and break it, or (b) assume the production code is correct and leave the grammar bug. Add a comment explaining the floor behavior and that the grammar is a known wart, or fix the production code to use singular "minute" when the count is 1.

### 3. ADVISORY: Network-dependent tests that silently pass on failure

**File**: `internal/executor/executor_test.go:16-59, 94-188, 797-884`

Multiple tests (`TestResolveVersionWith_CustomSource`, `TestResolveVersion_EmptyConstraint`, `TestResolveVersion_SpecificConstraint`, `TestResolveVersion_UnknownSource`, `TestDryRun_SuccessfulVersionResolution`, `TestDryRun_EmptySteps`, etc.) follow a pattern of:

```go
if err != nil {
    t.Logf("... failed (expected in offline tests): %v", err)
} else {
    // actual assertions
}
```

This means these tests contribute to coverage numbers but their assertions only fire when network is available. In CI behind a firewall or on a developer's laptop without connectivity, they pass without verifying anything. A future developer may rely on these tests as safety nets when refactoring version resolution code, only to find out the tests were never actually asserting.

**Suggestion**: Either mock the network calls so assertions always run, or clearly mark these as integration tests (e.g., with a build tag or `testing.Short()` skip) so the coverage contribution is honest.

### 4. ADVISORY: `TestDryRun_WithDependencies` and `TestDryRun_NoDependencies` don't test DryRun

**File**: `internal/executor/executor_test.go:557-617`

`TestDryRun_WithDependencies` is named as a DryRun test, but it never calls `DryRun()`. It only checks `len(r.Metadata.Dependencies) != 2`, which tests the recipe struct literal, not the executor. Similarly, `TestDryRun_NoDependencies` asserts `len(r.Metadata.Dependencies) != 0` -- testing Go struct initialization. These are test name lies: the names promise DryRun behavior testing but deliver struct literal verification.

### 5. ADVISORY: `TestResourceLimits_Timeout` tests nothing

**File**: `internal/validate/runtime_test.go:324-334`

```go
limits := ResourceLimits{Timeout: 5 * time.Second}
if limits.Timeout != 5*time.Second { ... }
```

This tests that Go struct assignment works. It contributes to coverage stats but verifies no production behavior. The next developer will read the test name and think timeout propagation to container runtimes is covered.

### 6. ADVISORY: Divergent `buildArgs` test styles between Podman and Docker

**File**: `internal/validate/runtime_test.go:232-505`

`TestPodmanRuntime_BuildArgs` and `TestDockerRuntime_BuildArgs_AllOptions` both test `buildArgs()` with full options, using near-identical assertion patterns (scan-for-expected-arg loops). The Podman version checks `--read-only` but not mounts/env/workdir/labels. The Docker version checks all of those. The difference appears accidental rather than intentional -- the next developer won't know if Podman's buildArgs is supposed to handle mounts differently or if the Podman test is just less thorough.

## Overall Assessment

The `validate/runtime_test.go`, `builders/errors_test.go`, and `userconfig/userconfig_test.go` files are well-structured with clear names that match their assertions. The userconfig tests in particular are thorough and readable.

The executor tests have two structural problems: the `TestShouldExecute` table that doesn't use its `expected` values (blocking -- will cause a false sense of coverage), and several network-dependent tests that silently pass without asserting (advisory -- inflates coverage numbers). The DryRun test name lies are minor but worth cleaning up for honesty.

The builders tests are solid except for the "1 minutes" assertion that enshrines a grammar bug without comment.

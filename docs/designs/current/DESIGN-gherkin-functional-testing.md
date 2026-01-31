---
status: Current
problem: >
  Tsuku's test suite covers internal correctness and specific integration dimensions
  but doesn't systematically test user-facing CLI workflows like install-verify-remove
  sequences, catching a different class of bugs around user experience and command
  interaction.
decision: >
  We'll use godog (official Cucumber for Go) with Gherkin feature files in
  test/functional/, isolated via Makefile ldflags that build a test binary with a
  dedicated home directory. Scenarios run sequentially in CI with tag-based filtering.
rationale: >
  godog's maturity and community support outweigh gocuke's ergonomic advantages for a
  small step definition library. ldflags isolation reuses the existing dev workflow
  pattern without adding parallel execution complexity that isn't needed for 30-60
  sequential CLI scenarios.
---

# DESIGN: Gherkin Functional Testing for CLI Workflows

## Status

Current

## Context and Problem Statement

Tsuku's test suite is strong on internal correctness. Unit tests cover Go code at the package level, golden file tests verify execution plan stability across platforms, and shell-based integration tests validate specific dimensions like checksum pinning, library dlopen, and Homebrew recipe builds. These tests answer "does the code work?" and "does this specific matrix of configurations produce correct results?"

What's missing is systematic coverage of user-facing CLI workflows. Nobody's testing "install a tool, verify the binary runs, remove it, verify it's gone." Or "run `tsuku outdated` on a system with three outdated tools and verify the output lists all three." These scenarios catch a different class of bugs: confusing error messages, broken installation flows, regressions in commands that work fine individually but fail in sequence.

The existing integration tests are optimized for depth on specific dimensions (5 Linux distros x N recipes). This design is about breadth: at least one test for every feature tsuku supports, written in a format that describes user interactions rather than implementation details.

Gherkin is the right format for this because the scenarios describe user intent, not implementation. "When I run `tsuku install rg`" reads the same to a contributor who's never seen the codebase. And the format enforces a useful discipline: each scenario has a clear setup, action, and verification. More practically, Gherkin's tag system (`@install`, `@critical`) maps directly to CI filtering, and Scenario Outlines handle parameterized cases without duplicating test logic.

### Target Workflows

The initial set of workflows to cover, roughly in priority order:

1. **Install**: install a tool, verify binary exists and runs
2. **Remove**: remove an installed tool, verify it's gone
3. **List**: install tools, verify they appear in `tsuku list`
4. **Update**: install an older version, update, verify new version
5. **Search**: search for a known tool, verify it appears
6. **Info**: get info about a tool, verify output structure
7. **Versions**: list versions for a tool, verify output
8. **Verify**: install a tool, run `tsuku verify`, check output
9. **Outdated**: install a tool, verify outdated detection
10. **Error handling**: run invalid commands, verify exit codes and error messages
11. **Registry**: update registry, verify recipe availability
12. **Multi-step flows**: install, use, update, verify, remove (lifecycle)

### Scope

**In scope:**
- Gherkin runner selection and integration with Go toolchain
- Step definition library for CLI testing
- Isolation mechanism for safe test execution
- CI integration strategy
- Directory structure for feature files

**Out of scope:**
- Replacing existing unit tests or integration tests
- Performance benchmarking
- GUI or TUI testing
- Cross-platform matrix testing (covered by existing integration tests)

## Decision Drivers

- **Gherkin is the format.** Scenarios should read like user stories. New scenarios should be addable as `.feature` files without code changes.
- **Go-native runner.** The runner must integrate with Go's test toolchain and CI. No Python or Ruby runtime dependencies.
- **Existing isolation mechanism.** The Makefile already supports building tsuku with a custom home directory via ldflags (`-X main.defaultHomeOverride`). The design should evaluate whether to build on this or use a different approach.
- **Complement, don't replace.** Functional tests cover different ground than existing unit and integration tests. The two should coexist without friction.
- **Low ceremony for new scenarios.** Adding a test for a new feature should mean writing a `.feature` file, not modifying Go code or CI configuration.

## Considered Options

This design has three independent decisions: which Gherkin runner to use, how to isolate test environments, and where to put the feature files.

### Decision 1: Gherkin Runner

#### Option 1A: godog

[godog](https://github.com/cucumber/godog) is the official Cucumber implementation for Go, maintained by the Cucumber team. It's been around since 2015, has ~2,600 GitHub stars, and is at v0.15.x.

Step definitions use regex matching:

```go
func iRunCommand(ctx context.Context, cmd string) (context.Context, error) {
    out, err := exec.CommandContext(ctx, "bash", "-c", cmd).CombinedOutput()
    return context.WithValue(ctx, outputKey, string(out)), err
}

func InitializeScenario(ctx *godog.ScenarioContext) {
    ctx.Step(`^I run "([^"]*)"$`, iRunCommand)
    ctx.Step(`^the exit code is (\d+)$`, theExitCodeIs)
}
```

It integrates with `go test` via `TestMain` or `godog.TestSuite`. Output is standard test output, compatible with CI.

**Pros:**
- Official Cucumber project, well-maintained, large community
- Mature (10+ years), well-documented
- Full Gherkin spec support (Backgrounds, Scenario Outlines, tags, hooks)
- Generates step definition stubs from undefined steps

**Cons:**
- Regex-based step matching is verbose compared to method signatures
- Context passing between steps uses `context.Context` values (stringly-typed)
- Doesn't use `*testing.T` directly, so `t.Fatal`/`t.Error` patterns don't apply
- Step definition registration is boilerplate-heavy

#### Option 1B: gocuke

[gocuke](https://github.com/regen-network/gocuke) is a newer Go Gherkin runner that wraps `*testing.T` directly. Step definitions are methods on a test suite struct:

```go
type CLISuite struct {
    gocuke.TestingT
    output string
    exitCode int
}

func (s *CLISuite) IRun(cmd string) {
    out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
    s.output = string(out)
    // ...
}

func (s *CLISuite) TheExitCodeIs(code int64) {
    if s.exitCode != int(code) {
        s.Fatalf("expected exit %d, got %d", code, s.exitCode)
    }
}
```

Step matching is by method name (camelCase of the Gherkin phrase) rather than regex. It uses `*testing.T` under the hood, so standard Go test patterns apply.

**Pros:**
- Native `*testing.T` integration -- `t.Fatal`, `t.Error`, `t.Skip` all work
- No regex boilerplate; method names auto-match step text
- Type-safe parameter extraction (string, int64, etc. from method signatures)
- Suite struct provides natural state management between steps
- Smaller API surface, less to learn

**Cons:**
- Smaller community (~200 stars), maintained by a single organization (Regen Network)
- Less mature (started 2022)
- No step stub generation
- Method naming convention is rigid (must match Gherkin phrases exactly)
- Fewer examples and tutorials available

### Decision 2: Test Isolation

#### Option 2A: Build-time ldflags override (extend Makefile)

The Makefile already builds tsuku with `-X main.defaultHomeOverride=.tsuku-dev`. For functional tests, we'd build with a test-specific path like `.tsuku-test` and clean it between scenarios.

```makefile
test-functional:
	go build -ldflags "-X main.defaultHomeOverride=.tsuku-test" -o tsuku-test ./cmd/tsuku
	go test -v ./test/functional/...
	rm -rf .tsuku-test tsuku-test
```

**Pros:**
- Reuses existing isolation mechanism (proven in dev workflow)
- Binary-level isolation -- no env var to forget or leak
- Test binary name (`tsuku-test`) prevents confusion with real binary
- Clean/predictable behavior

**Cons:**
- Requires a separate build step before running tests
- All scenarios share the same home path (need cleanup between scenarios)
- The home path is relative, so tests must run from repo root
- Can't run multiple scenarios in parallel (shared directory)

#### Option 2B: TSUKU_HOME environment variable per scenario

Set `TSUKU_HOME` to a unique temp directory for each scenario in the step definition:

```go
func aCleanTsukuEnvironment(ctx context.Context) context.Context {
    dir, _ := os.MkdirTemp("", "tsuku-test-*")
    return context.WithValue(ctx, homeKey, dir)
}
```

**Pros:**
- Per-scenario isolation by default (unique temp dirs)
- Scenarios can run in parallel
- No separate build step needed -- uses the standard `go build` binary
- Works from any working directory

**Cons:**
- `TSUKU_HOME` env var takes highest precedence in the config chain, which means tests bypass the ldflags path. This is fine for isolation but uses a different mechanism than what developers see day-to-day.
- Each test must set the env var before subprocess execution
- Temp dir cleanup needs an `After` hook to avoid disk buildup

#### Option 2C: Combined approach

Build with ldflags for a default test home, but allow individual scenarios to override via `TSUKU_HOME` for special cases (like testing paths with spaces).

**Pros:**
- Default case is simple (build once, run tests)
- Special scenarios can customize isolation
- Matches the precedence chain: `TSUKU_HOME` > ldflags > `~/.tsuku`

**Cons:**
- Two isolation mechanisms to understand and maintain
- Potential confusion about which mechanism is active in a given scenario

### Decision 3: Feature File Location

#### Option 3A: `test/functional/` directory

Place feature files alongside step definitions in `test/functional/`:

```
test/
├── functional/
│   ├── features/
│   │   ├── install.feature
│   │   ├── remove.feature
│   │   └── update.feature
│   ├── steps_test.go
│   └── suite_test.go
├── scripts/
│   ├── test-checksum-pinning.sh
│   └── ...
```

**Pros:**
- Groups with existing test infrastructure (`test/scripts/`)
- Clear separation from production code
- `test/` is already an established directory

**Cons:**
- Deeper nesting to reach feature files
- Go test discovery requires `./test/functional/...` path

#### Option 3B: `functional/` at repo root

Top-level directory like other monorepo components:

```
functional/
├── features/
│   ├── install.feature
│   ├── remove.feature
│   └── update.feature
├── steps_test.go
└── suite_test.go
```

**Pros:**
- Visible at top level, signals importance
- Short path for running: `go test ./functional/...`
- Consistent with monorepo pattern (recipes/, website/, telemetry/)

**Cons:**
- Adds another top-level directory to an already full root
- May confuse contributors who expect tests under `test/` or alongside source

## Evaluation Against Decision Drivers

| Driver | 1A: godog | 1B: gocuke |
|--------|-----------|------------|
| Go-native | Yes | Yes |
| Low ceremony | Regex boilerplate | Method naming |
| Gherkin compliance | Full spec | Full spec |
| Community/stability | Strong | Limited |

| Driver | 2A: ldflags | 2B: env var | 2C: combined |
|--------|-------------|-------------|--------------|
| Uses existing mechanism | Extends it | Different mechanism | Both |
| Per-scenario isolation | Needs cleanup | Built-in | Both |
| Parallel-safe | No | Yes | Partially |

| Driver | 3A: test/functional/ | 3B: functional/ |
|--------|---------------------|-----------------|
| Discoverability | Under test/ | Top-level |
| Convention alignment | Existing test/ dir | Monorepo pattern |

### Uncertainties

- godog's `context.Context` approach to state passing may be awkward for tests with many state fields. We haven't written enough step definitions to know if this is a real problem.
- gocuke's method naming convention is untested at scale for CLI testing. Long Gherkin phrases may produce unwieldy method names.
- Whether parallel scenario execution matters in practice depends on how long individual scenarios take. If each scenario is 2-3 seconds, parallel isn't needed.

## Decision Outcome

**Chosen: 1A (godog) + 2A (ldflags isolation) + 3A (test/functional/)**

### Summary

We'll use godog as the Gherkin runner, with build-time ldflags isolation via the Makefile. The test binary is built with `-X main.defaultHomeOverride=.tsuku-test`, giving every scenario a predictable, isolated home directory. Feature files and step definitions live in `test/functional/`.

### Rationale

**godog over gocuke**: The regex boilerplate is a one-time cost -- the step definition library for CLI testing is small (maybe 15-20 steps). godog's maturity matters more: it has IDE support, active maintenance from the Cucumber organization, and enough community usage that edge cases are documented. gocuke's `*testing.T` integration is nicer, but the project's bus factor is a concern for test infrastructure we'll depend on long-term.

**ldflags isolation over env var or combined**: The Makefile already builds tsuku with a custom home via ldflags for development. Extending this pattern to functional tests is natural -- build with `.tsuku-test` instead of `.tsuku-dev`, run scenarios sequentially, clean up between scenarios with a `Before` hook that empties the directory. Parallel execution isn't needed: CLI functional tests are inherently sequential (each scenario is a user story with ordered steps), individual scenarios run in 2-5 seconds, and CI already parallelizes at the job level. Using a single isolation mechanism keeps things simple.

**test/functional/ over root-level**: Tests belong with tests. The `test/` directory already holds `test/scripts/` for shell-based integration tests. Adding `test/functional/` for Gherkin tests groups them naturally.

### Trade-offs Accepted

- godog's `context.Context` state passing is less ergonomic than gocuke's struct fields. We'll mitigate this with a helper struct that wraps common state and provides typed accessors.
- The Makefile build step means `go test ./test/functional/...` alone won't work -- you need to build the binary first. We'll document this and add a `make test-functional` target.
- No parallel scenario execution. If the test suite grows to hundreds of scenarios and runtime becomes a problem, we can revisit with `TSUKU_HOME` per-scenario isolation. For the foreseeable suite size (30-60 scenarios), sequential runs in under 3 minutes.

## Solution Architecture

### Overview

The functional test system has three layers:

1. **Feature files** (`.feature`) describe scenarios in Gherkin. Adding a new test means writing a new file or adding a scenario to an existing file.
2. **Step definitions** (Go) map Gherkin phrases to actions. The library is small and stable -- most new scenarios reuse existing steps.
3. **Test harness** (Go + Makefile) builds the binary, runs godog, and manages cleanup.

### Directory Structure

```
test/
├── functional/
│   ├── features/
│   │   ├── install.feature
│   │   ├── remove.feature
│   │   ├── list.feature
│   │   ├── update.feature
│   │   ├── search.feature
│   │   ├── info.feature
│   │   ├── verify.feature
│   │   └── error_handling.feature
│   ├── steps_cli_test.go       # CLI execution steps (run, exit code, output)
│   ├── steps_env_test.go       # Environment steps (clean home, custom paths)
│   ├── steps_verify_test.go    # Verification steps (file exists, binary runs)
│   └── suite_test.go           # godog TestSuite setup, hooks
├── scripts/
│   └── ... (existing integration test scripts)
```

### Step Definition Library

The core vocabulary for CLI testing is small. These steps cover most scenarios:

**Environment setup:**
- `Given a clean tsuku environment` -- wipes and recreates `.tsuku-test` directory
- `Given tsuku has "{tool}" installed` -- pre-installs a tool for scenarios that need one

**Command execution:**
- `When I run "{command}"` -- executes command via the test binary, captures stdout, stderr, exit code
- `When I run "{command}" with timeout {seconds}` -- same with explicit timeout

**Assertions:**
- `Then the exit code is {code}` -- checks process exit code
- `Then the output contains "{text}"` -- substring match on stdout
- `Then the output does not contain "{text}"` -- negative substring match
- `Then the error output contains "{text}"` -- substring match on stderr
- `Then the file "{path}" exists` -- checks file existence relative to `$TSUKU_HOME`
- `Then the file "{path}" does not exist`
- `Then I can run "{command}"` -- executes a command and asserts exit code 0

**State between steps** is managed through a `testState` struct stored in `context.Context`:

```go
type testState struct {
    homeDir  string   // TSUKU_HOME for this scenario
    binPath  string   // path to test binary
    stdout   string   // last command's stdout
    stderr   string   // last command's stderr
    exitCode int      // last command's exit code
}
```

### Example Feature File

```gherkin
Feature: Install
  Install tools and verify they work.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: Install a tool from a recipe
    When I run "tsuku install serve"
    Then the exit code is 0
    And I can run "serve --version"

  @critical
  Scenario: Install and remove a tool
    When I run "tsuku install serve"
    Then the exit code is 0
    When I run "tsuku remove serve"
    Then the exit code is 0
    And the file "bin/serve" does not exist

  Scenario: Install a tool that doesn't exist
    When I run "tsuku install nonexistent-tool-xyz"
    Then the exit code is 1
    And the error output contains "not found"
```

### Makefile Integration

```makefile
# Build test binary with isolated home directory
build-test:
	go build -ldflags "-X main.defaultHomeOverride=.tsuku-test" -o tsuku-test ./cmd/tsuku

# Run functional tests (builds test binary first)
test-functional: build-test
	TSUKU_TEST_BINARY=./tsuku-test go test -v ./test/functional/...
	rm -rf .tsuku-test

# Run only critical scenarios
test-functional-critical: build-test
	TSUKU_TEST_BINARY=./tsuku-test go test -v ./test/functional/... -godog.tags=@critical
	rm -rf .tsuku-test
```

The `TSUKU_TEST_BINARY` env var tells the step definitions where to find the binary. This decouples the test code from the build location.

### CI Integration

Functional tests run as a separate job in the test workflow:

```yaml
functional-tests:
  name: Functional Tests
  runs-on: ubuntu-latest
  needs: [unit-tests]  # only run if unit tests pass
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
    - name: Run functional tests
      run: make test-functional
```

**Tag-based CI strategy:**
- **On every PR**: `@critical` scenarios only (fast feedback, ~30 seconds)
- **On merge to main**: full suite including `@regression` (complete coverage)
- **Nightly**: full suite with `@slow` scenarios (network-dependent tests, if any)

This means the PR workflow adds minimal time while main-branch builds catch regressions. The tag taxonomy:

| Tag | Meaning | When it runs |
|-----|---------|-------------|
| `@critical` | Core functionality (install, remove, list) | Every PR |
| `@regression` | Bug-fix coverage, edge cases | Merge to main |
| `@slow` | Scenarios that take >10 seconds | Nightly |
| `@install`, `@remove`, etc. | Feature category (for selective runs) | Manual filtering |

### godog Suite Setup

```go
// suite_test.go
func TestFeatures(t *testing.T) {
    suite := godog.TestSuite{
        ScenarioInitializer: InitializeScenario,
        Options: &godog.Options{
            Format:   "pretty",
            Paths:    []string{"features"},
            TestingT: t,
        },
    }
    if suite.Run() != 0 {
        t.Fatal("functional tests failed")
    }
}

func InitializeScenario(ctx *godog.ScenarioContext) {
    // Environment steps
    ctx.Step(`^a clean tsuku environment$`, aCleanTsukuEnvironment)
    ctx.Step(`^a clean tsuku environment with home "([^"]*)"$`, aCleanTsukuEnvironmentWithHome)
    ctx.Step(`^tsuku has "([^"]*)" installed$`, tsukuHasInstalled)

    // Command steps
    ctx.Step(`^I run "([^"]*)"$`, iRun)
    ctx.Step(`^I run "([^"]*)" with timeout (\d+)$`, iRunWithTimeout)

    // Assertion steps
    ctx.Step(`^the exit code is (\d+)$`, theExitCodeIs)
    ctx.Step(`^the output contains "([^"]*)"$`, theOutputContains)
    ctx.Step(`^the output does not contain "([^"]*)"$`, theOutputDoesNotContain)
    ctx.Step(`^the error output contains "([^"]*)"$`, theErrorOutputContains)
    ctx.Step(`^the file "([^"]*)" exists$`, theFileExists)
    ctx.Step(`^the file "([^"]*)" does not exist$`, theFileDoesNotExist)
    ctx.Step(`^I can run "([^"]*)"$`, iCanRun)

    // Reset home directory before each scenario
    ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
        os.RemoveAll(".tsuku-test")
        os.MkdirAll(".tsuku-test", 0o755)
        return ctx, nil
    })
}
```

## Implementation Approach

### Phase 1: Foundation

- Add godog dependency to `go.mod`
- Create `test/functional/` directory structure
- Implement `suite_test.go` with godog setup
- Implement core step definitions: environment setup, command execution, basic assertions
- Add `build-test` and `test-functional` Makefile targets
- Write 2-3 scenarios for `tsuku install` as proof of concept

### Phase 2: Step Library Expansion

- Add file assertion steps (`exists`, `does not exist`)
- Write feature files for remaining commands: remove, list, update, search, info
- Add `@critical` tags to core install/remove scenarios

### Phase 3: CI Integration

- Add functional test job to `.github/workflows/test.yml`
- Configure tag-based filtering (critical on PRs, full on main)
- Verify test output format works with CI reporting

### Phase 4: Coverage Expansion

- Add error handling scenarios (invalid commands, missing recipes)
- Add multi-step lifecycle scenarios (install -> use -> update -> remove)
- Add edge case scenarios (paths with spaces, concurrent installs)
- Add `@regression` and `@slow` tags as appropriate

## Security Considerations

### Download Verification

Functional tests exercise tsuku's install command, which downloads binaries from external sources. The tests themselves don't introduce new download paths -- they trigger the same download/verify logic that production tsuku uses. A test failure in verification means the production code has a bug, which is the point.

Tests should use recipes for tools with known-good checksums. The `serve` tool (a simple Go HTTP server) is a good candidate for the initial test suite because it has a small binary and fast install.

This risk profile matches the existing integration tests in `test/scripts/`, which already download and execute real binaries (Homebrew recipe tests, checksum pinning tests). The functional tests add no new download or execution patterns beyond what CI already runs.

### Execution Isolation

Tests execute the tsuku binary as a subprocess with a restricted home directory (`.tsuku-test`). The test binary is built with ldflags that hardcode this home path, so it can't write to the developer's real `$TSUKU_HOME` through normal operations. A `Before` hook wipes and recreates the directory between scenarios.

The test binary has the same filesystem permissions as the user running the tests. There's no privilege escalation. In CI, the `ubuntu-latest` runner provides additional isolation.

### Supply Chain Risks

godog is the only new dependency. It's maintained by the Cucumber organization, which is well-established in the testing ecosystem. The dependency is test-only (`_test.go` files), so it doesn't affect the production binary.

Feature files are plain text and can't execute code directly. Step definitions are Go code reviewed through the normal PR process.

### User Data Exposure

Not applicable. Functional tests run in isolated directories with no access to the developer's real `$TSUKU_HOME` or personal data. Test scenarios use synthetic environments created and destroyed per scenario.

## Consequences

### Positive

- Every tsuku command gets explicit test coverage from the user's perspective
- New features require a corresponding `.feature` file, making test expectations visible
- Gherkin format makes test intent readable to non-Go contributors
- Tag-based CI filtering keeps PR builds fast while main builds are thorough
- The step definition library is reusable -- adding scenarios is mostly writing `.feature` files

### Negative

- godog adds a test dependency (though it doesn't affect the production binary)
- Functional tests that install real tools are inherently slower than unit tests
- The Makefile build step is required before running functional tests
- No parallel scenario execution; runtime scales linearly with scenario count

### Neutral

- Existing unit tests and shell-based integration tests are unaffected
- The `test/` directory grows but stays organized by test type
- godog adds a test-only dependency that doesn't affect the production binary

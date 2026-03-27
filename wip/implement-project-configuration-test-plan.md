# Test Plan: Project Configuration

## Overview

This test plan covers the project configuration feature across all four issues: the `internal/project` package (Issue 1), `tsuku init` command (Issue 2), `tsuku install` no-args mode (Issue 3), and documentation accuracy (Issue 4).

Testing splits into two layers:
- **Unit tests** (Go `_test.go` files) -- validate internal logic with fine-grained control over filesystem and parsing
- **Functional tests** (Gherkin `.feature` files) -- validate end-to-end CLI behavior from a user's perspective

Unit tests are authored by the implementing agent alongside the code. This plan focuses on the functional test scenarios that exercise the feature through the CLI binary.

## Test Infrastructure Notes

The functional test harness runs commands via `iRun`, which:
- Replaces `tsuku` with the test binary path
- Sets `cmd.Dir` to the binary's parent directory (repo root)
- Sets `TSUKU_HOME` to `.tsuku-test` under the repo root
- Sets `TSUKU_NO_TELEMETRY=1` and `TSUKU_REGISTRY_URL` to the repo's recipes directory

**Limitation for project config tests:** The `iRun` step executes commands with `cmd.Dir` set to the repo root. `tsuku init` and `tsuku install` (no-args) depend on the working directory to discover `.tsuku.toml`. To test these scenarios, the implementation will need either:
1. A new step definition that sets the working directory (e.g., `Given I am in directory "{path}"`)
2. Or the `iRun` step adapted to support a `cd <dir> &&` prefix

This is a prerequisite for the functional tests below. The scenarios are written assuming option (1) exists. If a different approach is taken, adjust the step references.

**Alternative:** If adding new step definitions is deferred, some scenarios below can be validated through unit tests only and marked as needing future functional coverage.

## Proposed Feature Files

### Feature: Project Init (`test/functional/features/project-init.feature`)

Tests `tsuku init` command behavior.

| # | Scenario | Category | Issue |
|---|----------|----------|-------|
| 1 | Init creates .tsuku.toml in current directory | Automatable | 2 |
| 2 | Init fails when .tsuku.toml already exists | Automatable | 2 |
| 3 | Init --force overwrites existing .tsuku.toml | Automatable | 2 |
| 4 | Init output confirms file creation | Automatable | 2 |
| 5 | Created file contains [tools] section | Automatable | 2 |

### Feature: Project Install (`test/functional/features/project-install.feature`)

Tests `tsuku install` no-args mode.

| # | Scenario | Category | Issue |
|---|----------|----------|-------|
| 6 | Install no-args without config suggests tsuku init | Automatable | 3 |
| 7 | Install no-args with empty tools exits 0 | Automatable | 3 |
| 8 | Install no-args shows config path and tool list | Automatable | 3 |
| 9 | Install no-args with --yes skips confirmation | Automatable | 3 |
| 10 | Install no-args warns about unpinned versions | Automatable | 3 |
| 11 | Install no-args with --dry-run does not install | Automatable | 3 |
| 12 | Install no-args with incompatible flags errors | Automatable | 3 |
| 13 | Install no-args partial failure exits 15 | Environment-dependent | 3 |
| 14 | Install no-args all-success exits 0 | Environment-dependent | 3 |
| 15 | Install no-args with invalid TOML exits non-zero | Automatable | 3 |

### Feature: Documentation Accuracy (Issue 4)

| # | Scenario | Category | Issue |
|---|----------|----------|-------|
| 16 | tsuku init --help shows usage | Automatable | 4 |
| 17 | tsuku install --help mentions project config | Automatable | 4 |

## Scenario Details

### Scenario 1: Init creates .tsuku.toml in current directory
**Category:** Automatable
**Preconditions:** Clean directory with no `.tsuku.toml`
**Steps:**
```gherkin
Scenario: Init creates project config file
  Given a clean tsuku environment
  When I run "tsuku init"
  Then the exit code is 0
  And the output contains "Created .tsuku.toml"
```
**Note:** Requires working directory to be a directory without `.tsuku.toml`. May need a step to set working directory or run from a temp dir.

### Scenario 2: Init fails when .tsuku.toml already exists
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Init refuses to overwrite existing config
  Given a clean tsuku environment
  When I run "tsuku init"
  Then the exit code is 0
  When I run "tsuku init"
  Then the exit code is not 0
  And the error output contains "already exists"
```

### Scenario 3: Init --force overwrites existing .tsuku.toml
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Init with --force overwrites existing config
  Given a clean tsuku environment
  When I run "tsuku init"
  Then the exit code is 0
  When I run "tsuku init --force"
  Then the exit code is 0
  And the output contains "Created .tsuku.toml"
```

### Scenario 4-5: Init output and file content
Covered by scenarios 1-3 above -- the `Created .tsuku.toml` assertion validates output, and the `[tools]` section content is best validated in unit tests since functional tests can't easily read arbitrary file content with the current step vocabulary.

### Scenario 6: Install no-args without config suggests tsuku init
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Install without arguments and no project config
  Given a clean tsuku environment
  When I run "tsuku install"
  Then the exit code is 2
  And the error output contains "tsuku init"
```
**Note:** This depends on the working directory not containing a `.tsuku.toml` up to `$HOME`.

### Scenario 7: Install no-args with empty tools exits 0
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Install with empty project config
  Given a clean tsuku environment
  When I run "tsuku init"
  Then the exit code is 0
  When I run "tsuku install"
  Then the exit code is 0
  And the output contains "No tools declared"
```

### Scenario 8: Install no-args shows config path and tool list
**Category:** Automatable (needs a `.tsuku.toml` with tools written to disk)
**Steps:**
```gherkin
Scenario: Install shows project tool list
  Given a clean tsuku environment
  And a project config with tools "actionlint"
  When I run "tsuku install --dry-run --yes"
  Then the exit code is 0
  And the output contains "Using:"
  And the output contains "actionlint"
```
**Note:** Requires a new step to write a `.tsuku.toml` with specific tools, or a shell command to create the file before running install.

### Scenario 9: Install no-args with --yes skips confirmation
**Category:** Automatable
Validated implicitly by scenarios that use `--yes`. The key behavior is that without `--yes` on a non-TTY, the command should either auto-proceed or exit. This is best validated in unit tests since functional tests pipe stdout/stderr (non-TTY).

### Scenario 10: Install no-args warns about unpinned versions
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Install warns about unpinned versions
  Given a clean tsuku environment
  And a project config with tools "actionlint=latest"
  When I run "tsuku install --dry-run --yes"
  Then the exit code is 0
  And the error output contains "unpinned"
```

### Scenario 11: Install no-args with --dry-run does not install
**Category:** Automatable
Covered by Scenario 8's use of `--dry-run`.

### Scenario 12: Install no-args with incompatible flags errors
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Install no-args rejects --plan flag
  Given a clean tsuku environment
  When I run "tsuku install --plan plan.json"
  Then the exit code is 2

Scenario: Install no-args rejects --recipe flag
  Given a clean tsuku environment
  When I run "tsuku install --recipe recipe.toml"
  Then the exit code is 2

Scenario: Install no-args rejects --from flag
  Given a clean tsuku environment
  When I run "tsuku install --from homebrew:jq"
  Then the exit code is 2
  And the error output contains "--from requires exactly one tool name"

Scenario: Install no-args rejects --sandbox flag
  Given a clean tsuku environment
  When I run "tsuku install --sandbox"
  Then the exit code is 2
```
**Note:** Some of these flags already require a tool name and would fail with existing validation. The test verifies the error path is clear regardless. Flag incompatibility for `--plan` and `--recipe` with zero args already exists in the current code (they require args). The `--from` scenario already passes in the existing `install.feature`. The new code needs to ensure that when a `.tsuku.toml` exists, these flags still produce a clear error rather than silently entering project-config mode.

### Scenario 13: Install no-args partial failure exits 15
**Category:** Environment-dependent (requires network access for real tool installs)
**Steps:**
```gherkin
Scenario: Partial failure returns exit code 15
  Given a clean tsuku environment
  And a project config with tools "actionlint, nonexistent-tool-xyz-12345"
  When I run "tsuku install --yes --force"
  Then the exit code is 15
  And the output contains "Failed: 1"
  And the output contains "actionlint"
```
**Note:** This scenario requires network access to install actionlint and a guaranteed-nonexistent recipe to trigger partial failure. It validates the most important behavioral contract of the feature: lenient error handling with a distinct exit code.

### Scenario 14: Install no-args all-success exits 0
**Category:** Environment-dependent (requires network)
**Steps:**
```gherkin
Scenario: All tools installed successfully
  Given a clean tsuku environment
  And a project config with tools "actionlint"
  When I run "tsuku install --yes --force"
  Then the exit code is 0
  And the output contains "Installed:"
  And I can run "actionlint -version"
```

### Scenario 15: Install no-args with invalid TOML exits non-zero
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Invalid TOML in project config
  Given a clean tsuku environment
  And a project config with invalid TOML
  When I run "tsuku install --yes"
  Then the exit code is not 0
```

### Scenarios 16-17: Help text accuracy
**Category:** Automatable
**Steps:**
```gherkin
Scenario: Init help text is present
  Given a clean tsuku environment
  When I run "tsuku init --help"
  Then the exit code is 0
  And the output contains ".tsuku.toml"

Scenario: Install help mentions project config
  Given a clean tsuku environment
  When I run "tsuku install --help"
  Then the exit code is 0
  And the output contains ".tsuku.toml"
```

## New Step Definitions Needed

The scenarios above require new step definitions to create project config files in a controlled working directory:

1. **`Given a project config with tools "{tools}"`** -- writes a `.tsuku.toml` file with the specified tools in the command's working directory. Format: comma-separated `name` or `name=version` pairs.
2. **`Given a project config with invalid TOML`** -- writes a malformed `.tsuku.toml` to the working directory.
3. Potentially **`Given I am in directory "{path}"`** -- changes the working directory for subsequent commands (relative to `$TSUKU_HOME` or a temp dir).

The working directory question is the main infrastructure gap. The current `iRun` always runs from the repo root. For project config tests, we need commands to run from a directory that contains (or doesn't contain) `.tsuku.toml`. Options:
- Modify `iRun` to use a configurable `workDir` on the test state
- Use `cmd.Dir` override in a new step
- Write the `.tsuku.toml` to the repo root (where `iRun` already runs) -- simplest but pollutes the test environment

**Recommended approach:** Add a `workDir` field to `testState` (defaults to binary's directory as today). Add a step `Given I am in directory "{path}"` that creates a temp subdirectory under `$TSUKU_HOME` and sets `workDir`. Modify `iRun` to use `state.workDir` for `cmd.Dir`. This keeps isolation clean and lets project config scenarios control their environment.

## Unit Test Coverage Map

These are the unit test files specified in the acceptance criteria, with the scenarios they must cover:

### `internal/project/config_test.go` (Issue 1)
- String shorthand parsing (`node = "20.16.0"`)
- Inline table parsing (`python = { version = "3.12" }`)
- Empty/latest version handling
- Missing file returns nil (no error)
- Invalid TOML returns error
- MaxTools (256) exceeded returns error
- Parent directory traversal finds config in ancestor
- `TSUKU_CEILING_PATHS` stops traversal
- `$HOME` ceiling is unconditional
- Symlink resolution before traversal

### `cmd/tsuku/init_test.go` (Issue 2)
- Successful creation writes correct template
- Already-exists returns error
- `--force` overwrites without error

### `cmd/tsuku/install_project_test.go` (Issue 3)
- No config found exits `ExitUsage` (2)
- Empty tools section exits 0
- Partial failure exits `ExitPartialFailure` (15)
- All-success exits 0
- All-failure exits `ExitInstallFailed` (6)
- Flag incompatibility (`--plan`, `--recipe`, `--from`, `--sandbox`) errors
- Tools iterated in sorted alphabetical order

## Risk Areas

1. **Working directory dependency**: Both `tsuku init` and `tsuku install` (no-args) depend on the working directory for config discovery. The functional test harness doesn't currently support controlling this. This is the highest-risk gap.

2. **TTY detection for confirmation prompt**: The `Proceed? [Y/n]` prompt depends on TTY detection. Functional tests run with piped stdout/stderr (non-TTY). Need to verify that the non-TTY path either auto-proceeds or requires `--yes`. The design says interactive TTY without `--yes` prompts; non-TTY behavior should be tested.

3. **Exit code 15 is new**: CI scripts checking `$?` will see a new exit code. The functional test for partial failure (Scenario 13) is the key regression gate.

4. **Interaction with existing `tsuku install` flags**: The no-args mode shares the same Cobra command. Flag validation must distinguish between "no args because project config" vs. "no args and no --plan/--recipe" (the current usage error). The transition logic where `len(args) == 0` now branches to project config instead of erroring needs careful testing.

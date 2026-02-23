# Test Plan: Sandbox CI Integration

Generated from: docs/designs/DESIGN-sandbox-ci-integration.md
Issues covered: 6
Total scenarios: 18

---

## Scenario 1: PlanVerify gains ExitCode field and format version bumps to 5
**ID**: scenario-1
**Testable after**: #1942
**Commands**:
- `grep -q 'PlanFormatVersion = 5' internal/executor/plan.go`
- `grep -q 'ExitCode.*\*int.*json:"exit_code' internal/executor/plan.go`
**Expected**: `PlanFormatVersion` constant is 5 and `PlanVerify` struct has `ExitCode *int` field with the correct JSON tag
**Status**: passed

---

## Scenario 2: Shared CheckVerification function exists and handles all cases
**ID**: scenario-2
**Testable after**: #1942
**Commands**:
- `go test ./internal/sandbox/... -run 'CheckVerification' -count=1 -v`
**Expected**: Unit tests pass covering: exit code match with empty pattern returns true, exit code mismatch returns false, pattern match returns true, pattern mismatch returns false, non-default expected exit code works correctly
**Status**: passed

---

## Scenario 3: validate package uses shared CheckVerification
**ID**: scenario-3
**Testable after**: #1942
**Commands**:
- `grep -q 'sandbox\.\|CheckVerification\|verify\.Check' internal/validate/executor.go`
- `go test ./internal/validate/... -count=1 -timeout 60s`
**Expected**: The validate package references the shared verification function (not a duplicated local implementation), and all existing validate tests pass without behavior changes
**Status**: passed

---

## Scenario 4: buildSandboxScript appends verify block with marker files
**ID**: scenario-4
**Testable after**: #1942
**Commands**:
- `go test ./internal/sandbox/... -run 'SandboxScript.*Verify\|Verify.*Script' -count=1 -v`
**Expected**: When plan has a non-empty Verify.Command, the generated script contains `set +e`, redirects verify output to `/workspace/.sandbox-verify-output`, writes exit code to `/workspace/.sandbox-verify-exit`, and adds `$TSUKU_HOME/bin` and `$TSUKU_HOME/tools/current` to PATH. When plan has no verify command, no verify block is appended.
**Status**: passed

---

## Scenario 5: SandboxResult carries verification fields and Passed reflects both install and verify
**ID**: scenario-5
**Testable after**: #1942
**Commands**:
- `grep -q 'Verified.*bool' internal/sandbox/executor.go`
- `grep -q 'VerifyExitCode.*int' internal/sandbox/executor.go`
- `go test ./internal/sandbox/... -count=1 -timeout 60s`
**Expected**: `SandboxResult` has `Verified bool` and `VerifyExitCode int` fields. When no verify command exists, `Verified` is true and `VerifyExitCode` is -1. When install fails, `Verified` is false. When verify marker files exist, Go-side `CheckVerification` determines the value.
**Status**: passed

---

## Scenario 6: Plan generator copies ExitCode and plan cache includes it
**ID**: scenario-6
**Testable after**: #1942
**Commands**:
- `grep -q 'ExitCode' internal/executor/plan_generator.go`
- `grep -q 'ExitCode' internal/executor/plan_cache.go`
- `go test ./internal/executor/... -count=1 -timeout 120s`
**Expected**: Plan generator copies `recipe.Verify.ExitCode` into `PlanVerify.ExitCode` at both top-level and dependency plan generation sites. Plan cache hashing includes `ExitCode` so plans with different expected exit codes produce different cache keys. All executor tests pass.
**Status**: passed

---

## Scenario 7: SandboxRequirements gains ExtraEnv field
**ID**: scenario-7
**Testable after**: #1943
**Commands**:
- `grep -q 'ExtraEnv.*\[\]string' internal/sandbox/requirements.go`
**Expected**: `SandboxRequirements` struct has an `ExtraEnv []string` field for passing additional environment variables to the container
**Status**: passed

---

## Scenario 8: Env passthrough with key filtering protects hardcoded vars
**ID**: scenario-8
**Testable after**: #1943
**Commands**:
- `go test ./internal/sandbox/... -run 'Env|ExtraEnv|Override' -count=1 -v`
**Expected**: Unit tests pass covering: arbitrary KEY=VALUE pairs are appended to RunOptions.Env, attempts to override hardcoded keys (TSUKU_SANDBOX, TSUKU_HOME, HOME, DEBIAN_FRONTEND, PATH) are silently dropped, --env KEY form reads from host environment, --env KEY with missing host var passes empty string
**Status**: passed

---

## Scenario 9: --env CLI flag is registered and populates SandboxRequirements
**ID**: scenario-9
**Testable after**: #1943
**Commands**:
- `grep -q '"env"' cmd/tsuku/install.go || grep -q '"env"' cmd/tsuku/install_sandbox.go`
- `go build -o /dev/null ./cmd/tsuku`
**Expected**: The `--env` flag is registered as a repeatable string slice on the install command. The binary builds successfully. Passing `--env` without `--sandbox` has no effect (no error).
**Status**: passed

---

## Scenario 10: SandboxResult gains DurationMs and timing is measured
**ID**: scenario-10
**Testable after**: #1944
**Commands**:
- `grep -q 'DurationMs.*int64' internal/sandbox/executor.go`
- `grep -q 'time\.Now\|time\.Since\|DurationMs' internal/sandbox/executor.go`
**Expected**: `SandboxResult` has `DurationMs int64` field. The `Sandbox()` method measures wall-clock time from before runtime detection through result evaluation.
**Status**: passed

---

## Scenario 11: --json flag produces valid JSON for all sandbox result states
**ID**: scenario-11
**Testable after**: #1944
**Commands**:
- `grep -q 'installJSON\|json' cmd/tsuku/install_sandbox.go`
- `go test ./internal/sandbox/... ./cmd/tsuku/... -count=1 -timeout 60s`
**Expected**: When `--json` is set with `--sandbox`, stdout contains exactly one JSON object with fields: tool, passed, verified, install_exit_code, verify_exit_code, duration_ms, error. Human-readable output is suppressed. The JSON is valid for passed, failed, skipped (no runtime), and error states. The `error` field is null on success and a string on failure.
**Status**: passed

---

## Scenario 12: Sandbox verification works end-to-end with a real recipe
**ID**: scenario-12
**Testable after**: #1942, #1943, #1944
**Environment**: manual -- requires docker or podman runtime
**Commands**:
- `go build -o tsuku-test ./cmd/tsuku`
- `./tsuku-test install --sandbox --force --recipe recipes/serve.toml --json`
**Expected**: The JSON output shows `passed: true` and `verified: true`. The sandbox runs the recipe's verify command inside the container, reads marker files, and evaluates the result using Go-side pattern matching. The `duration_ms` field is a positive integer. This validates the full pipeline: plan generation with ExitCode, sandbox script generation with verify block, container execution, marker file reading, CheckVerification evaluation, JSON serialization.
**Status**: passed

---

## Scenario 13: --env passthrough works end-to-end in sandbox
**ID**: scenario-13
**Testable after**: #1942, #1943, #1944
**Environment**: manual -- requires docker or podman runtime
**Commands**:
- `go build -o tsuku-test ./cmd/tsuku`
- `TEST_MARKER=hello_from_host ./tsuku-test install --sandbox --force --recipe recipes/serve.toml --env TEST_MARKER="hello_from_host" --json`
**Expected**: The sandbox container receives the TEST_MARKER environment variable. The install succeeds (the env var doesn't affect the recipe, but confirms the passthrough plumbing works without errors). Attempting `--env PATH=/bad` does not break the container (hardcoded PATH wins).
**Status**: passed

---

## Scenario 14: test-recipe.yml Linux jobs use sandbox instead of docker run
**ID**: scenario-14
**Testable after**: #1945
**Commands**:
- `grep -c 'docker run' .github/workflows/test-recipe.yml`
- `grep -q 'tsuku-linux-amd64 install --sandbox' .github/workflows/test-recipe.yml`
- `grep -q 'tsuku-linux-arm64 install --sandbox' .github/workflows/test-recipe.yml`
- `grep -q '\-\-target-family' .github/workflows/test-recipe.yml`
- `grep -q '\-\-env.*GITHUB_TOKEN' .github/workflows/test-recipe.yml`
- `grep -q '\-\-json' .github/workflows/test-recipe.yml`
- `grep -q 'GITHUB_STEP_SUMMARY' .github/workflows/test-recipe.yml`
- `grep -q 'gtimeout' .github/workflows/test-recipe.yml`
**Expected**: No `docker run` calls remain. Both Linux jobs use sandbox calls with --target-family, --env GITHUB_TOKEN, and --json. The GITHUB_STEP_SUMMARY result table is preserved. macOS jobs retain `gtimeout` (unchanged). The `container-images.json` lookup and `.tsuku-exit-code` marker file pattern are removed. `continue-on-error: true` remains on Linux jobs.
**Status**: passed

---

## Scenario 15: recipe-validation-core.yml Linux jobs use sandbox with retry
**ID**: scenario-15
**Testable after**: #1946
**Commands**:
- `grep -c 'docker run' .github/workflows/recipe-validation-core.yml`
- `grep -q '\-\-sandbox' .github/workflows/recipe-validation-core.yml`
- `grep -q 'install_exit_code' .github/workflows/recipe-validation-core.yml`
- `grep -q 'attempt' .github/workflows/recipe-validation-core.yml`
- `grep -q 'auto_constrain' .github/workflows/recipe-validation-core.yml`
- `grep -q 'validation-results-\*\.json' .github/workflows/recipe-validation-core.yml`
- `grep -q 'gtimeout' .github/workflows/recipe-validation-core.yml`
**Expected**: No `docker run` calls remain. Linux validation jobs use --sandbox with --json and --env GITHUB_TOKEN. Retry logic is preserved: the sandbox call is wrapped in a 3-attempt loop, and exit code 5 is extracted via `jq .install_exit_code`. JSON result aggregation into `{recipe, platform, status, exit_code, attempts}` format is preserved. The `report` job and `auto_constrain` input are unchanged. macOS jobs retain `gtimeout` (unchanged). Matrix entries no longer include `install_cmd` or `libc`.
**Status**: passed

---

## Scenario 16: batch-generate.yml validation phase uses sandbox
**ID**: scenario-16
**Testable after**: #1947
**Commands**:
- `grep -c 'docker run' .github/workflows/batch-generate.yml`
- `grep -q '\-\-sandbox' .github/workflows/batch-generate.yml`
- `grep -q '\-\-target-family' .github/workflows/batch-generate.yml`
- `grep -q 'blocked_by\|missing_recipes' .github/workflows/batch-generate.yml`
**Expected**: No `docker run` calls remain in validation steps. Sandbox calls use --target-family and --env GITHUB_TOKEN. Exit code 8 / blocked_by extraction is preserved (missing_recipes comes from the normal --json error path during plan generation, not sandbox JSON). Retry logic for exit code 5 is preserved. The generate job, macOS validation jobs, and merge job are unchanged.
**Status**: passed

---

## Scenario 17: validate-golden-execution.yml container jobs use sandbox
**ID**: scenario-17
**Testable after**: #1947
**Commands**:
- `grep -c 'docker run' .github/workflows/validate-golden-execution.yml`
- `grep -q '\-\-sandbox' .github/workflows/validate-golden-execution.yml`
- `grep -q '\-\-target-family' .github/workflows/validate-golden-execution.yml`
**Expected**: No `docker run` calls remain in container validation steps. The batched per-family docker call is replaced with a loop of individual sandbox invocations (one per recipe-version pair). The `install-recipe-deps.sh` call is removed. The `validate-linux` (Debian native), `validate-macos`, `execute-registry-linux`, `detect-changes`, and `validate-coverage` jobs are unchanged.
**Status**: passed

---

## Scenario 18: Full CI pipeline runs successfully after all migrations
**ID**: scenario-18
**Testable after**: #1947
**Environment**: manual -- requires pushing the branch and triggering CI
**Commands**:
- Push branch with all changes to GitHub
- Trigger `test-recipe.yml` with a known-good recipe (e.g., `serve`)
- Trigger `recipe-validation-core.yml` for a subset of recipes
- Verify `batch-generate.yml` and `validate-golden-execution.yml` pass on next scheduled or manual run
**Expected**: All four migrated workflows complete successfully. Linux jobs use sandbox calls. macOS jobs are unaffected. Result tables in GITHUB_STEP_SUMMARY render correctly. Retry logic triggers on network errors (exit code 5). The sandbox produces valid JSON output that workflows parse with jq. No `docker run` calls remain in recipe validation jobs.
**Status**: pending

---

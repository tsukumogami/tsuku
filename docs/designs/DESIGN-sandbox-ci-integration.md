---
status: Proposed
spawned_from:
  issue: 1905
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-sandbox-image-unification.md
problem: |
  Tsuku's sandbox can't replace the 12 recipe validation docker calls in CI
  because it lacks post-install verification, environment variable passthrough,
  and machine-readable output. CI workflows maintain their own container logic
  (per-family package installation, volume mounting, exit code capture, retry,
  result aggregation) that duplicates what sandbox should provide.
decision: |
  Close three sandbox gaps with targeted extensions, then migrate all four
  affected CI workflows. Add verify command execution using the plan's Verify
  fields with Go-side pattern matching shared with the validate package. Add
  --env for explicit env passthrough with key filtering. Add --json for
  machine-readable output. Then replace docker run blocks in test-recipe.yml,
  recipe-validation-core.yml, batch-generate.yml, and
  validate-golden-execution.yml with sandbox calls. Retry and batching stay
  as workflow-layer concerns consuming sandbox JSON output.
rationale: |
  All three gaps have natural extension points in existing code. The plan
  already carries verify info that the sandbox ignores. The RunOptions struct
  accepts arbitrary env vars. The SandboxResult struct has everything needed
  for JSON output. Migrating workflows incrementally (one at a time) limits
  risk while each migration proves the pattern. After migration, all recipe
  validation uses the same code path locally and in CI, and direct docker
  calls in CI are limited to non-recipe purposes.
---

# DESIGN: Sandbox CI Integration

## Status

Proposed

## Upstream Design Reference

This design implements the "Future Work" item from [DESIGN-sandbox-image-unification.md](DESIGN-sandbox-image-unification.md), which identified three critical gaps preventing `tsuku install --sandbox` from replacing direct docker calls in CI recipe validation.

## Context and Problem Statement

Tsuku's CI validates recipes by running `tsuku install` inside docker containers across five Linux families and two macOS targets. Of the 22 container usages in CI, 12 are recipe validation calls that follow a pattern the sandbox already handles: start a distro container, install prerequisites, run `tsuku install`, check the result.

The sandbox can't actually replace these calls because of three gaps:

1. **No post-install verification.** The sandbox runs `tsuku install --plan` and checks exit code 0. That's it. CI separately runs `verify-tool.sh` (does the binary execute and produce expected output?) and `verify-binary.sh` (is the ELF properly linked?). A sandbox-installed binary could be broken and sandbox would still report success. The ironic part: the `validate` package already runs recipe verify commands inside containers. The sandbox just doesn't.

2. **No environment variable passthrough.** CI passes `GITHUB_TOKEN` (to avoid GitHub API rate limits during version resolution) and `TSUKU_REGISTRY_URL` (to test PR-branch recipes) into containers. The sandbox hardcodes five env vars (`TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, `PATH`) with no way to add more. Without `GITHUB_TOKEN`, sandbox hits rate limits during version resolution on the host (plan generation) and any network-requiring actions inside the container. `TSUKU_REGISTRY_URL` is consumed during plan generation on the host, so it needs to be set in the host environment before running `tsuku install --sandbox`, not inside the container. But `GITHUB_TOKEN` is needed in both places: on the host for plan generation and inside the container for actions that fetch dependencies at install time.

3. **No structured result reporting.** CI produces JSON with per-recipe per-platform results (status, exit code, attempt count) and renders GitHub step summaries. The sandbox returns a Go struct with `Passed bool`, `ExitCode int`, `Stdout string`, `Stderr string`. Any CI workflow using sandbox would need to parse stdout/stderr to extract results and rebuild all reporting logic externally.

These gaps force CI to maintain its own container management code: family-specific package installation, volume mounting, env passthrough, exit code capture, retry logic, and result aggregation. This code is fragile (it broke when Podman resolved short image names to the wrong registry) and duplicates what the sandbox should provide.

The sandbox image unification work (PR #1886, issues #1901-#1904) unified container image references, but the runtime behavior gap remains. Now is a good time to address it because the infrastructure is in place: centralized images, digest pinning, and drift checks all work. The missing piece is the sandbox itself.

### Scope

**In scope:**
- Adding post-install verification to sandbox using the plan's existing `Verify` fields
- Adding explicit environment variable passthrough via CLI flags
- Adding machine-readable JSON output for CI consumption
- Migrating all Linux recipe validation workflows from direct docker calls to `--sandbox`

**Out of scope:**
- Binary quality checks (ELF linking, RPATH verification via `verify-binary.sh`). These are useful but separate from the sandbox execution model. They could be added later as a `--verify-binary` flag or a separate `tsuku verify-binary` command.
- macOS CI jobs. macOS tests run on native runners (no containers), so sandbox doesn't apply.
- Non-recipe-validation docker usage. `platform-integration.yml` runs specialized build and integration tests (dltest, dlopen verification) that go beyond recipe install. These stay as docker calls.
- The `SourceBuildSandboxImage` constant (tracked by sandbox image unification).

**Done when:** Every Linux recipe validation job in CI uses `tsuku install --sandbox` instead of direct docker run. Retry logic and result aggregation stay in the workflow layer but consume sandbox JSON output instead of raw exit codes.

## Decision Drivers

- **Incremental migration**: CI workflows should be migrateable one at a time, not all-or-nothing
- **Backward compatible**: Existing `--sandbox` callers must not break
- **Recipe-driven**: Verification should use the recipe's `[verify]` section, not separate scripts
- **Secure by default**: Env passthrough must be opt-in, never automatic host env leakage
- **Minimal scope**: Close the gaps without redesigning the sandbox execution model
- **Validate parity**: The sandbox should offer the same verification capability as the `validate` package

## Considered Options

### Decision 1: How Should Post-Install Verification Work?

The sandbox currently runs `tsuku install --plan /workspace/plan.json --force` and checks exit code 0. The `validate` package does the same, but then also runs the recipe's verify command and checks the output against a pattern. The `InstallationPlan` already carries `Verify.Command` and `Verify.Pattern` from the recipe, so the data is available; the sandbox just ignores it.

One prerequisite: `PlanVerify` currently only has `Command` and `Pattern` fields, but the recipe's `VerifySection` also has `ExitCode *int` (for recipes where the verify command intentionally exits non-zero). The validate package reads `ExitCode` from the recipe struct directly, but the sandbox only has the plan. Adding `ExitCode` to `PlanVerify` is a plan format version bump (v3 to v4) that should happen first.

The question is where verification logic should live and how much of CI's verification the sandbox should absorb.

#### Chosen: Run verify command in container, check results in Go

Extend `buildSandboxScript()` to append the verify command from `plan.Verify` after the install step. The script uses `set +e` around the verify command and writes its exit code and output to marker files in the workspace. Pattern matching and result evaluation happen in Go after the container exits, reusing the same `checkVerification()` logic from the validate package.

This split is important. Running pattern matching inside the shell script would require `grep` (not available in all minimal containers) and creates an exit code ambiguity: with `set -e`, a tool that exits with code 2 would be indistinguishable from a "verify failed" exit code 2 convention. By doing all result analysis in Go, the container's exit code only reflects install status, and verification is determined from the marker files.

The concrete mechanism: after install succeeds, the script turns off `set -e`, runs the verify command with output captured to `/workspace/.sandbox-verify-output`, and writes `$?` to `/workspace/.sandbox-verify-exit`. The Go code reads these files from the mounted workspace directory (which persists after container exit), then calls `checkVerification()` to evaluate the results. `SandboxResult.Passed` is true only when both install and verification succeed.

Binary quality checks (`verify-binary.sh` -- RPATH analysis, ldd dependency checks) stay in CI. They require tools like `readelf` and `ldd` that aren't installed in minimal sandbox containers, and they test properties (relocatability, dependency resolution) that are orthogonal to "does the tool work."

#### Alternatives Considered

**Run verification as a separate post-sandbox step on the host.** After the sandbox installs the tool, run `tsuku verify <tool>` on the host against the container's workspace. Rejected because the installed binary lives inside the container's filesystem, which is cleaned up when the container exits. You'd need to keep the container running or extract the binary, adding complexity. Running verification inside the container where the binary actually lives is simpler and more accurate.

**Add full verify-tool.sh functionality to the sandbox.** Port the CI verify scripts into the sandbox, including tool-specific functional tests (git clone, curl HTTPS, etc.). Rejected because these are CI-specific tests that go beyond recipe verification. The recipe's `[verify]` section captures the tool's basic health check. Putting CI-specific functional tests inside the sandbox would couple the sandbox to CI test policies.

### Decision 2: How Should Environment Variable Passthrough Work?

CI passes `GITHUB_TOKEN` and `TSUKU_REGISTRY_URL` into containers via `docker run -e`. The sandbox needs a way to accept additional env vars without automatically exposing the host environment (which would break isolation and leak secrets).

#### Chosen: `--env` CLI flag with explicit key=value pairs

Add a repeatable `--env KEY=VALUE` flag to `tsuku install --sandbox`. Each `--env` flag appends to the container's environment. If `VALUE` is omitted (`--env KEY`), read from the host environment (matching docker's behavior).

The `sandbox.Executor.Sandbox()` method gets a new `ExtraEnv []string` field on `SandboxRequirements`. The CLI populates this from the `--env` flags. The executor appends these to the `RunOptions.Env` slice after the hardcoded vars.

Security constraint: the hardcoded vars (`TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `PATH`) cannot be overridden by `--env`. The implementation explicitly filters out user-provided keys that match hardcoded keys before building the env slice, rather than relying on append order (which is runtime-dependent). If a user passes `--env PATH=/bad`, it's silently dropped. This prevents accidental breakage of the sandbox environment.

#### Alternatives Considered

**Automatic passthrough of allowlisted vars.** Define a list of "safe" env vars (like `GITHUB_TOKEN`, `TSUKU_REGISTRY_URL`) that the sandbox always passes through when present. Rejected because the allowlist becomes a maintenance burden and couples the sandbox to specific CI patterns. New CI workflows with different env vars would require sandbox code changes.

**Config file for sandbox environment.** Read additional env vars from `~/.tsuku/sandbox.env` or a TOML section. Rejected because this is a CI-focused feature where the caller knows exactly which vars it needs. A config file adds indirection without benefit. Workflows already have the values; they just need to pass them through.

### Decision 3: How Should Structured Results Work?

CI needs machine-readable results to build per-recipe per-platform status tables. The sandbox returns a Go struct that the CLI currently renders as human-readable text.

#### Chosen: `--json` CLI flag for machine-readable output

Add a `--json` flag to `tsuku install --sandbox` that outputs a JSON object on stdout:

```json
{
  "tool": "ruff",
  "passed": true,
  "verified": true,
  "install_exit_code": 0,
  "verify_exit_code": 0,
  "duration_ms": 4523,
  "error": null
}
```

The `passed` field is the overall result (install AND verify). The `verified` field reflects verify specifically (true when no verify command exists). `install_exit_code` is the container's exit code. `verify_exit_code` is from the verify marker file (-1 when no verify command exists).

When `--json` is set, the CLI suppresses normal output (progress messages, etc.) and writes only the JSON object. This lets CI parse results with `jq` without filtering noise.

#### Alternatives Considered

**Write results to a file.** Output results to a `--result-file /path/to/result.json` file instead of stdout. Rejected because it adds a temp file management burden. CI workflows would need to create, read, and clean up the file. Stdout is the natural channel for structured output and composes well with shell pipelines (`tsuku install --sandbox --json | jq .passed`).

**Encode everything in exit codes.** Use distinct exit codes: 0=pass, 1=install fail, 2=verify fail, 3=skip, etc. Rejected because exit codes can't carry timing, error messages, or distinguish between "tool installed but verify failed" vs "tool installed but no verify command exists." Shell-level exit code conventions are also fragile: the verify command itself might exit with code 2, creating ambiguity. JSON output provides the full picture without these limitations.

## Decision Outcome

**Chosen: 1A + 2A + 3A**

### Summary

The sandbox closes its three CI integration gaps with targeted extensions that don't change its execution model. Verification uses the plan's `Verify` fields (after adding `ExitCode` to `PlanVerify`, bumping plan format to v4). The sandbox script runs the verify command and writes its exit code and output to marker files in the workspace. The Go code reads these files after the container exits and evaluates results using the same `checkVerification()` logic as the validate package. `SandboxResult.Passed` is true only when both install and verification succeed.

Environment passthrough uses a repeatable `--env KEY=VALUE` CLI flag. The values flow through `SandboxRequirements.ExtraEnv` into the container's `RunOptions.Env` slice. Hardcoded sandbox vars (`TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `PATH`) are protected via explicit key filtering. Note that `TSUKU_REGISTRY_URL` is consumed during plan generation on the host, not inside the container, so it only needs to be set in the host environment. `GITHUB_TOKEN` is the primary candidate for `--env` since it's needed inside the container for network-requiring actions.

Structured output uses a `--json` flag that writes a JSON object to stdout containing pass/fail status, verification status, exit codes, duration, and error details. The CLI suppresses human-readable output when `--json` is set. CI workflows parse results with `jq` to build their existing per-recipe per-platform tables.

With all three gaps closed, CI recipe validation workflows can replace their docker run blocks with:

```bash
# TSUKU_REGISTRY_URL is consumed on the host during plan generation
export TSUKU_REGISTRY_URL="https://raw.githubusercontent.com/$REPO/$BRANCH"

./tsuku install --sandbox --force --recipe "$recipe_path" \
  --target-family "$family" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN" \
  --json > result.json
```

This eliminates per-family package installation, volume mounting, exit code capture files, and the sandbox script generation that CI currently reimplements. Retry logic and result aggregation stay in the workflow layer but consume sandbox JSON output instead of raw exit codes and captured files.

After the sandbox code changes land, four CI workflows migrate their Linux recipe validation jobs from docker calls to sandbox:

| Workflow | What Migrates | What Stays |
|----------|--------------|------------|
| `test-recipe.yml` | Linux x86_64 (5 families) and arm64 (4 families) jobs | macOS native jobs |
| `recipe-validation-core.yml` | Linux x86_64 and arm64 validation matrix | macOS native validation, auto-constraint generation |
| `batch-generate.yml` | Validation phase Linux jobs | Generation phase (no containers), macOS validation |
| `validate-golden-execution.yml` | Linux container execution jobs | macOS jobs, registry recipe execution (already native) |

Two workflows already use sandbox: `sandbox-tests.yml` and `test-recipe-changes.yml`. Non-recipe workflows (`platform-integration.yml`, `build-essentials.yml`, etc.) are out of scope because they use containers for specialized builds, not recipe validation.

### Rationale

All three sandbox gaps have clean extension points in existing code. The plan already carries verify info (after adding `ExitCode` to `PlanVerify`). The `RunOptions.Env` slice accepts arbitrary entries. The `SandboxResult` struct contains everything needed for JSON serialization. None of these changes affect the sandbox execution model, the container runtime abstraction, or the validate package.

Keeping retry and batching out of sandbox is deliberate. Retry policies (which exit codes, how many attempts, what backoff) are workflow decisions, not sandbox decisions. A sandbox invocation is one run of one recipe. The workflow decides whether to retry and how to aggregate results across recipes and platforms. This separation means the sandbox API stays simple while CI workflows keep full control of their retry semantics.

Migrating all four workflows completes the integration. After migration, direct docker calls in CI are limited to non-recipe purposes (specialized builds, infrastructure testing), and all recipe validation goes through the same code path that users run locally with `tsuku install --sandbox`.

## Solution Architecture

### Overview

The changes touch four layers: the plan format (add `ExitCode` to `PlanVerify`), the sandbox Go package (execution and verification), the CLI (flags and output), and CI workflows (migration from docker calls to sandbox).

```
CLI (cmd/tsuku/install_sandbox.go)
  |-- parses --env and --json flags
  |-- passes env to SandboxRequirements.ExtraEnv
  |-- if --json: serializes SandboxResult to JSON on stdout
  |
sandbox.Executor.Sandbox()
  |-- filters ExtraEnv against hardcoded keys, appends to RunOptions.Env
  |-- buildSandboxScript() includes verify command with marker file output
  |-- after container exits, reads marker files from workspace
  |-- calls checkVerification() for pattern matching (Go-side)
  |-- returns SandboxResult with Verified field
  |
Container
  |-- runs install (set -e) + verify (set +e, writes marker files)
  |-- exit code reflects install status only
```

### Components

**`internal/executor/plan.go`**: `PlanVerify` gains `ExitCode *int`. Plan format version bumps to 4. The plan generator copies `recipe.Verify.ExitCode` into this field.

**`internal/sandbox/executor.go`**: The `buildSandboxScript()` method gains a verify step. After `tsuku install --plan`, if `plan.Verify.Command` is non-empty, the script:
1. Turns off `set -e` (install already succeeded at this point)
2. Sets `PATH` to include `$TSUKU_HOME/tools/current` and `$TSUKU_HOME/bin`
3. Runs the verify command, capturing output to `/workspace/.sandbox-verify-output`
4. Writes `$?` to `/workspace/.sandbox-verify-exit`

The `Sandbox()` method:
- Filters `reqs.ExtraEnv` to remove keys that match hardcoded vars, then appends the remainder to `RunOptions.Env`
- After container exits, reads `.sandbox-verify-exit` and `.sandbox-verify-output` from the workspace directory
- Calls `checkVerification()` to evaluate verify exit code and pattern matching
- Sets `result.Verified` based on Go-side analysis, `result.Passed` = install passed AND verified

**`internal/sandbox/requirements.go`**: `SandboxRequirements` gains `ExtraEnv []string`.

**`internal/sandbox/verify.go`** (new): Shared `checkVerification()` logic extracted from `validate.Executor.checkVerification()`. Both packages call this function. It checks exit code against expected (defaulting to 0) and pattern against combined output.

**`cmd/tsuku/install_sandbox.go`**: The CLI gains `--env` (repeatable string slice) and `--json` (bool) flags. `--env` populates `SandboxRequirements.ExtraEnv`. `--json` changes output from human text to JSON.

**`internal/sandbox/executor.go` (SandboxResult)**: Gains `Verified bool`, `VerifyExitCode int`, and `DurationMs int64`.

### Key Interfaces

No new interfaces. Existing structs gain fields:

```go
// PlanVerify (addition)
type PlanVerify struct {
    Command  string `json:"command,omitempty"`
    Pattern  string `json:"pattern,omitempty"`
    ExitCode *int   `json:"exit_code,omitempty"` // Expected exit code (default: 0)
}

// SandboxRequirements (addition)
type SandboxRequirements struct {
    // ... existing fields ...
    ExtraEnv []string // Additional env vars for the container (KEY=VALUE format)
}

// SandboxResult (additions)
type SandboxResult struct {
    // ... existing fields ...
    Verified      bool  // Whether verify command passed (true if no verify command)
    VerifyExitCode int  // Verify command's exit code (-1 if no verify command)
    DurationMs    int64 // Total execution time in milliseconds
}
```

New shared function:

```go
// checkVerification evaluates verify results against expectations.
// Used by both sandbox and validate packages.
func checkVerification(verifyExitCode int, output string, expectedExitCode int, pattern string) bool
```

### Data Flow

1. CLI parses `--env GITHUB_TOKEN=xxx` and `--json` flags
2. CLI creates `SandboxRequirements` with `ExtraEnv: ["GITHUB_TOKEN=xxx"]`
3. `Executor.Sandbox()` filters `ExtraEnv` against hardcoded keys, appends remainder to `RunOptions.Env`
4. `buildSandboxScript()` generates script with install (`set -e`) + verify (`set +e`, marker files)
5. Container runs script; exit code reflects install status only
6. `Sandbox()` reads verify marker files from workspace, calls `checkVerification()` in Go
7. `Sandbox()` sets `Passed` = install OK AND verified, `Verified` = verify specifically
8. CLI serializes `SandboxResult` as JSON (if `--json`) or renders as text

## Implementation Approach

### Phase 1: Verification

Add verify command execution to the sandbox, with Go-side result evaluation.

- Add `ExitCode *int` to `PlanVerify` and bump plan format version to 4
- Update plan generator to copy `recipe.Verify.ExitCode` into the plan
- Extract `checkVerification()` from `validate` into shared code (`internal/sandbox/verify.go`)
- Extend `buildSandboxScript()` to append verify command with `set +e` and marker file output
- Add `Verified` and `VerifyExitCode` fields to `SandboxResult`
- Read verify marker files from workspace after container exits
- Call `checkVerification()` for Go-side pattern matching
- Set `Passed` = install OK AND verified
- Add tests: sandbox with verify command (pass and fail cases), sandbox without verify command, non-default expected exit code
- Verify that existing sandbox and validate tests still pass unchanged

### Phase 2: Environment Passthrough

Add `--env` flag support.

- Add `ExtraEnv []string` to `SandboxRequirements`
- Update `Executor.Sandbox()` to append `ExtraEnv` to `RunOptions.Env`
- Add protection against overriding hardcoded vars
- Add `--env` flag to CLI with repeatable string slice
- Add tests: env passthrough, override protection, empty value (read from host)

### Phase 3: Structured Output

Add `--json` flag support.

- Add `DurationMs int64` to `SandboxResult` and measure execution time
- Add `--json` flag to CLI
- Implement JSON serialization of `SandboxResult`
- Suppress human-readable output when `--json` is set
- Add tests: JSON output format, JSON with various result states

### Phase 4: Migrate test-recipe.yml

Migrate the simplest recipe validation workflow as a proof-of-concept.

`test-recipe.yml` tests a single recipe across all Linux families. Each Linux job currently does a `docker run` with family-specific package installation, then runs `tsuku install --force --recipe`. The migration replaces each docker call with `tsuku install --sandbox --force --recipe --target-family --env GITHUB_TOKEN --json`, letting the sandbox handle container selection, package installation, and verification.

- Replace docker run blocks in `test-linux-x86_64` job (5 families) with sandbox calls
- Replace docker run blocks in `test-linux-arm64` job (4 families) with sandbox calls
- Keep macOS jobs unchanged (native runners, no containers)
- Preserve the existing result table in `$GITHUB_STEP_SUMMARY`, built from JSON output
- Preserve `continue-on-error: true` semantics (platform failures don't block merge)

Before:
```bash
docker run --rm -e GITHUB_TOKEN -v "$PWD:/workspace" -w /workspace "$image" sh -c "
  case '$family' in ...package install... esac
  timeout 300 ./tsuku install --force --recipe '$recipe_path'
  echo \$? > /workspace/.tsuku-exit-code
" 2>&1 || true
```

After:
```bash
export TSUKU_REGISTRY_URL="$REGISTRY_URL"  # consumed on host during plan gen
./tsuku install --sandbox --force --recipe "$recipe_path" \
  --target-family "$family" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN" \
  --json > ".result-$family.json" 2>/dev/null || true
```

### Phase 5: Migrate recipe-validation-core.yml

Migrate the most impactful workflow: all-recipes cross-platform validation.

`recipe-validation-core.yml` is the reusable workflow that validates every recipe across all platforms with retry logic. Each Linux matrix entry runs a docker call per recipe with up to 3 retries on exit code 5 (network errors). The migration replaces the docker calls but preserves the retry wrapper and JSON result aggregation.

- Replace docker run blocks in `validate-linux-x86_64` matrix (5 families) with sandbox calls
- Replace docker run blocks in `validate-linux-arm64` matrix (4 families) with sandbox calls
- Keep macOS validation jobs unchanged
- Preserve retry logic: wrap sandbox call in the existing retry loop, check `jq .install_exit_code` from JSON for exit code 5
- Preserve JSON result aggregation: transform sandbox JSON to the existing `{recipe, platform, status, exit_code, attempts}` format
- Preserve auto-constraint generation (`auto_constrain` input)
- Keep the `report` job that merges results unchanged (same aggregated JSON format)

### Phase 6: Migrate remaining workflows

Migrate `batch-generate.yml` and `validate-golden-execution.yml`.

**batch-generate.yml** — The validation phase runs docker calls per family to test generated recipes. The migration follows the same pattern as recipe-validation-core. One difference: batch-generate uses exit code 8 (`missing_dep`) to extract `blocked_by` dependencies from JSON output. The sandbox `--json` output must include the same `missing_recipes` field from the existing `--json` install output for this to work.

- Replace docker run blocks in validation phase (x86_64 and arm64) with sandbox calls
- Preserve exit code 8 / `blocked_by` extraction using `jq` on sandbox JSON output
- Keep generation phase unchanged (no containers)
- Keep macOS validation unchanged

**validate-golden-execution.yml** — This workflow validates embedded golden execution files across Linux families. It runs aggregated docker calls per family with multiple recipes batched in a single container. The sandbox processes one recipe at a time, so the migration replaces the batched docker call with a loop of sandbox calls.

- Replace per-family docker run blocks with a loop of sandbox calls
- One sandbox invocation per recipe-version pair instead of one docker call per family
- Keep registry recipe execution unchanged (already native)
- Keep macOS jobs unchanged

### Phasing and Dependencies

Phases 1-3 (sandbox code changes) can ship in a single PR. Phase 4 depends on phases 1-3 landing and should be a separate PR to isolate risk. Phase 5 depends on phase 4 proving the pattern works. Phase 6 depends on phase 5.

The incremental approach lets each workflow migration be reviewed and tested independently. If a migration introduces regressions, only that workflow is affected, and reverting is a single PR.

## Security Considerations

### Download Verification

Not applicable. This design doesn't change how artifacts are downloaded or verified. The sandbox execution model (pre-resolved plan with cached downloads) is unchanged. Digest pinning of container images is already handled by the image unification work.

### Execution Isolation

The `--env` flag introduces a controlled channel for passing data into the sandbox container. Two security properties are maintained:

1. **Opt-in only.** Environment variables are never passed automatically. The caller must explicitly list each variable with `--env`. This prevents accidental leakage of host environment secrets into containers.

2. **Hardcoded vars take precedence.** The sandbox's core env vars (`TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, `PATH`) cannot be overridden by `--env`. If someone passes `--env PATH=/malicious`, the hardcoded `PATH` value wins. This prevents the container environment from being subverted.

The verification step runs inside the same container with the same isolation as the install step. It doesn't escalate privileges or weaken the sandbox boundary.

### Supply Chain Risks

No change. The sandbox still runs pre-resolved plans with pre-downloaded artifacts. Verify commands come from the recipe's `[verify]` section, which is part of the recipe file reviewed in PRs. A compromised recipe could include a malicious verify command, but this is the same risk as a compromised recipe including malicious install steps -- the review process catches both.

The `--env GITHUB_TOKEN` passthrough does expose a token inside the container. This is the same pattern CI uses today with `docker run -e GITHUB_TOKEN`. The token's scope should be limited to read-only repository access (GitHub Actions' default `GITHUB_TOKEN` permissions).

### User Data Exposure

The `--env` flag could pass arbitrary data into the container. This is by design -- CI needs `GITHUB_TOKEN` and `TSUKU_REGISTRY_URL`. The sandbox doesn't log or persist env values; they exist only in the container process's environment for the duration of execution.

The `--json` output includes exit codes, timing, and error messages but not env values, stdout, or stderr (those are only shown in human-readable mode or available in the Go struct for programmatic callers). JSON output doesn't expose more data than the existing text output.

## Consequences

### Positive

- All Linux recipe validation in CI uses the same code path as local `tsuku install --sandbox`, eliminating per-family package installation code, volume mounting, and exit code capture hacks across four workflows
- The sandbox becomes a meaningful validation tool, not just an "install succeeded" check
- JSON output lets CI build status tables without parsing human-readable text
- Env passthrough unblocks sandbox use in any context that needs tokens or custom configuration
- The code changes are additive and backward compatible. Existing `--sandbox` callers see no difference unless they opt into `--json` or `--env`
- CI workflow code gets simpler: each Linux recipe test is one `tsuku install --sandbox` call instead of a docker run block with family-specific setup

### Negative

- The verify step adds execution time to sandbox runs (running one more command per invocation). For most recipes this is milliseconds (`tool --version`), but some verify commands may be slower.
- `--env` creates a mechanism for passing secrets into containers. While opt-in, users could accidentally expose tokens in CI logs if they echo the sandbox command. Mitigation: the sandbox doesn't log env values, and `--json` output doesn't include them.
- Passing `GITHUB_TOKEN` via `--env` into a container with network access lets a malicious recipe's verify command exfiltrate the token. This is the same risk as CI's current `docker run -e GITHUB_TOKEN` pattern, but worth noting since the sandbox didn't previously expose tokens.
- `validate-golden-execution.yml` currently batches multiple recipes into one docker call per family. Migrating to sandbox means one container invocation per recipe, increasing startup overhead. For N recipes, that's N container starts instead of 1. In practice this is mitigated by sandbox's image caching (the custom image is built once and reused).

### Mitigations

| Risk | Mitigation |
|------|------------|
| Verify command adds latency | Most verify commands are `tool --version` (milliseconds). Source builds already take minutes; verify is noise. |
| Secret leakage via --env | Sandbox never logs env values. JSON output excludes them. CI should use `${{ secrets.* }}` which are masked in logs. |
| Token exfiltration via malicious verify | Same risk as current CI pattern. Mitigated by: recipe review process, GITHUB_TOKEN has minimal read-only scope, network isolation when not needed. |
| Malicious verify command | Same review process as install steps. The recipe is the trust boundary, not the sandbox. |
| Golden validation container overhead | Sandbox caches custom images (same hash = no rebuild). The extra container starts are fast when the image already exists. |

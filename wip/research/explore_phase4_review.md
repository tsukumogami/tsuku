# Architecture Review: DESIGN-sandbox-ci-integration

## Reviewer: Architect

## 1. Problem Statement Specificity

The problem statement is well-grounded. It identifies three specific gaps, traces each to concrete code locations, and quantifies the CI surface area (22 container usages, 12 replaceable). The claim about the 12 recipe validation calls following a pattern the sandbox handles is confirmed by examining `test-recipe.yml` and `recipe-validation-core.yml` -- both run the same docker-run-install-check-exit-code sequence.

One area that could be more precise: the document says "12 recipe validation calls" from the upstream design but never maps which 12 specifically. The CI workflows I examined show at least two patterns:

- **recipe-validation-core.yml**: Uses `docker run` per recipe per family, with retry logic, JSON result aggregation, and artifact upload. This is the pattern with the most shell complexity (exit code capture files, retry on exit 5, jq result building).
- **test-recipe.yml**: Same docker-run pattern but simpler: no retry, no JSON results, just inline counting.
- **build-essentials.yml**: Runs `verify-tool.sh` and `verify-binary.sh` after `tsuku install`, but these are host-level tests, not containerized recipe validation.

The design should explicitly list which workflows are migration candidates and which are not, since the "12 replaceable" claim from the upstream design drives the value proposition.

### Unstated assumption: the `--recipe` flag gap

Current CI workflows pass `--recipe <path>` directly to `tsuku install` inside containers. The sandbox currently supports `--recipe` mode (line 43-51 in `install_sandbox.go`), generating the plan from a recipe file. But the sandbox `buildSandboxScript()` in `executor.go:444` always calls `tsuku install --plan /workspace/plan.json`, which means the plan is pre-generated on the host.

This is actually fine -- the sandbox generates the plan on the host then runs it in the container. But the design should acknowledge this difference from CI's current pattern (where plan generation happens inside the container). When CI passes `TSUKU_REGISTRY_URL` today, it's to override where the recipe is fetched during `tsuku install` inside the container. With the sandbox approach, the recipe is resolved on the host, so `TSUKU_REGISTRY_URL` would need to be set on the host, not just inside the container. The `--env` flag wouldn't help with this specific variable unless the sandbox changes how plan generation works. This may reduce the value of `--env TSUKU_REGISTRY_URL` compared to what the design claims.

## 2. Missing Alternatives

### Decision 1 (Verification): One alternative worth considering

**Share `checkVerification()` logic by having the sandbox call the validate package directly.** The sandbox already imports `internal/validate` (see `executor.go:16`). Rather than duplicating the verify logic in the sandbox script (shell-level pattern matching), the sandbox could read the container's stdout/stderr and run `checkVerification()` in Go after the container exits. This would mean:

- No exit code 2 convention needed in the shell script
- Pattern matching happens in Go (where `validate.Executor.checkVerification()` already works)
- The `Verified` field is set by Go code, not by interpreting exit codes

This avoids the parallel pattern introduction concern of having two implementations of pattern matching (one in shell, one in Go). The design's chosen approach would need to implement `grep`-style pattern matching in the shell script, which is a second place where the verify pattern logic lives.

The tradeoff is that stdout/stderr would need to distinguish install output from verify output. The validate package already handles this because it runs the verify command as the last thing in the script and checks the combined output. The sandbox could do the same -- the verify command's output is at the end of stdout.

This isn't a blocking concern because the exit code approach works, but it's worth noting the duplication.

### Decision 2 (Env passthrough): Alternatives seem complete

The three alternatives (explicit key=value, allowlist, config file) cover the reasonable design space. The chosen approach matches docker's `--env` semantics, which is the right call for CI users who already know this pattern.

### Decision 3 (Structured output): One gap in the JSON schema

The proposed JSON output omits `stdout` and `stderr`:

```json
{
  "tool": "ruff",
  "passed": true,
  "verified": true,
  "exit_code": 0,
  "verify_exit_code": 0,
  "duration_ms": 4523,
  "error": null
}
```

But the current CI workflows don't use stdout/stderr from the docker run either -- they only capture exit codes. So the omission is consistent with CI's needs. The document acknowledges this in the security section ("JSON output doesn't include stdout or stderr").

One alternative not considered: **`--output-format` instead of `--json`**. This would allow future formats (e.g., `--output-format=junit-xml` for CI systems that consume JUnit). However, with only one structured format needed today, `--json` is simpler and matches the convention used by tools like `docker inspect`, `kubectl`, and `gh`. Not a gap worth raising.

## 3. Rejection Rationale Fairness

### Decision 1 alternatives: Fair

"Run verification as a separate post-sandbox step on the host" is rejected because the container filesystem is cleaned up. This is accurate -- `executor.go:214` calls `os.RemoveAll(workspaceDir)` in a defer, and the container uses `--rm`. Keeping the container running would require changing the sandbox execution model, which is out of scope.

"Add full verify-tool.sh functionality" is rejected because it couples sandbox to CI test policies. This is fair -- `verify-tool.sh` and `verify-binary.sh` test properties beyond what the recipe defines (ELF linking, RPATH analysis), and putting those in the sandbox would be a scope creep.

### Decision 2 alternatives: Fair

"Automatic passthrough of allowlisted vars" is correctly rejected as a maintenance burden. Every new CI pattern would need a sandbox code change.

"Config file for sandbox environment" is rejected because it adds indirection for a caller-knows-best scenario. Fair -- CI workflows already have the values in their env; a config file is an unnecessary layer.

### Decision 3 alternatives: Fair

"Write results to a file" is rejected because it adds temp file management. Fair for the CI use case, though file output is actually useful for scenarios where stdout is polluted (which `--json` handles by suppressing other output). Not a strawman.

"Encode everything in exit codes" is rejected as the sole mechanism but kept as a complement. This is the right call -- exit codes can't carry timing or distinguish between "verify failed" and "no verify command."

### No strawmen detected

All alternatives address real design needs and have specific, technical rejection reasons. None are constructed to fail.

## 4. Unstated Assumptions

### 4a. PlanVerify does not carry ExitCode

The `PlanVerify` struct (`internal/executor/plan.go:80-83`) has only `Command` and `Pattern`:

```go
type PlanVerify struct {
    Command string `json:"command,omitempty"`
    Pattern string `json:"pattern,omitempty"`
}
```

But the recipe's `VerifySection` (`internal/recipe/types.go:772-779`) also has `ExitCode *int`. The validate package's `checkVerification()` uses `r.Verify.ExitCode` (line 336-337 in `validate/executor.go`). The sandbox doesn't have access to the recipe at verification time -- only the plan. So the sandbox's shell-level verification would assume exit code 0 means success, which is wrong for recipes that set a non-zero expected exit code.

The design should either:
1. Add `ExitCode *int` to `PlanVerify` (plan format version bump), or
2. Document that recipes with non-default exit codes won't verify correctly in sandbox mode, or
3. Have the sandbox access the recipe object (available in `runSandboxInstall` when `--recipe` is used, but not when `--plan` is used)

This is a potential correctness issue that could cause false negatives for recipes that intentionally exit non-zero during verification.

### 4b. Sandbox uses `--plan` mode, but CI uses `--recipe` mode

As noted in section 1, CI currently passes `--recipe <path>` to `tsuku install` inside containers. The sandbox pre-generates a plan on the host and passes `--plan` to the container. This means:

- Version resolution happens on the host (where GITHUB_TOKEN is available), not in the container
- `TSUKU_REGISTRY_URL` is consumed during plan generation on the host, not inside the container

The design says `--env TSUKU_REGISTRY_URL` passes the URL into the container, but the sandbox doesn't run `tsuku install --recipe` inside the container -- it runs `tsuku install --plan`. The registry URL isn't used during plan execution. The env passthrough of `TSUKU_REGISTRY_URL` would only matter if the sandbox changed to run `tsuku install --recipe` inside the container, which is a different execution model.

For `GITHUB_TOKEN`, the same logic applies: version resolution happens on the host during `generateInstallPlan()` at `install_sandbox.go:45-48`, not inside the container. Passing `GITHUB_TOKEN` into the container would only help if the install plan has network-requiring steps that call GitHub APIs (which some ecosystem builds might).

The design should clarify this distinction and explain which env vars need host-side availability vs container-side availability.

### 4c. `DEBIAN_FRONTEND` is hardcoded but not all families use apt

The hardcoded env vars include `DEBIAN_FRONTEND=noninteractive` (line 270 in `executor.go`). This is harmless on non-Debian systems but semantically misleading. The design says hardcoded vars can't be overridden by `--env`, which is fine. This isn't a bug, just a note that the hardcoded list is Debian-biased.

### 4d. Pattern matching in shell vs Go

The design says the sandbox script will run the verify command and check pattern matching. The validate package's `checkVerification()` uses `strings.Contains(output, r.Verify.Pattern)` in Go. Replicating this in shell would use `grep -F` or similar. These are semantically equivalent for simple substring matching, but the design should specify which shell construct to use and confirm it handles multi-line output and special characters the same way.

## 5. Structural Fit Assessment

### Positive: Extends existing patterns correctly

- `SandboxRequirements` gains `ExtraEnv []string` -- this follows the same struct-extension pattern used for `Resources` and `RequiresNetwork`. No new types needed.
- `SandboxResult` gains `Verified bool` and `DurationMs int64` -- same pattern. Existing callers that don't check these fields are unaffected (Go zero values are `false` and `0`).
- `--env` and `--json` are CLI flags on the existing `--sandbox` path -- no new subcommands, no CLI surface duplication.
- The sandbox reads `plan.Verify` from the `InstallationPlan`, which already carries this data. No new data flow needed.

### Positive: Maintains separation of concerns

- Retry logic stays in CI workflows (the document is explicit about this)
- Batching stays in CI workflows
- Binary quality checks stay in CI scripts
- The sandbox remains a "one run of one recipe" primitive

### Advisory: Verify logic duplication between sandbox and validate

The validate package has `buildPlanInstallScript()` which appends the verify command to the script and `checkVerification()` which checks the result in Go. The sandbox design proposes putting verify logic in `buildSandboxScript()` (shell-level) with exit code 2 for failures. This creates two implementations of verification:

1. `validate.Executor`: runs verify in shell, checks pattern in Go via `checkVerification()`
2. `sandbox.Executor` (proposed): runs verify in shell, checks result via exit code convention

These will diverge over time. When the validate package's verification evolves (e.g., supporting `Additional` verify commands from `VerifySection.Additional`), the sandbox won't get the update unless someone remembers to update it too.

The design mentions "The pattern matching logic (`checkVerification()`) can be extracted into a shared function or replicated in the sandbox." The "can be" framing is soft -- the implementation plan should specify which approach is taken. Extracting into a shared function in the `validate` package (or a new `internal/verify` package) would prevent divergence. Replicating introduces the parallel pattern risk.

### Not blocking because: the duplication is contained to two files, the verify logic is simple (substring match), and the sandbox and validate packages already share structural patterns (similar Result types, similar script generation). The risk of divergence is real but bounded.

## Summary of Findings

| # | Finding | Level |
|---|---------|-------|
| 1 | `PlanVerify` lacks `ExitCode` field -- recipes with non-default expected exit codes will produce false verify failures in sandbox mode | Should be addressed before implementation |
| 2 | `TSUKU_REGISTRY_URL` is consumed during plan generation on the host, not inside the container -- passing it via `--env` into the container has no effect for the sandbox's `--plan` execution mode | Should be clarified in the design |
| 3 | Verify logic duplication between sandbox and validate packages -- "can be extracted or replicated" should pick one approach | Advisory -- specify approach during implementation |
| 4 | Shell-level pattern matching (sandbox) vs Go-level pattern matching (validate) creates a parallel implementation | Advisory -- contained, but extraction into shared code is preferred |
| 5 | Missing explicit list of which 12 CI workflows are migration candidates | Advisory -- strengthens the value proposition |
| 6 | The problem statement and overall structure are sound. Alternatives are not strawmen. Rejection rationale is specific and fair. | Positive |

## Recommendations

1. **Add `ExitCode *int` to `PlanVerify` before implementing sandbox verification.** This is a plan format concern, not a sandbox concern. The plan already carries verify info; it should carry all of it. This may require a format version bump (or just an additive field, since JSON omitempty handles backward compatibility).

2. **Clarify in the design which env vars are host-side vs container-side.** `GITHUB_TOKEN` matters on the host during plan generation and potentially in the container for network-requiring steps. `TSUKU_REGISTRY_URL` only matters on the host. The `--env` flag is container-side only. Document this split.

3. **Commit to extracting `checkVerification()` into shared code** rather than leaving the approach open. The `validate` package's implementation already works; making it importable by the sandbox package (or moving it to a shared internal package) is straightforward.

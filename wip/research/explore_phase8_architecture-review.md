# Architecture and Security Review: DESIGN-sandbox-ci-integration

**Reviewer**: architect-reviewer
**Date**: 2026-02-22
**Design**: `docs/designs/DESIGN-sandbox-ci-integration.md`
**Status**: Proposed

---

## Architecture Review

### 1. Is the architecture clear enough to implement?

**Mostly yes**, with two gaps that would block or confuse implementation.

#### Blocking: `PlanVerify` is missing `ExitCode`

The design says the sandbox script will run the verify command from `plan.Verify.Command` and check against `plan.Verify.Pattern`. But the `PlanVerify` struct (`internal/executor/plan.go:80-83`) only has `Command` and `Pattern`:

```go
type PlanVerify struct {
    Command string `json:"command,omitempty"`
    Pattern string `json:"pattern,omitempty"`
}
```

Meanwhile, the `validate` package's `checkVerification()` (`internal/validate/executor.go:333-351`) also checks `r.Verify.ExitCode`, which is a `*int` on the recipe's verify struct (`internal/recipe/types.go:778`). Some recipes specify non-zero expected exit codes.

If the sandbox only checks exit code 0 for verify success (as the design implies with the "exit 2 for verify failure" convention), recipes with custom expected exit codes will incorrectly fail. The design needs to either:

1. Add `ExitCode *int` to `PlanVerify` and carry it through plan generation, or
2. Explicitly state that the sandbox assumes exit code 0 and document this as a known limitation.

Option 1 is the correct fix since it achieves the stated goal of "validate parity." This is a plan schema change (format version bump from 4 to 5).

#### Advisory: Pattern matching location not specified

The design says pattern matching "can be extracted into a shared function or replicated in the sandbox." This ambiguity is fine at design time, but implementors should know: the `checkVerification()` logic in `internal/validate/executor.go:333-351` operates on the `RunResult` (stdout + stderr) on the Go side, after the container exits. The sandbox script approach described in the design would need to do pattern matching *inside the shell script* (using `grep` or similar), since the exit code is the only signal from the container.

Two options:

- **Shell-side pattern matching**: The sandbox script runs the verify command, captures output, checks for the pattern with `grep -q`, and exits 2 if not found. Simple but duplicates logic and requires `grep` in minimal containers.
- **Go-side pattern matching**: The sandbox script runs the verify command but always exits 0 if the command succeeds. The Go code then checks `result.Stdout`/`result.Stderr` for the pattern, exactly like `validate.checkVerification()`. This is cleaner and can share the function.

The design's exit code convention (0/1/2) implies shell-side matching. The Go-side approach could use the same 0/1 scheme and handle pattern matching after container exit, which avoids the "exit 2" convention entirely. The implementor needs clarity on which path to take.

**Recommendation**: Go-side pattern matching. It enables direct reuse of `checkVerification()` (extractable to a shared `internal/verify` package or similar), avoids needing `grep` in Alpine containers, and keeps the shell script simple.

### 2. Are there missing components or interfaces?

#### The `--json` flag already exists on `install`

The design proposes a `--json` flag for `tsuku install --sandbox`. The `install` command already has `--json` (the `installJSON` variable at `cmd/tsuku/install.go:21`) with its own JSON schema (`installError` struct at line 329). The sandbox `--json` output uses a different schema (`{"tool", "passed", "verified", ...}`).

This is not a conflict since the existing `--json` applies to error output and the sandbox `--json` applies to result output. But the implementation needs to handle the interaction: when `--sandbox --json` is set, `installJSON` is already true. The sandbox code path in `install.go:57-86` calls `runSandboxInstall()` and currently uses `printError`/`exitWithCode` for failure, bypassing `handleInstallError()` (which is the `--json`-aware error handler). The sandbox `--json` output path needs to coexist with the existing `--json` error handling.

**Not blocking** -- the paths are separable since sandbox takes an early return at line 86. But the design should mention this interaction to prevent the implementor from accidentally producing two JSON objects (one sandbox result, one error) on failure.

#### `--env` with value-from-host needs a security note

The design describes `--env KEY` (without `=VALUE`) reading from the host environment. This is docker-compatible behavior. The design's security section says env vars are "opt-in only" but doesn't call out the value-from-host variant specifically. A user writing `--env AWS_SECRET_ACCESS_KEY` would pass the secret from their shell into the container, which is the expected docker behavior but worth a note in the documentation.

Not a design gap -- just a documentation note.

### 3. Are the implementation phases correctly sequenced?

**Yes.** The phases are independent and correctly ordered:

- Phase 1 (verification) changes `buildSandboxScript()` and `SandboxResult`. No external dependencies.
- Phase 2 (env passthrough) changes `SandboxRequirements` and `RunOptions.Env`. Independent of Phase 1.
- Phase 3 (JSON output) changes CLI output formatting. Depends on `SandboxResult` having the `Verified` field from Phase 1.
- Phase 4 (CI migration) is documentation only.

The design's suggestion to ship Phases 1-3 in a single PR is reasonable since they're all additive. Phase 3 has a soft dependency on Phase 1 (`Verified` field), but that's within the same PR.

### 4. Are there simpler alternatives we overlooked?

#### Alternative: Reuse `validate.Executor` instead of extending `sandbox.Executor`

The `validate.Executor.Validate()` method already does install + verify + pattern matching inside a container. The sandbox could delegate to the validate package for the verify step instead of reimplementing it. However, the two packages have different concerns:

- `validate` generates its own plan on the host and runs in a fixed Debian image
- `sandbox` accepts a pre-generated plan and runs across multiple families

Trying to unify them would be a larger refactoring that's not justified by this design. The design's approach of mirroring the verify logic is the simpler path. The code duplication is small (pattern matching is ~15 lines).

#### Alternative: Use `tsuku verify` inside the container

Instead of appending the verify command to the sandbox script, run `tsuku verify <tool>` inside the container after `tsuku install`. This would use tsuku's own verification logic end-to-end. However, `tsuku verify` needs the recipe to be in the registry, the tool to be in the state file, and the current binary symlinks to be set up. The sandbox installs via `--plan`, which may not fully populate all the state that `tsuku verify` expects. This approach would couple the sandbox to tsuku's internal state management, adding fragility for no real benefit.

### 5. Does the exit code convention (0/1/2) work reliably in practice?

**Partially.** There are two concerns:

#### Container exit codes vs process exit codes

The exit code from the sandbox script (0/1/2) becomes the container's exit code. The container runtime (docker/podman) may transform exit codes in edge cases:
- OOM kill: exit code 137
- Timeout kill: exit code 143 (or runtime-dependent)
- Signal-based termination: 128 + signal number

The design's `Sandbox()` method maps exit codes to results. It needs to handle codes outside 0/1/2 as "unknown failure" rather than treating all non-zero as install failure. The current code at `internal/sandbox/executor.go:313` does `passed := result.ExitCode == 0`, which is already correct for the binary case. The new code would need:

```go
switch result.ExitCode {
case 0:
    passed = true; verified = true
case 2:
    passed = true; verified = false  // install ok, verify failed
default:
    passed = false; verified = false  // install failed or infrastructure error
}
```

This works. The risk is exit code 2 from an unrelated cause (e.g., a command in the install step that happens to exit 2). The `set -e` in the script means any non-zero exit from the install step would terminate the script before reaching the verify step, so exit code 2 from the install step would be misclassified as "install passed, verify failed."

**Mitigation**: Use `set -e` through the install step, then explicitly capture the verify exit code:

```bash
# Install (exits script on failure, any non-zero = install failure)
set -e
tsuku install --plan /workspace/plan.json --force
set +e

# Verify
output=$(verify_command 2>&1)
verify_rc=$?
if [ $verify_rc -ne 0 ]; then
    exit 2
fi
# Pattern check here if shell-side, or just exit 0 if Go-side
exit 0
```

This ensures exit 2 can *only* come from the verify path. Worth specifying in the design or leaving as an implementation note.

#### Conflict with CLI's ExitUsage = 2

The sandbox *container* exit codes (0/1/2) are distinct from the CLI process exit codes (`exitcodes.go`). The CLI exits with `ExitInstallFailed = 6` when sandbox fails (`install.go:84`). So there's no confusion at the CLI level. But if a caller inspects the JSON output's `exit_code` field, they'll see the container's 0/1/2, which is fine since the JSON schema documents these as container exit codes, not CLI exit codes.

**Advisory, not blocking.** The design should note that the `exit_code` in JSON refers to the container exit code, not the CLI exit code. The CLI always exits 0 (success) or 6 (install failed) regardless of verify status.

---

## Security Review

### 1. Attack vectors with `--env`

#### Considered and adequate:

- **Host env leakage**: Mitigated by opt-in only. No automatic passthrough.
- **Sandbox environment subversion**: Mitigated by hardcoded vars taking precedence.
- **Token exposure in logs**: Mitigated by not logging env values.

#### Not explicitly considered:

**a) Shell injection via `--env` values**

If `--env` values are interpolated into the sandbox script (e.g., to set them via `export KEY=VALUE` lines), a value like `; rm -rf /` could inject commands. However, the design routes env vars through `RunOptions.Env`, which passes them to `docker run -e`, not through the shell script. Docker's `-e` flag handles values safely (no shell interpretation). **No vulnerability**, but worth confirming in implementation that the values never enter the shell script.

**b) Env var name injection**

A malicious `--env` value like `--env "FOO=bar\nMALICIOUS=evil"` could potentially inject additional env vars depending on how the container runtime parses environment variables. In practice, both Docker and Podman treat the entire string after the first `=` as the value, so this is safe. **No vulnerability.**

**c) Information exfiltration via DNS/network when `RequiresNetwork=true`**

When the sandbox has network access (ecosystem builds), a malicious recipe could exfiltrate passed env vars (like `GITHUB_TOKEN`) via DNS queries or HTTP requests. This is the same risk as running any untrusted code with network access and tokens -- the design correctly notes this is equivalent to the current CI pattern where `docker run -e GITHUB_TOKEN` is used with network access. **Accepted risk**, correctly characterized.

**d) `--env KEY` (value-from-host) with sensitive vars**

If a user accidentally types `--env HOME` or `--env PATH` without `=VALUE`, the design says hardcoded vars win. But what about other sensitive vars like `SSH_AUTH_SOCK`, `AWS_SECRET_ACCESS_KEY`, etc.? The design's "opt-in only" mitigation is sufficient -- the user has to explicitly name each variable. This is the same security model as `docker run -e`. **No additional mitigation needed.**

### 2. Are the mitigations sufficient?

**Yes**, with one note: the design should specify the *order* of env var processing to make the precedence guarantee verifiable:

```go
// Hardcoded vars first
env := []string{
    "TSUKU_SANDBOX=1",
    "TSUKU_HOME=/workspace/tsuku",
    // ...
}
// Then user vars, filtered
for _, e := range reqs.ExtraEnv {
    key := strings.SplitN(e, "=", 2)[0]
    if !isHardcodedVar(key) {
        env = append(env, e)
    }
}
```

The "last value wins" behavior for duplicate env var keys depends on the container runtime. Docker uses last-wins for `-e` flags, but if the vars are passed as a list, the runtime may use first-wins. The safer approach is to **filter out** user-provided vars that conflict with hardcoded ones, rather than relying on ordering. The design should specify filtering, not ordering.

### 3. Is there residual risk to escalate?

**No.** All identified risks are either mitigated or are accepted risks equivalent to the current CI pattern. The design doesn't expand the attack surface beyond what `docker run -e` already provides.

### 4. Are any "not applicable" justifications actually applicable?

#### Download Verification: correctly not applicable

The design doesn't change download or artifact verification. Plan execution with pre-resolved checksums is unchanged.

#### Supply Chain Risks: correctly characterized

The design notes that verify commands come from recipes reviewed in PRs. The trust boundary is the recipe, not the sandbox. This is accurate.

**One nuance**: The design says "A compromised recipe could include a malicious verify command, but this is the same risk as a compromised recipe including malicious install steps." This is true, but with env passthrough, a malicious verify command now has access to `GITHUB_TOKEN` if it was passed via `--env`. Previously, the sandbox didn't expose tokens, so a malicious recipe in sandbox mode couldn't exfiltrate them. With env passthrough, it can (when the sandbox has network access). This is a minor escalation of the existing risk, not a new category.

**Recommendation**: Add a note that `--env` should only be used with trusted recipes (i.e., recipes from the official registry or reviewed PRs). For untrusted local recipes (`--recipe`), the existing confirmation prompt could warn about env passthrough.

---

## Summary of Findings

### Blocking

| # | Finding | Location |
|---|---------|----------|
| 1 | `PlanVerify` missing `ExitCode *int` field -- recipes with custom expected exit codes will fail verification | `internal/executor/plan.go:80-83` |
| 2 | Shell script exit code 2 could come from install step (not just verify) unless `set -e`/`set +e` boundary is explicit | Design section: "Decision 1" |

### Advisory

| # | Finding | Recommendation |
|---|---------|---------------|
| 3 | Pattern matching location (shell-side vs Go-side) is ambiguous | Specify Go-side matching to reuse `checkVerification()` |
| 4 | Existing `--json` on install has different schema than proposed sandbox `--json` | Document the interaction; ensure only one JSON object on stdout per invocation |
| 5 | Env var precedence should use filtering, not ordering | Filter out conflicting keys explicitly rather than relying on append order |
| 6 | Env passthrough + network access is a minor escalation for untrusted recipes | Add documentation note; consider warning in confirmation prompt |
| 7 | JSON `exit_code` field is container exit code, not CLI exit code | Document in JSON schema description |

### Out of Scope

- Code duplication between `sandbox` and `validate` packages (acceptable; both packages serve different use cases)
- Whether Phases 1-3 should be one PR or separate (project management, not architecture)
- The four hardcoded images outside `container-images.json` (tracked by sandbox image unification work)

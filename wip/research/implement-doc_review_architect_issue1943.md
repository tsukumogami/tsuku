# Architect Review: Issue #1943 (--env flag for environment variable passthrough)

**0 blocking, 1 advisory**

Commit: da90728675d031e5f1c293b32101fbd9300b23b8

## Summary

The change adds an `ExtraEnv []string` field to `SandboxRequirements`, a `--env` CLI flag on the install command, env resolution logic in `install_sandbox.go`, and env filtering in `executor.go`. The implementation follows the existing architecture cleanly.

## Structural Assessment

**Layering is correct.** Data flows downward: CLI (`cmd/tsuku/install.go`) reads flags, `install_sandbox.go` resolves KEY-only entries against the host environment, populates `SandboxRequirements.ExtraEnv`, and the sandbox package (`executor.go`) filters protected keys before appending to `validate.RunOptions.Env`. No upward dependencies introduced. The sandbox package does not import cmd/, and the validate package is unaware of the feature.

**Interface contract preserved.** `validate.RunOptions.Env` already accepts `[]string` in `KEY=value` format (`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/validate/runtime.go:49`). The sandbox executor appends filtered entries to this existing slice. No changes to the validate package were needed or made.

**No parallel patterns.** The `filterExtraEnv` function in `executor.go` is the single filtering point. The `resolveEnvFlags` function in `install_sandbox.go` is the single resolution point for KEY-only entries. Both are called exactly once in their respective layers.

**`StringArrayVar` over `StringSliceVar` is the right choice.** `StringSliceVar` would split on commas, breaking values like `CONFIG=a,b,c`. This matches docker's `--env` semantics.

**Design doc alignment.** The implementation matches `docs/designs/DESIGN-sandbox-ci-integration.md` lines 293-302 and 373-374 exactly.

## Findings

### ADVISORY: --env silently ignored without --sandbox

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install.go:229`

The `--env` flag is registered on the install command globally but only consumed inside the `runSandboxInstall` path (line 65-67 of `install_sandbox.go`). A user running `tsuku install kubectl --env GITHUB_TOKEN` (without `--sandbox`) gets no error -- the flag is silently ignored.

This is consistent with how `--target-family` behaves (also only consumed in sandbox mode, also silently ignored otherwise). So it does not introduce a new pattern. But the help text says "Pass environment variable to sandbox container" which provides enough signal. If the team wants to add validation later, it would be a single check in the Run function before the sandbox branch.

**Impact:** Contained. Does not compound. Users reading `--help` will see the sandbox-specific description.

## What Fits Well

- The `protectedEnvKeys` map in `executor.go:34-40` is a clear, centralized blocklist. Adding new protected keys requires touching one place.
- Filtering happens at the executor level (right before `RunOptions` construction), not at the CLI level. This means any future caller of `Executor.Sandbox()` (e.g., batch orchestrator, programmatic API) gets the same protection without reimplementing filtering.
- The `SandboxRequirements` struct extension follows the same pattern as `RequiresNetwork`, `Image`, and `Resources` -- a data field populated by the caller and consumed by the executor.

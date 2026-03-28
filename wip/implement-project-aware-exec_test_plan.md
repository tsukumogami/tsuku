# Test Plan: Project-Aware Exec Wrapper

Generated from: docs/plans/PLAN-project-aware-exec.md
Issues covered: 4
Total scenarios: 14

---

## Scenario 1: Resolver returns pinned version for project-declared tool
**ID**: scenario-1
**Testable after**: #1
**Commands**:
- Create a temp directory with `.tsuku.toml` containing `[tools]\nripgrep = "14.1.0"`
- `go test ./internal/project/... -run TestResolver`
**Expected**: `ProjectVersionFor(ctx, "rg")` returns `("14.1.0", true, nil)` when the binary index maps `rg` -> `ripgrep` and the project config declares `ripgrep = "14.1.0"`
**Status**: pending

---

## Scenario 2: Resolver returns not-found for tool not in project config
**ID**: scenario-2
**Testable after**: #1
**Commands**:
- `go test ./internal/project/... -run TestResolverNotInConfig`
**Expected**: `ProjectVersionFor(ctx, "jq")` returns `("", false, nil)` when `jq` maps to recipe `jq` via the index but `jq` is not declared in `.tsuku.toml`
**Status**: pending

---

## Scenario 3: Resolver handles nil config gracefully
**ID**: scenario-3
**Testable after**: #1
**Commands**:
- `go test ./internal/project/... -run TestResolverNilConfig`
**Expected**: `NewResolver(nil, lookup)` creates a resolver where all calls return `("", false, nil)` without panicking
**Status**: pending

---

## Scenario 4: Resolver propagates index lookup errors
**ID**: scenario-4
**Testable after**: #1
**Commands**:
- `go test ./internal/project/... -run TestResolverLookupError`
**Expected**: When the `LookupFunc` returns an error, `ProjectVersionFor` propagates it as a non-nil error
**Status**: pending

---

## Scenario 5: Mode override to auto when resolver returns a version
**ID**: scenario-5
**Testable after**: #1
**Commands**:
- `go test ./internal/autoinstall/... -run TestModeOverride`
**Expected**: When `Runner.Run` is called with `ModeConfirm` and the resolver returns `ok=true`, the effective mode becomes `ModeAuto` -- the tool is installed without prompting and an audit log entry is written
**Status**: pending

---

## Scenario 6: No mode override when resolver returns not-found
**ID**: scenario-6
**Testable after**: #1
**Commands**:
- `go test ./internal/autoinstall/... -run TestNoModeOverride`
**Expected**: When the resolver returns `ok=false`, the original mode is preserved. With `ModeConfirm`, the user is prompted. With `ModeSuggest`, instructions are printed.
**Status**: pending

---

## Scenario 7: No mode override when resolver is nil
**ID**: scenario-7
**Testable after**: #1
**Commands**:
- `go test ./internal/autoinstall/... -run TestNilResolver`
**Expected**: When resolver is `nil`, behavior is identical to pre-feature: no version override, no mode escalation. This is the backward-compatibility case.
**Status**: pending

---

## Scenario 8: tsuku run wires resolver from project config
**ID**: scenario-8
**Testable after**: #1
**Commands**:
- Create a directory with `.tsuku.toml` declaring a tool
- `cd` into that directory and run `tsuku run <command> --mode=suggest`
**Expected**: `tsuku run` calls `LoadProjectConfig` with the working directory, constructs a resolver, and passes it to `Runner.Run` instead of `nil`. When the tool is declared in the config, the suggest mode is overridden to auto.
**Status**: pending

---

## Scenario 9: TTY gate bypassed for project-declared tools
**ID**: scenario-9
**Testable after**: #1
**Commands**:
- Create a directory with `.tsuku.toml` declaring `ripgrep = "14.1.0"`
- Run `echo "" | tsuku run rg` (piped stdin, no TTY)
**Expected**: Exit code is NOT 12 (ExitNotInteractive). The TTY gate does not block execution because the project config triggers the auto mode override before the gate is evaluated. Without `.tsuku.toml`, the same command would exit 12 in confirm mode.
**Status**: pending

---

## Scenario 10: Bash hook calls tsuku run for project-declared tool
**ID**: scenario-10
**Testable after**: #1, #2
**Commands**:
- Create a directory with `.tsuku.toml` declaring a tool
- Source the bash hook script
- Invoke `command_not_found_handle <tool>` in that directory
**Expected**: The hook calls `tsuku run <tool>` (not `tsuku suggest <tool>`) when `.tsuku.toml` declares the tool's recipe
**Status**: pending

---

## Scenario 11: Bash hook falls back to suggest when tool not in config
**ID**: scenario-11
**Testable after**: #1, #2
**Commands**:
- Create a directory with `.tsuku.toml` that does NOT declare the tool
- Source the bash hook script
- Invoke `command_not_found_handle <unknown-tool>`
**Expected**: The hook calls `tsuku suggest <unknown-tool>` (existing behavior preserved)
**Status**: pending

---

## Scenario 12: Shim install creates correct script and shim list shows it
**ID**: scenario-12
**Testable after**: #1, #3
**Commands**:
- `tsuku shim install ripgrep`
- `tsuku shim list`
- `cat $TSUKU_HOME/bin/rg`
**Expected**: `shim install` creates a shell script at `$TSUKU_HOME/bin/rg` containing `#!/bin/sh\nexec tsuku run "$(basename "$0")" -- "$@"`. `shim list` includes `rg` with recipe `ripgrep`.
**Status**: pending

---

## Scenario 13: Shim refuses to overwrite non-shim files
**ID**: scenario-13
**Testable after**: #1, #3
**Commands**:
- Place a regular file (not a shim) at `$TSUKU_HOME/bin/rg`
- `tsuku shim install ripgrep`
**Expected**: The command fails with a clear error message indicating it won't overwrite an existing non-shim file. The original file is preserved.
**Status**: pending

---

## Scenario 14: End-to-end project-aware install via tsuku run
**ID**: scenario-14
**Testable after**: #1, #2, #3
**Environment**: manual
**Commands**:
- Create a fresh directory with `.tsuku.toml`:
  ```toml
  [tools]
  serve = "0.8.1"
  ```
- Ensure `serve` is NOT installed: `tsuku remove serve` (if present)
- Run: `tsuku run serve --help`
**Expected**: tsuku detects the project pin, installs serve@0.8.1 without prompting (auto mode override from project config), and execs `serve --help` which prints usage information. The tool appears in `tsuku list` afterward at version 0.8.1.
**Status**: pending

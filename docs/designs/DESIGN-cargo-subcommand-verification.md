---
status: Accepted
problem: |
  The CargoBuilder generates `<executable> --version` as the verify command
  for all cargo crates. This fails for cargo subcommands (crates named
  `cargo-*`) that require invocation through cargo itself. The batch
  generation system creates these recipes at scale, so manual workarounds
  don't work. Users see verification failures on correctly installed tools.
decision: |
  Change `CargoBuilder.Build()` to detect cargo subcommands (executables
  matching `cargo-*`) and generate `cargo <subcommand> --version` as the
  verify command. Add a repair loop fallback for the same pattern to handle
  existing recipes. Also set the `Pattern` field to `{version}` to enable
  version validation, which the current code omits.
rationale: |
  Fixing the builder prevents bad verify commands from being generated. The
  repair loop fallback provides defense in depth for existing recipes. Using
  `cargo <subcommand> --version` for all subcommands is simpler than probing
  at build time, and the cargo PATH dependency is already satisfied by
  `cargo_install`'s own requirements.
---

# DESIGN: Cargo Subcommand Verification

## Status

**Accepted**

## Context and Problem Statement

Tsuku's `CargoBuilder` generates recipe files for Rust crates from crates.io. For every crate, it produces a `[verify]` section with `command = "<executable> --version"`. This works for standalone Rust binaries like `ripgrep` or `bat`, but fails for cargo subcommands — crates whose executables are named `cargo-<subcommand>`.

Cargo subcommands are designed to be invoked through cargo itself: `cargo hack`, not `cargo-hack`. When called directly as `cargo-hack --version`, many subcommands reject the `--version` flag because they expect cargo to have already consumed the subcommand name from the argument list. The result: verification fails on a correctly installed tool.

```
$ cargo-hack --version
error: expected subcommand 'hack', found argument '--version'

$ cargo hack --version
cargo-hack 0.6.43
```

This affects every cargo subcommand recipe the batch generation system creates. The behavior is inconsistent across crates — some (like `cargo-nextest`) handle direct invocation fine, while others (like `cargo-hack`, `cargo-llvm-cov`, `cargo-outdated`) don't. Since the batch system creates recipes at scale, manual workarounds aren't viable.

### Scope

**In scope:**
- Fixing verify command generation in `CargoBuilder` for cargo subcommands
- Adding a cargo subcommand fallback to the repair loop
- Handling the case where some subcommands work with direct invocation and others don't

**Out of scope:**
- Changing how `cargo_install` action installs crates (it works correctly)
- Fixing individual recipes manually (batch generation makes this unsustainable)
- Adding cargo subcommand detection to other builders (only the Cargo builder produces these)

## Decision Drivers

- Cargo subcommands require `cargo` in PATH to invoke via `cargo <subcommand>`. The `cargo_install` action uses `exec.LookPath("cargo")` to find cargo, so it must be in PATH. During sandbox validation, cargo is present because the `cargo_install` step just ran. During user installation, cargo is present for the same reason. The sandbox PATH includes system directories (`/usr/local/bin`, `/usr/bin`) and the tool's bin directory.
- The repair loop (`orchestrator.go`) already handles verify failures by trying `--help` and `-h` fallbacks, but doesn't try `cargo <subcommand>` invocation patterns.
- Verify commands should do version checking when possible (`--version` with pattern matching), not just prove the binary exists.
- The batch generation system (`cmd/batch-generate`) creates recipes at scale, so the fix must work automatically without manual recipe editing.
- The cargo-edit recipe demonstrates the current manual workaround: `command = "cargo-add --help"` with output pattern matching. This loses version validation.

## Considered Options

### Decision 1: Where to Apply the Fix

The verify command originates in `CargoBuilder.Build()` at `cargo.go:160`, where it always generates `fmt.Sprintf("%s --version", executables[0])`. The repair loop in `orchestrator.go` can catch verification failures post-hoc. The question is where the fix belongs.

#### Chosen: Fix CargoBuilder and Add Repair Loop Fallback

Change `CargoBuilder.Build()` to detect cargo subcommands (executables matching `cargo-*`) and generate `cargo <subcommand> --version` as the verify command. Also add `cargo <subcommand> --version` to the repair loop's fallback list so existing recipes with the wrong verify command get corrected during re-generation.

The CargoBuilder fix prevents bad verify commands from being generated in the first place. The repair loop fallback catches recipes that were generated before the fix, or edge cases where the initial `cargo <subcommand> --version` doesn't work and a different invocation pattern is needed.

#### Alternatives Considered

**Repair loop only**: Add `cargo <subcommand> --version` as a fallback in the repair loop without changing CargoBuilder. Every new recipe would still be generated with the wrong verify command, require a sandbox validation failure, then get repaired. This adds unnecessary latency to every cargo subcommand recipe build and means every recipe goes through the repair path even though the correct command is predictable.

**Manual recipe fixes**: Edit individual recipes to use `--help` with pattern matching (like the cargo-edit workaround). This loses version validation, doesn't scale to batch generation, and requires manual intervention for each new cargo subcommand recipe.

### Decision 2: How to Handle the Inconsistency

Some cargo subcommands work with `cargo-<name> --version` directly (cargo-nextest, cargo-expand) while others don't (cargo-hack, cargo-llvm-cov). The verify command needs to handle both cases.

#### Chosen: Default to Cargo Invocation for All Subcommands

For any executable matching `cargo-*`, generate `cargo <subcommand> --version` as the verify command. This works for all cargo subcommands regardless of whether they support direct invocation, because cargo always handles `--version` dispatching correctly.

For subcommands that also work with direct invocation, `cargo <subcommand> --version` is still correct — it just takes a slightly different path to the same result. The one trade-off is requiring cargo in PATH at verification time, but this is already guaranteed since `cargo_install` requires cargo.

#### Alternatives Considered

**Try direct first, fall back to cargo invocation**: Generate `cargo-<name> --version` and rely on the repair loop to fix failures by trying `cargo <name> --version`. This means the common case (most subcommands don't support direct `--version`) always goes through the repair path, wasting time.

**Detect at build time whether direct invocation works**: Run both patterns during recipe generation and pick the one that works. This doubles the work during generation and adds complexity. Since `cargo <subcommand> --version` always works, there's no need to probe.

## Decision Outcome

**Chosen: 1A + 2A**

### Summary

The fix changes `CargoBuilder.Build()` to detect cargo subcommands and generate the correct verify command from the start. When the first executable in the `executables` list matches the pattern `cargo-*`, the builder strips the `cargo-` prefix and generates `cargo <subcommand> --version` instead of `cargo-<subcommand> --version`. For executables that don't match this pattern, the existing `<exe> --version` behavior is unchanged.

In the repair loop (`orchestrator.go`), a new fallback is added between the existing `--help` and `-h` fallbacks. When the original verify command fails, the loop checks if the binary name starts with `cargo-`. If so, it tries `cargo <subcommand> --version` before falling back to `--help` and `-h`. This handles existing recipes with the wrong verify command, and also handles the unlikely case where `cargo <subcommand> --version` doesn't work in the initial generation but works at repair time.

The verify pattern should remain `{version}` so tsuku validates that the installed version matches the expected version. The `cargo <subcommand> --version` output follows the same format as `cargo-<subcommand> --version` for subcommands that support it — typically `cargo-<name> <version>`.

### Rationale

Fixing the builder is preferable to relying on the repair loop because it avoids the unnecessary sandbox failure and repair cycle. The repair loop fallback provides defense in depth for existing recipes. Together, they handle new and old recipes without manual intervention.

Using `cargo <subcommand> --version` for all subcommands (even those that support direct invocation) is simpler than probing at build time. The trade-off — requiring cargo in PATH at verify time — is already satisfied by the `cargo_install` action's own requirements.

## Solution Architecture

### Overview

Two changes in two files:

1. `internal/builders/cargo.go` — Modify `Build()` to detect cargo subcommands
2. `internal/builders/orchestrator.go` — Add cargo subcommand fallback to `attemptVerifyRepair()`

### Component 1: CargoBuilder Verify Command Generation

In `Build()`, after discovering executables, check if the first executable matches `cargo-*`. The `VerifySection` should also set `Pattern` to enable version validation — the current code omits it, so only exit code 0 is checked.

```go
// Generate verify command and pattern
verifyExe := executables[0]
var verifyCommand string
if strings.HasPrefix(verifyExe, "cargo-") {
    subcommand := strings.TrimPrefix(verifyExe, "cargo-")
    verifyCommand = fmt.Sprintf("cargo %s --version", subcommand)
} else {
    verifyCommand = fmt.Sprintf("%s --version", verifyExe)
}

// ... in the recipe construction:
Verify: recipe.VerifySection{
    Command: verifyCommand,
    Pattern: "{version}",
},
```

### Component 2: Repair Loop Fallback

In `attemptVerifyRepair()`, add a cargo subcommand fallback before the existing `--help` and `-h` fallbacks. The existing `tryFallbackCommand()` hardcodes `mode: "output"` and `pattern: "usage"` (help-text matching), which isn't right for a version check. The cargo fallback should run the command directly and check for exit code 0, or use a dedicated code path that doesn't force help-text mode.

The simplest approach: insert the cargo fallback check before the Phase 2 fallback loop. Try `cargo <subcommand> --version`, and if it succeeds (exit 0), create the repaired verify section with the version command and no forced mode/pattern override.

```go
// For cargo subcommands, try cargo invocation before general fallbacks
if strings.HasPrefix(binaryName, "cargo-") {
    subcommand := strings.TrimPrefix(binaryName, "cargo-")
    cargoCmd := "cargo " + subcommand + " --version"

    // Run directly rather than through tryFallbackCommand,
    // because tryFallbackCommand forces help-text matching mode
    candidateVerify := recipe.VerifySection{
        Command: cargoCmd,
    }
    candidate := r.WithVerify(candidateVerify)
    result, err := o.validate(ctx, candidate)
    if err == nil && !result.Skipped && result.Passed {
        repaired := r.WithVerify(candidateVerify)
        meta := &VerifyRepairMetadata{
            OriginalCommand: originalCommand,
            RepairedCommand: cargoCmd,
            Method:          "fallback_cargo_subcommand",
        }
        return repaired, meta
    }
}
```

### Data Flow

1. `CargoBuilder.Build()` fetches crate metadata
2. `discoverExecutables()` returns executable names (e.g., `["cargo-hack"]`)
3. Build detects `cargo-` prefix and generates `cargo hack --version`
4. Recipe is serialized with correct verify command
5. If verify fails at sandbox validation time, repair loop tries `cargo <subcommand> --version` as the first fallback for `cargo-*` binaries

## Implementation Approach

### Phase 1: CargoBuilder Fix

- Modify `Build()` in `cargo.go` to detect cargo subcommands
- Update `cargo_test.go` to test the new behavior (both `cargo-*` and non-`cargo-*` executables)

### Phase 2: Repair Loop Fallback

- Add cargo subcommand fallback to `attemptVerifyRepair()` in `orchestrator.go`
- Add test in `repair_loop_test.go` for the new fallback path
- Ensure the fallback is tried before `--help` and `-h` for `cargo-*` binaries

### Phase 3: Existing Recipe Cleanup

- Update `cargo-edit.toml` to use `cargo add --version` instead of the `--help` workaround. Verify the actual output format of `cargo add --version` first — different subcommands may format version strings differently.
- Any other existing cargo subcommand recipes with incorrect verify commands

## Security Considerations

### Download Verification

Not applicable. This change modifies how verify commands are generated and executed. It doesn't affect how binaries are downloaded or their integrity checked.

### Execution Isolation

The verify command runs in the same environment as today. Changing from `cargo-hack --version` to `cargo hack --version` doesn't change the execution model. Both run user-installed binaries through the existing verify infrastructure. No new privileges are required.

### Supply Chain Risks

Not applicable. This change doesn't affect where binaries come from or how they're authenticated. The same `cargo install` command runs regardless of how verification works.

### User Data Exposure

Not applicable. The verify command runs locally and produces version output. Changing the invocation pattern doesn't expose any user data.

## Consequences

### Positive

- Cargo subcommand recipes generated by the batch system will have correct verify commands
- Version validation works properly (instead of falling back to `--help` pattern matching)
- The repair loop handles legacy recipes that were generated before this fix
- No manual recipe editing needed for cargo subcommand crates

### Negative

- Verify commands for cargo subcommands now depend on cargo being in PATH at verify time. If a user installs a cargo subcommand but doesn't have cargo installed, verification fails.

### Mitigations

- The cargo dependency is already required by `cargo_install`, so any user installing a cargo subcommand necessarily has cargo available. The verify step runs in an environment where cargo was just used for installation.

# Architecture Review: DESIGN-cargo-subcommand-verification

**Reviewer**: architect-reviewer
**Design**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-cargo-subcommand-verification.md`

## 1. Problem Statement Evaluation

The problem statement is specific and well-grounded. It identifies:

- The exact source of the bug: `cargo.go:160` generating `fmt.Sprintf("%s --version", executables[0])` for all crates
- The behavioral inconsistency: some cargo subcommands accept direct `--version`, others don't
- Why manual fixes don't work: batch generation creates recipes at scale
- A concrete existing workaround: `cargo-edit.toml` uses `--help` with pattern matching

The scope boundaries are clear. One minor gap: the problem statement doesn't mention what happens to the `Pattern` field. The current `CargoBuilder.Build()` at line 159-161 creates a `VerifySection` with only `Command` set and no `Pattern`. The design says "The verify pattern should remain `{version}`" but the current code doesn't set a pattern at all. This matters because the design proposes adding version validation through the pattern, which would be a behavioral change beyond just fixing the invocation path.

**Verdict**: Specific enough to evaluate solutions. The pattern gap should be noted but doesn't undermine the problem statement.

## 2. Missing Alternatives

The design covers the reasonable solution space. One alternative worth mentioning:

**Symlink-based approach**: Instead of changing the verify command, create a `cargo` wrapper script that dispatches to the actual cargo binary. This would make `cargo-hack --version` work by having cargo in PATH handle the subcommand dispatch. However, this adds complexity for no benefit since `cargo` is already available, and the design's approach of generating `cargo <subcommand> --version` directly is simpler.

No significant missing alternatives.

## 3. Rejection Rationale Assessment

**"Repair loop only" rejection** -- Fair. The reasoning is specific: "every recipe would still be generated with the wrong verify command, require a sandbox validation failure, then get repaired." This accurately describes unnecessary latency in the generation pipeline.

**"Manual recipe fixes" rejection** -- Fair. "Doesn't scale to batch generation" is the core argument and it's correct.

**"Try direct first, fall back to cargo invocation" rejection** -- Fair. "The common case always goes through the repair path" is a valid efficiency argument.

**"Detect at build time whether direct invocation works" rejection** -- Fair. "Since `cargo <subcommand> --version` always works, there's no need to probe."

No strawmen detected. Each alternative has a plausible advocate and a specific reason for rejection.

## 4. Unstated Assumptions

### Assumption 1: `cargo <subcommand> --version` always works
The design states this as fact: "cargo always handles `--version` dispatching correctly." This is true for well-behaved cargo subcommands but should be verified. Some cargo subcommands may not implement the version subcommand through cargo's dispatch mechanism. The repair loop fallback handles this case, so the assumption failing doesn't break the system -- it just means the repair loop kicks in. Worth noting explicitly.

### Assumption 2: `cargo` will be in PATH at verify time in the sandbox
The design acknowledges this dependency and argues it's already satisfied since `cargo_install` requires cargo. Looking at the sandbox script (`executor.go:439`), PATH is set to `/workspace/tsuku/tools/current:$PATH`. Cargo would need to be either in the container's base PATH or installed by a dependency step. The `cargo_install` action uses cargo during the *install* step, but does the installed cargo end up in PATH for the *verify* step?

Looking at `validate/executor.go:325`:
```go
sb.WriteString("export PATH=\"/workspace/tsuku/tools/current:$PATH\"\n")
sb.WriteString(fmt.Sprintf("%s\n", r.Verify.Command))
```

And the sandbox base PATH at `sandbox/executor.go:271`:
```go
"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
```

If cargo isn't in any of these paths, `cargo hack --version` would fail with "command not found." The `cargo_install` action presumably relies on cargo being available in the container. For sandbox validation, the container image would need to have cargo pre-installed or it would need to be installed as a dependency step. This assumption needs explicit validation.

### Assumption 3: `extractBinaryName` is relevant for the repair loop
The design's repair loop pseudocode checks `if strings.HasPrefix(binaryName, "cargo-")`. But `binaryName` comes from `extractBinaryName(originalCommand)`, which extracts the first word. For a new recipe generated with the fix, the verify command would be `cargo hack --version`, so `extractBinaryName` would return `cargo`, not `cargo-hack`. The repair loop cargo fallback as described would never trigger for correctly-generated recipes because the binary name wouldn't start with `cargo-`.

This is actually fine for new recipes (the builder generates the right command, so the repair loop shouldn't need to fire). But for *legacy* recipes with `cargo-hack --version`, `extractBinaryName` returns `cargo-hack`, and the repair loop correctly detects it. The design's data flow description at line 150 says the repair loop handles "existing recipes with the wrong verify command" -- this is the case where it matters, and it works correctly.

### Assumption 4: Output format consistency
The design states: "The `cargo <subcommand> --version` output follows the same format as `cargo-<subcommand> --version`." This is important for pattern matching but not verified in the design. If the output format differs (e.g., `cargo-hack 0.6.43` vs `hack 0.6.43`), the pattern `{version}` would still match but `cargo-hack {version}` would not.

However, since the current builder doesn't set a Pattern at all (just Command), this only matters if the design also changes the Pattern. The design mentions `{version}` pattern but the current code doesn't set one.

## 5. Strawman Analysis

No options appear designed to fail. Each rejected alternative has real-world precedent (the `cargo-edit.toml` manual workaround exists, the repair-loop-only approach is how other verify failures are already handled).

## 6. Architecture Clarity

### CargoBuilder change (Component 1)
Clear and implementable. The pseudocode at lines 108-117 maps directly to the existing code at `cargo.go:159-161`. The change is localized and doesn't affect the builder interface or registration pattern.

### Repair loop change (Component 2)
The pseudocode at lines 124-141 has a structural issue. The design shows:

```go
if strings.HasPrefix(binaryName, "cargo-") {
    subcommand := strings.TrimPrefix(binaryName, "cargo-")
    cargoFallback := struct {
        command string
        method  string
    }{"cargo " + subcommand + " --version", "fallback_cargo_subcommand"}
    fallbacks = append([]struct{ command, method string }{cargoFallback}, fallbacks...)
}
```

This prepends the cargo fallback to the existing `--help` and `-h` fallbacks. For a legacy recipe with command `cargo-hack --version`, `extractBinaryName` returns `cargo-hack`, and the fallback list becomes:
1. `cargo hack --version` (new)
2. `cargo-hack --help` (existing)
3. `cargo-hack -h` (existing)

This is correct behavior. The new fallback preserves version checking (mode "version" with `{version}` pattern) rather than falling back to help-text matching. However, the design's pseudocode uses `tryFallbackCommand`, which currently hardcodes `mode: "output"` and `pattern: "usage"` (see `orchestrator.go:432-437`). If the cargo fallback goes through the same `tryFallbackCommand`, it would lose version validation and use help-text mode instead.

The design should specify that the cargo subcommand fallback should use version mode, not the generic output/usage mode that existing fallbacks use. This might require either:
- A different code path for the cargo fallback that uses version mode
- Extending `tryFallbackCommand` to accept mode/pattern parameters

This is an implementability gap.

## 7. Missing Components or Interfaces

### Missing: Pattern generation in CargoBuilder
The current `Build()` at `cargo.go:159-161` creates:
```go
Verify: recipe.VerifySection{
    Command: fmt.Sprintf("%s --version", executables[0]),
},
```

No `Pattern` field is set. The design mentions using `{version}` pattern but the current code doesn't set one. If the design intends to add pattern matching (which is valuable for actual version validation), that should be explicitly listed as part of the change. If not, the statement about `{version}` pattern is misleading.

### Missing: Test strategy for cargo availability
The design mentions tests in Phase 1 and Phase 2 but doesn't address how to test the `cargo <subcommand> --version` command in unit tests where cargo might not be available. The CargoBuilder tests presumably mock HTTP but the repair loop tests need to handle the sandbox execution. Looking at `repair_loop_test.go`, these likely use mock sandbox execution, so this may be fine -- but worth calling out.

### Missing: Recipe cleanup details
Phase 3 mentions updating `cargo-edit.toml` to use `cargo add --version`. The current recipe uses `cargo-add --help` with `mode = "output"` and `pattern = "Add dependencies to a Cargo.toml"`. Changing to `cargo add --version` with version mode is a semantic change that should be validated. Does `cargo add --version` actually output a version string? The `cargo-edit` crate installs `cargo-add`, `cargo-rm`, and `cargo-upgrade` -- three separate executables. The verify command would only check one of them.

## 8. Simpler Alternatives

The proposed solution is already minimal: two localized changes in two files. There's no simpler approach that achieves the same goal.

One micro-simplification: skip the repair loop fallback entirely. If the CargoBuilder fix is correct, no new recipes will need repair. Legacy recipes with wrong verify commands will fail sandbox validation and get regenerated (which would use the fixed builder). The repair loop fallback only helps if legacy recipes go through the repair path without regeneration.

However, looking at how the batch pipeline works, recipes are regenerated by the builder -- so legacy recipes *would* get the fix on regeneration. The repair loop fallback is defense-in-depth for recipes that go through `attemptVerifySelfRepair` without being rebuilt from scratch. Whether this path exists depends on the batch pipeline's architecture, which the design doesn't describe.

The defense-in-depth argument is reasonable but the value is limited if all legacy recipes will be regenerated anyway.

## Architectural Fit

### Does this change respect existing architecture?

**Yes.** Both changes are within the builder layer:

1. **CargoBuilder change**: Modifies the builder's own output generation. No new interfaces, no bypass of existing patterns. The builder already has full control over the VerifySection it generates.

2. **Repair loop change**: Extends the existing fallback list in `attemptVerifySelfRepair`. The repair loop already has a priority-ordered fallback mechanism. Adding a cargo-specific fallback fits the existing pattern.

No new packages, no dependency direction changes, no action dispatch bypass, no state contract changes.

### One concern: verify mode semantics in repair loop

The existing `tryFallbackCommand` function (`orchestrator.go:430-437`) hardcodes `mode: "output"` and `pattern: "usage"` for all fallbacks. A cargo subcommand fallback should ideally use `mode: "version"` with `pattern: "{version}"` since `cargo <subcommand> --version` produces version output, not help text. If the implementation routes through the same `tryFallbackCommand`, the cargo fallback would incorrectly use help-text mode.

This isn't a structural violation -- it's an implementation detail that the design should specify more precisely. The fix would likely be to pass mode/pattern as parameters to `tryFallbackCommand` or to create the fallback VerifySection directly before passing it to a more generic validation function.

## Summary of Findings

### Blocking

None. The design fits the existing architecture.

### Advisory

1. **Verify pattern not set by current builder** (design lines 87, Component 1 pseudocode): The current `CargoBuilder.Build()` doesn't set `Pattern` in the VerifySection. The design references `{version}` pattern but doesn't explicitly say "add Pattern to the VerifySection." Clarify whether this is in scope.

2. **Repair loop verify mode mismatch** (design lines 124-141): The cargo subcommand fallback should use version mode, not the `output`/`usage` mode that `tryFallbackCommand` currently hardcodes. The design's pseudocode doesn't address this, which could lead to the cargo fallback losing version validation. Specify the expected mode/pattern for the cargo fallback.

3. **`cargo` availability in sandbox PATH** (Assumption 2): The design assumes cargo is in PATH at verify time. This is likely true but the mechanism isn't documented. Worth a sentence confirming how cargo ends up in the sandbox's PATH for verify command execution.

4. **`cargo add --version` output format unverified** (Phase 3): The recipe cleanup for `cargo-edit.toml` proposes `cargo add --version` but doesn't confirm the output format. Verify before implementing.

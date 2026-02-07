# Design Review: Verification Self-Repair

## Executive Summary

The design exploration addresses a real problem with solid infrastructure awareness. The chosen options are reasonable, but the analysis could benefit from deeper consideration of edge cases and clearer articulation of the deterministic/LLM boundary. No options are strawmen, though one alternative rejection rationale needs strengthening.

## Problem Statement Assessment

### Strengths

The problem statement is specific and well-scoped:
- Clear trigger: sandbox validation failing on `verify.command`
- Defined cause: default `--version` pattern doesn't work for all tools
- Observable signal: help text in failure output indicates the tool works

### Gaps

1. **Missing failure categories**: The problem conflates two distinct scenarios:
   - Tool supports `--version` but outputs unexpected format (version parsing issue)
   - Tool doesn't support `--version` at all (flag support issue)

   Only the second is addressed. The first might benefit from different repair strategies (pattern adjustment vs. command adjustment).

2. **No quantification**: "Most generated recipes default to `tool --version`" - how many? The research claims 95% support rate for `--version`, but what percentage of sandbox failures are actually verification failures vs. other issues? This affects priority.

3. **Scope boundary is implicit**: The design says "out of scope: other failure types" but doesn't explain why verification is special. The answer seems to be that verification failures are uniquely recoverable without external data (the tool's help output is already in the failure output), but this should be explicit.

## Options Analysis

### Decision 1: Where to Insert Self-Repair Logic

**Chosen: Orchestrator pre-repair phase**

This is the right choice. The codebase shows a clean separation:
- `Orchestrator.Create()` owns the generate/validate/repair cycle (lines 130-212 in orchestrator.go)
- `Session.Repair()` is builder-specific (LLM or deterministic)
- `ParseValidationError` categorizes failures but doesn't fix them

Adding a pre-repair phase maintains this separation while adding deterministic capability that works for all builders.

**Rejected alternatives:**

| Alternative | Rejection Reason | Fair? |
|-------------|------------------|-------|
| New VerificationRepairer component | "Adds abstraction without clear benefit" | Somewhat fair, but understated. The real issue is that this would require passing the repairer to the orchestrator, adding constructor complexity for a single-purpose component. |
| Extend ParseValidationError with repair | "Mixes concerns" | Fair. ParseValidationError returns suggestions but doesn't modify state. Adding repair logic would change its contract. |

### Decision 2: Verification Fallback Strategy

**Chosen: Hybrid detection**

This is sound. The approach:
1. Analyze original failure output first
2. If inconclusive, try `--help` / `-h` fallbacks

The rationale (minimize sandbox executions) is valid. Re-running the sandbox for each fallback attempt is expensive (container spin-up, download cache verification).

**Concern:** The design says "use `pattern` from tool name" when detecting help output. This is underspecified. Does it mean:
- Literal tool name in output? (`grep "toolname"`)
- Case-insensitive match?
- Word boundary matching?

The existing `goimports.toml` uses `pattern = "usage:"` and `cobra-cli.toml` uses `pattern = "Cobra is a CLI"`. Neither uses the tool name. The design should either adopt an existing pattern or explain why tool name is better.

**Rejected alternatives:**

| Alternative | Rejection Reason | Fair? |
|-------------|------------------|-------|
| Fixed fallback hierarchy only | "Wastes sandbox execution time" | Fair. If the original output already shows help text, no need to try again. |
| Pattern detection only | "Some failures are ambiguous" | Fair. If stderr is empty or contains only error messages, we need to try an explicit fallback. |

### Decision 3: Recipe Modification Approach

**Chosen: Return modified recipe with repair metadata**

Good choice. Looking at the codebase:
- `BuildResult` already has metadata fields (`RepairAttempts`, `Provider`, `SandboxSkipped`)
- Adding `verify_repair` tracks what happened without losing the original recipe

**Rejected alternatives:**

| Alternative | Rejection Reason | Fair? |
|-------------|------------------|-------|
| Mutate recipe in-place | "Loses original verification" | Weak. The orchestrator could copy the recipe before mutation. The real issue is that in-place mutation makes the repair invisible to callers - they can't distinguish "original verify worked" from "repair fixed it". |
| Return structured feedback | "Pushes complexity to caller" | Fair. The orchestrator already handles recipe generation; it should also handle verification repair. |

## Unstated Assumptions

1. **Sandbox executes verify command directly**: The design assumes the sandbox runs `verify.command` as written. Looking at `executor.go:buildSandboxScript()`, this appears true - the sandbox runs `tsuku install --plan`, which presumably executes the verify section. But the exact mechanism should be confirmed.

2. **Non-zero exit + help output = tool works**: This heuristic is reasonable but has edge cases:
   - Tool might print help on startup, then crash
   - Tool might require initialization before any command works
   - Tool might print "usage:" in an error message unrelated to help

3. **Pattern matching is sufficient for detection**: The design proposes matching "usage:" but some tools use "Usage:", "USAGE:", or localized equivalents. Case-insensitivity and multiple patterns would improve robustness.

4. **Exit code conventions are reliable**: The design mentions exit codes 1-2 for invalid flags, but this varies:
   - Python argparse exits with 2 for unknown flags
   - Go flag package exits with 2
   - Some tools exit with 1
   - A few non-standard tools exit with 0 even on errors

5. **Repair metadata is preserved**: The design says track changes in metadata but doesn't specify where this metadata lives. Is it:
   - In the recipe TOML (persisted)?
   - In the BuildResult (ephemeral)?
   - In telemetry events?

## Strawman Check

None of the alternatives appear designed to fail. Each represents a reasonable architectural choice:
- New VerificationRepairer: Valid component extraction pattern
- Extend ParseValidationError: Reasonable if you view repair as an extension of diagnosis
- Fixed fallback hierarchy: Common approach in similar systems
- Mutate in-place: Simpler implementation

The rejections are based on genuine tradeoffs (complexity, separation of concerns, performance) rather than manufactured problems.

## Missing Considerations

### 1. Interaction with LLM Repair

The design says deterministic repair runs "before falling back to LLM." But what if deterministic repair partially fixes the issue? Consider:
- Original: `tool --version` fails with help output
- Deterministic repair: Change to `mode=output`, `pattern=usage:`
- Re-validate: Still fails because tool prints to stderr, not stdout

Should the LLM receive the partial repair context, or start fresh? The design doesn't address repair handoff.

### 2. Recipe Author Intent

If a recipe explicitly specifies `verify.command = "tool --version"` with `mode = "version"`, is it appropriate to automatically change it to `mode = "output"`? This might hide version compatibility issues the author intended to catch.

Consider adding a `verify.strict = true` option to disable automatic repair.

### 3. Multiple Verification Failures

The design focuses on single-command verification. But `VerifySection` supports `additional` commands (line 725 in types.go). If the primary verification passes after repair but an additional verification fails, what happens?

### 4. Telemetry and Debugging

The design mentions "repair metadata for debugging and telemetry" but doesn't specify events. Suggested events:
- `verify_repair_attempted`: What pattern was detected, what repair was attempted
- `verify_repair_succeeded`: Final working configuration
- `verify_repair_failed`: Why deterministic repair couldn't help

### 5. Sandbox Timeout Edge Case

The design acknowledges "some tools may have unusually long startup times." But it doesn't address what happens if the original failure was a timeout, not a verification failure. A tool that times out on `--version` will likely also time out on `--help`.

## Specific Recommendations

1. **Clarify the output pattern**: Define exactly what patterns indicate "tool works but doesn't support this flag":
   ```go
   var helpIndicators = []string{
       "usage:",
       "Usage:",
       "USAGE:",
       "options:",
       "Options:",
       "Available commands:",
   }
   ```

2. **Add error subcategory**: Extend `ParseValidationError` with a verification-specific subcategory:
   ```go
   type VerifyErrorSubcategory string
   const (
       VerifyVersionParseFailed VerifyErrorSubcategory = "version_parse"
       VerifyFlagNotSupported   VerifyErrorSubcategory = "flag_unsupported"
       VerifyCommandNotFound    VerifyErrorSubcategory = "command_not_found"
       VerifyTimeout            VerifyErrorSubcategory = "timeout"
   )
   ```

3. **Document repair handoff**: Specify that if deterministic repair succeeds, no LLM repair is needed. If deterministic repair fails but modified the recipe, pass both original and modified to LLM with context.

4. **Preserve author intent**: Only apply automatic repair to recipes generated by builders, not manually-authored recipes. Detect via presence of `llm_validation` metadata field or a new `auto_generated = true` field.

5. **Define metadata location**: Store repair metadata in `BuildResult.RepairMetadata map[string]string`:
   ```go
   result.RepairMetadata = map[string]string{
       "verify_repair":        "fallback_help",
       "original_command":     "tool --version",
       "original_mode":        "version",
       "repaired_command":     "tool --help",
       "repaired_mode":        "output",
       "detection_pattern":    "usage:",
   }
   ```

## Verdict

The design is **ready to proceed with minor clarifications**:
- Strengthen the in-place mutation rejection with the actual concern (visibility of repairs)
- Define the exact patterns for help output detection
- Specify repair metadata storage location
- Consider adding a `verify.strict` escape hatch

The chosen options are well-reasoned and align with the existing codebase architecture. The hybrid detection approach correctly prioritizes avoiding unnecessary sandbox executions while maintaining a fallback path.

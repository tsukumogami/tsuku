# Exploration Summary: verification-self-repair

## Problem (Phase 1)

When recipe creation runs sandbox validation, the verification step (`verify.command`) often fails because most generated recipes default to `tool --version`. Not all CLI tools support `--version`, but most print help text when given invalid arguments—and this help text often contains patterns that enable deterministic repair without LLM fallback.

## Decision Drivers (Phase 1)

- Reduce LLM costs during batch recipe creation
- Decrease validation failures in CI pipelines
- Maintain compatibility with existing recipe format
- Handle diverse CLI conventions (--version, -V, version, --help)
- Preserve explicit verification for tools with unusual patterns
- Must work in sandbox environment with no network access

## Research Findings (Phase 2)

**Existing Infrastructure:**
- `internal/validate/errors.go` already categorizes sandbox failures (binary_not_found, extraction_failed, verify_failed, etc.)
- `ErrorVerifyFailed` exists but targets checksum/signature failures, not CLI behavior
- Repair workflow: `Orchestrator.Create()` → sandbox validation → `Session.Repair()` with failure details
- Deterministic builders return `RepairNotSupportedError`; LLM builders pass failure to LLM

**CLI Conventions (from research):**
- `--version` supported by ~95% of tools, `--help` by ~99%
- Invalid flags exit with 1-2 (not 0), often print "usage:" plus help text
- Exit code 127 = command not found (shell convention)
- Help text reliably contains: "usage:", tool name, structured sections

**Verification Fallback Hierarchy:**
1. `tool --version` (exit 0, version in output)
2. `tool -V` (Rust/Go convention)
3. `tool --help` (exit 0, "usage:" in output)
4. `tool -h` (short form)
5. Pattern match on stderr from original failure

**Key Insight:**
When `--version` fails with non-zero exit but stderr contains "usage:" or help-like output, the tool IS working—it just doesn't support that flag. We can detect this pattern and automatically adjust the verify section.

## Options (Phase 3)

**Decision 1: Where to insert self-repair logic**
- A) Orchestrator layer (pre-repair phase before Session.Repair)
- B) New VerificationRepairer component
- C) Extend ParseValidationError with repair logic

**Decision 2: Verification fallback strategy**
- A) Fixed fallback hierarchy (--version → -V → --help → -h)
- B) Pattern detection from failure output first
- C) Hybrid: analyze output first, then try fallbacks

**Decision 3: Recipe modification approach**
- A) Mutate recipe in-place
- B) Return modified recipe with repair metadata
- C) Return structured feedback for caller to apply

**Leaning toward:** 1A + 2C + 3B - Pre-repair in orchestrator, hybrid detection, tracked modifications

## Decision (Phase 5)

**Problem:**
Recipe creation often fails at the verification step because generated recipes default to `tool --version`, which many CLI tools don't support. When this happens, the system falls back to LLM repair—but most of these failures are predictable: the tool IS working, it just doesn't recognize the flag. This increases costs, slows batch recipe creation, and adds unnecessary complexity to what should be a deterministic operation.

**Decision:**
Add a verification self-repair step to the orchestrator that intercepts verification failures before calling LLM repair. When sandbox validation fails, analyze the output for help-text patterns (exit code 1-2 with "usage:" or similar). If detected, automatically adjust the verify section to `mode=output` with appropriate pattern and exit code, then re-run validation. If self-repair fails or output is ambiguous, proceed to existing LLM repair. Track repairs in `BuildResult.RepairMetadata` for telemetry.

**Rationale:**
This approach minimizes changes while maximizing impact. The orchestrator already owns the validation loop, so adding a pre-repair check is straightforward. The hybrid detection strategy (analyze output first, try fallbacks if needed) avoids wasted sandbox executions while handling ambiguous cases. Tracking repairs separately from the recipe preserves the original verification intent while enabling debugging and cost measurement.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-06

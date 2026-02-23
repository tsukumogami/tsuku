# Scrutiny Review: Justification -- Issue #1938

## Overview

Issue #1938 adds the `BinaryNameProvider` interface and the orchestrator's `validateBinaryNames()` step. All 14 ACs are mapped as "implemented" with no deviations reported. Since the justification focus evaluates deviation explanations, and there are no deviations, the primary check is whether any AC claims disguise shortcuts or avoid work that should have been done.

## Requirements Mapping (Untrusted)

--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
All 14 ACs reported as "implemented" with no deviations, no reasons, no alternatives considered.
--- END UNTRUSTED REQUIREMENTS MAPPING ---

## Evaluation

### No Deviations to Justify

All ACs are marked "implemented." No deviation reasons or alternative_considered fields are present because none were needed. This is the correct format when everything is done.

### Avoidance Pattern Check

Checked for signs that "implemented" claims mask incomplete work:

1. **BinaryNameProvider interface defined** -- Confirmed at `binary_names.go:19-25`. Single method `AuthoritativeBinaryNames() []string`. Clean.

2. **CargoBuilder implements provider** -- Confirmed at `cargo.go:306-327`. Uses cached API response. Compile-time assertion in test (`binary_names_test.go:272`).

3. **NpmBuilder implements provider** -- Confirmed at `npm.go:359-376`. Uses cached package info. Compile-time assertion in test (`binary_names_test.go:276`). Previous issue's advisory about missing cache was addressed: `cachedPackageInfo` field added at `npm.go:61`, populated at `npm.go:135`.

4. **validateBinaryNames on Orchestrator** -- Confirmed at `binary_names.go:49-130`. Method on `*Orchestrator`, compares sets, corrects in-place, emits warning and telemetry.

5. **called between Generate and sandbox** -- Confirmed at `orchestrator.go:168-173`. After `session.Generate()`, before `SkipSandbox` check and validation loop.

6. **type-assert before NewSession** -- Confirmed at `orchestrator.go:151-153`. `binaryNameProvider, _ := builder.(BinaryNameProvider)` occurs before `builder.NewSession()`.

7. **warning on correction** -- Confirmed at `binary_names.go:109-113`. Warning appended to `result.Warnings`.

8. **telemetry event on correction** -- Confirmed at `binary_names.go:116-123`. `NewBinaryNameRepairEvent` + `SendBinaryNameRepair`.

9. **isValidExecutableName validation** -- Confirmed at `binary_names.go:60-68`. Provider names filtered through `isValidExecutableName` before comparison.

10. **empty slice skips validation** -- Confirmed at `binary_names.go:55-57`. Early return on `len(authoritative) == 0`.

11. **unit tests for validateBinaryNames** -- Confirmed. 8 test functions covering match, mismatch, empty provider, nil provider, no executables param, order-independent comparison, invalid names, and `[]interface{}` extraction.

12. **unit tests for provider impls** -- Confirmed. 4 Cargo tests + 6 npm tests covering after-build, empty bin_names, yanked versions, invalid names, map bin, string bin, scoped string bin, no bin, no latest version.

13. **interface definition for #1939** -- The `BinaryNameProvider` interface at `binary_names.go:19` is available for PyPI to implement.

14. **working orchestrator step for #1939** -- The `validateBinaryNames()` method accepts any `BinaryNameProvider`, so #1939 only needs to implement the interface on the PyPI builder.

### Proportionality Check

14 ACs, all implemented. The implementation spans 8 files: 2 new files (binary_names.go, binary_names_test.go), modifications to cargo.go (added AuthoritativeBinaryNames), npm.go (added cachedPackageInfo + AuthoritativeBinaryNames), orchestrator.go (added type-assert + validation call), telemetry event.go + client.go (added BinaryNameRepairEvent), and telemetry event_test.go. This is proportionate to the scope.

### Selective Effort Check

No peripheral ACs are "implemented" while core ones are deviated. All core work (interface, implementations, orchestrator integration, telemetry) is present.

## Findings

No blocking or advisory findings. All claims verified against the code. No deviations to evaluate.

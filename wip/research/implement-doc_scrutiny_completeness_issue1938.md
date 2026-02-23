# Scrutiny Review: Completeness -- Issue #1938

## Issue
**#1938: feat(builders): add BinaryNameProvider and orchestrator validation**

## Scrutiny Focus
`completeness`

## AC Coverage

All 14 mapping entries were verified against the implementation. No missing ACs, no phantom ACs.

### 1. "BinaryNameProvider interface defined" -- claimed: implemented
**Verified.** `internal/builders/binary_names.go:19-25` defines `BinaryNameProvider` with `AuthoritativeBinaryNames() []string`. The doc comment explains when the return value is empty vs nil and the `isValidExecutableName` requirement.

### 2. "CargoBuilder implements provider" -- claimed: implemented
**Verified.** `internal/builders/cargo.go:306-327` implements `AuthoritativeBinaryNames()` on `*CargoBuilder`. It reads from `cachedCrateInfo` (populated during `Build()`), iterates non-yanked versions, and filters through `isValidExecutableName()`. Compile-time check at `binary_names_test.go:272`.

### 3. "NpmBuilder implements provider" -- claimed: implemented
**Verified.** `internal/builders/npm.go:359-376` implements `AuthoritativeBinaryNames()` on `*NpmBuilder`. It reads from `cachedPackageInfo`, resolves the latest version's bin field via `parseBinField()`. Compile-time check at `binary_names_test.go:276`.

### 4. "validateBinaryNames on Orchestrator" -- claimed: implemented
**Verified.** `internal/builders/binary_names.go:49-130` defines `func (o *Orchestrator) validateBinaryNames(provider BinaryNameProvider, result *BuildResult, builderName string) *BinaryNameRepairMetadata`. The method cross-checks recipe executables against the provider's authoritative names and corrects mismatches in-place.

### 5. "called between Generate and sandbox" -- claimed: implemented
**Verified.** `internal/builders/orchestrator.go:163-173`. After `session.Generate(ctx)` at line 163 and before the sandbox validation loop starting at line 186, the orchestrator calls `o.validateBinaryNames(binaryNameProvider, result, builder.Name())` at line 172.

### 6. "type-assert before NewSession" -- claimed: implemented
**Verified.** `internal/builders/orchestrator.go:153`. The line `binaryNameProvider, _ := builder.(BinaryNameProvider)` occurs before `builder.NewSession(ctx, req, opts)` at line 156. The comment explains the reason: "the builder reference is not retained after NewSession()."

### 7. "warning on correction" -- claimed: implemented
**Verified.** `internal/builders/binary_names.go:109-113`. When a mismatch is detected, a warning string is formatted and appended to `result.Warnings`. Test at `binary_names_test.go:366-368` asserts warning presence after correction.

### 8. "telemetry event on correction" -- claimed: implemented
**Verified.** `internal/builders/binary_names.go:116-123`. After correcting the recipe, if `o.telemetryClient != nil`, the code calls `telemetry.NewBinaryNameRepairEvent(...)` and `o.telemetryClient.SendBinaryNameRepair(event)`. The event struct is at `internal/telemetry/event.go:315-324`, the constructor at line 340, and the send method at `internal/telemetry/client.go:158`.

### 9. "isValidExecutableName validation" -- claimed: implemented
**Verified.** `internal/builders/binary_names.go:60-68`. Each name from the provider is filtered through `isValidExecutableName()`. If all names are invalid, the method returns nil (no correction). Test at `binary_names_test.go:485-511` exercises the all-invalid-names case.

### 10. "empty slice skips validation" -- claimed: implemented
**Verified.** `internal/builders/binary_names.go:55-57`. If `len(authoritative) == 0`, the method returns nil immediately. Tests at `binary_names_test.go:371-402` (empty slice) and `binary_names_test.go:404-429` (nil slice) both confirm skip behavior.

### 11. "unit tests for validateBinaryNames" -- claimed: implemented
**Verified.** `internal/builders/binary_names_test.go` contains 8 test functions for `validateBinaryNames`:
- `TestValidateBinaryNames_Match_NoChange` (line 281)
- `TestValidateBinaryNames_Mismatch_Correction` (line 318)
- `TestValidateBinaryNames_EmptyProvider_Skip` (line 371)
- `TestValidateBinaryNames_NilProvider_Skip` (line 404)
- `TestValidateBinaryNames_NoExecutablesParam_Skip` (line 431)
- `TestValidateBinaryNames_SameNamesOrderDiffers_NoChange` (line 457)
- `TestValidateBinaryNames_InvalidProviderNames_Skip` (line 485)
- `TestValidateBinaryNames_InterfaceExtractExecutables` (line 513)

Plus helper tests for `extractExecutablesFromStep` (3 tests) and `executableSetsEqual` (1 table-driven test with 6 cases).

### 12. "unit tests for provider impls" -- claimed: implemented
**Verified.** `internal/builders/binary_names_test.go` contains:
- Cargo: 4 tests (AfterBuild, EmptyBinNames, SkipsYanked, FiltersInvalid) -- lines 15-134
- npm: 5 tests (MapBin, StringBin, ScopedStringBin, NoBin, NoLatestVersion) -- lines 138-267
- Compile-time interface checks for both (lines 271-277)

### 13. "interface definition for #1939" -- claimed: implemented
**Verified.** The `BinaryNameProvider` interface at `internal/builders/binary_names.go:19-25` is a standalone interface that #1939 (PyPI) can implement without modifying any existing code. The interface has a single method `AuthoritativeBinaryNames() []string`, which is the contract #1939 needs.

### 14. "working orchestrator step for #1939" -- claimed: implemented
**Verified.** The orchestrator's `Create()` method at `orchestrator.go:153` does `binaryNameProvider, _ := builder.(BinaryNameProvider)`, which means any builder implementing the interface gets validation automatically. The `validateBinaryNames` method is fully generic -- it receives a `BinaryNameProvider`, not a concrete builder type. #1939 only needs to implement the interface on `PyPIBuilder`.

## Integration Tests

Two orchestrator-level integration tests verify the full flow:
- `TestOrchestratorCreate_TypeAssertsToProvider` (line 611): Mock builder with wrong names, verifies correction happens
- `TestOrchestratorCreate_NonProviderBuilder_SkipsValidation` (line 656): Mock builder without the interface, verifies no crash or correction

## Findings

**No blocking findings.**

**No advisory findings.**

All 14 AC claims are confirmed against the diff. Evidence locations are accurate. The implementation structure cleanly separates the interface definition, the validation logic, the builder implementations, and the telemetry plumbing. The orchestrator integration tests exercise both the happy path (provider builder gets corrected) and the skip path (non-provider builder is left alone).

The telemetry event struct, constructor, and send method are all present and tested. The `isValidExecutableName` filter is applied at both the provider level (in `CargoBuilder.AuthoritativeBinaryNames()`) and the orchestrator level (in `validateBinaryNames()`), providing defense in depth.

The downstream enablement for #1939 is sound: the interface and orchestrator step are fully generic, so PyPI only needs to implement `AuthoritativeBinaryNames()` on its builder.

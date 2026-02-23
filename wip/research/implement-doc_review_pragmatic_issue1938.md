# Pragmatic Review: Issue #1938

## Findings

### 1. BinaryNameRepairMetadata is returned but discarded -- dead struct (Blocking)

`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/binary_names.go:28-38` defines `BinaryNameRepairMetadata` with three fields. The `validateBinaryNames()` method constructs and returns it (line 125-129). But the only caller in `orchestrator.go:172` discards the return value:

```go
o.validateBinaryNames(binaryNameProvider, result, builder.Name())
```

Compare with `VerifyRepairMetadata` which is stored on `BuildResult.VerifyRepair` (orchestrator.go:232). `BinaryNameRepairMetadata` has no corresponding field and no consumer. The struct, its construction, and the return value are dead code.

**Fix:** Either add a `BinaryNameRepair *BinaryNameRepairMetadata` field to `BuildResult` and store the return value (if downstream consumers need it), or delete the struct entirely and change `validateBinaryNames` to return nothing. The repair information is already captured in the warning string and telemetry event, so the struct is likely unnecessary.

### 2. CargoBuilder.AuthoritativeBinaryNames duplicates discoverExecutables logic (Advisory)

`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:306-327` -- `AuthoritativeBinaryNames()` re-implements the "find first non-yanked version, filter bin_names" logic that already exists in `discoverExecutables()` (lines 220-259). The only difference is that `discoverExecutables` falls back to the crate name when no valid bins are found, whereas `AuthoritativeBinaryNames` returns nil.

Both iterate `cachedCrateInfo.Versions`, skip yanked, filter through `isValidExecutableName`. This is copy-paste that will diverge. Consider extracting a shared `validBinNamesFromCache()` method.

### 3. Telemetry Send method duplication (Advisory)

`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/telemetry/client.go:154-171` -- `SendBinaryNameRepair` is identical to `SendDiscovery`, `SendLLM`, `SendVerifySelfRepair`, and `Send` except for the event type parameter. All five methods have the same body: check disabled, check debug, call `go c.sendJSON(event)`. Since `sendJSON` accepts `interface{}`, a single `Send(event interface{})` would suffice. This is a pre-existing pattern, not introduced by this PR, so not blocking.

### 4. Success field always true on BinaryNameRepairEvent (Advisory)

`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/binary_names.go:117-122` -- The telemetry event is only emitted on successful correction. The `success` parameter in `NewBinaryNameRepairEvent` is always `true` at the sole call site. The parameter exists for hypothetical failure tracking that doesn't exist. Minor -- the event is small.

## Summary

| Level | Count |
|-------|-------|
| Blocking | 1 |
| Advisory | 3 |

The core feature (BinaryNameProvider interface, validateBinaryNames orchestrator integration, Cargo/npm implementations) is well-scoped and correctly implemented. The one blocking issue is the dead `BinaryNameRepairMetadata` struct -- it's constructed, returned, and immediately discarded. Either wire it up or remove it.

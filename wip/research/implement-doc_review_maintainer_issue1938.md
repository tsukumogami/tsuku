# Maintainer Review: #1938 -- BinaryNameProvider and Orchestrator Validation

**Reviewer focus**: Can the next developer understand and modify this code confidently?

**Commit**: 0fd9d7f7b3b41c5ce18920f892f8546c0e6f8f0b

## Summary

1 blocking, 3 advisory findings. The design is clean overall: the `BinaryNameProvider` interface is well-documented, the orchestrator integration point is clear, and test coverage is thorough. The blocking issue is a name-behavior mismatch on the core function.

---

## Finding 1: `validateBinaryNames` mutates state despite "validate" name

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/binary_names.go:49`
**Severity**: Blocking

The function `validateBinaryNames` does three things that its name doesn't promise:

1. Overwrites `result.Recipe.Steps[stepIdx].Params["executables"]` with the provider's names (line 98)
2. Rewrites `result.Recipe.Verify.Command` via string replacement (lines 101-107)
3. Appends to `result.Warnings` (line 113)
4. Sends a telemetry event (lines 116-123)

The doc comment says "the recipe is corrected in-place" (line 42), so the behavior is documented. But the name says "validate." The next developer looking at the call site in `orchestrator.go:172`:

```go
o.validateBinaryNames(binaryNameProvider, result, builder.Name())
```

...will think this is a check, not a mutation. They might reorder it after sandbox validation ("validation should happen later") and break the intended flow where corrections happen _before_ sandbox runs.

Compare with `attemptVerifySelfRepair` on the same file (line 349) -- that name accurately signals it will try to change the recipe. `validateBinaryNames` should follow the same pattern. Suggested rename: `correctBinaryNames` or `applyBinaryNameCorrection`.

The return value (a `*BinaryNameRepairMetadata`) is also unused at the call site (line 172), which reinforces the "this is just a check" misread.

---

## Finding 2: Divergent binary name resolution logic between `discoverExecutables` and `AuthoritativeBinaryNames`

**Files**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:220` (`discoverExecutables`)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:306` (`AuthoritativeBinaryNames`)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/npm.go:234` (`discoverExecutables`)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/npm.go:359` (`AuthoritativeBinaryNames`)

**Severity**: Advisory

Both builders have comments saying "Use the same logic as discoverExecutables" in their `AuthoritativeBinaryNames()` implementations, but the implementations have meaningful differences:

**Cargo**:
- `discoverExecutables` (line 236-239): When all versions are yanked, falls back to crate name. When bin_names is empty but a non-yanked version exists, falls back to crate name.
- `AuthoritativeBinaryNames` (line 313-323): When all versions are yanked, returns nil. When bin_names is empty, returns nil (empty slice from `var names []string` with no appends).

The divergence is intentional here -- the provider _should_ return nil/empty when it has no authoritative data, letting the recipe's original names stand. But the comment "Use the same logic" is misleading. The next developer who sees a bug in `discoverExecutables` will change `AuthoritativeBinaryNames` to match, breaking the intended skip-on-empty behavior.

**Npm**: Same pattern -- `discoverExecutables` falls back to package name, `AuthoritativeBinaryNames` returns nil.

The comments should say something like "Mirrors discoverExecutables but without fallbacks -- returns nil when registry data is absent, so the orchestrator skips correction."

---

## Finding 3: Implicit temporal contract -- `AuthoritativeBinaryNames` requires `Build()` to have been called first

**Files**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:306`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/npm.go:359`

**Severity**: Advisory

Both `AuthoritativeBinaryNames()` depend on `cachedCrateInfo`/`cachedPackageInfo` being populated by `Build()`. The doc comments say "Returns nil if Build() hasn't been called yet" and the interface doc says "An empty slice means the provider has no data (skip validation)."

The current call chain enforces this: `Create()` type-asserts to `BinaryNameProvider` on line 153, then `session.Generate()` on line 163 calls `Build()` which populates the cache, then `validateBinaryNames` on line 172 reads it.

The comment on line 151-152 explains why the type-assertion happens early:

```go
// Type-assert to BinaryNameProvider before creating the session,
// because the builder reference is not retained after NewSession().
```

This is well-commented and the ordering is correct. The risk is low because the interface returns nil gracefully when uncached, and the orchestrator tests (`TestOrchestratorCreate_TypeAssertsToProvider`) validate the full flow. But a future refactor that moves the `BinaryNameProvider` assertion to after `NewSession()` (e.g., trying to get it from the session instead) would silently degrade -- the session doesn't implement the interface, so the assertion would fail and validation would be skipped without error.

This is acceptable given the test coverage and comments.

---

## Finding 4: Telemetry Send methods are copy-paste divergent twins

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/telemetry/client.go:158`

**Severity**: Advisory

`SendBinaryNameRepair` (line 158-171) is identical in structure to `SendVerifySelfRepair` (line 177-190), `SendDiscovery` (line 139-152), and `SendLLM` (line 121-134). All four methods:
1. Check `c.disabled`
2. Check `c.debug` and marshal to stderr
3. Fire-and-forget via `go c.sendJSON(event)`

The only difference is the parameter type. This is an existing pattern in the file (not introduced by this PR), but the new `SendBinaryNameRepair` adds another copy. The next developer adding `SendFoo` will copy one of these and possibly miss a future change (e.g., adding structured logging).

A generic `sendTyped[T any](c *Client, event T)` would collapse these, but that's a refactor beyond this PR's scope. Noting it as advisory because the existing pattern is well-established and consistent.

---

## What's Clear

- The `BinaryNameProvider` interface is well-documented: its purpose, when it's a no-op, and which builders implement it.
- Test coverage is thorough: match/mismatch/empty/nil/invalid/interface-slice/order-independent comparisons are all tested.
- The orchestrator integration tests (`TestOrchestratorCreate_TypeAssertsToProvider` and `TestOrchestratorCreate_NonProviderBuilder_SkipsValidation`) validate both the happy path and the opt-out path.
- `BinaryNameRepairMetadata` cleanly separates the correction details from the telemetry event.
- The `extractExecutablesFromStep` function correctly handles both `[]string` and `[]interface{}` TOML deserialization variants, with good doc explaining why.

---

## Memory Note: executables[0] invariant

Per my earlier tracking: this PR does NOT add a structural guard against the `executables[0]` access in `cargo.go:152` / `npm.go:171`. The `validateBinaryNames` function assumes the recipe already has executables (it's a no-op if no executables param exists), but it doesn't prevent the upstream `Build()` from panicking on an empty slice. The fallback paths in `discoverExecutables` ensure this doesn't happen in practice, but the invariant remains implicit. Keeping this at advisory per previous reviews since the fallback paths are reliable.

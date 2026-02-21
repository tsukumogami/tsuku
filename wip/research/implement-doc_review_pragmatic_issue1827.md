# Pragmatic Review: Issue #1827
**Issue**: feat(cli): check satisfies index before generating recipes in tsuku create
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Date**: 2026-02-21

---

## Files Changed

- `cmd/tsuku/create.go` -- `checkExistingRecipe()` helper + early guard in `runCreate`
- `cmd/tsuku/create_test.go` -- 6 new unit tests

---

## Findings

### [BLOCKING] Tautological `--force` test

**File**: `cmd/tsuku/create_test.go:771-790`

`TestCheckExistingRecipe_ForceSkipsCheck` purports to verify that `--force` bypasses the satisfies check, but the test cannot fail regardless of what the production code does.

The test sets `createForce = true`, then wraps `checkExistingRecipe()` in `if !createForce { ... }`. Since `createForce` is `true`, that branch is unreachable -- `checkReached` is always `false`. The final `if checkReached { t.Fatal(...) }` can never fire.

If the production guard at `create.go:485` were deleted entirely (always running the check even with `--force`), this test would still pass. It tests `if !true == false`, not the production code path.

**Fix**: Test `runCreate` or change the approach to call `checkExistingRecipe` conditionally and verify the return value is unused when `createForce` is true. The simplest fix is to set `createForce = true`, call `runCreate` with a cobra command stub, and verify it does not exit with `ExitGeneral`. Alternatively, since `checkExistingRecipe` itself doesn't read `createForce`, the correct test for the force-skip path must exercise `runCreate` where the guard lives. The duplicate `openssl` precondition test (lines 764-766) is already covered by `TestCheckExistingRecipe_DirectNameMatchEmbedded`, so the `ForceSkipsCheck` test adds nothing unless it exercises the guard.

---

### [ADVISORY] Single-caller abstraction for `checkExistingRecipe`

**File**: `cmd/tsuku/create.go:467-476`

`checkExistingRecipe` is a 5-line function (nil check + one call to `GetWithContext` + return) called from exactly one place in `runCreate`. The function name doesn't add clarity beyond the comment at the call site (lines 481-484). The nil guard is defensive for test injection -- passing `nil` as a loader would only happen in tests.

This is a minor point; the function does enable isolated unit testing of the lookup path (which the 5 other tests use legitimately). Not blocking.

---

### [ADVISORY] Stale `--force` flag description

**File**: `cmd/tsuku/create.go:134`

```go
createCmd.Flags().BoolVar(&createForce, "force", false, "Overwrite existing local recipe")
```

The description says "Overwrite existing local recipe" but the flag now also bypasses the satisfies duplicate check -- which covers embedded and registry recipes, not just local ones. A user running `tsuku create openssl@3 --force` would not know from `--help` why the flag is needed. Update to something like `"Skip duplicate recipe check and overwrite existing local recipe"`.

---

## Summary

**Blocking**: 1 -- `TestCheckExistingRecipe_ForceSkipsCheck` (create_test.go:771-790) is a tautological test that cannot fail. It replicates the production guard condition inline, which proves nothing about the production code's behavior.

**Advisory**: 2 -- The `checkExistingRecipe` abstraction is minimal and testable; borderline for inlining but not worth blocking. The `--force` flag description is now incomplete after this change; update it to mention the satisfies check bypass.

The production implementation itself is correct and well-structured. The early placement, tier coverage via `GetWithContext`, differentiated messages for direct vs. satisfies matches, and the `if !createForce` guard all work as designed. The only defect is the test that cannot fail.

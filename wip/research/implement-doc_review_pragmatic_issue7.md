---
focus: pragmatic
issue: 7
blocking_count: 2
advisory_count: 3
---

## Blocking

**1. Dead function `isDistributedName`**
`cmd/tsuku/install_distributed.go:73` -- `isDistributedName()` is never called outside its own test. `parseDistributedName()` already handles the detection (returns nil for non-distributed names). Delete `isDistributedName` and its test.

**2. `distributedTelemetryTag()` is a function returning a constant**
`cmd/tsuku/install_distributed.go:245` -- Single-caller function that returns the string literal `"distributed"`. Inline the string at the call site in `install.go:233`. The function adds indirection without naming clarity (the constant speaks for itself). Delete function and its test.

## Advisory

**3. `computeRecipeHash` wraps two lines of stdlib**
`cmd/tsuku/install_distributed.go:232-235` -- `sha256.Sum256` + `fmt.Sprintf("%x")` is two lines. The function is called once. Borderline -- the name documents intent, so not blocking. Could inline.

**4. `fetchRecipeBytes` is a one-line delegation**
`cmd/tsuku/install_distributed.go:239-241` -- Delegates to `loader.GetFromSource(globalCtx, recipeName, source)`. Called once. The wrapper adds no clarity over the call itself. Could inline.

**5. Redundant `hasDistributedProvider` check in `addDistributedProvider`**
`cmd/tsuku/install_distributed.go:150-152` -- `addDistributedProvider` checks `hasDistributedProvider` at line 150, but `ensureDistributedSource` already checks at line 89 before calling `addDistributedProvider`. The inner check is harmless (idempotent guard) but redundant for the only caller path.

## No issues found

- `parseDistributedName` parsing logic is clean and well-tested.
- `ensureDistributedSource` / `checkSourceCollision` / `recordDistributedSource` are appropriately scoped.
- `CacheRecipe` and `AddProvider` on the loader are minimal additions that serve the install flow directly.
- `RecipeHash` field on `ToolState` is a single field with a clear purpose.
- The install.go integration block (lines 188-246) is inline in the command handler where it belongs -- no premature abstraction.

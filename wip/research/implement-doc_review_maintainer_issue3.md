# Maintainer Review: Issue 3 (feat(config): add registry configuration and GetFromSource)

## Overall

The code is well-structured. The userconfig additions are clean, and `GetFromSource` has good test coverage for its happy paths. Two findings -- one blocking, one advisory.

## Findings

### 1. SourceCentral is untyped while its siblings are RecipeSource -- the switch asymmetry is a trap

**File:** `internal/recipe/loader.go:519-524`

```go
SourceLocal    RecipeSource = "local"
SourceEmbedded RecipeSource = "embedded"
SourceRegistry RecipeSource = "registry"
SourceCentral  = "central"  // untyped string
```

This creates a visible asymmetry in `GetFromSource` at line 146:

```go
case SourceCentral:          // works -- untyped string matches string
case string(SourceLocal):    // needs cast -- RecipeSource != string
```

The next developer adding a new source branch will copy one pattern or the other. If they add `case SourceRegistry:` for some reason, it won't compile against the `string` switch variable. They'll waste time figuring out why `SourceCentral` works without a cast. The reason -- that `SourceCentral` is intentionally untyped because it's a user-facing alias mapping to multiple provider sources -- is invisible.

**Advisory.** The compiler catches the dangerous direction. A one-line comment on the `SourceCentral` declaration explaining why it lacks the `RecipeSource` type would prevent the head-scratch.

### 2. Central branch error propagation is untested

**File:** `internal/recipe/loader.go:128-130`

Lines 128-130 propagate non-not-found errors from the registry provider:

```go
if err != nil && !isNotFoundError(err) && !os.IsNotExist(err) {
    return nil, fmt.Errorf("central registry error: %w", err)
}
```

This was added to fix the scrutiny finding about silent error swallowing. The fix is correct -- but there's no test exercising it. The `mockProvider` returns "not found" errors for missing recipes, so every existing central test either succeeds or hits the not-found path. No test returns, say, a network or permission error from a registry mock.

This matters because the error propagation is the behavioral difference between "source-directed fetch" and "normal resolution." Issue 8 depends on this: `outdated` must distinguish "registry unavailable" from "recipe not in registry." If someone later simplifies the `if` condition (e.g., drops the `!isNotFoundError` guard), no test breaks, and `outdated` silently degrades.

**Blocking.** Add a test where the registry mock returns a non-not-found error (e.g., `errors.New("connection refused")`) and verify `GetFromSource` propagates it rather than falling through to embedded. This is a two-minute test that protects the contract Issue 8 consumes.

## What reads well

- `GetFromSource` returning raw `[]byte` instead of `*Recipe` cleanly separates "which provider" from "parse and cache." The next developer won't accidentally assume the result is cached.
- The userconfig round-trip tests, backward compat test, and spurious-TOML test cover the exact edge cases that would bite Issue 4.
- `recipeSourceFromProvider` in `cmd/tsuku/helpers.go` has a clear doc comment explaining why embedded maps to central. Good.

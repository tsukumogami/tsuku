# Pragmatic Review: Issue 10 - extend update-registry for distributed sources

## Files Changed

- `internal/recipe/loader.go` (added `Providers()` method)
- `cmd/tsuku/update_registry.go` (added `refreshDistributedSources()` and call site)
- `cmd/tsuku/update_registry_test.go` (new file, 5 tests)

## Findings

### 1. [Advisory] `Providers()` exposes internal slice without copy

**File:** `internal/recipe/loader.go:427-429`

```go
func (l *Loader) Providers() []RecipeProvider {
    return l.providers
}
```

Returns the backing slice directly. A caller doing `append(loader.Providers(), ...)` could corrupt the Loader's state. The only current caller (`refreshDistributedSources`) iterates read-only, so this is not a live bug. But `AddProvider()` exists on the same struct, and a future caller mixing both could hit subtle issues.

**Suggestion:** Return a copy, or leave as-is since the only caller is controlled. Advisory because the current usage is safe.

### 2. [Advisory] Single-caller accessor `Providers()`

**File:** `internal/recipe/loader.go:427-429`

`Providers()` is called from exactly one place: `refreshDistributedSources()`. An alternative would be to add a `RefreshAll(ctx)` method on the Loader itself, keeping provider iteration internal. However, the current approach is consistent with the existing `ProviderBySource()` escape hatch pattern, and the method is small and named well. Not blocking.

### 3. No blocking findings

The implementation correctly:
- Skips central registry (already refreshed above) via `p.Source() == recipe.SourceRegistry`
- Type-asserts to `RefreshableProvider` and skips non-refreshable providers
- Reports errors per-source without blocking other sources
- Silently returns when no distributed providers exist (no spurious output)
- Tests cover: no distributed providers, multiple providers, central registry skipping, error isolation, non-refreshable provider skipping

All acceptance criteria are met. Error handling is correct. No edge cases are missed.

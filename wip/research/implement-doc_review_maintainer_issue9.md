# Maintainer Review: Issue 9 - Source Display in info, list, and recipes

## Finding 1: Divergent source mapping in info.go vs recipeSourceFromProvider

**File:** `cmd/tsuku/info.go:118-126`
**Severity:** Blocking

The `info` command manually maps `RecipeSource` values to user-facing strings:

```go
switch providerSource {
case recipe.SourceLocal:
    recipeSource = "local"
case recipe.SourceRegistry, recipe.SourceEmbedded:
    recipeSource = "central"
default:
    recipeSource = string(providerSource)
}
```

This is a copy of the logic in `recipeSourceFromProvider()` at `cmd/tsuku/helpers.go:159-171`, which does the exact same mapping. Today they produce identical results. But the next developer who needs to change the source mapping (e.g., adding a new provider type, or changing how distributed sources display) will find and update `recipeSourceFromProvider` -- the canonical, well-named function -- and miss this inline copy in `info.go`. The two will silently diverge.

**Suggestion:** Replace the switch block with a call to `recipeSourceFromProvider(providerSource)`.

## Finding 2: Distributed source detection uses implicit convention

**Files:** `cmd/tsuku/list.go:150`, `cmd/tsuku/recipes.go:108`, `cmd/tsuku/recipes.go:133`
**Severity:** Advisory

Three places detect distributed sources by checking `strings.Contains(source, "/")`. This is a convention (distributed sources are stored as `"owner/repo"`) but it's not encoded anywhere as a named function or documented constant. The next developer seeing `strings.Contains(tool.Source, "/")` has to work backward to understand what this means.

Today this is safe because only distributed sources contain `/`. But a named helper like `IsDistributedSource(s string) bool` would make the intent self-documenting and give a single place to update if the convention changes.

## Finding 3: recipes.go counts sources but uses default case for distributed detection

**File:** `cmd/tsuku/recipes.go:99-111`
**Severity:** Advisory

The source counting switch has explicit cases for `SourceLocal`, `SourceEmbedded`, and `SourceRegistry`, then falls through to `default` with a `/` check for distributed. This is fine logically, but the display logic at line 116-120 conditionally includes the distributed count in the header only when `distributedCount > 0`. If a new source type is added that contains `/` but isn't distributed, the count will silently be wrong. This is minor given the current architecture but worth noting alongside Finding 2 -- both depend on the same implicit convention.

## Finding 4: No unit tests for any of these commands

**Files:** No `info_test.go`, `list_test.go`, or `recipes_test.go` exist
**Severity:** Advisory

The acceptance criteria call for source display in both human-readable and JSON formats across three commands. None of these commands have unit tests. The next developer modifying the output format (e.g., changing the `[alice/tools]` suffix format in `list`) has no safety net to tell them they broke the JSON `"source"` field.

This isn't new debt introduced by this issue -- the commands lacked tests before -- but the source display logic adds conditional output (source suffix only for distributed, source line only when non-empty) that would benefit from test coverage.

## Finding 5: info.go calls loader.Get then loader.GetWithSource for the same recipe

**File:** `cmd/tsuku/info.go:100, 116`
**Severity:** Advisory

The info command calls `loader.Get(toolName, ...)` at line 100, then calls `loader.GetWithSource(toolName, ...)` at line 116 to discover the source. The first call populates the in-memory cache, so the second call is cheap (cache hit). But the next developer looking at this will wonder why the recipe is loaded twice -- it looks like a bug or oversight. A comment explaining "Get was called above for the recipe data; GetWithSource is called here only for the source tag" would prevent the double-take. Alternatively, use `GetWithSource` for both needs and discard the recipe from the second call (or use it for both).

## Overall Assessment

The implementation matches the issue requirements. Source display works in all three commands for both human-readable and JSON output. The blocking issue is the duplicated source mapping in `info.go` -- it's a straightforward fix to call the existing `recipeSourceFromProvider` helper instead of reimplementing the same switch. The advisory findings are about implicit conventions (`/` as distributed source indicator) and missing test coverage, neither of which will cause immediate bugs but both reduce confidence for the next person making changes.

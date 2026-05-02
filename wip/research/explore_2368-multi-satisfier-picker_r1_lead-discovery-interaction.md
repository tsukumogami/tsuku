# Research: Issue #2368 Multi-Satisfier Alias Picker
## Lead 4: Discovery Layer Integration

**Date:** 2026-04-30  
**Thoroughness:** medium  
**Focus:** How does the existing "Multiple sources found" discovery-layer error work, and should the new alias picker subsume it or stay parallel?

---

## Summary: Discovery Layer Error Flow

The discovery layer (in `internal/discover/`) handles the case where **no recipe exists** in the local registry but the ecosystem probe finds matches in external package registries (npm, PyPI, RubyGems, Homebrew, crates.io, etc.).

### Key Files and Mechanisms

1. **`internal/discover/resolver.go`** (lines 150-194):
   - Defines `AmbiguousMatchError` type, returned when multiple ecosystem registries match a tool name.
   - Error message (line 180): `"Multiple sources found for %q. Use --from to specify:\n"` followed by list of `--from` suggestions.

2. **`cmd/tsuku/install.go`** (lines 305-312 and 597-633):
   - **Fallback trigger** (line 307-309): When a recipe is not found (`loader.Get()` fails), calls `tryDiscoveryFallback()`.
   - **AmbiguousMatchError handler** (line 602-603): Catches `AmbiguousMatchError`, delegates to `handleAmbiguousInstallError()` (lines 547-570).
   - **Handler behavior** (lines 566-569): Prints error message to stderr with formatted list, exits with exit code `ExitAmbiguous`.

3. **`internal/discover/ecosystem_probe.go`** (lines 130-153):
   - The probe runs all ecosystem probers in parallel (Homebrew, crates.io, PyPI, npm, RubyGems, Go, CPAN).
   - Calls `disambiguate()` to rank and select from matches (line 152).

4. **`internal/discover/disambiguate.go`** (lines 77-138):
   - Implements three-tier logic:
     - **Single match**: auto-select.
     - **Clear winner** (≥10x downloads, version count ≥3, has repository): auto-select.
     - **Close matches** (no clear winner) with no interactive callback → return `AmbiguousMatchError`.

### Current User Experience: `tsuku install java`

Today:
```
$ tsuku install java --yes
Error: Multiple sources found for "java". Use --from to specify:
  tsuku install java --from rubygems:java
  tsuku install java --from npm:java
```

The error lists ecosystem hits (discovered packages) that have no corresponding recipe.

---

## Design Decision: Subsume vs. Combine

### Two Options

**Option A: Subsume** (Recommended)
- When ANY first-class satisfier recipe exists for an alias, **hide discovery hits entirely**.
- The picker shows only curated/first-class recipes (openjdk, temurin, corretto, microsoft-openjdk for "java").
- `--from rubygems:java` still works as an explicit override for advanced users.
- **Benefit**: Aligns with tsuku's design philosophy ("curated > discovery" everywhere). Simplifies the picker UX—only one result set to show.
- **Drawback**: Users cannot easily discover that npm/RubyGems have "java" packages (though those are typically wrong-tool hits anyway).

**Option B: Combine**
- Picker shows curated entries on top, then discovery entries below, in a single ranked list.
- User can pick either first-class or discovery source.
- Discovery entries are visually distinguished (color, icon, or "via discovery" marker).
- **Benefit**: Maximizes user choice. If all JDK recipes fail, user can fall back to discovery alternatives.
- **Drawback**: More UX complexity. For "java", discovery hits (npm, RubyGems) are **wrong-tool** hits (testing frameworks, not the JDK). Mixing them dilutes signal.

### Recommendation: **Subsume** (Option A)

**Rationale:**

1. **Alignment with tsuku philosophy**: The discovery layer is a fallback when the curated registry has no recipe. Once curated recipes exist, the discovery layer should recede. This is consistent with the overall priority model: exact recipe name > satisfier recipe > discovery fallback.

2. **Wrong-tool filtering**: The discovery hits for "java" (npm:java, rubygems:java) are not legitimate alternatives to a JDK installer. They are false positives in the discovery registry that happen to match the name. Including them would confuse users and undermine trust in the picker.

3. **Simpler UX**: A single list (recipes) is easier to implement, test, and reason about than a mixed list with visual distinction markers.

4. **Deterministic behavior**: Under `-y` (non-interactive), subsuming ensures a stable error if the picker would show >1 recipe. Combining would require additional logic to rank cross-source (recipe vs. discovery), adding complexity.

5. **Escape hatch preserved**: Users who want to use a discovery source explicitly can still use `--from npm:java` directly. This is documented in the discovery layer's own error messages.

### Code Integration Point

If this option is chosen, the change lives in `cmd/tsuku/install.go` around lines 305-312:

```go
// Check if a recipe exists. If not, try discovery before the full install flow.
// This avoids the confusing "recipe not found" message when discovery can resolve.
_, recipeErr := loader.Get(toolName, recipe.LoaderOptions{})
if recipeErr != nil {
    // NEW: Check for satisfier recipes BEFORE discovery fallback
    if satisfierRecipes := findSatisfierRecipes(toolName); len(satisfierRecipes) > 0 {
        // Show the multi-satisfier picker, NOT the discovery fallback.
        // This subsumes the discovery layer error.
        showSatisfierPicker(toolName, satisfierRecipes)
        continue
    }
    
    // Only try discovery if NO satisfier recipes exist.
    if result := tryDiscoveryFallback(toolName); result != nil {
        continue
    }
}
```

**Execution path for `tsuku install java --yes`:**
1. Recipe not found.
2. Satisfier index lookup → finds 4 recipes (openjdk, temurin, corretto, microsoft-openjdk).
3. Under `-y`: error with picker candidates (4 recipes, no discovery hits).
4. Discovery layer is bypassed entirely.

---

## Consequences of Subsuming

### What changes:
- The discovery layer's `AmbiguousMatchError` is no longer reached when **any** first-class satisfier exists.
- The discovery layer still runs as a fallback if **no satisfier recipes** exist (e.g., `tsuku install ripgrep` when no recipe exists but RubyGems/npm have matches).

### What stays the same:
- The discovery layer's error message and exit code are unchanged for tools with no satisfier recipes.
- `--from` override still works (discovery or otherwise).
- Batch pipeline behavior unchanged (discovery fallback still available).

### Edge case handling:
- **Alias collides with recipe name** (e.g., a future `recipes/j/java.toml` plus other recipes satisfying "java"):
  - The satisfier index takes precedence (existing tsuku design).
  - Discovery layer not consulted.
- **All satisfier recipes fail to generate/install**:
  - The original install error is shown (not a discovery fallback).
  - User can manually use `--from` if desired.

---

## Summary for Issue #2368

**Decision:** Subsume the discovery layer into the multi-satisfier picker.

**Why:** Aligns with tsuku's "curated > discovery" philosophy, simplifies UX, filters out wrong-tool discoveries (e.g., npm:java is a test framework, not a JDK), and preserves the `--from` escape hatch for advanced users.

**Code location:** `cmd/tsuku/install.go`, lines 305–312, add satisfier recipe lookup **before** discovery fallback.

**Testing strategy:**
- `tsuku install java --yes` → shows 4-recipe picker error (not discovery error).
- `tsuku install ripgrep --yes` → shows discovery error (no satisfier recipes exist).
- `tsuku install java --from npm:java` → still works (explicit override).

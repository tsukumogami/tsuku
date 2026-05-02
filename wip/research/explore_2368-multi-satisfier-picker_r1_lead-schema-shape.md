# Issue #2368: Multi-Satisfier Aliases - Schema Shape Analysis

**Date:** 2026-04-30  
**Thoroughness:** Medium  
**Focus:** Smallest schema change for multi-recipe alias support

---

## Current State

### Schema: `internal/recipe/types.go:178`
```go
Satisfies                map[string][]string `toml:"satisfies,omitempty"`
```
- Already multi-valued per ecosystem (e.g., `homebrew = ["openssl@3", "openssl@11"]`)
- Currently supports ecosystems like: `homebrew`, `crates-io`, `npm`, `pypi`, etc.
- Keyed by ecosystem name; validates ecosystem names (lowercase alphanumeric + hyphens)

### Index: `internal/recipe/loader.go`
- **Structure:** `satisfiesIndex map[string]satisfiesEntry` (single recipe per name)
- **Entry:** `satisfiesEntry` struct with `recipeName` and `source`
- **Builder:** `buildSatisfiesIndex()` iterates providers, builds 1:1 mapping
- **Lookups:**
  - `lookupSatisfies(name)` → returns `(recipeName string, bool)`
  - `lookupSatisfiesFiltered(name, source)` → filtered to source
  - `LookupSatisfies(name)` → public API wrapper
- **Resolution:** Called from `resolveFromChain()` as fallback when recipe by name not found

### Current Consumers
1. **`loader.resolveFromChain()` (lines 300-303):** Calls `lookupSatisfies(name)` when direct lookup fails; expects single recipe name or none
2. **`loader.getEmbeddedOnly()` (line 244):** Calls `lookupSatisfiesFiltered()` for embedded-only fallback
3. **Tests:** `satisfies_test.go` validates schema parsing, index building, and 1:1 fallback behavior

### Recipes Using Satisfies Today
Examined 5 representative recipes:

1. **dav1d.toml** → `[metadata.satisfies]` with `homebrew = ["dav1d"]`
2. **utf8proc.toml** → ecosystem-only entries, no cross-ecosystem aliases
3. **openssl.toml** → `homebrew = ["openssl@3"]` (embedded recipe, used in tests)
4. No current recipe uses non-ecosystem "alias" naming
5. No collision risk with current ecosystem-keyed structure

---

## Three Candidate Schema Shapes

### Option 1: **Extend Existing Satisfies with `aliases` Key**

**Schema change (types.go):**
```go
Satisfies map[string][]string `toml:"satisfies,omitempty"`
// Existing: Satisfies["homebrew"] = ["openssl@3"]
// New:      Satisfies["aliases"] = ["java", "node"]
```

**TOML example:**
```toml
[metadata.satisfies]
homebrew = ["openssl@3"]
npm = ["my-pkg"]
aliases = ["java", "node"]  # New: non-ecosystem, multi-recipe eligible
```

**Index change:**
- Build phase flattens `aliases` list into index alongside ecosystem entries
- Index entry tracks whether it came from alias (non-ecosystem) or ecosystem
- Picker logic: if alias lookup returns multiple recipes via index, engage picker

**Blast radius:** LOW
- `buildSatisfiesIndex()`: Add loop over `Satisfies["aliases"]` if present
- `lookupSatisfies()`: No change (still returns single entry, but index may have multiple)
- `validateSatisfies()`: Allow "aliases" as special ecosystem key (no validation of individual alias names against ecosystem pattern)
- Validation test updates: Accept "aliases" as a valid key

**Backward compatibility:** PERFECT
- Existing recipes with ecosystem-only satisfies work unchanged
- No field rename; all existing recipes parse as-is
- Validation only needs to exempt "aliases" from ecosystem name validation

**Clarity for authors:** GOOD
- Clear distinction: `homebrew = [...]` vs. `aliases = [...]`
- Single field, no new top-level fields, minimal schema expansion
- Authors understand "aliases" is non-ecosystem, cross-package

**Behavior change for existing recipes:** NONE
- Existing ecosystem entries remain 1:1 in index
- Picker only engages if alias resolves to multiple recipes
- No change to existing fallback behavior

---

### Option 2: **Top-Level `provides` Field**

**Schema change (types.go):**
```go
type MetadataSection struct {
    // ... existing fields ...
    Satisfies map[string][]string `toml:"satisfies,omitempty"`  // unchanged
    Provides  []string             `toml:"provides,omitempty"`    // NEW
}
```

**TOML example:**
```toml
[metadata]
name = "openjdk"
satisfies.homebrew = ["java@11"]
provides = ["java", "java-compiler"]  # NEW
```

**Index change:**
- Separate `providesIndex map[string][]string` (multi-recipe per alias)
- Build phase: loop `recipe.Metadata.Provides` for each recipe
- Picker engages on `providesIndex` lookup if len > 1

**Blast radius:** MEDIUM
- New field in `MetadataSection` struct
- New index variable in `Loader` struct
- New builder logic: `buildProvidesIndex()` (parallel to satisfies builder)
- New lookup function: `lookupProvides()` (returns `[]string`)
- Update `resolveFromChain()` to try `provides` fallback after `satisfies` fallback
- Validation: new rules for `Provides` field (if any)

**Backward compatibility:** PERFECT
- Entirely new field; existing recipes unchanged
- Existing satisfies behavior preserved
- Pure additive change

**Clarity for authors:** OKAY
- Slight conceptual duplication: "satisfies" (ecosystem-specific) vs. "provides" (generic)
- Naming borrowed from apt/dpkg ecosystem (familiar to Linux users)
- Requires explanation: why two alias mechanisms?

**Behavior change for existing recipes:** NONE
- Satisfies behavior unchanged
- Provides only applies to recipes that declare it
- No picker engagement for existing recipes

---

### Option 3: **Re-Key Existing Satisfies as Multi-Valued**

**Schema change (types.go):**
```go
// Current: map[string][]string  (ecosystem -> [names])
// Change to: map[string][]satisfiesValue where satisfiesValue is a struct
type SatisfiesValue struct {
    Recipe  string // recipe name
    IsAlias bool   // true = cross-ecosystem, false = ecosystem-specific
}

// Or simpler: keep map[string][]string but change INDEX to map[string][]string
```

**Index change:**
```go
// Current: map[string]satisfiesEntry (single recipe per name)
// Change to: map[string][]satisfiesEntry (multiple recipes per name)
```

**TOML example:** (unchanged - still looks the same)
```toml
[metadata.satisfies]
homebrew = ["openssl@3"]
aliases = ["java"]  # new ecosystem key
```

**Resolution change:**
- `resolveFromChain()` checks if index lookup returns len > 1
- If len > 1: picker asks user to choose, or returns error "ambiguous"
- If len == 1: existing behavior
- If len == 0: not found

**Blast radius:** HIGH
- Change index data structure throughout `loader.go`
- Update `buildSatisfiesIndex()` to aggregate multiple recipes per name
- Rewrite `lookupSatisfies()` to return `([]string, bool)` instead of `(string, bool)`
- All callers of `lookupSatisfies()` must handle multiple results
- Picker integration point in `resolveFromChain()` or new wrapper
- **RISK:** Could accidentally enable picker behavior for existing ecosystem entries if not careful (e.g., if two recipes both list `homebrew = ["nodejs"]`)

**Backward compatibility:** RISKY
- Index structure change could affect callers expecting single recipe
- If two existing recipes both list same ecosystem package (collision), picker engages unexpectedly
- Need strict validation to prevent recipe collisions on existing ecosystem entries

**Clarity for authors:** POOR
- No clear distinction between "ecosystem-specific" and "cross-ecosystem" aliases in schema
- All entries look identical in TOML; difference is only in meaning/validation
- Recipe authors must read docs to understand when picker triggers

**Behavior change for existing recipes:** RISKY
- If any two existing recipes both satisfy the same package within same ecosystem, picker engages automatically
- Could break existing callers that expect single recipe per ecosystem package
- Requires migration: audit all recipes for ecosystem collisions

---

## Recommendation: **Option 1 (Extend Existing Satisfies with `aliases` Key)**

**Option 1 is the smallest and safest change.** It requires minimal code modifications, zero blast radius on existing recipes, and crystal-clear semantics for recipe authors. The `aliases` key is visually distinct from ecosystem keys, making the intent obvious in TOML files. The picker logic can be added later in a separate phase (index lookup detects multiple recipes, engages picker only for alias entries).

**Why not the others:**
- **Option 2** is nearly as good but introduces conceptual redundancy (satisfies vs. provides) and requires parallel infrastructure (second index, second builder, second lookup function). It's broader in scope.
- **Option 3** is too risky: re-keying the index as multi-valued could accidentally turn existing 1:1 satisfies entries into picker-eligible entries, changing behavior for recipes already in use.

---

## Exact Files and Functions to Change

### `internal/recipe/types.go`
- **No schema change needed** — `Satisfies map[string][]string` already supports `aliases` key
- Validation logic (see next) will treat "aliases" as special

### `internal/recipe/validate.go`
- **Function: `validateSatisfies()` (lines 74-109)**
  - Update: Skip ecosystem name validation for `"aliases"` key
  - Add: Validate that all entries in `Satisfies["aliases"]` are non-empty strings (like ecosystems)
  - New rule (optional): Aliases should not contain `@` or ecosystem separators (to distinguish from ecosystem packages)

### `internal/recipe/loader.go`
- **Function: `buildSatisfiesIndex()` (lines 432-458)**
  - Add: After ecosystem loop, check if `recipe.Metadata.Satisfies["aliases"]` exists
  - For each alias in the aliases list, add to index: `l.satisfiesIndex[alias] = satisfiesEntry{recipeName, source}`
  - Note: Index remains 1:1 for now; picker logic comes later

### Tests
- **`internal/recipe/satisfies_test.go`**
  - Add: Test case for parsing `aliases` key in TOML
  - Add: Test case for index building with aliases
  - Add: Test case for alias lookup via `lookupSatisfies()`
  - Verify: Backward compatibility (recipes without aliases still work)

### Optional: `internal/recipe/validate_test.go`
- Add test case: `aliases` key passes validation
- Add test case: `aliases` with empty entry fails validation
- Verify: No validation errors for properly-formed aliases

---

## Summary

**Minimal change path:**
1. Update `validateSatisfies()` to allow `"aliases"` as a special ecosystem key
2. Update `buildSatisfiesIndex()` to flatten `Satisfies["aliases"]` into the index
3. No changes to index structure, lookup signatures, or resolution logic
4. Add tests for alias parsing, validation, and index building

**Picker integration (future):**
When issue #2368 milestone includes picker logic, update `resolveFromChain()` or add a wrapper to check if a lookup result came from an alias (tracked by new field in satisfiesEntry), and engage picker if multiple recipes claim the same alias.

**Risk:** None to existing behavior. New feature is entirely additive and behind the "aliases" key.


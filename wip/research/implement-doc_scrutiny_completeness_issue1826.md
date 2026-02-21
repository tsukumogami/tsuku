# Scrutiny Review: Completeness -- Issue #1826

**Focus**: completeness
**Reviewer perspective**: Verify that the mapping is complete and that "implemented" claims hold up against the diff.

---

## AC-by-AC Evaluation

### AC 1: "MetadataSection has Satisfies field"
**Mapping claim**: implemented, evidence: types.go line 161
**Assessment**: CONFIRMED. `types.go` line 162 (not 161, off by one in the mapping) contains:
```go
Satisfies map[string][]string `toml:"satisfies,omitempty"` // Ecosystem name mappings: ecosystem -> []package_names
```
The field type, TOML tag, and comment all match the design doc's specification (`map[string][]string`). The `omitempty` tag ensures backward compatibility (recipes without the field work unchanged).

**Severity**: No finding. The line number is off by one but evidence is valid.

---

### AC 2: "Loader fallback in GetWithContext"
**Mapping claim**: implemented, evidence: loader.go GetWithContext after 4-tier chain
**Assessment**: CONFIRMED. `loader.go` lines 133-136 show the fallback placement after the registry fetch fails:
```go
// Satisfies fallback: check if another recipe satisfies this name
if canonicalName, ok := l.lookupSatisfies(name); ok {
    return l.GetWithContext(ctx, canonicalName, opts)
}
```
This is positioned exactly after the 4-tier chain (cache -> local -> embedded -> registry), matching the design doc's "after the existing 4-tier lookup chain." The recursive call to `GetWithContext` with the canonical name means the resolved recipe goes through the full chain, including caching. The original registry error is preserved and returned only if the satisfies fallback also fails.

**Severity**: No finding.

---

### AC 3: "getEmbeddedOnly fallback"
**Mapping claim**: implemented, evidence: loader.go lookupSatisfiesEmbeddedOnly
**Assessment**: CONFIRMED. `loader.go` lines 161-164 show the embedded-only fallback:
```go
// Satisfies fallback (restricted to embedded-only index entries)
if canonicalName, ok := l.lookupSatisfiesEmbeddedOnly(name); ok {
    return l.getEmbeddedOnly(canonicalName)
}
```
The `lookupSatisfiesEmbeddedOnly` function (lines 332-343) calls the shared `lookupSatisfies` first, then verifies the canonical recipe actually exists in embedded FS via `l.embedded.Has(canonicalName)`. This is the correct restriction for `RequireEmbedded` mode.

**Severity**: No finding.

---

### AC 4: "Validation (ecosystem names, self-ref)"
**Mapping claim**: implemented, evidence: validate.go validateSatisfies()
**Assessment**: CONFIRMED with note. `validate.go` lines 74-109 implement:
- Ecosystem name format validation via `ecosystemNamePattern` (`^[a-z][a-z0-9-]*$`)
- Self-referential check (`pkgName == r.Metadata.Name`)
- Empty package name check

The design doc Phase 1 step 5 also mentions "no entries that collide with existing recipe canonical names." This is NOT implemented here, but the mapping correctly identifies only what IS implemented and the deviated AC below covers the gap. The validation is integrated into `ValidateStructural()` at line 55.

Note: The `validateSatisfies` is integrated into `ValidateStructural()` (called during structural validation and by `ValidateFull`) but is NOT integrated into `runRecipeValidations()` in `validator.go` (the `ValidateFile`/`ValidateBytes`/`ValidateRecipe` path). This means `tsuku validate` (which calls `ValidateFile`) does NOT run satisfies validation. However, the loader's `parseBytes()` calls `validate()` (lines 237-238 of loader.go) which is a DIFFERENT function that doesn't include satisfies validation either. So satisfies validation only runs through the `ValidateStructural`/`ValidateFull` paths. This is an observation but doesn't directly contradict the AC.

**Severity**: Advisory. The two validation paths (`validate.go:ValidateStructural` vs `validator.go:runRecipeValidations`) don't share satisfies validation logic. Users running `tsuku validate` on a recipe with malformed satisfies entries won't see the errors. However, this is an architectural observation about validation path consistency, not a missing AC.

---

### AC 5: "Embedded openssl.toml satisfies entry"
**Mapping claim**: implemented, evidence: openssl.toml lines 8-9
**Assessment**: CONFIRMED. `internal/recipe/recipes/openssl.toml` lines 8-9:
```toml
[metadata.satisfies]
homebrew = ["openssl@3"]
```
Matches the design doc exactly.

**Severity**: No finding.

---

### AC 6: "17 unit tests"
**Mapping claim**: implemented, evidence: satisfies_test.go
**Assessment**: CONFIRMED. `satisfies_test.go` contains 17 test functions:
1. TestSatisfies_ParseFromTOML
2. TestSatisfies_BackwardCompatible
3. TestSatisfies_EmbeddedOpenSSL
4. TestSatisfies_BuildIndex
5. TestSatisfies_LookupKnownName
6. TestSatisfies_LookupUnknownName
7. TestSatisfies_PublicLookup
8. TestSatisfies_GetWithContext_FallbackToSatisfies
9. TestSatisfies_GetWithContext_ExactMatchTakesPriority
10. TestSatisfies_GetEmbeddedOnly_Fallback
11. TestSatisfies_GetEmbeddedOnly_NonEmbeddedSatisfier
12. TestSatisfies_Validation_SelfReferential
13. TestSatisfies_Validation_MalformedEcosystem
14. TestSatisfies_Validation_EmptyPackageName
15. TestSatisfies_Validation_ValidRecipe
16. TestSatisfies_Validation_NoSatisfiesField
17. TestSatisfies_LazyBuild
18. TestSatisfies_ClearCacheResetsIndex

That's actually 18 test functions, not 17. The count is slightly off in the mapping but this is a minor imprecision (more is fine).

**Severity**: No finding.

---

### AC 7: "Cross-recipe canonical name collision CI validation"
**Mapping claim**: deviated, reason: "Deferred to #1829 - requires full recipe set"
**Assessment**: REASONABLE DEVIATION. The design doc Phase 1 step 5 mentions "no entries that collide with existing recipe canonical names" but also says in Uncertainties: "A `satisfies` entry that matches another recipe's canonical name... should be rejected by validation." The design doc's Phase 4 (#1829) includes "cross-recipe duplicate detection at CI time."

The deviation reason is accurate: single-recipe structural validation cannot check whether a satisfies entry collides with another recipe's canonical name without access to the full recipe set. This naturally belongs in CI-time validation or the registry generation script, which is #1829's scope.

**Severity**: No finding. The deviation is genuine and well-placed.

---

## Downstream Enablement Check (informational, per completeness focus)

The mapping doesn't explicitly claim downstream support, but checking briefly:

- **#1827** needs `lookupSatisfies()` or public API: `LookupSatisfies()` is provided at loader.go lines 345-350. Confirmed available.
- **#1828** needs satisfies field + loader fallback working: Both confirmed above.
- **#1829** needs `Satisfies` field on `MetadataSection` and `satisfiesIndex` on Loader: Both confirmed. The `buildSatisfiesIndex` has a comment at line 318: "Registry manifest entries would be added here by #1829", explicitly marking the extension point.

---

## Phantom AC Check

All mapping entries correspond to design doc Phase 1 requirements or test plan scenarios. No phantom ACs detected.

---

## Missing AC Check

Comparing design doc Phase 1 steps against the mapping:

1. Add Satisfies field to Metadata struct -- covered (AC 1)
2. Add satisfiesIndex and buildSatisfiesIndex() to Loader -- covered (AC 2, AC 3 implicitly)
3. Add fallback lookup in GetWithContext() -- covered (AC 2)
4. Add satisfies field to embedded openssl.toml -- covered (AC 5)
5. Add validation for the satisfies field -- covered (AC 4, AC 7)
6. Add tests for the new lookup path -- covered (AC 6)

No missing ACs.

---

## Summary

All 7 mapping entries verified against the diff. 6 "implemented" claims confirmed with valid evidence in the correct files and functions. 1 deviation ("cross-recipe canonical name collision CI validation" deferred to #1829) is genuine and well-justified since single-recipe validation cannot access the full recipe set.

One advisory observation: the `validateSatisfies` function is integrated into `ValidateStructural` but not into the `validator.go:runRecipeValidations` path, meaning `tsuku validate` (the CLI command) may not surface satisfies validation errors. This is an existing architectural split between two validation paths and doesn't block this issue.

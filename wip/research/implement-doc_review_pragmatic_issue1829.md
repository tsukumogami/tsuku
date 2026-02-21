# Pragmatic Review: Issue #1829

**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Issue**: #1829 (feat(registry): include satisfies data in registry manifest)

---

## Finding 1 (Blocking): `FetchManifest` has zero callers outside tests

`internal/registry/manifest.go:53` -- `FetchManifest()` is a 40-line exported method with HTTP fetching, local file handling, caching, and response body parsing. It is called only from test files (`manifest_test.go`). No CLI command, loader path, or other production code calls it. The `update-registry` command refreshes individual recipe caches but never fetches the manifest.

The `readLocalManifest` helper (line 117) and `isLocalPath` dispatch within `FetchManifest` (line 57) exist solely to support `FetchManifest`'s test path.

The only manifest method used in production is `GetCachedManifest()` (called from `loader.go:403`). This means the manifest cache file is never populated in production, making the entire manifest integration in `buildSatisfiesIndex` a dead path too -- `GetCachedManifest()` will always return `nil, nil` because nothing wrote `manifest.json` to the cache dir.

**Suggestion**: Either wire `FetchManifest` into the `update-registry` command (so the manifest actually gets cached), or remove `FetchManifest`, `readLocalManifest`, `manifestURL()`, `DefaultManifestURL`, and `EnvManifestURL`. The first option makes the manifest path reachable; the second removes ~80 lines of dead code.

---

## Finding 2 (Advisory): `LookupSatisfies` (public) has zero production callers

`internal/recipe/loader.go:449` -- The comment says "Currently unused." It was added by #1826 for #1827's anticipated use, but #1827 used `GetWithContext` instead. 3-line wrapper, low cost, but it's speculative API surface.

---

## Finding 3 (Advisory): `readLocalManifest` single-caller helper

`internal/registry/manifest.go:117-123` -- Three lines, one caller. Could be inlined. Not blocking since the function is small and named clearly.

---

## Finding 4 (Advisory): `EnvManifestURL` and `DefaultManifestURL` constants are dead in production

`internal/registry/manifest.go:13-23` -- Only consumed by `manifestURL()` which is only called by `FetchManifest` which has no production callers (Finding 1). These become relevant if Finding 1 is resolved by wiring in `FetchManifest`.

---

## Summary

| # | Finding | Severity | File |
|---|---------|----------|------|
| 1 | `FetchManifest` zero production callers; manifest cache never populated | Blocking | `manifest.go:53` |
| 2 | `LookupSatisfies` exported with zero production callers | Advisory | `loader.go:449` |
| 3 | `readLocalManifest` single-caller helper | Advisory | `manifest.go:117` |
| 4 | Constants dead due to Finding 1 | Advisory | `manifest.go:13-23` |

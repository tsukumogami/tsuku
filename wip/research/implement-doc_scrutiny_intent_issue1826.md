# Scrutiny Review: Intent -- Issue #1826

## Focus: Design Intent Alignment + Cross-Issue Enablement

---

## Sub-check 1: Design Intent Alignment

### Satisfies Field on MetadataSection
**AC:** MetadataSection has Satisfies field
**Design says:** `Satisfies map[string][]string` on `Metadata`, optional, `toml:"satisfies,omitempty"`
**Implementation:** `types.go:162` -- exact match. Field type, tag, and position are consistent with the design's Key Interfaces section.
**Verdict:** Aligned.

### Loader Fallback in GetWithContext
**AC:** Loader fallback in GetWithContext
**Design says:** After the existing 4-tier chain fails, search the satisfies index and return the satisfying recipe. Index built lazily on first fallback.
**Implementation:** `loader.go:133-136` -- after registry fetch fails, calls `lookupSatisfies(name)`, then recursively calls `GetWithContext` with the canonical name. Index uses `sync.Once` for lazy build.
**Verdict:** Aligned. The recursive call to `GetWithContext` is a reasonable approach since it reuses the full lookup chain for the canonical name, including caching.

### getEmbeddedOnly Fallback
**AC:** getEmbeddedOnly fallback
**Design says:** The satisfies fallback should also work when `RequireEmbedded` is set, but only return results from embedded recipes.
**Implementation:** `loader.go:161-164` -- `lookupSatisfiesEmbeddedOnly` checks the satisfies index but then verifies the canonical recipe is actually in embedded FS. This is a design intent that isn't explicitly stated in an AC but is captured in the implementation. Correct behavior for action dependency validation where only embedded recipes are trusted.
**Verdict:** Aligned. Goes slightly beyond the explicit ACs to correctly handle the embedded-only path.

### Validation
**AC:** Validation (ecosystem names, self-ref)
**Design says Phase 1 step 5:** "Add validation for the satisfies field: well-formed ecosystem names, no self-referential entries, no entries that collide with existing recipe canonical names."
**Implementation:** `validate.go:74-109` implements ecosystem name format validation and self-referential check. Does NOT implement canonical name collision check (where a satisfies entry matches another recipe's canonical name, e.g., recipe `foo` declaring `satisfies.homebrew = ["sqlite"]` when `sqlite` is an existing recipe).
**Analysis:** The design explicitly places canonical name collision validation in Phase 1 (not Phase 4). The coder's mapping labels this as "deferred to #1829" but conflates it with cross-recipe duplicate detection, which IS a Phase 4 concern. Per-recipe canonical name collision is structural validation that belongs in `validateSatisfies()` since it doesn't require scanning other recipes -- it only requires knowing the recipe's own name vs. its satisfies entries. However, the design's Uncertainties section notes this "can't cause resolution issues at runtime (exact match takes priority)" and that it's "confusing and likely an error." Since it has no runtime impact and the coder's reasoning (requires full recipe set) is plausible for the cross-recipe variant, this is advisory rather than blocking.
**Verdict:** Minor intent gap. The per-recipe canonical name collision check is a single-recipe validation concern specified in Phase 1 that was not implemented. It doesn't affect runtime or downstream issues.

### Embedded openssl.toml
**AC:** Embedded openssl.toml satisfies entry
**Design says:** `[metadata.satisfies] homebrew = ["openssl@3"]`
**Implementation:** `openssl.toml:8-9` -- exact match.
**Verdict:** Aligned.

### Tests
**AC:** 17 unit tests
**Design says Phase 1 step 6:** "Add tests for the new lookup path." Also notes: "Phase 1 tests should use a test-only satisfies entry to validate the fallback path independently."
**Implementation:** `satisfies_test.go` contains 17 tests organized into Schema, Index, Loader Fallback, Validation, and Lazy Initialization categories. Tests use both the real embedded openssl recipe and test-only recipes (via `setupSatisfiesTestLoader` helper with manual index population). Test for exact-match priority is included (line 237). Test for embedded-only non-embedded satisfier rejection is included (line 303).
**Verdict:** Aligned. Test coverage matches the design's intent for independent fallback validation.

---

## Sub-check 2: Cross-Issue Enablement

### #1827 (tsuku create satisfies check)
**Needs:** Public API to check if a name is already satisfied before generating a recipe.
**Provides:** `LookupSatisfies(name string) (string, bool)` at `loader.go:345-350`. Returns canonical recipe name and whether found.
**Assessment:** Sufficient. The `tsuku create` command can call `loader.LookupSatisfies("openssl@3")` to get `("openssl", true)` and display an appropriate message. The return signature gives both the canonical name (for the user message) and the boolean (for control flow).

### #1828 (data cleanup and migration)
**Needs:** Working satisfies field + loader fallback so that deleting `openssl@3.toml` doesn't break resolution. Other recipes can declare satisfies entries (e.g., `gcc-libs`, `sqlite`, etc.).
**Provides:** The full mechanism: field parsing, index building from embedded recipes, and loader fallback. Once #1828 deletes the duplicate recipe and migrates satisfies entries, the loader will resolve them automatically.
**Assessment:** Sufficient. No additional API surface needed.

### #1829 (registry manifest integration)
**Needs:** `Satisfies` field on `MetadataSection` for registry generation script to read. `satisfiesIndex` on `Loader` to populate from manifest data. Potentially needs `buildSatisfiesIndex` to be extensible for registry data.
**Provides:** The `Satisfies` field is on the struct. The `buildSatisfiesIndex` method has a clear comment at line 318: "Registry manifest entries would be added here by #1829." The index data structure (`map[string]string`) is the same regardless of source.
**Assessment:** Sufficient. #1829 would add registry manifest scanning inside `buildSatisfiesIndex()`, which is the intended extension point. The `satisfiesIndex` field is package-internal, so #1829 code in the same package can extend the builder.

---

## Sub-check 3: Backward Coherence

No previous summary provided (first issue in sequence). Skipped.

---

## Summary of Findings

### Advisory (1)

**Per-recipe canonical name collision validation not implemented.**
The design doc's Phase 1 step 5 specifies validation for "entries that collide with existing recipe canonical names." The implementation validates ecosystem name format and self-referential entries but omits the canonical name collision check. The coder's mapping conflates this per-recipe check with the cross-recipe duplicate detection that genuinely belongs in #1829. However, the design doc itself notes this "can't cause resolution issues at runtime" and the gap doesn't affect downstream issues.

### Blocking (0)

None. The implementation captures the design's core intent: satisfies field on metadata, lazy index in the loader, fallback after the 4-tier chain, embedded-only variant, and validation. All three downstream issues have the API surface and data structures they need.

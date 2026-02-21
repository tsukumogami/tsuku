# Architecture Review: DESIGN-ecosystem-name-resolution.md

**Reviewer**: architect-reviewer
**Date**: 2026-02-21
**Document**: `docs/designs/DESIGN-ecosystem-name-resolution.md`

---

## 1. Problem Statement Specificity

**Verdict: Strong.** The problem statement identifies three concrete, verifiable failure modes: duplicate recipe generation (`openssl@3.toml` alongside embedded `openssl`), false pipeline blockers (`afflib` blocked by `openssl@3`), and fragile dependency parsing (`apr-util.toml` declaring `openssl@3` as a dependency name that happens to work only because `parseDependency` splits on `@`). Each is traceable to code locations.

One gap: the problem statement says `data/dep-mapping.json` was created but "no code consumes it." I verified this is accurate -- grep for `dep-mapping` finds no Go source references, only the JSON file itself and the data README. The dead file is correctly identified as evidence that the static-mapping approach doesn't sustain itself.

The scope boundary with issue #969 (`provides` field for sonames) is clean. `satisfies` answers "what ecosystem package names does this recipe cover?" while `provides` would answer "what sonames or CLI commands does this recipe expose?" Different consumers, different data shapes. The design correctly calls this out.

## 2. Missing Alternatives

Two alternatives worth considering that the design doesn't discuss:

### 2a. Two-way index at `tsuku create` time only

Instead of adding a loader fallback that runs on every failed lookup, build the `satisfies` index only in the `tsuku create` and batch pipeline code paths. The `install` path already works for `openssl@3` because `parseDependency` splits it into `openssl` + version `3`. This would be simpler to implement (no loader changes) at the cost of incomplete coverage.

**Why it's likely rejected**: The design's second decision explicitly addresses this as "per-caller resolution" and rejects it for logic duplication. Fair -- but the argument could be stronger. The real problem with per-caller resolution isn't just duplication; it's that the batch pipeline's `extractBlockedByFromOutput` extracts raw names from stderr and feeds them back into the loader. If the loader can't resolve those names, the pipeline's blocker tracking breaks regardless of what `tsuku create` does. The loader fallback is the correct integration point because the pipeline doesn't control the names it receives.

### 2b. Recipe aliases (symlink-like)

A `recipes/o/openssl@3.toml` that contains only `alias = "openssl"` -- a single field that redirects to the canonical recipe. This would work with the existing loader chain (exact name match succeeds) without adding a new fallback step.

**Why it's weaker than `satisfies`**: It requires creating a file per alias per ecosystem name. For the current 6 mappings this is fine, but it doesn't scale for recipes that satisfy many ecosystem names. It also lacks the ecosystem namespace -- you can't distinguish "this name in Homebrew" from "this name in apt" -- which matters if two ecosystems use the same name for different packages.

I don't think either missing alternative is strong enough to change the recommendation. The design's chosen approach is the right structural decision.

## 3. Rejection Rationale Fairness

### Static mapping file (dep-mapping.json)

**Fair.** The design uses the codebase's own history as evidence: the file was created in #1200, never wired in, and no code consumes it. This is specific and verifiable. The scaling argument (every new Homebrew formula requires manual update) is accurate for a centralized file.

### Convention-based stripping (strip `@N`)

**Fair.** The design correctly identifies two failure modes: (1) it only works for Homebrew's `@` convention, and (2) it fails for names with no `@` relationship (`sqlite3` -> `sqlite`, `gcc` -> `gcc-libs`). The `dep-mapping.json` confirms these non-`@` cases exist. This isn't a strawman.

### Reverse index from step formulas

**Fair.** Building a reverse map from `formula = "openssl@3"` to recipe name is fragile because the formula field is a step parameter, not a metadata declaration. Changing a formula in a step would silently break the index. The design's objection that this "only works for Homebrew" is also accurate -- PyPI, npm, and crate names aren't declared as step formulas.

### Per-caller resolution

**Fair.** The design's rejection is correct but understated. The strongest argument isn't that it "duplicates logic across 4+ call sites" -- it's that the batch orchestrator's `extractBlockedByFromOutput` produces raw names from error messages, and those names flow back through the loader. Per-caller resolution can't intercept that flow without duplicating the loader's own lookup chain.

### Pre-resolution normalization

**Fair.** Normalizing before the loader changes input semantics. If `openssl@3` gets rewritten to `openssl` during argument parsing, the user can't install the actual `openssl@3` recipe if one legitimately exists. The loader fallback preserves the distinction: try the exact name first, only fall back to `satisfies` if no exact match exists.

**No strawmen detected.** All alternatives address real approaches; none are designed to fail.

## 4. Unstated Assumptions

### 4a. The `@` ambiguity is resolved by lookup order, not syntax

The design doesn't explicitly state this, but the chosen solution relies on an implicit precedent rule: exact name match wins over `satisfies` fallback. So if both `openssl@3.toml` (registry) and `openssl.toml` (embedded, with `satisfies.homebrew = ["openssl@3"]`) exist, the loader returns `openssl@3` from the registry. This is fine **after** the duplicate is deleted (Phase 3), but during the transition there's a window where both exist.

**Recommendation**: State explicitly that Phase 1 (loader fallback) and Phase 3 (delete duplicate) should ship together, or that the fallback only activates when the exact-match recipe doesn't exist.

### 4b. Embedded recipes are the primary source of `satisfies` data

The design says "scan all embedded recipes and (if available) the registry manifest" to build the index. In practice, the 6 known mismatches are all for embedded recipes (`openssl`, `sqlite`, `gcc-libs`, `python-standalone`, `libcurl`, `libnghttp2`). Registry recipes are pipeline-generated and less likely to have `satisfies` entries because the pipeline doesn't know about them yet.

This assumption is fine today but worth stating. If registry-only recipes eventually need `satisfies` entries, Phase 4 (registry manifest integration) becomes more than "nice to have."

### 4c. `parseDependency` behavior is preserved

The design's third data flow example (lines 227-230) shows that `parseDependency("openssl@3")` still splits to `name=openssl, version=3`, and the loader finds `openssl` by exact match. This means the `satisfies` fallback is never triggered for this common path. The design correctly identifies this but doesn't explicitly state: **`parseDependency` behavior is not changed by this design.** This matters because the `@` splitting is what makes `apr-util`'s `runtime_dependencies = ["openssl@3"]` work today. If someone later removes the `@` splitting (because `satisfies` now handles it), `apr-util` would break unless the dependency is also changed to `openssl`.

**Recommendation**: Add an explicit note that `parseDependency` is unchanged. The Phase 3 fix to `apr-util.toml` (changing `openssl@3` to `openssl`) is defensive and correct -- it removes reliance on the accidental `@` splitting.

### 4d. Index key format

The design proposes `satisfiesIndex map[string]string` with keys as `ecosystem:package_name`. But the `lookupSatisfies` method signature takes just `name string`, not `(ecosystem, name)`. How does the loader know which ecosystem namespace to search?

Looking at the use case: the loader receives a bare name like `openssl@3` from a dependency declaration. It doesn't know if this is a Homebrew name, an apt name, or something else. So the lookup presumably searches **all** ecosystem namespaces for a match. This means the index key should be just the package name (not namespaced by ecosystem), or the lookup must iterate all ecosystems.

**This is a design gap.** If two different ecosystems use the same name for different packages (e.g., Homebrew's `foo` maps to recipe `bar`, but apt's `foo` maps to recipe `baz`), the flat lookup is ambiguous. The design's Uncertainties section mentions collision between recipes but not collision across ecosystems.

For the current scope (6 Homebrew-only mappings), this doesn't matter. But the `satisfies` map structure supports multiple ecosystems (`homebrew`, `apt`, etc.), suggesting future use. The design should either:
1. State that the lookup is ecosystem-agnostic (search all namespaces, error on cross-ecosystem collision), or
2. Require callers to specify which ecosystem context they're resolving in

Option 1 is simpler and sufficient for now. It should be stated explicitly.

## 5. Strawman Analysis

**No strawman options detected.** See detailed analysis in section 3. Every alternative represents a real design approach that was considered and rejected for specific, verifiable reasons.

## 6. Architecture Clarity

### Structural fit: Good

The `satisfies` field on `MetadataSection` follows the existing pattern of recipe metadata fields. Adding it to the `Metadata` struct in `types.go` is the natural location. The loader fallback follows the existing pattern of the 4-tier chain (cache -> local -> embedded -> remote) -- it's a 5th tier, not a parallel mechanism.

The `sync.Once` for lazy initialization follows Go conventions and the codebase doesn't currently use it in the recipe package, but it's idiomatic and contained.

### Integration points: Well-chosen

The loader is the right place. Every code path that needs a recipe goes through `GetWithContext()`. The design avoids the action dispatch bypass anti-pattern -- it doesn't add satisfies checking to individual callers.

The `tsuku create` integration at `cmd/tsuku/create.go:737` is specifically identified as needing expansion from `os.Stat` to a full loader check. This is correct -- the current check only catches local file collisions, not embedded or satisfies matches.

### Return type question

The design says `lookupSatisfies` returns `(string, bool)` -- the recipe name, not the recipe itself. The caller must then do a second lookup (`GetWithContext` with the returned name). This is a reasonable separation: the index maps names, the loader loads recipes. But it creates a subtle issue: the fallback path makes two lookups (one to check satisfies, one to load the recipe). If the satisfying recipe's name is in the cache, the second lookup is instant. If not, it goes through the full 4-tier chain. This is fine for performance but should be documented.

## 7. Missing Components or Interfaces

### 7a. Validation for `satisfies` field

The `validate()` function in `loader.go:481-504` checks metadata name, steps, and verify command. It doesn't validate `satisfies` entries. Phase 1 should include validation that:
- Ecosystem names are known strings (or at least non-empty)
- Package names in each ecosystem are valid (match `NAME_PATTERN`)
- No recipe's `satisfies` entries include its own name (tautological)

The design mentions "recipe validation can warn when a recipe uses `formula = 'foo@3'` in steps but doesn't declare a corresponding `satisfies` entry" -- this is the forward-looking validation and is correctly deferred to Consequences/Mitigations. But basic field validation belongs in Phase 1.

### 7b. Registry JSON schema update

The `generate-registry.py` script currently outputs `name`, `description`, `homepage`, `dependencies`, and `runtime_dependencies` per recipe (line 222-228). Phase 4 needs to add `satisfies` to this output. The design acknowledges this as an uncertainty ("the exact format of this inclusion hasn't been designed") but doesn't discuss backward compatibility of the JSON schema.

The script has a `SCHEMA_VERSION = "1.1.0"` -- bumping this would be the right signal to consumers. This is a small detail but worth noting to avoid a schema version oversight.

### 7c. `ListAllWithSource` doesn't surface satisfies data

`ListAllWithSource()` returns `RecipeInfo` structs with `Name`, `Description`, and `Source`. Code that lists recipes (e.g., `tsuku recipes`, `tsuku search`) won't show that `openssl` satisfies `openssl@3`. This is probably fine for the initial implementation, but `tsuku search openssl@3` should find `openssl`. The search path would need to check the satisfies index. The design doesn't mention search integration.

### 7d. `ClearCache` doesn't reset the satisfies index

The `ClearCache()` method at `loader.go:260` resets `l.recipes`. If the satisfies index is built via `sync.Once`, clearing the cache won't rebuild the index. This is probably correct (the satisfies data comes from embedded recipes and the manifest, not the cache), but should be confirmed.

## 8. Phase Sequencing

### Phase 1 (Schema and Loader) -> Phase 2 (Create Command) -> Phase 3 (Data Cleanup) -> Phase 4 (Registry)

**Correct sequencing with one concern.**

Phase 1 and Phase 3 have a dependency the document understates. Phase 1 adds the `satisfies` fallback. Phase 3 deletes `recipes/o/openssl@3.toml`. If Phase 1 ships without Phase 3, the loader finds `openssl@3.toml` by exact match in the registry and never hits the fallback. The `satisfies` field on the embedded `openssl` recipe is dead code until Phase 3 removes the duplicate.

This isn't a sequencing error -- Phase 3 depends on Phase 1 (need the fallback before deleting the recipe) and shipping Phase 1 first is safe (it's backward compatible). But the design should note that the fallback's value is only realized after Phase 3. Reviewers of Phase 1 might question the new code's utility without this context.

Phase 2 (create command) correctly follows Phase 1 because it depends on the loader's `satisfies` index existing.

Phase 4 (registry integration) is correctly last. It's a scaling concern, not a correctness concern -- the embedded recipes handle the known 6 cases. Registry integration only matters when non-embedded recipes need `satisfies` entries.

## 9. Simpler Alternatives Overlooked

### 9a. Fix only the known 6 cases without new infrastructure

The 6 known mismatches are: `openssl@3` -> `openssl`, `sqlite3` -> `sqlite`, `gcc` -> `gcc-libs`, `python@3` -> `python-standalone`, `curl` -> `libcurl`, `nghttp2` -> `libnghttp2`. You could:

1. Delete `openssl@3.toml`
2. Fix `apr-util.toml` to use `openssl`
3. Add a hardcoded map in the batch orchestrator for dashboard purposes

This is simpler but doesn't scale. The design is building infrastructure for a problem that's currently small but will grow as more Homebrew formulas are converted to tsuku recipes. The batch pipeline generates recipes from Homebrew, and Homebrew has many versioned formulas (`python@3.14`, `node@22`, `ruby@3.3`, etc.). The `satisfies` field preempts the N+1 case.

**Judgment**: The design's approach is proportionate. The implementation cost is low (one new field, one loader fallback), and it addresses the root cause rather than patching symptoms.

### 9b. Rename embedded recipes to match ecosystem names

Instead of `openssl.toml` with `satisfies.homebrew = ["openssl@3"]`, create `openssl@3.toml` as the canonical name. This eliminates the need for name resolution entirely -- the names just match.

**Why this fails**: tsuku's naming convention is kebab-case without version suffixes. The `@` character in filenames creates problems on some filesystems and in URL routing. More importantly, `openssl@3` is Homebrew's convention for a versioned formula -- when OpenSSL 4 comes out, Homebrew might create `openssl@4`, but the tsuku recipe should still be `openssl` with the version updated. The recipe name shouldn't encode a third-party ecosystem's versioning scheme.

## Summary of Findings

### Blocking

None. The design fits the existing architecture. The `satisfies` field is a natural metadata extension, the loader fallback follows the existing tier chain pattern, and no parallel dispatch or resolution mechanism is introduced.

### Advisory

1. **Index key semantics**: The design should state whether `satisfiesIndex` keys are namespaced by ecosystem or flat. The interface description (`lookupSatisfies(name string)`) implies flat lookup across all ecosystems. This works for current scope but should be explicit about cross-ecosystem collision behavior. (Section 4d)

2. **Phase 1/3 coupling**: Note that the `satisfies` fallback is effectively dormant until Phase 3 removes the duplicate `openssl@3.toml`. Reviewers of Phase 1 need this context. (Section 8)

3. **`parseDependency` invariant**: State explicitly that `parseDependency` behavior is unchanged. The `apr-util.toml` fix in Phase 3 is defensive against future changes to `@` splitting semantics. (Section 4c)

4. **Field validation in Phase 1**: The `validate()` function should check `satisfies` entries for well-formedness (valid ecosystem names, valid package names). Currently not mentioned in Phase 1 scope. (Section 7a)

5. **Search integration not addressed**: `tsuku search openssl@3` should find `openssl` via the satisfies index, but search is not mentioned. This can be a follow-up. (Section 7c)

### Out of Scope

- Code quality of the proposed Go snippets (tester/maintainer concern)
- Whether `sync.Once` is the right concurrency primitive (pragmatic concern)
- Performance of scanning embedded recipes (the design correctly identifies this as a latency-on-uncommon-path issue)

---

## Architectural Verdict

The design respects the existing architecture. The `satisfies` field extends the recipe metadata pattern. The loader fallback extends the existing tier chain rather than creating a parallel resolution mechanism. All callers benefit through the single integration point (`GetWithContext`), avoiding action dispatch bypass. No dependency direction violations. No state contract drift -- the new field has clear consumers (loader index, create command, batch pipeline).

The design is ready for implementation with the advisory items above addressed in the document or tracked as follow-up issues.

# Design Review: Deterministic Binary Name Discovery for Recipe Builders

**Reviewed:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-deterministic-recipe-repair.md`
**Date:** 2026-02-22

---

## 1. Problem Statement Evaluation

**Verdict: Specific and well-grounded.**

The problem statement identifies the exact failure mode (workspace monorepos), names concrete examples (sqlx-cli, probe-rs-tools), and references the PR where failures were caught (#1869). It correctly distinguishes this problem from the verification self-repair problem (DESIGN-verification-self-repair) -- one is about wrong binary names, the other is about tools that don't support `--version`.

The scope section is clean: it explicitly excludes LLM builders, the verification self-repair system, and a batch repair command. All three are things someone might try to bundle in, so calling them out is useful.

**One gap:** The problem statement says "these errors aren't caught until sandbox validation" but doesn't quantify the cost. How many recipes in the registry have this issue? How long does a wasted sandbox run take? The rationale section (line 110) gives "100ms API check beats a 60-second container failure" which partially answers this, but it would be stronger in the problem statement itself to justify the work.

---

## 2. Missing Alternatives

### 2a. Crate tarball approach (for crates.io)

The explore summary at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/wip/explore_summary.md` line 15 mentions: "Crate tarballs on crates.io contain the correct sub-crate Cargo.toml." This is a third option for crates.io that isn't mentioned as an alternative -- downloading the crate tarball and extracting the member Cargo.toml. It's more complex than using `bin_names` and thus correctly not chosen, but the fact that it was discovered during research and not documented as a rejected alternative is a minor omission.

That said, since the `bin_names` approach is clearly superior (no download needed), skipping this alternative doesn't weaken the decision.

### 2b. Version resolution coupling

The design assumes the correct version's `bin_names` can be matched to the resolved version, but the current `fetchCrateInfo()` calls `/api/v1/crates/{name}` which returns ALL versions. The `Versions` field is currently `[]struct{}`. The design says "use the `bin_names` from the version matching the resolved version" (line 154) but doesn't address how version resolution happens. Looking at the code, the builder doesn't resolve a specific version -- it generates a recipe without a pinned version. So which version's `bin_names` should be used? The latest? The design should state this explicitly: use the latest version's `bin_names`, or the first version entry (which in the crates.io API response is the latest).

This isn't a missing alternative per se, but it's a missing detail in the chosen option.

### 2c. No missing major alternatives

The three options considered for Decision 1 (registry APIs, fix repo parsing, use cargo metadata) cover the reasonable design space. I don't see a fourth option worth considering.

---

## 3. Rejection Rationale Fairness

### Decision 1 Rejections

**"Fix repository parsing for monorepos"** -- Fair rejection. The rationale correctly identifies the combinatorial complexity of workspace layouts (path deps, virtual manifests, nested workspaces). The registry has already done this resolution.

**"Use cargo metadata"** -- Fair rejection. Requiring the Rust toolchain for recipe generation is a real constraint, and it doesn't generalize.

### Decision 2 Rejections

**"Rely on sandbox validation alone"** -- Fair. The 100ms vs 60s comparison is concrete.

**"Post-sandbox binary name repair"** -- Described as "viable but slower" and "could be added later." This isn't rejected -- it's deferred. The framing is fair and doesn't dismiss it.

### Decision 3 Rejections

**"All registries at once"** -- Fair. Bundling trivial fixes with complex artifact download work is wasteful.

**No strawmen detected.** All alternatives represent legitimate approaches that someone could reasonably advocate for.

---

## 4. Unstated Assumptions

### 4a. `bin_names` field availability (IMPORTANT)

The design claims `bin_names` "has been present since at least 2016, well-documented in OpenAPI spec" (line 252). The explore summary contains an internal contradiction: line 14 says "The crates.io API does NOT expose a `bin_names` field (contrary to the issue's assumption)" while line 20 says "API has `bin_names` field on version objects."

This is a critical factual question for the design. The crates.io API **does** include `bin_names` in version objects in the `/api/v1/crates/{name}` response. The Phase 1 note in the explore summary was written before the research was completed and reflects an incorrect early assumption. The Phase 2 finding is the correct one.

**Recommendation:** Verify by making a live API call to `https://crates.io/api/v1/crates/sqlx-cli` and confirming `bin_names` appears in the version objects with value `["sqlx"]`. This is the keystone of the entire design -- if `bin_names` doesn't exist or is always empty, the approach falls apart.

### 4b. Library-only crates

The design says "falling back to the crate name only if `bin_names` is empty or null (which indicates a library-only crate)" (line 155). But the current system generates recipes for library-only crates? If `bin_names` is empty, the crate has no binaries and probably shouldn't have a recipe at all. The design treats this as a fallback path but doesn't address whether a library-only crate should even reach this point.

### 4c. npm string-type bin field semantics

The design says when `bin` is a string, "there's a single executable whose name matches the package name" (line 160). Looking at the actual npm documentation, when `bin` is a string, the executable name is derived from the package name (stripping scope prefix for scoped packages). The current code at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/npm.go:263-264` returns nil with a comment "we can't determine the name from this." The fix proposed in the design is correct: return `[]string{packageName}`. But for scoped packages like `@scope/tool`, the executable name is `tool`, not `@scope/tool`. The design doesn't address scope stripping.

### 4d. `BinaryNameProvider` interface vs existing builder patterns

The design proposes an optional `BinaryNameProvider` interface (line 172-178) that builders can implement. The existing codebase uses `SessionBuilder` as the main builder interface. Adding an optional interface that the orchestrator checks via type assertion is a Go idiom, but the design doesn't address where the authoritative names are stored during the build lifecycle. The builder calls `fetchCrateInfo()` in `Build()`, but the `BinaryNameProvider.AuthoritativeBinaryNames()` method would need access to that data after `Build()` returns. This implies the builder must cache the registry response, which is a behavior change not called out in the design.

---

## 5. Structural Fit (Architect Review)

### BinaryNameProvider interface placement -- Advisory

The proposed `BinaryNameProvider` interface lives in the builders package and is consumed by the orchestrator (also in the builders package). This is self-contained and doesn't cross package boundaries. The pattern of optional interfaces checked by type assertion is already used in the codebase (`ConfirmableError` at builder.go:44). Fits the existing architecture.

### Orchestrator changes -- Advisory

The design adds `validateBinaryNames()` between `buildRecipe()` and `validateInSandbox()`. Looking at the actual orchestrator code (`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/orchestrator.go`), the flow is `session.Generate()` -> sandbox validation loop. The design's data flow diagram (line 183-197) references `builder.Build()` but the actual orchestrator calls `session.Generate()`. This is a naming discrepancy in the design, not a structural problem -- the integration point is the same.

The validation step runs between generation and sandbox. This mirrors the existing `attemptVerifySelfRepair()` pattern, which runs between sandbox failure and LLM repair. The two are complementary: binary name validation prevents sandbox failures; verify self-repair handles sandbox failures that still occur. No structural conflict.

### No parallel pattern introduced

The design replaces the existing `discoverExecutables()` approach rather than adding a parallel path. The `BinaryNameProvider` is a new capability, not a duplication. Clean.

---

## 6. Writing Style

The document generally follows the project's writing guidelines. A few observations:

**Good:**
- Uses contractions ("we'd need", "it's", "doesn't")
- Varied sentence length
- Concrete examples (sqlx-cli, probe-rs-tools)
- No "comprehensive", "robust", "leverage", etc.

**Minor issues:**
- Line 94: "we'd be reimplementing part of Cargo's workspace resolution" -- slightly hand-wavy. Could say "we'd need to handle path dependencies, virtual manifests, and nested workspaces, which is reimplementing Cargo's workspace resolver."
- Line 77: "The availability varies by registry" -- passive. Could just lead into the table directly.
- The document title "Deterministic Binary Name Discovery for Recipe Builders" uses "deterministic" which is accurate and not an AI tell, but the filename is `DESIGN-deterministic-recipe-repair.md` -- "repair" is misleading since this is about prevention, not repair. The document itself is about improving discovery accuracy, not repairing broken recipes.

---

## 7. Technical Accuracy

### Mostly accurate, with caveats:

1. **crates.io API claim** -- Needs live verification (see 4a above). If the `bin_names` field is confirmed, the technical approach is sound.

2. **npm `parseBinField()` fix** -- Technically correct. The current code at npm.go:263 does return nil for string-type bin. The fix is to return `[]string{packageName}`. One edge case: scoped packages (see 4c).

3. **Current code references** -- The design accurately describes `discoverExecutables()`, `buildCargoTomlURL()`, `fetchCargoTomlExecutables()`, and the structs to remove. All verified against `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go`.

4. **PyPI/RubyGems characterization** -- Accurate. Both builders currently fetch source files from GitHub (pyproject.toml, gemspec) with the same monorepo vulnerability as Cargo.

5. **Go builder characterization** -- Accurate. Uses `inferGoExecutableName()` with the last-path-segment heuristic.

6. **Version struct needs updating** -- The design correctly identifies that `cratesIOCrateResponse.Versions` is `[]struct{}` and needs a `BinNames` field. Verified against cargo.go:37.

---

## 8. Scope Assessment

**Appropriate.** The design handles a real problem with a targeted fix. Phase 1 (crates.io + npm) has clear, small scope. Phase 2 (orchestrator validation) adds the safety net. Phase 3 (PyPI + RubyGems) is explicitly deferred.

The only scope concern is whether the orchestrator validation step (Decision 2) is premature. If the crates.io and npm fixes resolve the known failures, the orchestrator layer adds complexity without immediate benefit. But the design argues it's a safety net for future builders, which is reasonable given the pattern will be needed when PyPI and RubyGems are added.

---

## Actionable Recommendations

### Must-fix before implementation

1. **Verify `bin_names` field with a live API call.** This is the foundation of the crates.io fix. The explore summary contradicts itself on whether this field exists. A single `curl https://crates.io/api/v1/crates/sqlx-cli | jq '.versions[0].bin_names'` settles it.

2. **Specify which version's `bin_names` to use.** The API returns all versions. The design should state: "use the latest version (first element in the versions array, which crates.io sorts by recency)."

3. **Handle scoped npm packages.** When `bin` is a string and the package is `@scope/tool`, the executable name is `tool`, not `@scope/tool`. The `parseBinField()` fix needs the package name passed in to strip scope.

### Should-fix for clarity

4. **Rename the file** from `DESIGN-deterministic-recipe-repair.md` to something like `DESIGN-binary-name-discovery.md`. This design is about improving discovery, not repairing recipes.

5. **Address the builder state lifecycle** for `BinaryNameProvider`. Where does the builder store the `bin_names` data between `Build()` and the orchestrator's call to `AuthoritativeBinaryNames()`? A one-line note about the builder caching the API response is sufficient.

6. **Clean up the explore summary contradiction** at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/wip/explore_summary.md` lines 14 vs 20.

### Nice-to-have

7. **Quantify the wasted sandbox cost** in the problem statement. "X recipes in registry Y have wrong binaries, each wasting Z seconds of container time."

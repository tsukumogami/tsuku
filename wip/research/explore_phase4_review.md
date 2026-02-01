# Design Review: Discovery Registry Bootstrap

**Document:** DESIGN-discovery-registry-bootstrap.md
**Reviewer:** Phase 4 & 8 Analysis
**Date:** 2026-02-01

## Executive Summary

This design is **ready for implementation** with minor clarifications. The identity-vs-install separation is clearly motivated and the schema evolution is well-reasoned. Architecture is implementable with sufficient detail. Security analysis is solid for supply chain risks. Main gaps: edge cases in validation sequencing, missing network failure handling, and unaddressed GitHub API abuse scenarios.

**Key Strengths:**
- Clear problem framing: identity metadata vs. install instructions
- Pragmatic schema evolution (required install fields now, optional metadata enables future)
- Well-sequenced implementation phases
- Comprehensive supply chain risk analysis with specific mitigations

**Key Gaps:**
- Missing alternatives for data sourcing (Repology aggregator)
- Validation sequencing assumptions (API call ordering affects error messages)
- No network failure/retry strategy for enrichment
- GitHub API abuse vectors (malicious redirects, rate limit exhaustion)

---

## Phase 4: Problem & Options Analysis

### 1. Problem Statement Specificity

**Assessment:** The problem statement is **specific and well-articulated**.

The design clearly distinguishes:
- **Identity:** What a tool is (repo, homepage, description) — facts about the tool's existence
- **Install path:** How to install it (builder, source) — facts about obtaining binaries

The three reasons given (resolver needs, future inference, UX improvement) justify why this distinction matters. The "bootstrap challenge" is explicitly stated: populate ~500 entries with both types of data.

**Minor gap:** The problem statement doesn't quantify the cost of *not* separating identity from install. For example, what happens when a tool switches from Homebrew bottles to GitHub releases? With the current flat schema, every field must change. With separated identity, only the install path changes while metadata persists. This consequence is implied but not stated.

**Specific evidence:**
- Line 32-42: "The current schema conflates two different things: what a tool *is* (its identity) and how to *install* it... A tool like `jq` is maintained on GitHub at `jqlang/jq`, documented at `https://jqlang.github.io/jq/`, and installable via Homebrew bottles. These are separate facts."
- Line 35-42: Three concrete reasons given (today's resolver needs, future builders could infer, richer data improves disambiguation)

**Recommendation:** Add a sentence to Problem Statement: "When a tool's install method changes (e.g., Homebrew to GitHub releases), the current schema requires rewriting the entire entry. Separating identity from install path makes these transitions cheaper — only the install fields change."

### 2. Missing Alternatives for Schema Decision

**Assessment:** Schema alternatives are **adequate but could be expanded**.

The design considers:
1. **Chosen:** Required install + optional metadata (flat)
2. Nested structure (`install: {...}`)
3. Optional install fields now

**Missing alternative 1: Union type with discriminator**

Instead of always-present `builder`+`source`, use a discriminated union:

```json
{
  "type": "installable",
  "install": {"builder": "github", "source": "..."},
  "metadata": {...}
}
// vs
{
  "type": "metadata-only",
  "metadata": {...}
}
```

This makes the "install info optional" concept explicit without nil-handling complexity. The resolver filters for `type: "installable"`. When the LLM builder arrives, it consumes `type: "metadata-only"` entries.

**Why this matters:** The chosen approach says "all entries must be installable now." The union approach says "entries have different roles." This is a **semantic** difference — it makes the future evolution explicit in the schema.

**Verdict:** Not a fatal omission. The chosen approach is simpler and backward-compatible. But the union type is worth a rejection paragraph if the authors considered it.

**Missing alternative 2: Registry aggregator (Repology)**

The design proposes curated seed lists validated via GitHub API. An alternative data source: Repology (https://repology.org/), which aggregates package metadata from 500+ repositories including Homebrew, GitHub releases, distro packages, etc.

**How it would work:**
- Query Repology API for top 500 CLI tools by popularity
- Repology provides: tool name, repo URL, homepage, description, version info across ecosystems
- Tool validates the repo exists, enriches with GitHub stars, detects collisions from Repology's multi-ecosystem data

**Pros:**
- Automated population from a single API (no manual seed list curation)
- Cross-ecosystem collision data already computed by Repology
- Ongoing updates as Repology tracks new tools

**Cons:**
- Repology data quality varies (some entries have stale URLs)
- API rate limits may be restrictive
- Doesn't solve builder selection (still need human judgment for `homebrew` vs `github`)
- Adds external dependency on Repology infrastructure

**Why this is missing:** The design focuses on "curated seed lists" as the only input method. Repology is a legitimate alternative that trades manual curation for automated aggregation.

**Verdict:** Worth rejecting explicitly. Add to Decision 2 alternatives: "Repology aggregator — Rejected because builder selection (homebrew vs github) requires human judgment that Repology doesn't provide. Useful as a validation cross-check but not primary source."

### 3. Rejection Rationale Fairness

**Assessment:** Rejection rationales are **fair and well-justified**.

**Well-justified rejections:**

1. **Nested structure rejected** (line 111): "Migration cost high for cosmetic difference." Fair — the design acknowledges this is purely structural, not functional.

2. **Optional install fields rejected** (line 113): "Premature — LLM builder doesn't exist." Fair — YAGNI principle applied correctly.

**Potential unfairness:**

**LLM-assisted bulk generation** (line 133): "Fast but unreliable — hallucinated repo names pass no validation, and the cost of fixing errors exceeds manual curation."

**Issue:** The rationale says "hallucinated repo names pass no validation" but the design proposes GitHub API validation (line 337-341) that would *catch* hallucinations. An LLM could generate 1000 entries, the validation tool filters to 500 valid ones, and manual review checks for typosquats. This hybrid approach isn't considered.

**Counter-argument:** Even with validation, LLM errors require manual review to distinguish hallucinations from legitimate API failures (e.g., is `kubernetes/kubernets` a typo or a real repo?). This negates the speed advantage.

**Verdict:** The rejection is fair but could be strengthened. **Recommendation:** Add: "Even with validation, LLM errors require manual review to distinguish hallucinations from legitimate API failures, negating the speed advantage."

**Scrape awesome-lists** (line 136): "Good coverage but noisy — many entries are libraries, and URLs go stale. Rejected as primary source but useful as input when building seed lists."

**Issue:** The rejection softens to "useful as input" which contradicts "rejected." This is actually a *partial acceptance* — awesome-lists are a data source for seed list curation.

**Verdict:** Fair and honest. The design acknowledges a middle ground.

### 4. Unstated Assumptions

**Identified assumptions:**

**Assumption 1: Builder semantics won't change**

The schema assumes `builder: "github"` has stable meaning. If the github builder splits into `github-release` and `github-tarball`, entries must be rewritten. No migration strategy mentioned.

**Evidence:** Line 248 defines builder values as strings (`"github"`, `"homebrew"`, `"cargo"`, etc.) with no versioning or deprecation plan.

**Impact:** Medium. Builder evolution is likely (e.g., adding `nix`, `docker`, `flatpak`). The design doesn't say how to handle builder deprecation or renaming.

**Recommendation:** Add to "Future Evolution" (line 432): "Builder namespace evolution: If builders split or merge (e.g., `github` → `github-release` and `github-source`), schema v3 can add a `builder_version` field. Entries without this field default to v1 semantics."

**Assumption 2: Metadata fields are sufficient**

The schema defines `description`, `homepage`, `repo`. What if we need `license`, `language`, `category`, `tags` later? Adding fields is easy (schema is extensible), but there's no discussion of "what metadata is enough."

**Evidence:** Line 251-256 lists optional fields. No rationale for *why these specific fields* were chosen.

**Impact:** Low. The design says metadata is optional, so adding fields is non-breaking. But it doesn't justify the current set.

**Recommendation:** Add to Decision 1 rationale (line 80-107): "Metadata fields chosen based on immediate consumer needs: `description` for search, `homepage` for info display, `repo` for verification. Additional fields (license, language, tags) can be added in future schema versions without breaking changes."

**Assumption 3: Name uniqueness within discovery.json**

The design says seed lists are "deduplicated by name" (line 333) but doesn't specify tie-breaking. If two seed lists both define `kubectl` with different builders, which wins?

**Evidence:** Line 333 says "Merge all entries (deduplicate by name)" without explaining how.

**Impact:** High. This is a **concrete gap**. The processing pipeline is underspecified.

**Recommendation:** Add to "Processing Pipeline" (line 329): "Deduplication strategy: last seed file wins per tool. If multiple seed files define the same tool, later files override earlier files. Flag fields (`disambiguation`) are OR'd across all definitions."

**Assumption 4: Ecosystem API availability**

Collision detection queries npm, crates.io, PyPI, RubyGems (line 348). Assumes these APIs are stable and rate-limit-friendly. What if crates.io is down during generation?

**Evidence:** Line 348 mentions ecosystem queries but no fallback strategy.

**Impact:** Medium. No fallback strategy for unavailable APIs.

**Recommendation:** Add to "Processing Pipeline" error handling: "Ecosystem API failures: If an ecosystem API is unavailable (timeout, 5xx error), skip collision detection for that ecosystem and log a warning. Entry is still valid but may have undetected collisions."

**Assumption 5: Validation ordering is flexible**

The pipeline (line 337-350) says "1. Validate, 2. Enrich, 3. Cross-reference, 4. Collision detect." But enrichment happens *after* validation. If validation fails, do we skip enrichment?

**Evidence:** Line 337-345 specifies ordering but not failure behavior.

**Impact:** Medium. This affects error reporting. A failed entry with enriched metadata provides better debugging than a bare failure.

**Example:** If `kubernetes/kubernets` (typo) fails validation, does the tool report "github:kubernetes/kubernets not found" or does it enrich first and say "Repository kubernetes/kubernets (similar to kubernetes/kubernetes) not found"? The latter is better UX but requires enrichment-before-validation or enrichment-despite-failure.

**Recommendation:** Add to "Processing Pipeline": "Validation runs first. If validation fails, enrichment is skipped (no API calls for invalid entries). Failed entries logged with reason but no metadata."

---

## Phase 8: Architecture Analysis

### 5. Architecture Clarity for Implementation

**Assessment:** Architecture is **clear enough to implement** with gaps in edge cases and error handling.

**Well-specified components:**

1. **Schema v2 structure** (line 86-104): Exact JSON format given with concrete examples
2. **Go struct changes** (line 272-281): Fields and tags specified
3. **CLI interface** (line 362-371): Flags and defaults defined
4. **Processing pipeline** (line 329-358): Step-by-step flow

**Implementation-ready details:**

- Builder-specific validation (line 337-341): Clear per-builder checks (GitHub: repo exists, not archived, has releases; Homebrew: formula exists; ecosystems: package exists)
- Enrichment sources (line 342-345): What data comes from which API (GitHub API → description, homepage, stars; Homebrew API → description, homepage)
- Output handling (line 351-357): Valid/skipped/failed/collision cases enumerated

**Gaps requiring clarification:**

**Gap 1: Merge strategy underspecified**

Line 333 says "Merge all entries (deduplicate by name)" but doesn't explain how. Needs: last-wins? First-wins? Error on conflict? Per-field merge (seed A provides builder, seed B provides description)?

**Concrete impact:** If `data/discovery-seeds/dev-tools.json` has:
```json
{"name": "bat", "builder": "github", "source": "sharkdp/bat"}
```
And `data/discovery-seeds/disambiguations.json` has:
```json
{"name": "bat", "builder": "github", "source": "sharkdp/bat", "disambiguation": true}
```

Which entry wins? Probably want the disambiguation flag to merge in, not overwrite.

**Recommendation:** Add "Processing Details" subsection after line 358:
```markdown
### Merge Strategy

When multiple seed files define the same tool:
- Last seed file wins for scalar fields (builder, source, binary, description, etc.)
- Boolean flags (disambiguation) are OR'd across all definitions
- If any definition sets `disambiguation: true`, final entry has `disambiguation: true`
```

**Gap 2: Retry/caching for API calls**

500 entries × 3 API calls = 1500 requests. What happens if request #347 fails (network timeout)? Does the tool retry? Cache successful responses and resume? Start over?

**Concrete impact:** Without retry, a single transient failure wastes 346 successful API calls. Without caching, re-running the tool after fixing one entry re-fetches all 500.

**Recommendation:** Add "Error Handling" subsection after processing pipeline:
```markdown
### Error Handling and Resilience

**Network failures:**
- HTTP client retries: 3 attempts with exponential backoff (1s, 2s, 4s)
- Timeout: 30 seconds per request
- Final failure: Log entry with reason, continue to next entry

**Resume support:**
- Successful API responses cached to `.cache/seed-discovery/<name>.json`
- On re-run, check cache before querying API
- Cache TTL: 24 hours (stale cache invalidated)

**Rate limiting:**
- GitHub API: 1 request/second (3600/hour, well under 5000 authenticated limit)
- Ecosystem APIs: 2 requests/second per ecosystem
- On rate limit error: Pause for 60 minutes, resume from cache
```

**Gap 3: Recipe cross-reference mechanism**

"Recipe already exists in recipes/?" (line 346) — how is this checked? Glob for `recipes/<name>.toml`? Parse recipe TOML and check `name` field? Case-sensitive match?

**Concrete impact:** If recipe is `kubectl.toml` but discovery entry is `kubectL`, does it match? Unicode normalization? (The resolver has normalize.go but is that used here?)

**Recommendation:** Add to line 346:
```
3. Cross-reference: Check if recipes/{name}.toml exists (case-insensitive)
   - Apply same normalization as resolver (NFD + lowercase + ASCII transliteration)
   - If normalized names match, skip entry (unless disambiguation: true)
```

**Gap 4: Validation criteria ambiguity**

Line 337-341 says "has release in last 24 months with binary assets" but doesn't specify:
- Does the latest release need binary assets, or any release in 24 months?
- What constitutes a "binary asset"? `.tar.gz`, `.zip`, `.exe`, `.dmg`?
- What if the latest release is a pre-release?

**Recommendation:** Add "Validation Criteria" subsection:
```markdown
### GitHub API Validation Criteria

For each GitHub entry:
1. **Repo exists:** GET /repos/{owner}/{repo} returns 200
2. **Not archived:** `archived: false` in repo metadata
3. **Has releases:** Latest non-prerelease release is within 24 months of today
4. **Binary assets:** Latest release has ≥1 asset matching patterns:
   - `*.tar.gz`, `*.tgz`, `*.zip`, `*.tar.xz`
   - Excludes source archives: `*-source.*`, `*src.*`

Entries failing any check are logged and excluded from output.
```

### 6. Missing Components

**Assessment:** Core components are present. Missing: observability, contributor workflow, data quality feedback loop.

**Present components:**
- Seed lists (input) — line 307-323
- `cmd/seed-discovery` (processor) — line 299
- `discovery.json` (output) — line 301
- CI freshness (maintenance) — line 375-396

**Missing components:**

**Component 1: Validation report output**

The tool logs failures (line 355-357), but is there a structured report? CI freshness creates an issue on failure (line 396) — what does that issue contain? A list of stale entries? A diff of what changed?

**Why this matters:** Without structured output, debugging failures requires reading logs. With JSON/markdown reports, contributors can fix issues independently.

**Recommendation:** Add component: `wip/discovery-report.md` (generated by tool, lists collisions/typosquats/stale entries for review). Add CLI flag: `--report-file` to write structured validation report.

**Component 2: Contributor documentation**

Scope says "Contributor workflow for adding entries" (line 53) but there's no component for this. Where do contributors learn how to add an entry? What file do they edit? How do they test locally?

**Why this matters:** External contributors need guidance. Without it, PRs will be malformed.

**Recommendation:** Add component: `docs/CONTRIBUTING-discovery.md` (contributor guide for adding seed entries).

**Component 3: Data quality dashboard**

The design mentions "collision detection surfaces both sides with metadata" (line 348) but doesn't say where this surfaces. Logged to console? Written to a file? Sent to an issue?

**Why this matters:** Human review of collisions requires seeing the data. If it's only in console logs, reviewers must re-run the tool.

**Recommendation:** Enhance `--report-file` to include collision section with both sides of each conflict (npm package info vs GitHub repo info).

### 7. Implementation Phase Sequencing

**Assessment:** Phases are **correctly sequenced** with clear dependencies.

**Phase 1:** Schema + basic tool (50-100 entries)
- **Dependencies:** None. This is the foundation.
- **Correctness:** Yes. Schema must exist before tool can output v2 format.
- **Files:** Lines 445-451 list all affected files

**Phase 2:** Scale to 500 entries
- **Dependencies:** Phase 1 complete. Tool must work end-to-end before scaling.
- **Correctness:** Yes. Scaling after validation reduces wasted effort on malformed seeds.
- **Files:** Lines 455-460

**Phase 3:** CI freshness
- **Dependencies:** Phase 2 complete. No point validating an empty/small registry.
- **Correctness:** Yes. CI checks the production registry, not the dev registry.
- **Files:** Lines 464-467

**Potential issue:** Phase 1 "Start with 50-100 entries to validate end-to-end" (line 443) — but the design doesn't say *which* 50-100. If we start with obscure tools that have API quirks, we'll hit edge cases early. If we start with popular tools (ripgrep, jq, kubectl), we validate the happy path but miss edge cases.

**Recommendation:** Add to Phase 1 (line 443): "Initial 50-100 entries: mix of high-popularity tools (ripgrep, fd, bat, jq, gh) and edge cases (tools where binary!=name like kubectl, homebrew-only tools like jq, disambiguation overrides like bat) to exercise all validation code paths."

**Sequencing correctness verified:**
- Schema must exist before tool reads it ✓
- Tool must work before scaling to 500 ✓
- CI validation makes sense only after production data exists ✓

No circular dependencies detected.

---

## Phase 8: Security Analysis

### 8. Unconsidered Attack Vectors

**Assessment:** Supply chain risks are **well-covered** (lines 481-515). Network/API attacks are **underexplored**.

**Covered attack vectors:**

1. **Registry poisoning via seed PRs** (line 483): CODEOWNERS review, typosquatting detection (line 487)
2. **Stale entries to transferred repos** (line 490): Weekly freshness, ownership check
3. **Wrong disambiguation** (line 493): Collision detection + review
4. **Metadata poisoning** (line 499): Metadata is informational only, install fields from curated seed list

**Unconsidered attack vectors:**

**Attack 1: GitHub API response manipulation**

The tool fetches repo metadata from the GitHub API. An attacker with a GitHub account creates `evil/ripgrep`, adds a description like "Fast line-oriented search tool (OFFICIAL)", and submits a seed PR. The tool validates that `evil/ripgrep` exists, enriches with the malicious description, and outputs it.

**Why current mitigations don't cover this:** The design says "Reviewer checklist: verify source matches official tool" (line 485). But how does a reviewer know what's official? They'd have to Google it. For 500 tools, that's error-prone.

**Proposed mitigation:** Seed lists require a `verification_url` field (e.g., `https://github.com/BurntSushi/ripgrep` or official project homepage). The tool compares enriched `repo` field to `verification_url`. Mismatch fails validation. This forces attackers to compromise the verification URL, not just the seed PR.

**Attack 2: GitHub API redirect attacks**

GitHub allows repo renames. If `BurntSushi/ripgrep` becomes `BurntSushi/rg`, the old URL redirects. The tool fetches `BurntSushi/ripgrep`, gets redirected to `BurntSushi/rg`, and enriches with data from the *new* repo. If the old repo name is claimed by an attacker, the redirect could point to a malicious repo.

**Why current mitigations don't cover this:** Weekly freshness checks ownership (line 490), but doesn't detect redirects during initial population.

**Proposed mitigation:** Tool compares the requested source (`BurntSushi/ripgrep`) to the API response's canonical name (from the `full_name` field). If they differ, log a warning and require manual review. Add to security mitigations table:

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| GitHub redirect to malicious repo | Compare requested source to API `full_name`, flag mismatches | Attacker claims old name after rename (detectable in PR review) |

**Attack 3: Rate limit exhaustion DoS**

A malicious contributor submits a PR with 10,000 entries. CI runs `seed-discovery --validate-only`, which makes 30,000 API calls and exhausts the GitHub token's rate limit. Legitimate CI runs fail for the next hour.

**Why current mitigations don't cover this:** No mention of rate limit protection or entry count limits.

**Proposed mitigation:** Tool enforces a max entry limit (e.g., 1000). PRs adding >100 entries at once require CODEOWNERS approval. CI uses a dedicated token with isolated rate limits. Add to security section:

**Rate limit DoS prevention:**
- Tool rejects seed lists with >1000 total entries
- PRs adding >100 entries trigger manual review
- CI uses dedicated GitHub token (not shared with other workflows)

**Attack 4: Typosquatting via Unicode homoglyphs**

Seed PR adds `ripgrep` (with Cyrillic 'р' U+0440) pointing to a malicious repo. Levenshtein distance is 0 because it's the *same* string in Unicode. The tool validates, enriches, and outputs it. The discovery registry now has two entries for visually identical names.

**Why current mitigations don't cover this:** Levenshtein distance (line 487) operates on Unicode code points, not visual similarity. The design says "handled by resolver's normalize.go" (line 515) but doesn't explain how normalization prevents this during seed list processing.

**Proposed mitigation:** Tool applies the same normalization as the resolver (NFD + case-fold + ASCII transliteration from `internal/normalize.go`) *before* deduplication. If normalized names collide, reject the entry. Clarify in security section (line 515):

**Typosquatting:** Unicode normalization (NFD + case-fold + ASCII transliteration from `internal/normalize.go`) is applied before deduplication. Homoglyph attacks (e.g., Cyrillic 'р' in `ripgrep`) are detected as duplicates. Levenshtein distance check flags ASCII typosquats (e.g., `ripgrep` vs `ripgerp`).

**Attack 5: Ecosystem API hijacking**

The tool queries npm, crates.io, PyPI, RubyGems for collision detection (line 348). An attacker could run a fake registry at `crates.io` (via DNS poisoning or typo domain like `crates-io.com`) and return false collision data. The tool thinks `ripgrep` exists on crates.io when it doesn't, logs it as a collision, and a reviewer marks it for disambiguation when it shouldn't be.

**Why current mitigations don't cover this:** No mention of ecosystem API verification (certificate pinning, hardcoded URLs).

**Proposed mitigation:** Tool uses HTTPS with certificate validation. Ecosystem API URLs are hardcoded (not configurable). Add to security section (line 505):

**Ecosystem API security:** All ecosystem APIs (npm, crates.io, PyPI, RubyGems) are accessed over HTTPS with certificate validation. URLs are hardcoded in source code (not user-configurable) to prevent typo domain attacks. MITM attacks blocked by TLS certificate validation.

### 9. Sufficiency of Mitigations

**Assessment:** Mitigations are **strong for supply chain risks**, **adequate for data integrity**, **weak for operational security**.

**Strong mitigations:**

**Registry poisoning** (line 483-487): CODEOWNERS + automated validation + typosquatting detection. Residual risk is "sophisticated attack with legitimate-looking source" — fair. A determined attacker could create a clone of `kubernetes/kubernetes` with identical commit history and submit it. But that's detectable via verification URL cross-check (if added as recommended above).

**Stale entries** (line 490-492): Weekly freshness with ownership check. Residual risk is "7-day window" — acceptable for most tools. Critical tools (kubectl, docker, gh) could have daily checks if needed.

**Adequate mitigations:**

**Wrong disambiguation** (line 493-497): Collision detection + human review + documented rationale. Residual risk is "obscure collisions not in seed lists" — true, but those are edge cases discoverable via user reports.

**Metadata poisoning** (line 499-501): "Metadata is informational only; install fields from curated seed list." Correct — misleading descriptions don't affect install behavior. But they *do* affect `tsuku search` and `tsuku info` output. A user searching for "JSON processor" could get a malicious tool with a fake description. This is mitigated by install instructions being separate (the wrong tool won't install), but it's still a UX attack vector.

**Verdict:** Acceptable. The impact is limited to search/info UX, not actual malicious installs.

**Weak mitigations:**

**Rate limit DoS:** Not addressed. No entry limit, no rate limit handling.

**GitHub redirects:** Not addressed. Tool could follow a redirect to a malicious repo.

**Unicode homoglyphs:** Claimed to be handled by normalize.go (line 515) but not explained how seed list processing prevents duplicates.

**Recommendations:**

1. Add to Security Considerations (after line 501):

```markdown
### API Abuse Prevention

**Rate limit DoS:**
- Tool enforces max 1000 entries per generation
- PRs adding >100 entries require CODEOWNERS approval
- CI uses dedicated GitHub token with isolated rate limits
- Residual risk: Large malicious PR could block CI for ~10 minutes before timeout

**GitHub redirects:**
- Tool compares requested source to API canonical name (`full_name` field)
- Mismatch logged as warning, requires manual review
- Residual risk: Attacker claims old name after rename (detectable in PR review)
```

2. Add to Mitigations table (line 509-515):

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Rate limit DoS | Max 1000 entry limit, isolated CI token | Large PR blocks CI ~10 min |
| GitHub redirect attack | Compare source to canonical name | Attacker claims old name post-rename |
| Unicode homoglyphs | NFD normalization before dedup | Novel Unicode attacks (caught by resolver normalize.go) |

3. Clarify Unicode handling (line 515):

**Typosquatting:** Levenshtein distance check flags ASCII typosquats (e.g., `ripgrep` vs `ripgerp`). Unicode homoglyphs (e.g., Cyrillic 'р' in `ripgrep`) detected by applying NFD normalization + case-fold + ASCII transliteration before deduplication. Same normalization as resolver (`internal/normalize.go`).

---

## Additional Observations

### Positive Aspects

1. **Reuse of batch pipeline patterns** (line 421-429): The design explicitly compares to the priority queue and identifies shared patterns (GitHub API client, JSON I/O, recipe cross-reference). This makes implementation faster — the team can copy proven code.

2. **Future evolution is concrete** (line 432-437): Schema v3 is described with a clear trigger ("when LLM builder exists"). This prevents premature abstraction while keeping the roadmap visible.

3. **Scope boundaries are crisp** (line 55-59): The "Out of scope" section prevents feature creep. Recipe format changes, LLM builder implementation, and resolver architecture are explicitly excluded.

4. **Trade-offs are honestly stated** (line 195-199): The design acknowledges "Manual seed list curation requires effort" and "builder+source still required limits future flexibility." No overselling.

### Potential Issues

**Issue 1: Seed list maintenance burden**

The design says "incremental additions afterward" (line 198) but doesn't explain how contributors discover *what* to add. Is there a backlog of "wanted tools"? A priority system? Or do contributors add whatever they personally need?

**Impact:** Without guidance, seed list growth is chaotic. Popular tools get added by whoever needs them first, niche tools never get added.

**Recommendation:** Add to Consequences (line 517-540):

```markdown
### Seed List Maintenance

- Update cadence: Quarterly review of Homebrew analytics top-500
- Contributor additions: Reactive (user requests via issues) rather than proactive
- No prioritization system for new entries (tools added as needed)
- Consider linking to priority queue data for proactive population of high-value tools
```

**Issue 2: Homebrew builder overuse**

The builder selection guidance (line 259-265) says "Tool has Homebrew bottles or requires complex build dependencies." But it doesn't warn against over-relying on Homebrew. If every tool defaults to `builder: homebrew`, the discovery registry becomes macOS-only. The design doesn't address cross-platform concerns.

**Impact:** Linux/Windows users can't use discovery-resolved tools if they're all Homebrew.

**Recommendation:** Add to Decision Drivers (line 61-70):

```markdown
- **Cross-platform support:** Prefer builders that work on all platforms (github, cargo, npm, pypi, gem, go) over platform-specific builders (homebrew, cask) when both options are viable
```

And update builder selection guidance (line 259-265):

```markdown
**Builder selection guidance:**

Prefer `github` builder for cross-platform tools with binary releases. Use `homebrew` only when:
- Tool requires complex build dependencies that Homebrew handles
- Tool doesn't publish GitHub release binaries
- Tool is macOS-specific (e.g., system utilities)
```

**Issue 3: Validation vs. enrichment cost asymmetry**

The design says "Enrichment is best-effort — missing metadata doesn't fail the entry" (line 144). But validation is strict (repo must exist, not archived, has release in 24 months). This asymmetry could cause confusion: "My entry validated but has no description. Is that okay?" Yes, but it's not obvious.

**Impact:** Contributors might think missing metadata is a bug.

**Recommendation:** Add to CLI output examples (in implementation phase):

```
Entry ripgrep: VALID (enriched: description, homepage, stars)
Entry obscure-tool: VALID (no metadata available)
Entry invalid-tool: FAILED (repo not found)
```

---

## Summary

**Overall assessment:** This design is **implementation-ready** with minor clarifications. The identity-vs-install separation is well-justified, schema evolution is pragmatic, and implementation phases are correctly sequenced. Security analysis covers the main supply chain risks but misses some API abuse vectors.

**Critical gaps to address before implementation:**

1. **Merge strategy** for duplicate entries across seed lists (last-wins with OR'd flags)
2. **API retry/caching strategy** for 500-entry generation (3 retries, 24-hour cache)
3. **GitHub API redirect detection** (compare source to canonical name)
4. **Rate limit protection** (max 1000 entry limit, isolated CI token)
5. **Validation criteria precision** (define "binary asset" patterns, pre-release handling)

**Nice-to-haves for better UX:**

1. Contributor documentation (`docs/CONTRIBUTING-discovery.md`)
2. Structured validation report (`--report-file` flag)
3. Cross-platform builder preference in decision drivers
4. Verification URL field in seed lists for authenticity checks
5. Phase 1 entry selection guidance (mix of popular + edge cases)

**Strengths to preserve:**

- Clear separation of concerns (identity vs. install)
- Backward-compatible schema evolution
- Honest trade-off discussion
- Well-sequenced implementation phases
- Reuse of proven batch pipeline patterns

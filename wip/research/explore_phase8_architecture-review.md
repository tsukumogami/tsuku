# Architecture Review: Discovery Registry Bootstrap

**Design Document:** `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/docs/designs/DESIGN-discovery-registry-bootstrap.md`

**Reviewer Context:**
- Existing registry code: `internal/discover/registry.go`
- Existing seed-queue tool: `cmd/seed-queue/main.go`
- Batch pipeline orchestrator: `internal/batch/orchestrator.go`
- Current discovery.json: 1 entry (jq with homebrew builder)

---

## Executive Summary

The design is **ready for implementation** with minor clarifications needed on collision detection mechanics. The architecture is well-structured, properly sequenced into implementable phases, and correctly reuses existing patterns from the seed-queue tool and batch pipeline. However, there are opportunities to simplify the collision detection approach and clarify the relationship between discovery.json entries and the batch pipeline lifecycle.

**Recommendation:** Proceed with implementation after addressing the questions below about collision detection mechanics.

---

## 1. Is the Architecture Clear Enough to Implement?

**Answer: Yes, with minor clarifications needed.**

### What's Clear

The design provides sufficient detail for implementation:

1. **Component structure** is well-defined:
   - Seed lists location: `data/discovery-seeds/*.json`
   - Tool location: `cmd/seed-discovery/main.go`
   - Output: `recipes/discovery.json`
   - CI workflow: `.github/workflows/discovery-freshness.yml`

2. **Data formats** are specified:
   - Seed list JSON schema (lines 204-218)
   - Output format matches existing `internal/discover/registry.go` (lines 269-277)
   - Schema validation exists in `DiscoveryRegistry.validateEntries()`

3. **Processing pipeline** is documented (lines 242-263):
   - Read seed lists
   - Merge and deduplicate
   - Validate via GitHub API
   - Cross-reference recipes and priority queue
   - Detect ecosystem collisions
   - Output validated discovery.json

4. **Reuse patterns** are identified:
   - JSON load/save from `internal/seed/queue.go`
   - HTTP client with retry (mentioned, pattern exists in homebrew.go)
   - Merge with deduplication (exists in `queue.Merge()`)

### What Needs Clarification

**Collision Detection Mechanics:**

The design states (line 131): "The seed-discovery tool queries all ecosystem builders' `CanBuild()` (or equivalent API check) for each entry."

However, looking at the existing builder interface (`internal/builders/builder.go`):

```go
CanBuild(ctx context.Context, req BuildRequest) (bool, error)
```

The `CanBuild()` method requires a `BuildRequest` which contains `Package`, `Version`, and `SourceArg`. For most ecosystem builders, `CanBuild()` likely makes an API call to check if the package exists (e.g., npm registry lookup, crates.io API check).

**Questions:**
1. Should the tool call `CanBuild()` for each ecosystem builder with the tool name?
2. Should it use a simpler API check (like direct HTTP GET to ecosystem APIs)?
3. How should the tool distinguish between "package exists" and "this is the package users expect"?

**Example scenario:** The tool is validating a seed list entry for `bat` (sharkdp/bat). The collision detection would:
- Check npm registry: `bat` exists (it's a testing framework)
- Check crates.io: `bat` exists (it's sharkdp/bat)
- Flag as collision
- Human reviews and marks `bat: {builder: github, source: sharkdp/bat, disambiguation: true}`

This makes sense, but the design should clarify whether `CanBuild()` is the right method or if direct API checks are more appropriate for this use case.

**Recommendation:** Add a section clarifying collision detection implementation. Options:
- **Option A:** Implement lightweight API checks directly in `cmd/seed-discovery` (HTTP GET to npm, crates.io, PyPI, etc.)
- **Option B:** Reuse `CanBuild()` but acknowledge this couples seed-discovery to the builders package
- **Option C:** Create a new `builders.CheckExistence()` helper that does lightweight checks without full `CanBuild()` logic

### What's Implementable Now

Everything except collision detection can be implemented immediately:
1. Seed list format and JSON parsing
2. GitHub API validation (repo exists, not archived, has releases)
3. Cross-referencing against recipes/ and priority queue
4. Merge and deduplication logic
5. Output generation to discovery.json
6. CI freshness workflow

The collision detection can be deferred to Phase 2 (line 359) since Phase 1 starts with 50-100 entries from "most obvious sources" where collisions are likely already known.

---

## 2. Are There Missing Components or Interfaces?

**Answer: No critical gaps, but three minor additions would strengthen the design.**

### Missing: GitHub API Client Abstraction

The design mentions "reuses patterns from the existing cmd/seed-queue and internal/seed packages: HTTP client with retry" (line 161), but there's no GitHub API client in the existing `internal/seed` package.

Looking at `internal/seed/homebrew.go` (not shown in context but referenced), it likely has HTTP client code for Homebrew analytics. The GitHub API validation logic will be new.

**Impact:** Low. The implementation will need to write GitHub API client code, but this is straightforward (GitHub API is well-documented, standard HTTP with JSON responses).

**Recommendation:** Add a helper package `internal/githubapi` or embed the client in `cmd/seed-discovery` directly. Document the API endpoints used:
- GET `/repos/:owner/:repo` - check repo exists and archived status
- GET `/repos/:owner/:repo/releases` - verify releases exist with binary assets

### Missing: Validation Rules for "Has Binary Assets"

The design says (line 252): "GitHub API: latest release has binary assets?"

What counts as a "binary asset"?
- Any file attachment?
- Only files matching common binary patterns (`.tar.gz`, `.zip`, `.exe`, `.deb`)?
- Source archives (e.g., `Source code (tar.gz)`) excluded?

**Impact:** Medium. Without clarity, the tool might accept repos that only have source releases, which the github builder can't use.

**Recommendation:** Define validation rules in the design:
- Binary assets are attachments matching patterns: `*.tar.gz`, `*.zip`, `*.deb`, `*.rpm`, `*.exe`, `*.dmg`, `*.pkg`
- Exclude GitHub's auto-generated source archives (they're named `{tag}.tar.gz` or `{tag}.zip` with no prefix)
- Require at least one binary asset in the latest release

### Missing: Seed List Contribution Workflow

The design mentions "contributor-friendly: add an entry to a seed list, run the tool, submit PR" (line 423), but doesn't specify the PR review process.

**Questions:**
- Do contributors run `seed-discovery` locally or does CI run it?
- How do reviewers verify the seed list entry is correct?
- Should there be a CONTRIBUTING.md section for discovery entries?

**Impact:** Low. This is a process question, not an architecture gap.

**Recommendation:** Add a "Contributing New Entries" section documenting:
1. Add entry to appropriate category file in `data/discovery-seeds/`
2. Run `go run ./cmd/seed-discovery` to regenerate `discovery.json`
3. Submit PR with both seed list change and discovery.json update
4. CI validates both files

### What's Well-Covered

The design correctly identifies all major components:
- Seed lists with clear schema
- Go CLI tool with flag interface
- CI freshness workflow
- Cross-referencing logic
- Output format matching existing registry schema

The relationship to existing code is well-documented (lines 331-346).

---

## 3. Are the Implementation Phases Correctly Sequenced?

**Answer: Yes, the phasing is sound.**

### Phase 1: Seed Lists and Go Tool (Lines 350-357)

**Scope:** Create seed list files, build `cmd/seed-discovery`, start with 50-100 entries.

**Assessment:** Correct sequencing. Starting with a small, known-good dataset validates the pipeline end-to-end before scaling. This is the same approach used by `cmd/seed-queue`.

**Dependencies:**
- No external dependencies (seed lists are curated files)
- Requires GitHub API token for validation
- Can be developed and tested locally

**Risk:** Low. The 50-100 entry count is conservative and should stay within GitHub API rate limits (5000/hour authenticated).

### Phase 2: Scale to 500 Entries (Lines 359-366)

**Scope:** Expand seed lists, run collision detection, add disambiguation entries.

**Assessment:** Correct sequencing. This phase assumes Phase 1 validated the pipeline works. The 500-entry target is explicitly from the parent design (DESIGN-discovery-resolver.md, line 22).

**Dependencies:**
- Phase 1 complete
- Collision detection implemented (either via CanBuild or direct API checks)
- Manual review capacity for disambiguation decisions

**Risk:** Medium. The collision detection is the most complex part of the tool. If it's implemented in Phase 1 (even without running it on all entries), Phase 2 becomes lower risk.

**Recommendation:** Move collision detection implementation to Phase 1, but only run it on the initial 50-100 entries. This validates the collision detection logic before scaling to 500.

### Phase 3: CI Freshness (Lines 368-373)

**Scope:** Add weekly GitHub Actions workflow for validation.

**Assessment:** Correct sequencing. CI should be added after the manual workflow is validated. This avoids having CI fail repeatedly during initial development.

**Dependencies:**
- Phase 2 complete (so discovery.json has substantial content worth validating)
- `seed-discovery --validate-only` flag implemented

**Risk:** Low. The workflow is straightforward (lines 284-304) and follows standard GitHub Actions patterns.

### Alternative Sequencing Considered

Could CI be added in Phase 1? Yes, but it would run against a small dataset (50-100 entries), which doesn't provide much value. The weekly validation is more useful when there are hundreds of entries that could go stale.

### What About Ongoing Maintenance?

The design doesn't explicitly phase "adding new entries after initial bootstrap," but this is implied in the contributor workflow (line 423). New entries can be added incrementally via PR after Phase 3.

**Recommendation:** Add a Phase 4 section or "Post-Bootstrap Workflow" clarifying:
- New entries added via PR to seed lists
- `seed-discovery` regenerates `discovery.json`
- CI validates on PR (not weekly, but per-PR for changes to seed lists)
- Weekly CI checks for staleness of existing entries

---

## 4. Are There Simpler Alternatives We Overlooked?

**Answer: Yes, there are two simplifications worth considering.**

### Simplification 1: Skip Collision Detection Entirely

**Current approach:** Automated collision detection flags tools that exist in multiple ecosystems, human reviews and adds disambiguation entries.

**Alternative:** Maintain disambiguation entries as a manually curated list based on user reports. When users file issues like "tsuku install bat gave me the wrong tool," add a disambiguation entry.

**Pros:**
- Much simpler implementation (no ecosystem API checks needed)
- Avoids coupling seed-discovery to the builders package
- Only adds disambiguations for tools users actually install (not all 500)

**Cons:**
- Reactive rather than proactive
- Poor initial user experience for colliding tools
- Manual curation misses obscure collisions

**Assessment:** The automated detection is worth the complexity. The parent design (DESIGN-discovery-resolver.md) explicitly calls out disambiguation as a key feature (lines 35-37 of the parent design). Starting with automated detection reduces user pain from day one.

**Recommendation:** Keep automated collision detection, but simplify the implementation (see Simplification 2).

### Simplification 2: Lightweight Collision Detection Without CanBuild

**Current approach (implied):** Call `builders.CanBuild()` for each ecosystem builder.

**Alternative:** Implement lightweight existence checks directly in `seed-discovery`:

```go
func checkCollisions(toolName string) []string {
    var ecosystems []string

    // npm
    if httpGet("https://registry.npmjs.org/"+toolName).StatusCode == 200 {
        ecosystems = append(ecosystems, "npm")
    }

    // crates.io
    if httpGet("https://crates.io/api/v1/crates/"+toolName).StatusCode == 200 {
        ecosystems = append(ecosystems, "cargo")
    }

    // PyPI
    if httpGet("https://pypi.org/pypi/"+toolName+"/json").StatusCode == 200 {
        ecosystems = append(ecosystems, "pypi")
    }

    // RubyGems
    if httpGet("https://rubygems.org/api/v1/gems/"+toolName+".json").StatusCode == 200 {
        ecosystems = append(ecosystems, "gem")
    }

    return ecosystems
}
```

**Pros:**
- No dependency on `internal/builders`
- Simple HTTP GET requests (all these registries have public JSON APIs)
- Fast (parallel requests can check all ecosystems in <1 second per tool)
- Easy to test

**Cons:**
- Duplicates ecosystem knowledge (URLs are also in builder code)
- Doesn't check Homebrew (Homebrew API is more complex)
- Doesn't check cpan, go ecosystem, etc.

**Assessment:** This is a good middle ground. The most common collisions are npm/cargo/pypi, which this approach covers. Homebrew collisions are less likely because Homebrew uses the tool name as-is (no aliasing).

**Recommendation:** Implement lightweight collision detection for npm, crates.io, PyPI, and RubyGems. Skip Homebrew, cpan, and go ecosystem checks (those can be added later if needed).

### Simplification 3: Merge cmd/seed-discovery into cmd/seed-queue

**Current approach:** Separate tools (`seed-queue` for priority queue, `seed-discovery` for discovery registry).

**Alternative:** Extend `cmd/seed-queue` with a `--mode=discovery` flag to output discovery.json instead of priority-queue.json.

**Pros:**
- Single tool for all seed data generation
- Shared validation logic
- Simpler maintenance

**Cons:**
- Conflates different concerns (priority queue is for batch generation, discovery is for runtime name resolution)
- Different validation requirements (priority queue checks Homebrew metadata, discovery checks GitHub releases)
- Different update cadence (priority queue updated as batch pipeline progresses, discovery updated as new tools are discovered)

**Assessment:** The design correctly rejects this (lines 99-100). The tools serve different purposes and have different lifecycles. Keeping them separate is the right call.

**Recommendation:** Maintain separation. Share utility code via `internal/seed` package (JSON I/O, HTTP client), but keep the tools distinct.

### What About Skipping CI Freshness?

The weekly CI validation (Phase 3) is marked as addressing "staleness risk identified in the parent design" (line 164).

**Alternative:** Skip automated freshness checks, rely on user reports of broken tools.

**Assessment:** Not recommended. The parent design explicitly identifies registry staleness as a security concern (line 108). Repos can be transferred to malicious owners, making automated verification necessary.

**Recommendation:** Keep the CI freshness workflow. It's low-cost (runs once per week) and addresses a real security risk.

---

## 5. Data Flow and Lifecycle Analysis

### Discovery Registry vs Priority Queue Relationship

The design states (lines 331-346):

| Aspect | Discovery Registry | Priority Queue |
|--------|-------------------|----------------|
| Purpose | Name resolution for `tsuku install` | Batch recipe generation |
| Builder | Primarily `github` | Primarily `homebrew` |

**Observation:** The current `discovery.json` has 1 entry using the `homebrew` builder:

```json
{
  "schema_version": 1,
  "tools": {
    "jq": {"builder": "homebrew", "source": "jq"}
  }
}
```

This contradicts the design's assumption that discovery entries are "Primarily `github`" (line 280).

**Question:** Should discovery.json support all builders, or just GitHub releases?

Looking at the parent design (DESIGN-discovery-resolver.md) and the existing `internal/discover/registry.go`, the registry supports any builder. The schema has `builder`, `source`, and optional `binary` fields.

**Clarification needed:** The design should acknowledge that:
1. Discovery entries can use any builder (github, homebrew, cargo, etc.)
2. The GitHub-release use case is the primary motivation (lines 33-37), but not the only valid entry type
3. Disambiguation entries might use non-GitHub builders (e.g., `bat: {builder: cargo, source: bat}` if the Rust version is preferred)

### Entry Lifecycle: When Do Discovery Entries Become Redundant?

The design states (line 346): "The seed-discovery tool's cross-reference step handles this: it skips tools that already have recipes, so re-running the tool naturally shrinks the registry."

**Scenario walkthrough:**
1. `ripgrep` is added to `data/discovery-seeds/dev-tools.json` as `{name: ripgrep, repo: BurntSushi/ripgrep}`
2. `seed-discovery` validates it and adds to `discovery.json`: `ripgrep: {builder: github, source: BurntSushi/ripgrep}`
3. User runs `tsuku install ripgrep` → resolver finds it in discovery.json, generates recipe on-the-fly (or batch pipeline creates a committed recipe)
4. Once `recipes/r/ripgrep.toml` exists, the discovery entry is redundant
5. Next time `seed-discovery` runs, it sees the recipe exists and skips adding ripgrep to the output

**Question:** Should the tool actively remove entries that have recipes, or just skip adding them?

The current language "naturally shrinks" implies active removal. But the processing pipeline (lines 242-263) says "Skipped (has recipe) -> logged", which implies entries persist.

**Recommendation:** Clarify the behavior:
- **Option A (Active Pruning):** If a recipe exists, remove the entry from discovery.json in the output
- **Option B (Preservation):** Keep the entry but log it as redundant (useful if the recipe is later removed)
- **Option C (Disambiguation Preservation):** Remove regular entries when recipes exist, but keep disambiguation entries marked with `disambiguation: true`

Option C seems most aligned with the design's intent (line 224: disambiguation entries "should be preserved even after a recipe exists").

---

## 6. Security and Validation Concerns

### GitHub API Validation Sufficiency

The design validates (lines 251-252):
1. Repo exists
2. Not archived
3. Has releases with binary assets

**Missing validation:**
- Repo ownership matches expected owner (defends against repo transfers)
- Release is recent (defends against abandoned projects)
- Binary assets have reasonable checksums (defends against corrupted releases)

The weekly CI freshness check (line 395) mentions "ownership comparison," which is good. But the initial validation doesn't check ownership.

**Question:** Should the seed list include expected owner, or trust that the repo name is correct?

**Recommendation:** Add optional owner verification:
- Seed list entry: `{name: ripgrep, repo: BurntSushi/ripgrep, expected_owner: BurntSushi}`
- Validation checks current owner matches expected owner
- If missing, skip the check (useful for newly added entries)

### Rate Limit Handling

The design mentions (lines 147-148): "500 entries with 2-3 API calls each = 1000-1500 requests. Within the 5000/hour authenticated limit."

**Risk:** If validation runs frequently or multiple developers run it simultaneously, rate limits could be hit.

**Recommendation:** Add retry-after logic and rate limit awareness:
- Check `X-RateLimit-Remaining` header
- If below threshold, pause and resume
- Log rate limit status at the end

This is mentioned as a pattern to reuse (line 342: "GitHub API client with retry and rate limiting"), so it should be implemented.

---

## 7. Code Reuse Assessment

### What Can Be Reused

From `internal/seed/queue.go`:
- `Load()` and `Save()` patterns for JSON I/O
- `Merge()` logic for deduplication
- Schema version validation

From `cmd/seed-queue/main.go`:
- Flag parsing structure
- Source interface pattern (though discovery uses seed lists, not API sources)

From `internal/batch/orchestrator.go`:
- HTTP client patterns (implied, not directly visible)
- Cross-referencing against recipes (lines 254, 317-320)

### What Needs To Be Written

New code required:
1. GitHub API client for validation
2. Seed list JSON parsing (different schema from priority queue)
3. Collision detection (ecosystem API checks)
4. Cross-referencing logic (check if recipe exists in `recipes/` directory)
5. Validation rules for binary assets

**Estimate:** ~300-500 lines of new Go code for `cmd/seed-discovery/main.go`, plus ~200 lines for tests.

### Architecture Fit

The tool follows established patterns:
- Separate CLI tool in `cmd/` (like seed-queue, like tsuku itself)
- Uses `internal/seed` package for shared utilities
- Outputs to `recipes/` directory (same as batch pipeline)
- CI workflow in `.github/workflows/`

This is consistent with the monorepo structure and existing conventions.

---

## 8. Open Questions for Design Author

1. **Collision detection mechanics:** Should the tool use `builders.CanBuild()`, direct API checks, or a new helper method?

2. **Entry removal policy:** When a recipe exists, should the discovery entry be actively removed, preserved, or preserved only if it's a disambiguation?

3. **Builder scope:** Is the discovery registry limited to GitHub releases, or does it support all builders (homebrew, cargo, etc.)? The design says "primarily github" but doesn't prohibit others.

4. **Ownership verification:** Should seed lists include expected repo owner for transfer detection, or trust the repo name as-is?

5. **Contributor PR workflow:** Should contributors run `seed-discovery` locally and commit the updated `discovery.json`, or should CI regenerate it on every PR?

6. **Phase 2 collision count:** The design mentions uncertainty about collision rate (line 148). Should Phase 1 include collision detection on the initial 50-100 entries to validate the approach before scaling?

---

## 9. Overall Recommendation

**Verdict: Approve with clarifications.**

The design is solid and implementable. The architecture correctly reuses existing patterns, the phasing is logical, and the security considerations are addressed. The main gaps are:

1. Clarify collision detection implementation (direct API checks vs. CanBuild)
2. Document validation rules for binary assets
3. Specify entry removal policy when recipes exist
4. Add ownership verification to validation

These are minor issues that can be addressed during implementation or via design amendment.

**Suggested implementation order:**
1. Start with Phase 1 (50-100 entries, no collision detection)
2. Add collision detection in Phase 1 but test on small dataset
3. Proceed to Phase 2 (scale to 500)
4. Add CI freshness in Phase 3
5. Document contributor workflow post-launch

**Risk level: Low.** The tool is a data generation utility with well-defined inputs/outputs. Failure modes are graceful (validation errors skip entries, CI flags stale data). No runtime impact on the CLI.

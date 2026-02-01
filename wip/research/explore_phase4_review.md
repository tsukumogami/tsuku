# Design Review: Discovery Registry Bootstrap

## Executive Summary

The design is **implementation-ready** with minor refinements needed. The problem statement is specific and well-grounded. The three-option breakdown (data sourcing, validation, disambiguation) correctly identifies the key decisions. However, there are unstated assumptions about reuse opportunities with existing infrastructure, and the rejection rationale for alternatives could be strengthened with more specific tradeoffs.

**Key Recommendations:**
1. **Strengthen reuse with batch pipeline**: Consolidate GitHub API validation infrastructure across both systems
2. **Clarify the builder field decision**: Address why discovery entries use `github` vs `homebrew` builder
3. **Make collision detection testable**: Provide concrete collision examples beyond the 4 known cases
4. **Explicit maintenance cost model**: Quantify ongoing curation effort for the ~500 entry target

## Detailed Analysis

### 1. Problem Statement Specificity

**Question:** Is the problem statement specific enough to evaluate solutions against?

**Answer:** YES, with minor gaps.

**Strengths:**
- Clearly scoped: "~500 entries before the resolver delivers value"
- Two concrete entry categories identified (GitHub-release tools, disambiguation overrides)
- Three challenges articulated (data sourcing, validation reliability, batch pipeline overlap)
- Success criteria implicit: registry must cover tools not discoverable via ecosystem probe

**Gaps:**
- Missing concrete failure mode: What happens if registry is 70% complete instead of ~500? Is 500 a hard requirement or aspirational?
- Overlap with batch pipeline needs quantification: "As recipes get generated, discovery entries become redundant" — what's the expected shrinkage timeline? If batch generates 200 recipes/week, how does that affect the 500 target?
- No definition of "most popular developer tools" — is this Homebrew top-500? GitHub stars? Cross-ecosystem aggregate?

**Recommendation:**
Add a "Success Criteria" subsection:
```
- Registry covers 90%+ of Homebrew top-500 CLI tools that distribute via GitHub releases
- Overlap with batch pipeline: expect 30-40% shrinkage as recipes get generated
- Discovery resolver latency <100ms for registry hits (O(1) lookup)
```

### 2. Missing Alternatives

**Question:** Are there missing alternatives we should consider?

**Answer:** Two alternatives missing, one worth considering.

#### 2A. Missing Alternative: Reuse Batch Pipeline Infrastructure

The batch pipeline (DESIGN-batch-recipe-generation.md) already implements:
- GitHub API validation (repo exists, not archived, has releases)
- Rate limiting for GitHub API (5000/hr authenticated)
- Retry logic with exponential backoff
- JSONL artifact generation for CI

The design proposes a separate `cmd/seed-discovery` tool with its own GitHub API client. This duplicates infrastructure. An alternative not considered:

**Alternative 2D (Data Sourcing): Extend Batch Pipeline to Emit Discovery Entries**

The batch validation already checks "repo exists, has releases with binary assets." When a GitHub-release tool fails recipe generation (no pre-built binaries matching our patterns, or LLM-required), emit a discovery.json entry instead of just a failure record.

**Pros:**
- Reuses existing GitHub API validation (no duplication)
- Automatically keeps discovery registry fresh as batch runs
- Discovery entries are by-products of batch failures (zero marginal cost)

**Cons:**
- Couples discovery bootstrap to batch pipeline schedule
- Discovery entries would lag behind initial population need
- Batch pipeline focused on deterministic recipes, not discovery entries

**Evaluation:** The design correctly rejects shared code ("Forcing them together conflates the concerns") but doesn't acknowledge the infrastructure duplication cost. A hybrid is viable: separate `cmd/seed-discovery` tool that imports `internal/seed` utilities for HTTP client, retry logic, and JSON I/O patterns. The design mentions this ("the `seed-discovery` tool should reuse `internal/seed` package utilities where applicable") but doesn't make it a requirement.

**Recommendation:** Elevate internal package reuse to "Required" in the solution architecture. Add an "Infrastructure Reuse" section listing which `internal/seed` utilities MUST be reused vs duplicated.

#### 2B. Considered but Missing: Discovery Entry Lifecycle

The design states "As recipes get generated, discovery entries become redundant." An alternative approach:

**Alternative (Validation): Discovery Entries with Expiration Metadata**

Add `expires_after_recipe: true` to seed list entries. When a recipe exists for a tool, keep the discovery entry but mark it `stale: true`. The resolver skips stale entries. Periodic cleanup removes entries stale for >90 days.

**Pros:**
- Historical record of which tools needed discovery entries
- Enables re-activation if recipe gets removed
- Supports metrics: "discovery usage decreased 80% as recipes grew"

**Cons:**
- Adds schema complexity (stale field, expiration logic)
- Discovery.json file size doesn't shrink with recipe growth

**Evaluation:** Not essential for Phase 1. The design's approach (cross-reference existing recipes, skip tools) is simpler. Expiration could be added later if needed.

### 3. Rejection Rationale Quality

**Question:** Is the rejection rationale for each alternative specific and fair?

**Answer:** Mostly fair, but some rejections lack depth.

#### Decision 1 Rejections

**LLM-assisted bulk generation:**
- Rationale: "LLMs hallucinate repo names, and validation becomes the bottleneck."
- **Strength:** Correctly identifies accuracy problem.
- **Gap:** Doesn't acknowledge that LLM could be used for *initial generation* followed by automated validation, not as final source. The validation infrastructure proposed (GitHub API checks) could catch hallucinations.
- **Recommendation:** Strengthen with: "Even with validation, LLM errors require manual review to distinguish hallucinations from legitimate API failures, negating the speed advantage."

**Scrape awesome-cli-apps:**
- Rationale: "Noisy — many entries are libraries, not CLI tools."
- **Strength:** Identifies signal/noise issue.
- **Gap:** Misses the opportunity cost. Awesome lists are maintained by humans and could seed the initial curated lists.
- **Recommendation:** Soften to: "Rejected as *primary* source but useful as input for seed lists."

#### Decision 2 Rejections

**One-time script with no CI:**
- Rationale: "Entries go stale silently."
- **Strength:** Clear failure mode.
- **Gap:** Doesn't explain *why* staleness matters for discovery vs recipes. A stale discovery entry points to the wrong repo — this fails during install, not silently.
- **Recommendation:** Add: "Stale entries are a security concern per parent design — a transferred repo could become malicious."

#### Decision 3 Rejections

**Fully manual curation:**
- Rationale: "At 500 entries, manual cross-referencing against 7 ecosystem registries is impractical."
- **Strength:** Quantifies the effort (500 × 7 = 3500 checks).
- **Fair:** Yes. Manual review at this scale is legitimately impractical.

**Fully automated resolution by popularity:**
- Rationale: "npm's `bat` has more downloads than sharkdp/bat has GitHub stars, but sharkdp/bat is what CLI users want."
- **Strength:** Concrete example shows why download count alone fails.
- **Gap:** Doesn't explain how the chosen approach (manual resolution after automated detection) avoids this problem. What criteria does the human use to decide?
- **Recommendation:** Add: "Manual resolution allows context-aware decisions (e.g., 'bat' as cat-replacement vs npm testing framework) that download counts can't capture."

### 4. Unstated Assumptions

**Question:** Are there unstated assumptions that need to be explicit?

**Answer:** YES, five key assumptions.

#### Assumption 1: Discovery Registry Builder Field

The design shows examples with `"builder": "github"` but also mentions `"builder": "homebrew"` for the existing `jq` entry. The decision for which builder to use is unstated.

**Question:** When should a discovery entry use `github` vs `homebrew` builder?

**Context from related designs:**
- DESIGN-registry-scale-strategy.md: "Discovery entries for these tools would use the `github` builder only if the tool distributes pre-built binaries via GitHub releases."
- Priority queue shows tools like `homebrew:jq` with source `homebrew`.
- Discovery registry example: `"jq": {"builder": "homebrew", "source": "jq"}`

**Implication:** For tools available via both GitHub releases AND Homebrew bottles, the discovery entry should prefer `homebrew` builder (deterministic, $0 cost) over `github` builder (LLM-required, $0.10/recipe). This preference is unstated.

**Recommendation:** Add "Builder Selection Criteria" to Solution Architecture:
```markdown
### Builder Selection for Discovery Entries

1. If tool has Homebrew bottle: `{"builder": "homebrew", "source": "formula-name"}`
2. If tool has GitHub releases with binary assets but no bottle: `{"builder": "github", "source": "owner/repo"}`
3. Disambiguation overrides can use any builder matching user intent
```

#### Assumption 2: Collision Rate

The design states "Could be 10 or 50" collisions but doesn't state what happens if it's 200. At what collision rate does the manual resolution approach break down?

**Recommendation:** Add to "Uncertainties": "If actual collision rate exceeds 100 (20% of entries), manual resolution becomes bottleneck. Mitigation: prioritize known collisions (bat, fd, serve, delta), defer long-tail collisions to user feedback."

#### Assumption 3: GitHub API Rate Limits During Bootstrap

The design acknowledges this uncertainty: "500 entries with 2-3 API calls each = 1000-1500 requests. Within the 5000/hour authenticated limit."

**Gap:** Doesn't state what happens if validation takes longer than 1 hour. Does the tool pause and resume? Fail and require re-run?

**Recommendation:** Add to cmd/seed-discovery processing pipeline:
```
- Rate limiting: 1 request/second (3600/hour, well under 5000 limit)
- Resume support: Save partial results every 100 entries
- On rate limit error: pause for 60 minutes, resume
```

#### Assumption 4: Seed List Maintenance Burden

The design treats seed lists as "one-time work" but they need updates as new tools become popular. Who maintains them? How often?

**Recommendation:** Add to "Consequences":
```markdown
### Ongoing Maintenance
- Seed lists require quarterly updates to capture new popular tools
- Estimated effort: 2-4 hours/quarter to review Homebrew analytics and add ~20 new entries
- Maintainer responsibility: Same as recipe contributions (PR review)
```

#### Assumption 5: Cross-Reference Logic

The design states: "Cross-references existing recipes — skips tools that already have recipes in `recipes/`."

**Gap:** How is the cross-reference performed? By exact name match? What if recipe name differs from tool name (e.g., `ripgrep` recipe installed as `rg`)?

**Recommendation:** Add to "Go Tool: cmd/seed-discovery" processing pipeline:
```
3. Cross-reference: does recipes/{first-letter}/{name}.toml exist?
   - Exact name match only (case-insensitive)
   - If tool installs with different binary name, discovery entry is NOT redundant
```

### 5. Strawman Options

**Question:** Is any option a strawman (designed to fail)?

**Answer:** NO. All alternatives are legitimate, with honest tradeoffs.

Evidence:
- **LLM-assisted bulk generation** is used by many projects successfully, but the design correctly identifies accuracy/cost issues for this specific use case
- **Scrape awesome-lists** is a valid data source; the design acknowledges it as "useful as input"
- **Fully automated resolution by popularity** works for some ecosystems (npm's default behavior); the design shows why it fails here (disambiguation requires context)

The alternatives are real options with specific rejection reasons, not strawmen constructed to make the chosen approach look better.

## Reuse Analysis with Existing Infrastructure

The user specifically requested analysis of reuse opportunities with DESIGN-registry-scale-strategy.md and the batch pipeline. Here's the detailed breakdown:

### Overlap with Batch Pipeline

| Component | Batch Pipeline | Discovery Bootstrap | Reuse Opportunity |
|-----------|---------------|-------------------|-------------------|
| GitHub API client | `internal/seed` package (HTTP with retry) | Proposed in `cmd/seed-discovery` | **HIGH**: Should reuse `internal/seed` HTTP client |
| Rate limiting | 1 req/sec per ecosystem | Unstated | **HIGH**: Reuse rate limiter abstraction |
| JSON I/O | Load/save priority queue JSON | Load/save discovery.json | **MEDIUM**: Similar patterns, could share utilities |
| Validation (repo exists) | Part of batch validate step | GitHub API check in seed-discovery | **HIGH**: Identical logic, extract to `internal/github` |
| Validation (not archived) | Not implemented yet | Proposed | **MEDIUM**: Implement once, reuse in both |
| Validation (has releases) | Not implemented yet | Proposed | **MEDIUM**: Implement once, reuse in both |
| Cross-reference recipes | Batch skips if recipe exists | seed-discovery skips if recipe exists | **HIGH**: Extract `RecipeExists(name)` to `internal/recipe` |

**Current State:** The design mentions reuse ("the `seed-discovery` tool should reuse `internal/seed` package utilities where applicable") but doesn't enforce it.

**Recommendation:** Make this explicit in the solution architecture:

```markdown
### Infrastructure Reuse Requirements

The `cmd/seed-discovery` tool MUST reuse the following from existing infrastructure:

1. **HTTP client with retry**: `internal/seed.NewHTTPClient()` — already used by batch pipeline
2. **GitHub API validation**: Extract to `internal/github.ValidateRepo(owner, repo)` — shared by both systems
3. **Recipe existence check**: `internal/recipe.Exists(name)` — shared by both systems
4. **Rate limiter**: `internal/seed.RateLimiter` — already used by batch pipeline

No duplication of GitHub API logic. Both systems share the same validation codepath.
```

### Overlap with Priority Queue

The priority queue (`data/priority-queue.json`) contains 204 Homebrew entries. Many are tools that also distribute via GitHub releases (gh, node, uv, etc.).

**Current relationship (per design):**
> The tool cross-references the priority queue and "notes overlap" but doesn't state what action to take.

**Gap:** What does "note overlap" mean? Are overlapping entries included in discovery.json or excluded?

**Analysis:**
- If `gh` (GitHub CLI) is in the priority queue as `homebrew:gh` AND distributes via GitHub releases, should discovery.json have an entry?
- Answer depends on Builder Selection (see Assumption 1): Prefer `homebrew` builder if bottle exists. The discovery entry is only needed if GitHub releases are the *only* source.

**Recommendation:** Clarify cross-reference behavior:
```markdown
### Priority Queue Cross-Reference

When a tool exists in both the seed list AND the priority queue:

1. If queue entry uses `homebrew` builder: Skip discovery entry (Homebrew bottle is deterministic)
2. If queue entry uses different source: Add discovery entry as disambiguation override
3. Log overlap for manual review (ensure builder choice is correct)
```

## Testability and Validation Gaps

### Gap 1: No Test Data for Collision Detection

The design identifies 4 known collisions (bat, fd, serve, delta) but proposes automated collision detection for the full 500 entries. How is this testable without running the full bootstrap?

**Recommendation:** Add to Phase 1 deliverables:
```
- Test collision detection on 10-tool subset BEFORE full 500-entry run
- Validate that known collisions (bat, fd, serve, delta) are correctly flagged
- Manual review of 10 collision reports to tune detection thresholds
```

### Gap 2: No Dry-Run Mode

The design proposes a tool that "outputs validated discovery.json" but doesn't specify a dry-run mode for testing without side effects.

**Recommendation:** Add to cmd/seed-discovery flags:
```
--dry-run    Validate and report without writing discovery.json
--verbose    Show detailed validation output per entry
```

### Gap 3: Validation Metrics Unclear

The design states "Validation: GitHub API: repo exists? not archived? has releases?" but doesn't specify what constitutes "has releases."

**Ambiguity:**
- Does the repo need releases in the last 12 months?
- Does it need binary assets in releases?
- What if the latest release is a pre-release?

**Recommendation:** Add "Validation Criteria" subsection:
```markdown
### GitHub API Validation Criteria

For each entry, validate:
1. Repo exists: GET /repos/{owner}/{repo} returns 200
2. Not archived: `archived: false` in repo metadata
3. Has releases: GET /repos/{owner}/{repo}/releases returns >0 releases in last 24 months
4. Binary assets: Latest non-prerelease has >0 assets matching pattern `*.tar.gz` or `*.zip`

Entries failing any check are logged and excluded.
```

## Disambiguation Data Gaps

The design proposes "Automated Collision Detection + Manual Resolution" but doesn't provide concrete examples beyond the 4 known cases.

### Missing Context

The design states:
> "When a tool name exists in both the discovery registry (as a GitHub release) and an ecosystem registry, flag it as a potential collision."

**Gap:** What about collisions WITHIN the registry? If two different GitHub repos both claim to be "serve", which one wins?

**Example Scenario:**
- `sharkdp/bat` (cat replacement, 40K stars)
- GitHub user creates `popular-tools/bat` (typosquat, 50 stars)
- Both distribute binaries via GitHub releases
- Which entry goes in discovery.json?

**Answer (implied but unstated):** The seed lists are curated, so this doesn't happen. The human creating the seed list chooses the correct repo.

**Recommendation:** Make this explicit:
```markdown
### Disambiguation Within Discovery Registry

Seed lists are curated to include only the correct/canonical repo for each tool name. If two repos claim the same tool name:
- Seed list includes only the canonical one (based on stars, official project, etc.)
- Typosquats and forks are excluded at seed list creation time
```

### Collision Examples Needed

The design mentions "bat, fd, serve, delta" but doesn't show the disambiguation resolution.

**Recommendation:** Add "Disambiguation Examples" table:

| Tool Name | Collision | Discovery Entry | Reasoning |
|-----------|-----------|----------------|-----------|
| bat | npm:bat (testing framework, 200/day) vs GitHub:sharkdp/bat (cat replacement, 45K/day) | `{"builder": "github", "source": "sharkdp/bat", "disambiguation": true}` | CLI users want cat replacement, not test framework |
| fd | npm:fd (file descriptor utility) vs GitHub:sharkdp/fd (find alternative) | `{"builder": "github", "source": "sharkdp/fd", "disambiguation": true}` | Popularity: 10K+ stars vs <100 downloads/day |
| serve | npm:serve (static file server, 1M downloads/week) vs others | `{"builder": "npm", "source": "serve", "disambiguation": true}` | npm's serve is the popular one for CLI users |
| delta | cargo:delta vs GitHub:dandavison/delta | `{"builder": "github", "source": "dandavison/delta", "disambiguation": true}` | Cargo entry is different tool, dandavison/delta is git diff viewer |

This makes the "manual resolution" criteria explicit: stars, download count, and user intent context.

## Maintenance and Lifecycle Gaps

### Gap 1: No Definition of "Stale"

The design proposes weekly freshness checks but doesn't define what constitutes a stale entry.

**Ambiguity:**
- Repo archived?
- Repo deleted?
- Repo transferred to new owner?
- No releases in last 24 months?

**Recommendation:** Add "Freshness Criteria":
```markdown
An entry is considered STALE if:
1. Repo returns 404 (deleted)
2. Repo is archived (`archived: true`)
3. Repo owner changed (transfer detected)

An entry is flagged for REVIEW if:
- No releases in last 24 months (may be stable, not abandoned)
```

### Gap 2: Seed List Update Cadence

The design treats seed lists as "one-time work" but new popular tools emerge constantly.

**Question:** How often are seed lists updated? Who triggers updates?

**Recommendation:** Add to "Consequences":
```markdown
### Seed List Maintenance

- Update cadence: Quarterly (or when user requests accumulate)
- Trigger: Operator reviews Homebrew analytics for new top-500 entries
- Process: Add new entries to appropriate seed list file, re-run seed-discovery tool
- Estimated effort: 2-4 hours per quarter
```

## Quantitative Analysis: Shrinkage Model

The design states "discovery entries become redundant as recipes get generated" but doesn't model the shrinkage.

**Context from batch pipeline:**
- Target: 200+ recipes/week (from DESIGN-batch-recipe-generation.md)
- 155 recipes exist today
- Priority queue has 204 Homebrew entries

**Shrinkage Model:**

| Week | Recipes Generated | Total Recipes | Discovery Entries Needed | Shrinkage |
|------|------------------|---------------|------------------------|-----------|
| 0 (today) | 0 | 155 | ~500 | 0% |
| 4 | 800 | 955 | ~200 | 60% |
| 8 | 1600 | 1755 | ~50 | 90% |

**Assumptions:**
- 40% of discovery entries overlap with batch pipeline queue
- As recipes get generated, overlapping entries are removed
- Discovery registry shrinks to ~50 entries (pure GitHub-release tools + disambiguations)

**Implication:** The 500-entry target is a *peak*, not steady state. By week 8, discovery.json shrinks to ~50 entries.

**Recommendation:** Add "Expected Lifecycle" subsection:
```markdown
### Discovery Registry Lifecycle

The ~500 entry target is the bootstrap peak. Expected trajectory:
- Week 0: ~500 entries (GitHub-release tools + disambiguations)
- Week 4: ~200 entries (batch pipeline generates overlapping recipes)
- Week 8: ~50 entries (steady state: pure GitHub-release + disambiguations)

Steady state maintains:
1. GitHub-release tools with no Homebrew bottle (terraform, kubectl, stripe-cli)
2. Disambiguation overrides (bat, fd, serve, delta)
3. New popular tools pending batch generation
```

## Security Considerations Review

The security section is solid. Two additions recommended:

### Addition 1: Disambiguation Injection

**Risk:** A malicious PR adds a disambiguation entry pointing to a compromised repo.

Example: PR changes `bat` entry from `sharkdp/bat` to `malicious-actor/bat`.

**Mitigation:**
- Seed lists are committed files reviewed via normal PR process (stated)
- **ADD:** Disambiguation entries flagged with `"disambiguation": true` require extra scrutiny in PR review (checklist item)

### Addition 2: Seed List Source Trust

The design proposes seeding from "Homebrew analytics, curated lists, tools mentioned in issues."

**Risk:** Curated lists from external sources (awesome-cli-apps) could include malicious entries.

**Mitigation:**
- **ADD:** "Seed lists sourced only from official ecosystem APIs (Homebrew, crates.io) or first-party tsuku documentation. Third-party curated lists (awesome-*) used for reference only, not direct import."

## Final Recommendations Summary

### Critical (Blocking Implementation)

1. **Builder Selection Criteria:** Add explicit rules for choosing `homebrew` vs `github` builder
2. **Infrastructure Reuse Requirements:** Mandate reuse of `internal/seed` and `internal/github` packages
3. **Validation Criteria:** Define concrete thresholds for "has releases," "not stale"
4. **Disambiguation Examples:** Provide 4+ concrete examples with resolution reasoning

### Important (Strengthens Design)

5. **Cross-Reference Behavior:** Clarify what "notes overlap" means for priority queue entries
6. **Maintenance Cost Model:** Quantify ongoing seed list curation effort
7. **Shrinkage Timeline:** Model expected registry size over 8-week batch pipeline ramp
8. **Dry-Run Mode:** Add to cmd/seed-discovery for testability

### Nice-to-Have (Improves Clarity)

9. **Strengthen LLM Rejection:** Explain why LLM + validation still requires manual review
10. **Collision Rate Threshold:** Define when manual resolution breaks down
11. **Seed List Source Trust:** Restrict to first-party sources only

## Conclusion

The design is fundamentally sound and **ready for implementation** with the refinements above. The three-option structure correctly identifies key decisions. The main gap is **unstated assumptions** about builder selection and infrastructure reuse. Addressing these before Phase 1 implementation will prevent mid-stream design changes.

**Confidence Level:** High. The problem is well-understood, alternatives are legitimate, and the chosen approach balances accuracy (manual seed lists) with automation (GitHub API validation). The batch pipeline provides proven patterns to reuse.

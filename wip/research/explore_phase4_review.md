# Review: DESIGN-automated-seeding.md

## Options Review (Phase 4)

### 1. Problem Statement Specificity

The problem statement is well-defined and backed by concrete data (5,275 entries, 97% homebrew). It identifies three distinct gaps -- multi-ecosystem discovery, disambiguation at seeding time, and freshness tracking -- each with clear evidence of the current shortcoming.

One area that could be sharper: the claim that Rust CLI tools "fail because Homebrew doesn't have bottles for them" is misleading. Homebrew *does* have bottles for ripgrep, fd, bat, eza, and hyperfine. The actual problem is that the batch pipeline's homebrew builder may not produce optimal recipes for these tools (e.g., they might work better with cargo or github sources). The problem statement should distinguish between "fails entirely" and "suboptimal source routing."

**Recommended change**: Reword the first gap to focus on suboptimal source routing rather than claiming homebrew can't build these tools.

### 2. Missing Alternatives

**Decision 1 (Discovery Strategy):**
- The design doesn't consider **Repology** (repology.org), which aggregates package metadata across many ecosystems and has an API. It would provide cross-ecosystem name mapping without querying each ecosystem individually.
- **DistroWatch** or **Awesome CLI** GitHub lists as seeding sources are mentioned in passing (Go exclusion note) but not evaluated as alternatives.

These are minor gaps. The chosen per-ecosystem approach is sound and the alternatives are reasonable omissions.

**Decision 2 (Disambiguation Integration):**
- No alternative is missing. The three options (full disambiguation, use discovery source, re-disambiguate everything) cover the spectrum well.

**Decision 4 (Source Change Alerting):**
- The design doesn't consider a **staged rollout** approach: auto-accept source changes into a "staging" queue status, let batch generation try the new source, and only promote to the main queue if generation succeeds. This would provide a safety net beyond the priority-based split.

### 3. Rejection Rationale Fairness

**GitHub Stars alternative (Decision 1)**: The rejection is fair but slightly misleading. It says "a Rust crate can have 50K GitHub stars but be distributed via cargo, not GitHub releases." This is true, but the alternative was about *discovery* (finding tools to seed), not about *source selection* (which is disambiguation's job). GitHub stars could be a valid discovery signal even if cargo is the installation source. The rejection would be stronger if it focused on the practical issue: GitHub API rate limits (30 req/hour unauthenticated, 5K/hour authenticated) make it impractical for discovering 500+ tools per ecosystem.

**Libraries.io alternative (Decision 1)**: Fair rejection. Third-party dependency is a legitimate concern.

**"Use discovery ecosystem as source" (Decision 2)**: The rejection example ("httpie" from PyPI might have better GitHub releases) is a good concrete case. Fair.

**"Re-disambiguate everything weekly" (Decision 2)**: The math checks out (5K * 8 = 40K calls, crates.io at 1/s = 83 min). Fair rejection.

**JSONL append log (Decision 3)**: The rejection overstates the difficulty. `grep "ripgrep" audit.jsonl` is trivial. But the per-file approach is still better for git diffs and tooling. Fair enough.

### 4. Unstated Assumptions

**Critical -- Two separate queue systems exist, and the design conflates them.**

The codebase has TWO different queue structures:

1. **`seed.PriorityQueue`** (in `internal/seed/queue.go`): Uses `seed.Package` with fields `{ID, Source, Name, Tier, Status, AddedAt, Metadata, ForceOverride}`. Written to `data/queues/priority-queue-homebrew.json`. This is what the current `cmd/seed-queue` reads and writes.

2. **`batch.UnifiedQueue`** (in `internal/batch/queue_entry.go` + `bootstrap.go`): Uses `batch.QueueEntry` with fields `{Name, Source, Priority, Status, Confidence, DisambiguatedAt, FailureCount, NextRetryAt}`. Written to `data/queues/priority-queue.json`. This is what the batch orchestrator reads.

The design says it operates on `data/queues/priority-queue.json` (the unified queue) but references `seed.Package` and `queue.Merge()` (the seed queue). These are different types with different schemas. The `seed.Package.ID` field is `"homebrew:ripgrep"` while `batch.QueueEntry.Source` is `"homebrew:ripgrep"` -- similar but structurally different objects.

The design needs to be explicit about which queue it targets:
- If it targets the unified queue (`priority-queue.json`), the seed command needs to work with `batch.QueueEntry`, not `seed.Package`. The `Merge()` method would need to understand `Confidence`, `DisambiguatedAt`, and `FailureCount`.
- If it targets the per-ecosystem queue (`priority-queue-homebrew.json`), the freshness checking and disambiguation integration don't make sense because those fields don't exist on `seed.Package`.

**Most likely intent**: The design wants to modify the seed command to write directly to the unified queue. This means either (a) migrating `cmd/seed-queue` to use `batch.QueueEntry` or (b) adding the missing fields to `seed.Package`. This is a significant architectural decision that the design doesn't address.

**Other unstated assumptions:**

- **Ecosystem probe name matching**: The `EcosystemProbe.Resolve()` function (line 122 of `ecosystem_probe.go`) filters probe results where `outcome.result.Source` exactly matches `toolName` (case-insensitive). For tools discovered via one ecosystem with a different name in another (e.g., `python-black` in homebrew vs `black` in PyPI), the probe will miss cross-ecosystem matches. The design assumes all ecosystems use the same package name, which isn't always true.

- **"review_pending" confidence value doesn't exist**: The design proposes setting `confidence: "review_pending"` for flagged source changes, but `batch.QueueEntry.Validate()` only accepts `"auto"` and `"curated"` (see `validConfidences` in `queue_entry.go`). Adding this value requires a schema change and validation update.

- **`-limit 0` semantics are undefined**: The bootstrap procedure uses `-limit 0` to mean "process all entries." The current `HomebrewSource.Fetch()` treats `limit <= 0` as "no limit" (line 69: `if limit > 0 && limit < len(items)`). This works, but the semantics should be documented since it's load-bearing for a 3-4 hour operation.

- **The existing workflow writes to per-ecosystem queue files, not the unified queue**: The current `seed-queue.yml` writes to `data/queues/priority-queue-$SOURCE.json` (line 64-65 of the workflow). The design's proposed workflow writes to `data/queues/priority-queue.json`. This is a breaking change in how the seeding pipeline integrates with the batch pipeline.

### 5. Strawman Analysis

None of the alternatives are strawmen. Each rejected option has genuine trade-offs and the rejections cite specific practical concerns. The "no audit log" alternative is the weakest (hard to imagine choosing it), but it serves as useful documentation of why audit logs matter rather than as a fake option.

---

## Architecture Review (Phase 8)

### 1. Implementation Clarity

The architecture is clear enough to implement at a high level. The component diagram, data flow, and command interface are well-specified. However, several implementation details need resolution:

**The `Disambiguator.Resolve()` wrapper**: The design shows a clean `Resolve(name string)` interface, but the underlying `EcosystemProbe.Resolve()` takes a `context.Context` and requires `[]builders.EcosystemProber` at construction time. The wrapper needs to handle:
- Context creation and timeout management (separate from the per-source fetch timeout)
- Prober initialization (which probers to include -- all 8? just the relevant ones?)
- Error handling when some probers fail but others succeed (already handled by `EcosystemProbe` but the wrapper should surface partial failures)

This is implementable but not trivial. A sentence or two about how the Disambiguator initializes its probers would help.

### 2. Missing Components and Interfaces

**a) Queue type bridging**: As noted above, the design doesn't specify how `seed.Package` (from source Fetch) maps to `batch.QueueEntry` (what the unified queue stores). A conversion function or unified type is needed.

**b) Tier assignment for non-homebrew sources**: `HomebrewSource` assigns tiers using `tier1Formulas` (a curated map) and `tier2Threshold` (download counts). The new sources need equivalent tier assignment logic. The design doesn't specify how tiers are assigned for crates.io, npm, PyPI, or RubyGems discoveries. Options:
- Use the same curated `tier1Formulas` map (but it only contains homebrew formula names)
- Define per-ecosystem tier thresholds
- Always assign tier 3 and let disambiguation/curation promote

**c) Package ID generation for non-homebrew sources**: Current IDs are `"homebrew:ripgrep"`. For a crate, should the ID be `"cargo:ripgrep"` (ecosystem name) or `"crates.io:ripgrep"` (registry name)? The design says `Source` is set to ecosystem name (e.g., `"cargo"`, `"npm"`), but this should be explicit since IDs must be globally unique and consistent with the unified queue.

**d) Deduplication across ecosystems**: If `CratesIOSource` discovers "ripgrep" and `HomebrewSource` also has "ripgrep", how are duplicates handled? `queue.FilterExisting()` checks by ID, but IDs include the ecosystem prefix. So `"cargo:ripgrep"` and `"homebrew:ripgrep"` would both be added. The disambiguation step should handle this, but the data flow diagram shows `FilterExisting()` running before disambiguation, not after.

The intended flow seems to be: discover "ripgrep" from cargo, see it's not in the queue by name, run disambiguation (which picks the best source across all ecosystems), then add it with the disambiguated source. But `FilterExisting()` checks by ID, and at the discovery stage the ID is `"cargo:ripgrep"`. If `"homebrew:ripgrep"` already exists in the queue, `FilterExisting()` won't catch the duplicate because the IDs differ. The deduplication should check by **name**, not by ID.

### 3. Implementation Phase Sequencing

The four phases are correctly sequenced:
1. Sources first (they produce the raw data)
2. Disambiguation integration (transforms raw data)
3. Command extensions (wires everything together)
4. Workflow and bootstrap (production deployment)

One suggestion: Phase 2 (disambiguation integration) should include the queue type bridging work mentioned above. If that's deferred to Phase 3, the disambiguator won't have the right types to work with during testing.

### 4. Simpler Alternatives

**Alternative: Skip full disambiguation for new packages, just use the discovery source.**

The design correctly rejects this as a general approach, but there's a middle ground: run disambiguation only for tools that are already in the queue with a different source. For genuinely new tools (not in the queue at all), the discovery ecosystem is usually correct. Full 8-ecosystem probing for every new package is expensive and most of the time confirms what the discovery source already said.

This would reduce the probing load from ~500 probes per source to ~50-100 (only packages that overlap with existing entries). The weekly runtime would drop from ~15 minutes to ~3-5 minutes.

The trade-off: a small percentage of new packages would get suboptimal sources. But the 30-day freshness cycle would catch these on the next pass. This simplification may be worth considering for the initial implementation.

### 5. Command Interface Coverage

The command interface is thorough. A few gaps:

- **No `-context` or `-timeout` flag**: The command should allow configuring the global timeout separately from the per-source timeout. The 120-minute workflow timeout is the outer bound, but the command itself might want a 90-minute timeout to allow time for the commit step.

- **No `-skip-freshness` flag**: The bootstrap procedure uses `-freshness 0` to force all entries. A boolean flag to skip freshness checking entirely would be cleaner for the bootstrap case.

- **No `-exclude` flag**: The design mentions curated entries are never re-disambiguated, but there's no way to exclude specific packages from the command line. This is probably fine since curated entries are identified by `confidence: "curated"`, but it limits debugging.

- **The summary JSON output goes to stdout while progress goes to stderr**: This is a good design for pipeline composition, but it should be documented that `-verbose` output goes to stderr.

### 6. Workflow YAML Correctness

Several issues:

**a) Missing `permissions` for issue creation**: The workflow has `permissions: contents: write` but doesn't include `issues: write`. The `gh issue create` step will fail without it.

**b) Issue creation parsing is fragile**: The `jq` + `while read` pipeline for creating issues has a bug. It reads multi-line `$body` with `IFS= read -r body`, but `read -r` reads one line at a time. A 4-line body (Package, Old, New, Priority) won't be captured as a single variable. This needs `read -r -d ''` or a different approach.

**c) Missing `GITHUB_TOKEN` in issue creation step**: The proposed workflow uses `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` in the seeding step, but the issue creation step (which calls `gh issue create`) doesn't have access to this env var. The `gh` CLI needs `GH_TOKEN` or `GITHUB_TOKEN` in the environment. The existing workflow uses a GitHub App token (`steps.app-token.outputs.token`), which the proposed workflow drops.

**d) The proposed workflow replaces the retry-on-push logic**: The existing workflow has sophisticated push retry logic (3 attempts with exponential backoff and rebase). The proposed workflow has a simple `git push` without retry. Given the weekly cadence, this is probably fine, but it's a regression from the existing workflow.

**e) `workflow_dispatch` input type changes**: The existing workflow uses `type: choice` with options for the `source` input. The proposed workflow uses a plain `description` with a default. This loses the dropdown UI in the GitHub Actions interface.

**f) Missing `concurrency` group**: The existing workflow has a concurrency group (`queue-operations-$SOURCE`) to prevent parallel runs. The proposed workflow drops this. Since seeding modifies the queue file, concurrent runs could cause conflicts.

**g) Pushing directly to main from a workflow**: The existing workflow pushes directly to main with bot credentials. The proposed workflow does the same. This is established practice for this repo but worth noting that it bypasses branch protection rules.

### 7. Freshness Check Edge Cases

**a) Null `disambiguated_at`**: Looking at the actual queue data, all entries currently have `"disambiguated_at": null`. The freshness check needs to treat null as "stale" (force re-disambiguation). The design says "entries with stale disambiguated_at (> 30 days)" but doesn't mention null handling. This is critical because bootstrap Phase B depends on it.

**b) Race with batch processing**: If a package is being processed by the batch orchestrator while the seeding command re-disambiguates it and changes its source, the batch run may use the old source. The design doesn't address concurrency between the weekly seeding workflow and the batch generation workflow. The existing `concurrency` group in the workflow only prevents concurrent seeding runs, not seeding + batch conflicts.

**c) "Success" entries**: Should entries with `status: "success"` be re-disambiguated? These already have working recipes. Changing their source would mean the existing recipe was generated for a different source. The design doesn't explicitly exclude success entries from freshness checking. Only curated entries are mentioned as excluded.

**d) Failure count threshold**: The design says "entries with failure_count >= 3" trigger re-disambiguation, but the `QueueEntry` tracks `failure_count` as consecutive batch generation failures. A package might fail 3 times because of a builder bug, not because of wrong source selection. Re-disambiguating won't help if the source is correct but the builder is broken. Consider only re-disambiguating failed entries that also have stale `disambiguated_at`.

**e) `NextRetryAt` interaction**: `QueueEntry` has a `NextRetryAt` field for exponential backoff. If the seeding command re-disambiguates an entry and changes its source, should `NextRetryAt` and `FailureCount` be reset? The design doesn't specify, but changing the source is effectively a new attempt.

---

## Summary of Top Findings

### Critical (must fix before implementation)

1. **Two queue systems -- design conflates them.** The seed package uses `seed.Package`/`seed.PriorityQueue` while the batch pipeline uses `batch.QueueEntry`/`batch.UnifiedQueue`. The design must specify which queue the extended seed command targets, and how the type mapping works. This affects almost every component in the design.

2. **Cross-ecosystem deduplication by name, not by ID.** The data flow runs `FilterExisting()` before disambiguation, but it checks by ID (including ecosystem prefix). A package in the queue as `homebrew:ripgrep` won't be caught as a duplicate when discovered as `cargo:ripgrep`. Deduplication needs to be name-based.

3. **`"review_pending"` confidence value doesn't exist in the schema.** The `validConfidences` map in `queue_entry.go` only accepts `"auto"` and `"curated"`. Either add the new value or change the alerting approach.

### Important (should fix)

4. **Workflow YAML has multiple bugs**: missing `issues: write` permission, broken multi-line issue body parsing, missing GitHub App token for `gh` CLI, dropped concurrency group and push retry logic.

5. **Null `disambiguated_at` handling not specified.** All existing entries have null values. The freshness check must treat null as stale.

6. **Success entries should probably be excluded from re-disambiguation.** Changing the source of an entry that already has a working recipe is counterproductive.

7. **Tier assignment for non-homebrew sources is unspecified.** New sources need tier logic, and the design doesn't define it.

### Minor (good to address)

8. **Problem statement overstates homebrew failures** for Rust tools. Homebrew does have bottles for ripgrep/fd/bat. The issue is suboptimal routing, not build failures.

9. **`FailureCount` reset on source change** should be specified.

10. **Ecosystem probe name matching may miss cross-ecosystem name variations** (e.g., `python-black` vs `black`).

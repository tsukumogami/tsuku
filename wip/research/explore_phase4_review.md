# Architect Review: DESIGN-system-lib-backfill.md

**Reviewer**: architect-reviewer
**Date**: 2026-02-22
**Design**: `docs/designs/DESIGN-system-lib-backfill.md`
**Issue**: #1191

---

## 1. Problem Statement Assessment

### Is it specific enough to evaluate solutions against?

Mostly yes. The problem is well-scoped: 2,830 pending entries will hit missing library dependencies, and the reactive loop adds one batch cycle of latency per library. The design correctly identifies this as a throughput problem, not a correctness problem.

### Does the problem statement accurately reflect current state?

**Verified accurate:**
- Queue has 5,275 total entries (confirmed via `data/queues/priority-queue.json`)
- 2,830 entries in `pending` status (confirmed)
- 0 entries in `blocked` status (confirmed -- grep returned no matches)
- 22 library recipes exist (confirmed via `type = "library"` grep across `recipes/`)

**Partially inaccurate:**
- The design says "0 active blockers" but there are 8 `missing_dep` failure records across 7 failure JSONL files. These include blockers like `ada-url`, `bdw-gc`, `gmp`, `notmuch`, `openssl@3`, and `dav1d`. The entries aren't in `blocked` queue status because they appear to transition to `failed` or `requires_manual` rather than `blocked`. This is a subtle distinction the design papers over -- the reactive loop description (line 58-66) assumes entries get blocked and requeued, but the current failure records show entries in `failed` status with `blocked_by` metadata in legacy-format JSONL files. The design should clarify whether the blocking mechanism it describes actually matches the current queue status model.

**Claim requiring verification:**
- "Only 3 of 22 existing library recipes currently declare `satisfies` metadata." I found `satisfies` entries in only 2 library-typed recipes (`libcurl.toml` comment says "No satisfies", `libnghttp2.toml` has one). `sqlite.toml` has `satisfies` but is not typed as `library`. The count of "3 of 22" is plausible if sqlite is counted despite not being `type = "library"`, but the framing is slightly misleading.

### Unstated Assumptions

1. **`tsuku create` has `--dry-run` capability.** The design's process flow (line 227) shows `tsuku create --dry-run` as the discovery mechanism. The actual `create.go` has no `--dry-run` flag. `install` has `--dry-run`, but `create` does not. The design should clarify whether this is planned new functionality or whether it means running `create` normally and capturing failures. The Solution Architecture section (line 255) says `tsuku create --from homebrew <name>` with `--json` flag, but create also has no `--json` flag. Both are nonexistent capabilities presented as "existing interfaces."

2. **Failure records include `blocked_by` field.** This is true for the legacy batch format (per-ecosystem JSONL files from Feb 7-18, which nest failures in a `failures` array with `blocked_by`), but the newer per-recipe format (batch files from Feb 19+) uses a flat structure with only `category` and `exit_code` -- no `blocked_by` field. See `batch-2026-02-21T07-27-58Z.jsonl` where `missing_dep` entries for `apr-util` lack `blocked_by`. The design assumes `blocked_by` is reliably present, but the format migration has dropped it from newer records.

3. **Homebrew is the only relevant ecosystem for library creation.** The design mentions "or the appropriate ecosystem source" but every concrete example uses Homebrew. All 22 existing library recipes use the `homebrew` action. This assumption is probably correct but should be stated explicitly rather than hedged.

4. **`queue-maintain` handles the `blocked` -> `pending` transition.** The current queue has 0 blocked entries but 2,111 failed/requires_manual entries. The requeue mechanism in `cmd/queue-maintain/` runs `requeue.Run()`, which reads failure JSONL. The design assumes the requeue-on-recipe-merge path works for the backfill scenario, but it's unclear whether entries that never entered `blocked` status (because they went to `failed` directly) would be picked up.

---

## 2. Missing Alternatives

### Alternative: Homebrew API Dependency Pre-Analysis + Pipeline Verification

The design rejects "Homebrew API analysis" (Decision 1) because it "only covers Homebrew" and "doesn't exercise the actual resolution path." But the chosen approach (dry-run the pipeline) is also effectively Homebrew-only -- all missing_dep failures in the failure data are from Homebrew formulas. The real limitation of API analysis is more specific: Homebrew dependency names don't always map 1:1 to tsuku recipe names (that's what `satisfies` metadata solves). The rejection should say that rather than the generic "doesn't exercise the resolution path."

A hybrid is worth considering: use the Homebrew API to build a candidate list quickly (minutes, not hours of compute), then verify the top candidates against the pipeline. This gets 90% of the discovery benefit at 10% of the dry-run cost.

### Alternative: Parallel Reactive (Increase Batch Cadence)

The design frames this as "proactive vs reactive" but doesn't consider accelerating the reactive loop. If the batch pipeline runs hourly and processes 25 entries per run, the current 2,830 pending entries will take ~113 batch runs (~5 days). Missing libraries discovered during those runs could be created and requeued within the same week. The design doesn't quantify the actual delay cost of the reactive approach to compare against the proactive approach's compute cost.

---

## 3. Rejection Rationale Assessment

| Alternative | Rejection Fair? | Notes |
|---|---|---|
| Reactive only (Decision 1) | **Partially fair.** Claims "many weeks" but doesn't show the math. With 25 entries/hour, 2,830 entries process in ~5 days. Library creation is the bottleneck regardless of discovery method. | Quantify the actual delay. |
| Homebrew API analysis (Decision 1) | **Slightly unfair.** Rejects because "only covers Homebrew" but the proactive approach is also effectively Homebrew-only. Real issue is name resolution mismatch, which is valid but understated. | Sharpen the rejection to the real reason. |
| Static analysis of recipes (Decision 2) | **Fair.** Correctly notes limited predictive power for pending entries. | No issues. |
| Dedicated library generator (Decision 3) | **Fair.** Divergence from main creation path is a real concern. | No issues. |
| Depth ceiling (Decision 4) | **Fair.** Arbitrary limits don't match the actual constraint. | No issues. |
| Skip complex libraries (Decision 4) | **Fair.** Friction log approach is better than blanket exclusion. | No issues. |
| All platforms required (Decision 5) | **Fair.** Reactive platform gap discovery is consistent with existing platform strategy. | No issues. |
| Library coverage ratio (Decision 6) | **Borderline strawman.** "Requires building a full dependency graph upfront" overstates the difficulty -- Homebrew's API provides this data for free. But the outcome-based metric argument is genuinely stronger. | The metric itself is weak, but the stated reason for rejecting it is slightly misleading. |

---

## 4. Decision Drivers vs. Chosen Options

The decision drivers (line 85-91) align well with the chosen options, with one gap:

**"Cover all ecosystems, not just Homebrew"** -- the design acknowledges Homebrew is 98%+ of the queue but lists this as a driver. However, every concrete mechanism in the design is Homebrew-specific: `tsuku create --from homebrew <library>`, Homebrew formula resolution, bottle downloads. No specific guidance is given for how a non-Homebrew library dependency would be discovered or created. This driver is aspirational rather than addressed.

---

## 5. Acceptance Criteria Coverage

Issue acceptance criteria vs. what the design delivers:

| Criteria | Covered? | Notes |
|---|---|---|
| Priority libraries identified across categories (compression, graphics, data, network, crypto) | **No.** The design describes a *process* for discovering libraries but does not identify specific priority libraries. The parent design's "System Dependency Analysis" section lists categories, but this design defers identification to the dry-run discovery phase. The acceptance criteria asks for the libraries to be identified, not just a method to identify them. | Gap. |
| Library recipe creation strategy documented (manual vs templated) | **Yes.** Decision 3 documents this: standard pipeline + manual fixes with friction log. | Covered. |
| Integration with failure analysis documented | **Yes.** The design describes how `missing_dep` failures drive discovery and how `queue-maintain` handles requeue after library creation. | Covered. |
| Success metrics defined | **Yes.** Decision 6 defines three metrics: blocked-to-total ratio, time-to-unblock, new missing_dep count per batch run. | Covered. |
| Platform coverage requirements specified | **Yes.** Decision 5 specifies "match the blocking platform" with reactive gap-fill. | Covered. |

The "priority libraries identified" gap is significant. The issue asks for a concrete list ("libpng, sqlite, curl" etc. across categories), and the design punts this to the dry-run discovery phase. A first-pass list could be built today from existing failure data -- the `blocked_by` fields in legacy JSONL files already name specific libraries (`ada-url`, `bdw-gc`, `gmp`, `notmuch`, `dav1d`, `openssl@3`).

---

## 6. Structural Fit (Architect Perspective)

### Positive

- **No new infrastructure.** The design explicitly avoids building new systems, which is consistent with the project's pattern of using existing tools.
- **Consistent with purpose-built CLI tool pattern.** The design doesn't propose a new `cmd/lib-backfill/` tool -- it uses `tsuku create` directly. This is appropriate since the workflow is operational, not automated.
- **Friction log as feedback mechanism.** This creates a clean separation between the operational process (create libraries) and pipeline improvements (fix deterministic generation failures).
- **Reactive safety net.** Keeping the reactive loop as the backstop avoids over-engineering the proactive pass.

### Concerns

1. **Nonexistent interfaces presented as existing.** The design's "Existing interfaces used (no changes needed)" section lists:
   - `tsuku create` with `--json` flag -- create has no `--json` flag
   - `tsuku create --dry-run` in the process flow -- create has no `--dry-run` flag

   If these need to be built, that's new work that should be scoped. If the design means to use `create` without these flags and parse stderr output, that should be stated.

2. **Failure format assumption mismatch.** The design relies on `blocked_by` fields in failure JSONL records. The newer per-recipe failure format (visible in `batch-2026-02-21T07-27-58Z.jsonl`) does not include `blocked_by`. The design's data flow depends on extracting blocker information from failure records, but the format that will be produced during the discovery dry-run may not contain it. This is either a bug in the newer failure writer or a format evolution the design hasn't accounted for.

3. **Satisfies backfill count may be wrong.** Phase 3 says "Add `satisfies` metadata to the 19 existing library recipes that don't have it." With 22 library recipes, that implies 3 have it. But `libcurl.toml` explicitly says it does NOT have satisfies (comment at line 7). `libnghttp2.toml` has satisfies. `sqlite.toml` has satisfies but isn't typed as library. The actual count of library-typed recipes with satisfies may be 1 (libnghttp2), not 3.

---

## 7. Summary of Findings

### Must address before accepting

1. **Clarify `tsuku create` capabilities.** The process flow references `--dry-run` and `--json` flags that don't exist on `create`. Either scope the work to add them or describe the actual mechanism (run create, capture exit code 8 + stderr).

2. **Verify `blocked_by` availability in current failure format.** The newer per-recipe JSONL format lacks `blocked_by`. The discovery phase depends on this field. Either the failure writer needs updating or the design needs an alternative extraction method.

3. **Add a concrete initial library list.** The acceptance criteria ask for priority libraries identified across categories. Existing failure data already names specific blockers. Include a table derived from that data, even if the dry-run will discover more.

### Should address

4. **Quantify reactive loop delay.** The rejection of "reactive only" claims "many weeks" without showing the math. With current batch parameters (25/hour), the entire pending queue processes in ~5 days. Showing the actual numbers strengthens the case for proactive discovery.

5. **Correct the satisfies backfill count.** Verify which library recipes actually have `satisfies` metadata vs. which don't. The current "3 of 22" count appears to include a non-library recipe and exclude one that has a "no satisfies" comment.

6. **Clarify the blocked vs. failed status distinction.** The design describes entries getting `blocked` but current queue shows entries go to `failed` or `requires_manual`. The requeue path needs to account for the actual status values in use.

### Minor

7. **Sharpen Homebrew API rejection.** The real issue is name resolution mismatch (Homebrew dep names vs. tsuku recipe names), not that it "only covers Homebrew."

8. **The `subcategory` field from PR #1854 is referenced in the context section** but never appears in actual failure data. Verify this was shipped or remove the reference.

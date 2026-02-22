# Architecture Review: DESIGN-structured-error-subcategories

**Reviewer**: architect-reviewer
**Date**: 2026-02-21

## Verdict

The design is architecturally sound. It follows the existing data flow (CLI -> orchestrator -> JSONL -> dashboard), adds a single field at each layer, and doesn't introduce new interfaces or bypass existing ones. Three findings need attention before implementation: a schema contract that will reject the new field, a taxonomy table that claims exit codes the CLI doesn't produce, and an ambiguity in how the generate() path should set subcategories.

---

## 1. Is the architecture clear enough to implement?

**Yes, with caveats on the generate() path.**

The install path is unambiguous: extend `classifyInstallError()` to return a subcategory string, add the field to `installError`, thread it through `parseInstallJSON()` into `FailureRecord`, and read it in the dashboard. The data flow diagram on lines 102-114 is precise.

The generate() path is less clear. The design says "the orchestrator extracts bracketed tags from the output (the existing Level 1 logic) and stores them as the subcategory" (line 149). But this logic currently lives in `extractSubcategory()` in `internal/dashboard/failures.go`, not in the orchestrator. Implementing this means either:

(a) Duplicating the bracket-extraction logic in the orchestrator, or
(b) Moving/exporting it so both can use it.

Option (a) creates a parallel pattern -- two bracket parsers. Option (b) requires deciding which package owns it. The design should specify which approach to use.

**Severity**: Advisory. The ambiguity is contained to one function, and either approach works. But the implementer needs to make a call the design currently dodges.

---

## 2. Are there missing components or interfaces?

### 2a. JSON Schema blocks the new field (Blocking)

`data/schemas/failure-record.schema.json` (line 62) sets `"additionalProperties": false` on failure items. Adding a `subcategory` field to `FailureRecord` in `batch/results.go` means `WriteFailures()` will serialize it into JSONL, but any schema validation will reject records containing the new field.

The schema needs updating to include `subcategory` as an optional property. This isn't mentioned anywhere in the design. If the schema is validated in CI or by consumers, this is a runtime failure.

**Files affected**: `data/schemas/failure-record.schema.json`

### 2b. Dashboard FailureRecord struct needs updating

The design's "Key Changes" section (lines 119-153) mentions updating `batch.FailureRecord` and the dashboard's `loadFailureDetailsFromFile()`, but doesn't mention `dashboard.FailureRecord` (defined at `internal/dashboard/dashboard.go:143`). This is the struct used to unmarshal JSONL records in the dashboard. It currently has no `Subcategory` field. Without it, `json.Unmarshal` will silently drop the subcategory from new records, and the fallback path in `loadFailureDetailsFromFile()` will handle both old and new records identically -- defeating the purpose.

The design does mention that `loadFailureDetailsFromFile()` reads the field from per-recipe records (line 153), so the intent is there. But the struct change isn't called out, and `dashboard.FailureRecord` is a different type from `batch.FailureRecord`.

**Severity**: Advisory. An implementer reading the code will discover this quickly. But noting it prevents a round of confusion.

### 2c. PackageFailure struct (legacy batch format) also needs the field

Similarly, `dashboard.PackageFailure` (at `internal/dashboard/dashboard.go:159`) is the struct for individual failures inside the legacy batch format's `failures[]` array. New batch runs will embed the subcategory in these inner records (since `batch.FailureRecord` gains the field and `WriteFailures()` serializes it into the `failures[]` array). The dashboard's `PackageFailure` struct also needs the field to read it back.

**Severity**: Advisory. Same reasoning as 2b.

---

## 3. Are the implementation phases correctly sequenced?

**Yes.** Phase 1 (CLI output) -> Phase 2 (orchestrator passthrough) -> Phase 3 (dashboard preference) follows the data flow direction. Each phase is independently deployable:

- After Phase 1 alone: CLI emits subcategories in JSON but nobody reads them. No breakage.
- After Phase 1+2: Subcategories are stored in JSONL but the dashboard still uses heuristics. No breakage.
- After all three: Dashboard prefers the structured field.

The current `extractSubcategory()` call in `loadFailureDetailRecords()` (lines 135-141 of `failures.go`) unconditionally overwrites `Subcategory` for every record. Phase 3 must change this to conditional (only when empty). This is correctly described in the design.

---

## 4. Are there simpler alternatives we overlooked?

No. The design is already the simple option. The alternatives section correctly identifies and rejects the more complex approaches (backfill migration, orchestrator-level classification). The chosen approach adds one field to three structs and updates one function's return type. There isn't a simpler path that achieves the same goal.

One minor simplification the design could consider: rather than teaching the orchestrator to extract bracketed tags for the generate() path (duplicating dashboard logic), the dashboard could continue using `extractSubcategory()` for generate-path records (which don't come from `tsuku install --json`). This would mean: only the install path gets structured subcategories; generate-path records continue using heuristic parsing, which already handles bracketed tags well. This reduces the scope of Phase 2 and avoids the bracket-extraction duplication question from Finding 1.

---

## 5. Is the problem statement specific enough to evaluate solutions against?

**Yes.** The problem is concrete: `extractSubcategory()` misclassifies `recipe_not_found` errors as `verify_pattern_mismatch` because the suggestion text contains "Verify." The design traces this to the root cause (text parsing in the wrong layer) and proposes a structural fix (classify where typed error info is available). The scope section clearly bounds what changes and what doesn't.

---

## 6. Are there missing alternatives we should consider?

No significant alternatives are missing. The three options considered (CLI-level classification, orchestrator-level classification, tighter heuristics) cover the reasonable design space. A fourth option -- embedding the subcategory in the error message itself with a structured prefix (e.g., `[subcategory:verify_failed] actual message`) -- would be worse than a dedicated JSON field and isn't worth discussing.

---

## 7. Is the rejection rationale for each alternative specific and fair?

**Yes.** Each rejection cites a concrete deficiency:

- Orchestrator-level classification: "It would need to reparse error messages, which is the same fragile approach we're replacing." Accurate -- the orchestrator only sees stdout/stderr, not typed errors.
- Tighter heuristics: "Each fix is reactive: we discover a false positive, patch the regex, and wait for the next one." This correctly identifies the maintenance trajectory.
- Backfill migration: "A bulk migration creates a large diff, risks merge conflicts with in-flight batch runs." Practical concern, and the records age out naturally.
- Drop heuristics immediately: "The dashboard would temporarily lose subcategory information for all existing records." Correctly identifies the regression.

None of these are straw-man rejections. Each alternative has a genuine structural drawback.

---

## Findings Summary

### Blocking

| # | Finding | Location |
|---|---------|----------|
| 1 | JSON schema has `additionalProperties: false` and will reject the new `subcategory` field. Schema must be updated alongside the Go struct. | `data/schemas/failure-record.schema.json:62` |

### Advisory

| # | Finding | Location |
|---|---------|----------|
| 2 | Subcategory taxonomy table (lines 159-172) lists exit codes 4 and 7 as if `classifyInstallError()` produces them, but it doesn't. Exit 4 (`ExitVersionNotFound`) is never returned by that function. Exit 7 (`ExitVerifyFailed`) is only used by the `verify` command, not `install`. The design should clarify whether Phase 1 will also extend `classifyInstallError()` to produce these exit codes, or whether those rows describe future/aspirational mappings. | `cmd/tsuku/install.go:300-318`, design lines 159-172 |
| 3 | The generate() path's subcategory extraction requires either duplicating the bracket-parsing logic from `extractSubcategory()` or extracting it into a shared function. The design doesn't specify which. Consider the simpler alternative: let the dashboard continue using `extractSubcategory()` for generate-path records, limiting structured subcategories to the install path only. | `internal/batch/orchestrator.go:320-377`, design line 149 |
| 4 | `dashboard.FailureRecord` (`internal/dashboard/dashboard.go:143`) and `dashboard.PackageFailure` (`internal/dashboard/dashboard.go:159`) both need `Subcategory` fields to unmarshal the new data. The design's Key Changes section only mentions `batch.FailureRecord`, not the dashboard's separate types. | `internal/dashboard/dashboard.go:143-165` |

### Observations (not findings)

- The two `categoryFromExitCode()` functions (one in `cmd/tsuku/install.go`, one in `internal/batch/orchestrator.go`) intentionally diverge. The design correctly avoids touching this -- a subcategory is orthogonal to the category. Good.
- `FailureDetail.Subcategory` already exists in the dashboard (`internal/dashboard/failures.go:24`), so the output struct is ready. The work is in the input/loading path.
- The backward compatibility approach (prefer structured, fall back to heuristic) is the standard pattern for this kind of migration and is correctly described.

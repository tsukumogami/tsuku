# Architecture Review: Update Outcome Telemetry

## 1. Is the architecture clear enough to implement?

**Yes, with minor gaps noted below.**

The design is implementation-ready for the most part. The struct definition, blob layout, API contract, file list, and emission points are all specified precisely enough that an implementer wouldn't need to make ambiguous judgment calls.

Strengths:
- The `UpdateOutcomeEvent` struct is fully specified with JSON tags and blob mapping
- The error taxonomy is enumerated with clear "when used" descriptions
- The API response contract includes a concrete JSON example
- File-by-file change descriptions are specific about what changes in each file
- Security considerations address the critical privacy boundary (classifyError)

Gaps that would surface during implementation:

- **classifyError() matching strategy is unspecified.** The design says "maps Go errors to taxonomy values" but doesn't define the matching mechanism. Go errors in the install flow come from multiple packages (net/http, os, archive/tar, etc.) and are often wrapped with `fmt.Errorf("... %w", err)`. The implementer needs guidance on whether to use `errors.Is`, `errors.As`, string matching, or a type-switch approach. Recommendation: specify `errors.Is` / `errors.As` for standard library errors and `strings.Contains` as a fallback, with `unknown` as the catch-all.

- **"Tag success event trigger" in update.go is vague.** The design says manual success should emit both the existing `Event{Action: "update"}` and a new `UpdateOutcomeEvent`. But "tag" isn't a defined operation. Looking at the code, `update.go` line 110 fires `telemetry.NewUpdateEvent()` after successful install. The implementer just needs to add a `client.SendUpdateOutcome(telemetry.NewUpdateOutcomeSuccess(...))` call alongside it. The design should say "emit an additional UpdateOutcomeEvent" not "tag."

- **stats.js doesn't exist.** The design references `website/stats/stats.js` as a modified file, but the dashboard JavaScript is inline in `website/stats/index.html` (lines 331-570). The implementer needs to either extract JS to a separate file (scope creep) or add code inline. Since the existing pattern is inline, adding inline is the right call. The file list should be corrected.

## 2. Are there missing components or interfaces?

**Two missing items, one cosmetic.**

- **Missing: `MaybeAutoApply` signature change needs propagation.** Adding `*telemetry.Client` as a parameter changes `MaybeAutoApply`'s signature. The only call site is in `cmd/tsuku/main.go` line 75. The design mentions modifying `main.go` to "create telemetry client early, pass to MaybeAutoApply" but doesn't note that the `PersistentPreRun` closure currently passes `nil` as the telemetry client to `runInstallWithTelemetry`. The telemetry client created in `PersistentPreRun` for auto-apply needs to be the same one passed to `MaybeAutoApply` for outcome events. This is straightforward but should be explicit.

- **Missing: telemetry client for cmd_rollback.go.** The rollback command currently has no telemetry client. Looking at the code (cmd_rollback.go), it doesn't import or instantiate a telemetry client. The design says "add rollback event after mgr.Activate" but doesn't note that a `telemetry.NewClient()` call is needed in the rollback command's Run function. This follows the same pattern as update.go line 30-31.

- **Cosmetic: UpdateOutcomeActionType union type missing from worker spec.** The design specifies the Go struct and worker validation but doesn't define the TypeScript type alias for the action union (`"update_outcome_success" | "update_outcome_failure" | "update_outcome_rollback"`). Minor, but the existing code defines type aliases for every event category.

## 3. Are the implementation phases correctly sequenced?

**Yes. The phasing is correct and follows the only viable dependency order.**

- Phase 1 (CLI) must come first: the worker can't be tested without knowing the exact payload shape
- Phase 2 (Worker) depends on Phase 1: needs the action prefix and field names to write validation
- Phase 3 (Dashboard) depends on Phase 2: needs the stats endpoint to fetch from

One optimization: Phase 1 and Phase 2 can be developed in parallel if the API contract (which is already specified in the design) is treated as the shared interface. The CLI tests can use a mock HTTP server, and the worker tests can use hardcoded payloads matching the contract. This is standard practice and the design's contract section is precise enough to enable it.

Within Phase 1, the natural order is:
1. `UpdateOutcomeEvent` struct + constructors + `classifyError()` in event.go
2. `SendUpdateOutcome()` in client.go
3. Instrumentation in apply.go (requires signature change)
4. main.go wiring (passes client to MaybeAutoApply)
5. update.go and cmd_rollback.go instrumentation
6. Unit tests

This is implicitly correct in the design but not explicitly stated.

## 4. Are there simpler alternatives we overlooked?

**No fundamental simplification available. Two minor simplifications worth considering.**

The design already chose the simplest viable approach at each decision point. The separate-struct pattern is well-validated (5 precedents), direct emission avoids abstraction, and a dedicated endpoint follows established conventions.

Minor simplifications:

- **Skip the dashboard in the initial implementation.** Phase 3 adds user-visible value but is the least critical component. The stats endpoint (Phase 2) is queryable via curl, and the data can be analyzed directly. If the goal is to ship outcome tracking quickly, Phase 3 could be deferred to a follow-up without blocking the data collection pipeline. The design's phasing already supports this since Phase 3 is independent.

- **Combine success and rollback into one action with a field.** Instead of three distinct action values (`update_outcome_success`, `update_outcome_failure`, `update_outcome_rollback`), use a single `update_outcome` action with an `outcome` field (`"success"`, `"failure"`, `"rollback"`). This would simplify the worker dispatch (one action to match instead of prefix-matching three), reduce the type surface, and let Analytics Engine queries filter on a blob rather than grouping by action strings. The trade-off is one fewer blob available (blob0 is action, blob1 would be outcome) but the layout only uses 10 of 20 blobs so this is not a constraint. **However**, the chosen approach is consistent with how discovery events use `discovery_registry_hit`, `discovery_not_found`, etc. as distinct actions. Consistency across the codebase outweighs the marginal simplification, so the design's choice is correct.

## Summary of Recommendations

1. **Specify classifyError() matching strategy** -- define whether to use `errors.Is`, `errors.As`, or string matching, and in what order.
2. **Correct the stats.js reference** -- JS is inline in index.html; update the file list.
3. **Note the telemetry client instantiation needed in cmd_rollback.go** -- currently has no telemetry client.
4. **Clarify "tag success event"** -- replace with "emit an additional UpdateOutcomeEvent alongside the existing Event."
5. **Consider deferring Phase 3** -- data collection is the primary value; dashboard can follow.

None of these are blocking issues. The design is solid and ready for implementation planning.

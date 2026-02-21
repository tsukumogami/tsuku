# Scrutiny Review: Justification Focus -- Issue #1278

**Issue**: #1278 (re-order queue entries within tiers by blocking impact)
**Scrutiny focus**: justification
**Reviewer perspective**: Evaluate quality of deviation explanations

## Requirements Mapping Under Review

| AC | Claimed Status | Claimed Reason |
|----|---------------|----------------|
| scoring formula | implemented | -- |
| Go tool | implemented | -- |
| reuse dashboard computation | deviated | "reimplemented same algorithm - dashboard funcs unexported and coupled to dashboard types" |
| tier boundaries preserved | implemented | -- |
| periodic maintenance step | implemented | -- |

## Deviation Analysis

### AC: "reuse dashboard computation" (deviated)

**Issue body text**: "Reuse the existing transitive blocker computation from `internal/dashboard/dashboard.go` rather than reimplementing"

**Claimed reason**: "reimplemented same algorithm - dashboard funcs unexported and coupled to dashboard types"

#### Reason Quality Assessment

The reason identifies two specific technical barriers:
1. The dashboard functions are unexported (lowercase: `computeTransitiveBlockers`, `buildBlockerCountsFromQueue`, `computeTopBlockers`)
2. The functions are coupled to dashboard-specific types (`PackageInfo`, `Blocker`)

Both claims are factually accurate. Verified in the diff and source:
- `internal/dashboard/dashboard.go:473` defines `func computeTransitiveBlockers(dep string, ...)` -- lowercase, unexported
- `internal/dashboard/dashboard.go:501` defines `func buildBlockerCountsFromQueue(packages []PackageInfo)` -- takes `PackageInfo` slices, not `batch.QueueEntry`
- `internal/dashboard/dashboard.go:511` defines `func computeTopBlockers(blockers map[string][]string, limit int) []Blocker` -- returns `Blocker` type

The deviation explanation identifies what was traded away: code reuse. The reason given is a genuine technical constraint rather than convenience. Exporting the dashboard functions would require either:
1. Exporting them plus their associated types, which would expand the public API surface of the dashboard package for a single consumer
2. Refactoring into a shared package, which is a larger effort beyond this issue's scope

The implementation in `internal/reorder/reorder.go:256-279` (`computeTransitiveBlockers`) is structurally identical to `internal/dashboard/dashboard.go:473-497`. Same algorithm, same signature pattern, same cycle detection mechanism (memo map with 0-initialization), same deduplication approach. The code comment at line 255-256 explicitly notes: "This is the same algorithm used in internal/dashboard/dashboard.go."

#### Alternative Depth Assessment

The mapping does not include an `alternative_considered` field. The deviation reason is terse -- it states the constraint but doesn't describe what alternatives were evaluated. Possible alternatives that could have been considered:

1. **Export the dashboard functions**: Move `computeTransitiveBlockers` to an exported function. This would work for the algorithm itself, but `buildBlockerCountsFromQueue` takes `[]PackageInfo` while the reorder package works with JSONL failure files and `batch.QueueEntry` types. The input data shapes are different enough that only the core recursive function would be directly reusable.

2. **Extract shared `internal/blockers` package**: Create a new package containing the common algorithm with generic types. More principled but a larger refactoring effort for a single shared function.

3. **Import dashboard package and adapt**: Call dashboard functions through an adapter. But since the functions are unexported, this isn't possible without modifying the dashboard package.

The absence of documented alternatives is a minor gap. The deviation reason is clear enough that the technical constraint is self-evident, but for a cleaner audit trail, stating "considered exporting dashboard functions but they're coupled to PackageInfo types; extracting a shared package was out of scope for this issue" would have been stronger.

#### Avoidance Pattern Check

The deviation does not use any concerning avoidance language ("too complex," "not needed yet," "can be added later," "out of scope"). The reason is a factual statement about unexported functions and type coupling. This is not disguising a shortcut -- it's describing a real Go visibility constraint.

#### Proportionality Assessment

This is the only deviation among 5 ACs. The 4 implemented ACs cover the core functionality: scoring formula, Go tool, tier boundaries, and periodic maintenance integration. The one deviation is on a code reuse AC rather than a functional AC. The implementation still achieves the functional goal (transitive blocker computation) by reimplementing the same algorithm. The deviation is proportionate -- it affects code organization, not behavior.

## Findings

### Finding 1: Deviation reason is genuine but lacks alternative documentation (advisory)

**Severity**: advisory

The deviation reason accurately identifies real technical constraints (unexported functions, type coupling). The reimplemented algorithm is structurally identical to the dashboard version, confirming it's not a simplification. However, the mapping omits what alternatives were considered. Documenting even briefly that "exporting the functions would couple the dashboard API to batch types" would make the deviation more defensible for future readers.

### Finding 2: Refactoring opportunity noted but not deferred (advisory)

**Severity**: advisory

The comment in `internal/reorder/reorder.go:255` acknowledges the code is duplicated ("This is the same algorithm used in internal/dashboard/dashboard.go"). There is no issue tracking the future extraction of this algorithm into a shared package. For two consumers of the same algorithm, this is acceptable. If a third consumer appears, the duplication debt grows. This is not blocking for this issue -- the duplication is conscious and documented in-code.

## Summary

The single deviation in this issue is well-justified. The technical barriers (unexported Go functions, type coupling to dashboard-specific types) are real and verified in the source. The reimplemented code is structurally identical to the original, confirming the algorithm was faithfully reproduced rather than simplified. No avoidance patterns detected. The deviation is proportionate -- it's the only one among 5 ACs, and it affects code organization rather than functional behavior. Two advisory findings: the mapping could document considered alternatives more explicitly, and the duplication could be tracked for future extraction if more consumers emerge.

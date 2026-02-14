# Issue 1654 Summary

## What Was Implemented

Added `DisambiguationRecord` tracking to the batch package for recording disambiguation decisions during batch recipe generation. Extended the discover package with selection reason tracking and deterministic selection mode for batch workflows.

## Changes Made

- `internal/batch/results.go`: Added `DisambiguationRecord` type with fields for tool, selected source, alternatives, selection reason, downloads ratio, and high-risk flag. Extended `BatchResult` to include disambiguations slice. Added `WriteDisambiguations()` function for persisting records. Updated `Summary()` to include disambiguation statistics.

- `internal/discover/disambiguate.go`: Added selection reason constants (`single_match`, `10x_popularity_gap`, `priority_fallback`). Updated `disambiguate()` to populate selection metadata in results and support `forceDeterministic` mode for batch workflows.

- `internal/discover/resolver.go`: Extended `Metadata` struct with `SelectionReason`, `Alternatives`, and `DownloadsRatio` fields for tracking disambiguation decisions.

- `internal/discover/ecosystem_probe.go`: Added `WithForceDeterministic()` option for batch mode deterministic selection without callbacks.

- `internal/batch/disambiguation_test.go`: Added comprehensive unit tests for record fields, selection reasons, JSON marshaling, BatchResult summary, and WriteDisambiguations.

## Key Decisions

- **Selection reason stored in Metadata**: Instead of creating a new wrapper type, added disambiguation fields to the existing `Metadata` struct. This maintains compatibility with existing code that reads `DiscoveryResult`.

- **ForceDeterministic mode**: Added a separate mode flag rather than relying on callback absence. This makes the intent explicit - batch mode wants deterministic selection AND tracking, not just silent errors.

- **Priority fallback = HighRisk**: When selection uses `priority_fallback` (no clear popularity winner), the record should be flagged as high-risk for human review.

## Trade-offs Accepted

- **Orchestrator integration deferred**: The current orchestrator uses `--from` which bypasses disambiguation. Full integration requires either removing `--from` from batch mode or adding a separate discovery phase. This is acceptable as the batch package now has all the types and tracking infrastructure needed.

## Test Coverage

- New tests added: 5 test functions with 6 sub-tests
- Coverage: All `DisambiguationRecord` functionality tested including JSON serialization, file writing, and BatchResult summary generation

## Known Limitations

- The orchestrator currently passes `--from` explicitly, so disambiguation doesn't occur during batch runs. The tracking infrastructure is in place but full integration requires workflow changes.

## Future Improvements

- Add `--json` flag to `tsuku create` to expose disambiguation info
- Update orchestrator to use discovery instead of explicit `--from`
- Add dashboard visualization for high-risk disambiguation decisions (issue #1655)

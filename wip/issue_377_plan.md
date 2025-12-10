# Issue 377 Implementation Plan

## Summary

Add progress indicators during LLM recipe generation to show users the workflow stages: metadata fetch, LLM analysis, container validation, and repair attempts.

## Approach

Add a `ProgressReporter` callback interface to the `GitHubReleaseBuilder` that reports workflow stages. The CLI creates a reporter that prints to stdout. This keeps the builder testable (pass nil or mock reporter) while enabling user feedback.

### Alternatives Considered

- **Direct print statements in builder**: Would work but makes testing harder and couples builder to output format.
- **Separate progress package**: Overkill for this use case; a simple callback is sufficient.
- **Channels for progress**: More complex, not needed for synchronous workflow.

## Files to Modify

- `internal/builders/github_release.go` - Add ProgressReporter interface and callbacks
- `cmd/tsuku/create.go` - Create and pass progress reporter to builder

## Files to Create

None - all changes fit within existing files.

## Implementation Steps

- [ ] Add `ProgressReporter` interface to builders package with stage reporting methods
- [ ] Add `WithProgressReporter` option to `GitHubReleaseBuilder`
- [ ] Add progress callbacks at key points: fetch metadata, LLM analysis, validation, repair
- [ ] Create CLI progress reporter implementation in create.go
- [ ] Pass reporter to builder in runCreate
- [ ] Add unit tests for progress reporting

## Progress Stages

Based on the issue's acceptance criteria:

1. **Fetching metadata**: "Fetching release metadata... done (v2.42.0, 24 assets)"
2. **LLM analysis**: "Analyzing assets with Claude... done" (includes provider name)
3. **Validation**: "Validating in container... done" or "Validating in container... failed"
4. **Repair**: "Repairing recipe (attempt N/3)... done"

## Interface Design

```go
// ProgressReporter receives progress updates during recipe generation.
type ProgressReporter interface {
    // OnStageStart is called when a stage begins.
    OnStageStart(stage string)
    // OnStageDone is called when a stage completes successfully.
    OnStageDone(detail string)
    // OnStageFailed is called when a stage fails.
    OnStageFailed()
}
```

## Testing Strategy

- Unit tests for:
  - Progress reporter is called at correct points
  - Progress reporter handles nil gracefully
  - Stage names are correct
  - Details are properly formatted

## Risks and Mitigations

- **Output format changes**: The issue specifies exact output format; follow it precisely.
- **Test flakiness**: Use mock reporter in tests, not stdout capture.

## Success Criteria

- [ ] Progress shows during metadata fetch with version and asset count
- [ ] Progress shows during LLM analysis with provider name
- [ ] Progress shows during container validation
- [ ] Failed validation shows "failed" then repair progress
- [ ] Repair attempts numbered (attempt N/3)
- [ ] All existing tests pass
- [ ] `go vet` and `golangci-lint` pass

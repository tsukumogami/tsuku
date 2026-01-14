# Issue 880 Implementation Plan

## Summary

Fix `ResolvePythonStandalone()` and `ResolvePipx()` to respect the `$TSUKU_HOME` environment variable instead of hardcoding `~/.tsuku/tools/`.

## Root Cause Analysis

The bug is in `internal/actions/util.go`:

1. `ResolvePythonStandalone()` (line 322) hardcodes `~/.tsuku/tools/`
2. `ResolvePipx()` (line 362) hardcodes `~/.tsuku/tools/`

Meanwhile, `CheckEvalDeps()` in `eval_deps.go` correctly checks `$TSUKU_HOME` first:
```go
if tsukuHome := os.Getenv("TSUKU_HOME"); tsukuHome != "" {
    return filepath.Join(tsukuHome, "tools")
}
```

In CI, each test uses a separate `TSUKU_HOME`:
```bash
export TSUKU_HOME="/Users/runner/work/_temp/tsuku-libsixel-source"
```

When python-standalone is installed, it goes to `$TSUKU_HOME/tools/python-standalone-VERSION`, but `ResolvePythonStandalone()` looks in `~/.tsuku/tools/` - the wrong location.

## Approach

Extract the common `getToolsDir()` pattern from `eval_deps.go` into a shared helper, then update both `ResolvePythonStandalone()` and `ResolvePipx()` to use it.

### Alternatives Considered

1. **Duplicate the TSUKU_HOME check in each function**: Works but violates DRY
2. **Pass toolsDir as parameter**: Would require changing all callers
3. **Extract shared helper**: Cleanest approach, already exists in eval_deps.go

## Files to Modify

- `internal/actions/util.go`: Update `ResolvePythonStandalone()` and `ResolvePipx()` to respect $TSUKU_HOME
- `internal/actions/eval_deps.go`: Export `getToolsDir()` or move to util.go
- `.github/workflows/build-essentials.yml`: Re-enable libsixel-source test

## Implementation Steps

- [x] Move `getToolsDir()` from eval_deps.go to util.go (or export it)
- [x] Update `ResolvePythonStandalone()` to use the shared helper
- [x] Update `ResolvePipx()` to use the shared helper
- [x] Update all other Resolve* functions to use the shared helper
- [x] Re-enable libsixel-source in macOS Apple Silicon CI workflow
- [x] Run tests locally
- [x] Verify with fresh TSUKU_HOME simulation

## Testing Strategy

- Unit tests: Add test for `ResolvePythonStandalone()` with $TSUKU_HOME override
- Integration: CI run will verify libsixel-source works with fresh TSUKU_HOME

## Risks and Mitigations

- **Risk**: Breaking existing behavior for default case
- **Mitigation**: Keep fallback to ~/.tsuku when TSUKU_HOME not set (same as current eval_deps.go behavior)

## Success Criteria

- [ ] libsixel-source test passes on macOS Apple Silicon CI
- [ ] All existing tests pass
- [ ] `ResolvePythonStandalone()` respects $TSUKU_HOME

## Open Questions

None - the fix is straightforward.

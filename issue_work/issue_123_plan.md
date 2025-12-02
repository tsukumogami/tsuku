# Issue 123 Implementation Plan

## Summary

Add integration test for Go tool installation that validates the complete installation flow including automatic Go toolchain bootstrap.

## Approach

1. Fix ResolveGo bug that incorrectly matches go-migrate and similar tools
2. Create local test recipes for Go toolchain and gofumpt
3. Validate the full installation flow with local registry
4. Add test matrix entries for CI integration

### Alternatives Considered

- **Docker-only testing**: Would require recipes in public registry first
- **Mock HTTP server in Go tests**: Added complexity; local file server is simpler

## Files Modified

- `internal/actions/util.go` - Fix ResolveGo to only match go-<version> directories
- `internal/actions/util_test.go` - Add tests for ResolveGo fix
- `test-matrix.json` - Add T52 (go toolchain) and T53 (gofumpt) test entries

## Files Created

- `issue_work/issue_123_baseline.md` - Baseline documentation
- `issue_work/issue_123_plan.md` - This file
- `issue_work/issue_123_summary.md` - Implementation summary
- `tmp/registry/recipes/g/go.toml` - Local Go toolchain recipe (gitignored)
- `tmp/registry/recipes/g/gofumpt.toml` - Local gofumpt recipe (gitignored)

## Testing Strategy

1. Unit tests for ResolveGo fix (in util_test.go)
2. Manual integration test with local registry (validated)
3. Test matrix entries for future CI integration (T52, T53)

## Success Criteria

- [x] ResolveGo correctly ignores go-migrate and similar tools
- [x] Go toolchain installation works with local registry
- [x] gofumpt installation works (auto-installs Go as dependency)
- [x] Test matrix entries added for CI
- [ ] Registry recipes submitted (separate PR to tsuku-registry)

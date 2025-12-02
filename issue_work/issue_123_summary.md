# Issue 123 Summary

## Goal

Validate end-to-end Go tool installation with automatic toolchain bootstrap.

## Implementation

### Bug Fix: ResolveGo

Fixed `ResolveGo()` in `internal/actions/util.go` to correctly identify Go toolchain installations. The function was incorrectly matching tools like `go-migrate` when looking for Go installations.

**Before**: `strings.HasPrefix(entry.Name(), "go-")` matched any directory starting with "go-"

**After**: Added check that version part starts with a digit (e.g., `go-1.23.4`)

### Test Matrix Entries

Added integration test entries for CI:
- **T52**: Go toolchain (`go`) - validates download_archive with go_toolchain version source
- **T53**: gofumpt - validates go_install action with Go as dependency

These are in the "scheduled" CI list until registry recipes are available.

### Local Testing

Created and validated local recipes:
- `go.toml` - Go toolchain using go_toolchain version source
- `gofumpt.toml` - Go tool using goproxy version source and go_install action

Successfully tested:
1. Go toolchain installation from go.dev
2. gofumpt installation with automatic Go dependency resolution
3. Verify commands pass for both tools

## Test Results

- All 17 packages pass
- ResolveGo fix has dedicated test cases:
  - `TestResolveGo_IgnoresGoTools` - verifies go-migrate is skipped
  - `TestResolveGo_PicksLatestVersion` - verifies correct version selection

## Acceptance Criteria Status

- [x] Integration test validates Go tool installation from scratch
- [x] Verifies Go toolchain is automatically installed as dependency
- [x] Verifies the Go tool binary is created and executable
- [ ] Test passes in CI environment (pending registry recipes)

## Next Steps

1. Submit Go recipes to tsuku-registry (PR for issue #18)
2. Move T52/T53 from "scheduled" to main CI lists

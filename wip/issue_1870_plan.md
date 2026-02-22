# Issue 1870 Implementation Plan

## Summary

Fix `GetToolsDir()` to resolve `TSUKU_HOME` to an absolute path before joining with `/tools`, and apply the same resolution to the 3 `os.Getenv("TSUKU_HOME")` calls in `findCargoForEval()` and `findBundlerForEval()` / `generateGemfileLock()` / `getRubyVersionForGem()`.

## Approach

The fix applies absolute-path resolution at the lowest level possible, so all callers inherit correct behavior without modification. `GetToolsDir()` is the single function that 9+ Resolve* callers use, so fixing it there covers the majority of the bug. The remaining 4 direct `os.Getenv("TSUKU_HOME")` calls in `cargo_install.go` and `gem_install.go` need the same treatment because they build paths independently.

A helper function `resolveTsukuHome()` centralizes the pattern of reading TSUKU_HOME and converting it to absolute, preventing future code from repeating this logic.

### Alternatives Considered

- **Fix at caller sites (each Resolve* function)**: Would require changing 9+ functions and is fragile -- any new caller of `GetToolsDir()` would need to remember to resolve the path. Rejected because the issue is in the shared utility, not the callers.
- **Resolve TSUKU_HOME once at program startup**: Would require threading the resolved value through many call chains or using a package-level variable. More invasive than necessary and changes initialization order. Rejected for scope creep.
- **Fix only `GetToolsDir()` and ignore `findCargoForEval`/`findBundlerForEval`**: Those functions build paths from `TSUKU_HOME` independently (not via `GetToolsDir()`). They'd remain broken with relative paths. Rejected because it leaves known broken code paths.

## Files to Modify

- `internal/actions/util.go` - Add `resolveTsukuHome()` helper; update `GetToolsDir()` to use it
- `internal/actions/cargo_install.go` - Update `findCargoForEval()` to use `resolveTsukuHome()`
- `internal/actions/gem_install.go` - Update `findBundlerForEval()`, `generateGemfileLock()`, and `getRubyVersionForGem()` to use `resolveTsukuHome()`
- `internal/actions/util_test.go` - Add tests for `GetToolsDir()` with relative, absolute, and unset TSUKU_HOME

## Files to Create

None.

## Implementation Steps

- [ ] Add `resolveTsukuHome()` helper to `internal/actions/util.go` that reads `TSUKU_HOME`, converts relative paths to absolute via `filepath.Abs()`, and falls back to `~/.tsuku` when unset
- [ ] Update `GetToolsDir()` to call `resolveTsukuHome()` instead of raw `os.Getenv("TSUKU_HOME")`
- [ ] Update `findCargoForEval()` in `cargo_install.go` to call `resolveTsukuHome()` instead of `os.Getenv("TSUKU_HOME")` + manual fallback
- [ ] Update `findBundlerForEval()` in `gem_install.go` to call `resolveTsukuHome()` instead of `os.Getenv("TSUKU_HOME")` + manual fallback
- [ ] Update `generateGemfileLock()` in `gem_install.go` (2 occurrences at lines 454 and 470) to call `resolveTsukuHome()` instead of `os.Getenv("TSUKU_HOME")` + manual fallback
- [ ] Update `getRubyVersionForGem()` in `gem_install.go` to call `resolveTsukuHome()` instead of `os.Getenv("TSUKU_HOME")` + manual fallback
- [ ] Add unit tests for `GetToolsDir()` covering: absolute TSUKU_HOME, relative TSUKU_HOME, and unset TSUKU_HOME (falls back to ~/.tsuku/tools)
- [ ] Add unit test for `resolveTsukuHome()` directly to verify absolute resolution
- [ ] Run `go test ./internal/actions/...` to confirm all tests pass
- [ ] Run `go vet ./...` to confirm no issues

## Testing Strategy

- **Unit tests**: Test `GetToolsDir()` with `t.Setenv("TSUKU_HOME", ...)` for three cases:
  1. Absolute path: `TSUKU_HOME=/tmp/foo` should return `/tmp/foo/tools`
  2. Relative path: `TSUKU_HOME=relative/path` should return `<cwd>/relative/path/tools` (absolute)
  3. Unset: `TSUKU_HOME=""` should return `<home>/.tsuku/tools`
- **Unit tests**: Test `resolveTsukuHome()` for the same three cases, verifying it always returns an absolute path or falls back to `~/.tsuku`
- **Verify existing tests pass**: The existing 50+ test files in `internal/actions/` must continue to pass
- **Manual verification**: Build with `go build -o tsuku ./cmd/tsuku`, then run with `TSUKU_HOME=.tsuku-test ./tsuku install --recipe recipes/c/cargo-expand.toml` (if rust is available) to confirm decomposed actions work

## Risks and Mitigations

- **`filepath.Abs()` failure**: If the working directory has been deleted (extremely rare), `filepath.Abs()` could fail. Mitigation: fall through to the original relative path on error, which preserves current behavior in this edge case.
- **Changed behavior for callers expecting relative paths**: No caller should depend on relative paths from `GetToolsDir()` since the returned path is used for file operations. Absolute paths are strictly better. Risk is negligible.
- **`homebrew_relocate.go` also reads TSUKU_HOME directly**: This code path (line 60) has the same potential issue but constructs `libsDir` rather than tool paths. It doesn't change `cmd.Dir`, so relative paths still work from the original CWD. Noted but not in scope for this fix -- the bug report specifically covers decomposed actions.

## Success Criteria

- [ ] `GetToolsDir()` returns an absolute path when `TSUKU_HOME` is set to a relative path
- [ ] `findCargoForEval()` uses an absolute tsuku home path
- [ ] `findBundlerForEval()` uses an absolute tsuku home path
- [ ] `generateGemfileLock()` uses an absolute tsuku home path (both occurrences)
- [ ] `getRubyVersionForGem()` uses an absolute tsuku home path
- [ ] All existing tests in `internal/actions/` pass
- [ ] New tests cover the relative-path scenario
- [ ] `go vet ./...` reports no issues

## Open Questions

None. The fix is well-scoped and the implementation context document confirms the root cause and suggested approach.

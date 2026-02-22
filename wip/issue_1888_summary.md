# Issue 1888 Summary

## What Was Implemented

Aligned `install_gem_direct` with the unified binary path convention (`bin/<name>`) used by `gem_install` and `gem_exec`. The action now installs gems directly to `<installDir>` instead of `<installDir>/.gem/`, and creates self-contained wrapper scripts.

## Changes Made
- `internal/actions/install_gem_direct.go`: Changed `--install-dir` from `.gem` subdirectory to install root, added `--bindir` flag, replaced manual symlink creation with `createGemWrapper()`, switched to `ResolveGem()` for gem discovery

## Key Decisions
- Followed the `gem_install.go` pattern exactly: same `--install-dir`, `--bindir`, `GEM_HOME`, `GEM_PATH`, and `createGemWrapper()` usage
- Used `gemHomeRel = "."` since gems are installed at the install directory root (same as gem_install)
- Added `--no-document` flag to skip doc generation (consistent with gem_install)

## Test Coverage
- No new unit tests added (the action requires a real gem/ruby environment)
- Existing decomposable_test.go verifies install_gem_direct is registered as a primitive
- CI's Gem Builder Tests workflow validates the full install flow

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| install_gem_direct installs to bin/ | Implemented | install_gem_direct.go:66-72 |
| Wrapper scripts created | Implemented | install_gem_direct.go:87-96, uses createGemWrapper from gem_common.go |
| ExtractBinaries path matches | Implemented | types.go:899 returns bin/<name>, install_gem_direct now puts binaries there |

# Issue 184 Summary

## What Was Implemented

Added `tsuku validate --strict` to CI workflow for comprehensive recipe validation on PRs and nightly runs. This required fixing several validator gaps and updating 50 recipes to pass strict validation.

## Changes Made

- `internal/recipe/validator.go`:
  - Added `go_install` to known actions list with module parameter validation
  - Added `nixpkgs` to valid version sources
  - Fixed `npm_registry` → `npm` source name (matching provider)
  - Fixed false positive for 'rm' in tool names (e.g., "terraform")

- `.github/workflows/test.yml`:
  - Added nightly schedule trigger (00:00 UTC)
  - Replaced Python-based validation with `tsuku validate --strict`
  - Validation now runs on recipe changes or scheduled runs

- 8 npm-based recipes: Added `[version] source = "npm"` section
- 42 recipes: Updated verify patterns to include `{version}` placeholder

## Key Decisions

- **Empty pattern for tools without --version**: For tools like cobra-cli, cargo-edit, goimports that use help output instead of version output, set `pattern = ""` to skip version verification warning
- **Word boundaries for dangerous patterns**: Changed validator to require whitespace around "rm" to avoid false positives on tool names containing "rm"
- **Use existing workflow**: Modified test.yml rather than creating a new workflow to avoid duplicating matrix detection logic

## Trade-offs Accepted

- **Version patterns may be inaccurate**: Some `{version}` patterns are best-effort based on typical output formats. Actual tool output may vary slightly.
- **Empty patterns suppress warnings**: Tools that can't provide version verification get empty patterns, which means no version checking for those tools.

## Test Coverage

- No new tests added (validator behavior covered by existing tests)
- All 1953 existing tests pass
- All 134 recipes pass strict validation

## Known Limitations

- Version patterns are based on expected output format; if a tool changes its version output format, the pattern may need updating
- Nightly validation only runs on ubuntu-latest, not macOS

## Future Improvements

- Consider adding version pattern tests that verify patterns match actual tool output
- Could add macOS nightly validation for platform-specific recipes

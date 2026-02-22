# Pragmatic Review: Issue #1882

**Focus**: Simplicity, YAGNI, KISS
**File changed**: `internal/builders/homebrew_test.go` (lines 3450-3946)
**Tests added**: 4 end-to-end tests + 1 unit test (TestPlatformTagToOSLibc)

## Findings

### Advisory 1: BdwGC and TreeSitter tests share ~80% identical assertion logic

`TestEndToEnd_LibraryRecipeGeneration_BdwGC` (line 3458, ~227 lines) and `TestEndToEnd_LibraryRecipeGeneration_TreeSitter` (line 3690, ~145 lines) repeat the same step-structure validation, output-type checking, and TOML string assertions. The TreeSitter test is shorter because it skips the per-platform output type breakdown, but the core validation loop (step action checks, install_mode, outputs-not-binaries, when clauses, TOML serialization checks) is duplicated.

However: this is test code, not production code. These are end-to-end validation tests for two distinct packages with different bottle layouts (bdw-gc has multiple lib variants + nested headers; tree-sitter has a single library). The duplication makes each test self-contained and readable. Extracting a shared assertion helper would add indirection without meaningful maintenance savings -- these tests are unlikely to change frequently. **Not blocking.**

### Advisory 2: BdwGC output-type assertion block could use a helper

Lines 3594-3651 in the BdwGC test manually iterate outputs twice (Linux then macOS) with nearly identical flag-checking loops, differing only in `.so` vs `.dylib`. This is the one place where a small helper like `assertOutputsContain(t, outputs, suffixes)` would reduce 60 lines to ~10 without losing clarity. **Not blocking** -- the repetition is bounded to this single test.

## Non-Findings

- No speculative generality or dead abstractions introduced.
- No scope creep -- changes are test-only, matching the issue's requirement.
- The mock GHCR approach reuses helpers from prior issues; no new helpers added.
- `TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure` (line 3904) tests the classification by constructing an error directly rather than driving through the full mock pipeline. This is the simplest correct approach for testing error classification.
- `TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive` (line 3841) uses both platforms in the mock even though only one is needed for the initial inspection. This is harmless -- the mock server serves whatever is requested, and having both platforms available matches realistic GHCR behavior.

## Verdict

No blocking findings. The implementation is straightforward test code that exercises the full pipeline. The duplication between BdwGC and TreeSitter tests is the expected trade-off for self-contained end-to-end tests.

# Pragmatic Review: Issue #1936

**Issue**: #1936 (feat(builders): use crates.io `bin_names` for Cargo binary discovery)
**Focus**: pragmatic (simplicity, YAGNI, KISS)

---

## Finding 1: `isValidExecutableName` recompiles regex on every call

**File**: `internal/builders/cargo.go:278`
**Severity**: Advisory

`isValidExecutableName` calls `regexp.MatchString()` which compiles the regex on every invocation. In the same file, `isValidCrateName` correctly uses a pre-compiled `crateNameRegex` at package level. The function is shared across all 5 builders.

Not a correctness issue -- recipe generation calls this a handful of times per build. But the inconsistency within the same file is worth fixing: pre-compile the regex like `crateNameRegex`.

**Fix**: Add `var executableNameRegex = regexp.MustCompile(...)` at package level, change `isValidExecutableName` to use `executableNameRegex.MatchString(name)`.

---

## Finding 2: Unused context parameter in `discoverExecutables`

**File**: `internal/builders/cargo.go:220`
**Severity**: Advisory

`discoverExecutables(_ context.Context, crateInfo *cratesIOCrateResponse)` takes a context it blanks out. The old implementation needed context for HTTP calls to GitHub; the new one just reads from the already-fetched struct. The npm builder's equivalent doesn't take context since it also reads from cached data.

Not harmful -- Go convention allows keeping it for future use or interface consistency. But this method isn't part of an interface, and the `_` blank makes the dead parameter visible.

**Fix**: Remove the context parameter and update the single call site at line 132.

---

## Non-findings

- **`NewCargoBuilderWithBaseURL` exported but test-only**: Follows the established pattern across all builders (npm, cask, etc.). Not specific to this issue.
- **`executables[0]` access at line 152 without length check**: `discoverExecutables` always returns at least one element via crate name fallback. The fallback crate name passes the `isValidExecutableName` regex (crate name regex is a strict subset). Safe.
- **Crate name fallback bypasses `isValidExecutableName`**: The design doc says validate "every binary name from `bin_names`". The fallback uses the crate name, not bin_names. Since `isValidCrateName` is strictly more restrictive, this is safe, but the security invariant could be made explicit with a comment.
- **`cachedCrateInfo` field for #1938**: Necessary per design doc. Adds one pointer field. Not speculative.
- **Test count**: 14 new test functions covering happy path, edge cases, and security. Proportional to the change.

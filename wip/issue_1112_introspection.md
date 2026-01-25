# Issue #1112 Introspection

## Issue Summary

**Title:** feat(actions): add system_dependency action for musl support

**Goal:** Create `system_dependency` action that checks for system packages and guides users to install them on musl systems.

**Milestone Position:** Middle (M47 - Platform Compatibility Verification)

## Sibling Issues Review

### #1109 (CLOSED) - feat(platform): add libc detection for glibc vs musl

**Implementation Details:**
- **File created:** `internal/platform/libc.go`
- **Primary detection:** Examines ELF interpreter of `/bin/sh` via `detectLibcFromBinary()`
- **Fallback detection:** Checks for `/lib/ld-musl-*.so.1` via `DetectLibcWithRoot()`
- **Integration:** `libc` field added to `Target` struct, `Libc()` method on `Matchable` interface
- **Test fixtures:** `internal/platform/testdata/libc/` with glibc and musl variants

**Key patterns established:**
- `ValidLibcTypes` constant: `[]string{"glibc", "musl"}`
- Detection returns `"musl"` if musl found, `"glibc"` otherwise (never empty)
- `platform.DetectLibc()` is called in `plan_generator.go` at line 104 when `targetOS == "linux"`

### #1110 (CLOSED) - feat(recipe): add libc filter to recipe conditional system

**Implementation Details:**
- **File modified:** `internal/recipe/types.go`
- **WhenClause changes:** Added `Libc []string` field at line 242
- **Matching logic:** Added at lines 301-313 in `WhenClause.Matches()`
- **Parsing:** Added in `UnmarshalTOML()` at lines 429-442
- **Serialization:** Added in `ToMap()` at lines 517-519
- **Tests:** `internal/recipe/when_test.go` (+333 lines)

**Key patterns established:**
- Libc filter only evaluated when `os == "linux"` (non-Linux platforms skip the check)
- Empty `Libc` array means "all libc types" (no filtering)
- Supports both single string and array: `when = { libc = "glibc" }` or `when = { libc = ["glibc"] }`
- Validation: libc values must be "glibc" or "musl" only

### #1111 (CLOSED) - feat(recipe): add step-level dependency resolution

**Implementation Details:**
- **Files modified:** `internal/recipe/types.go`, `internal/actions/resolver.go`
- **Step struct changes:** Added `Dependencies []string` field at line 326
- **Resolver changes:** `ResolveDependenciesForTarget()` function (lines 78-207 in resolver.go) filters step deps by target match
- **Parsing:** Added in `UnmarshalTOML()` at lines 459-472
- **Tests:** `internal/actions/resolver_test.go` (+203 lines), `internal/recipe/types_test.go` (+133 lines)

**Key patterns established:**
- Step `Dependencies` is a struct field (not just in Params map)
- Dependencies only resolved if `step.When != nil && !step.When.Matches(target)` returns false
- Recipe-level dependencies still take precedence via `r.Metadata.Dependencies`
- Step-level deps are additive with action implicit deps

## Gap Analysis

### 1. File Location Pattern

**Issue specifies:** `internal/actions/system_dependency.go`

**Analysis:** This follows established action file naming convention. All actions are in `internal/actions/` with snake_case names (e.g., `require_system.go`, `apt_actions.go`, `homebrew.go`).

**Status:** Specification aligns with codebase patterns.

### 2. DependencyMissingError Type

**Issue specifies:** Create `DependencyMissingError` structured error type with fields: Library, Package, Command, Family

**Analysis:** The design doc defines this error type at lines 427-444, but the issue acceptance criteria doesn't specify the exact location. Looking at similar patterns:
- `require_system.go` defines `SystemDepMissingError` in the same file (lines 178-196)
- No central error types package exists for actions

**Gap:** The issue should specify that `DependencyMissingError` and `IsDependencyMissing()` go in `internal/actions/system_dependency.go`, following the `require_system.go` pattern.

### 3. isInstalled() Implementation

**Issue specifies:** Detection for Alpine via `apk info -e <pkg>`, extensible to other families

**Analysis:** The design doc shows isInstalled() implementations at lines 481-494. The issue mentions "extensible" but only requires Alpine implementation initially.

**Gap clarification needed:** Issue says "Extensible to other families (Debian: dpkg-query, etc.)" but only acceptance criterion is Alpine. Other families would be future work.

### 4. getInstallCommand() with Root Detection

**Issue specifies:** Check `os.Getuid() == 0` to skip sudo/doas prefix when already root, prefer doas if available

**Analysis:** This is well-specified. Design doc shows implementation at lines 499-518.

**Status:** Specification is complete.

### 5. Plan Generator Integration

**Issue specifies:** "Plan generator collects ALL missing deps before failing (aggregate behavior)"

**Analysis:** Current plan generator (`internal/executor/plan_generator.go`) doesn't have any special handling for system dependency collection. The design doc describes `collectMissingDeps()` at lines 526-539.

**Gap:** The issue acceptance criteria mentions "Plan generator collects ALL missing deps" but doesn't specify:
- Where this logic goes (plan_generator.go vs executor.go)
- Whether this requires modifying existing functions or adding new ones
- How this interacts with the existing dependency resolution in `generateDependencyPlans()`

This is a significant implementation detail that isn't fully specified in the issue.

### 6. CLI Formatting of Aggregated Errors

**Issue specifies:** "CLI formats aggregated missing deps with combined install command"

**Analysis:** The design doc shows CLI output format at lines 545-558. However, the issue doesn't specify which CLI file handles this formatting.

**Gap:** The issue should specify where CLI formatting logic goes:
- `cmd/tsuku/install.go` - handles install command
- `cmd/tsuku/output.go` - if there's centralized output formatting
- Or a new function in the executor package

### 7. Action Registration

**Issue specifies:** "Action registered in actions registry"

**Analysis:** Looking at `internal/actions/action.go` line 139-205, all actions are registered in `init()`. The pattern is clear: `Register(&SystemDependencyAction{})`.

**Status:** Specification aligns with established pattern.

### 8. WhenClause Integration

**Issue specifies:** Action uses `when = { libc = ["musl"] }` in recipes

**Analysis:** The WhenClause filtering for libc is already implemented (#1110). The system_dependency action just needs to work with the existing step filtering.

**Status:** No additional implementation needed in the action itself for WhenClause handling.

### 9. Packages Parameter Structure

**Issue specifies:** `packages` (map of family -> package name)

**Analysis:** The design doc shows recipe usage at lines 586-590:
```toml
packages = { alpine = "curl-dev" }
```

This is a nested map structure in TOML. The action needs to parse this from `params["packages"]`.

**Gap:** The issue acceptance criteria don't specify validation for the packages map:
- What happens if family isn't in the map?
- What if packages map is missing entirely?
- Should validation require `alpine` key when `libc = ["musl"]`?

The design doc addresses this at lines 1501-1520 but it's not in the issue's acceptance criteria.

### 10. Missing Validation Command

**Issue validation script includes:**
```bash
./tsuku list-actions | grep -q "system_dependency"
```

**Analysis:** Looking at the codebase, there's no `tsuku list-actions` command visible in the acceptance criteria context.

**Gap:** This validation step may need updating if `list-actions` doesn't exist, or the issue assumes it will be created.

## Summary of Gaps

### Critical (affects implementation):

1. **Plan generator integration not specified** - The issue mentions aggregating missing deps but doesn't specify the implementation approach in the existing plan generator code.

2. **CLI error formatting location not specified** - Where does the formatted output for aggregated missing deps go?

### Minor (clarification needed):

3. **Packages map validation rules** - What errors when the family isn't found in packages map?

4. **list-actions command assumption** - Validation script uses a potentially nonexistent command.

### Implicit Requirements (established by siblings):

These patterns are established but worth confirming the issue author intended to follow them:

- Error types defined in same file as action (following require_system.go pattern)
- Uses `BaseAction` embedding for default implementations
- Uses `GetString()` helper for parameter extraction
- Implements `Preflight()` for parameter validation

## Recommendations

1. **Before implementation:** Clarify where plan generator aggregation logic goes. Options:
   - Add `collectMissingDeps()` to `plan_generator.go`
   - Add check in `executor.go` during plan execution
   - Create new file for system dependency handling

2. **Consider splitting:** The "aggregate missing deps" behavior could be a separate small issue if it requires significant changes to the plan generator.

3. **Validation script:** Update to use an existing command or note that `list-actions` needs to be implemented first.

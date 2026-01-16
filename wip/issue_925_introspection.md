# Issue 925 Introspection

## Context Reviewed
- Design doc: docs/designs/DESIGN-non-deterministic-validation.md (milestone context)
- Sibling issues reviewed: #921 (pip), #922 (go_build), #923 (cargo_install), #924 (npm_install)
- Prior patterns identified: All four sibling issues follow identical implementation pattern

## Prior Work Analysis

### Established Pattern (from #921, #922, #923, #924)

Each sibling issue implemented constraint support by:

1. **EvalConstraints field** (internal/actions/decomposable.go):
   - `GemLock string` field already exists (line 80)
   - Comment references issue #925

2. **Extraction function** (internal/executor/constraints.go):
   - Need to add `extractGemConstraintsFromSteps()` function
   - Need to call it in `ExtractConstraintsFromPlan()` and `extractConstraintsFromDependency()`
   - Need to add `HasGemLockConstraint()` helper function

3. **Tests** (internal/executor/constraints_test.go):
   - Need test cases mirroring pip/go/cargo/npm patterns:
     - `TestExtractConstraints_GemExec` - basic extraction
     - `TestExtractConstraints_GemExecInDependency` - extraction from dependencies
     - `TestExtractConstraints_GemExecFirstWins` - first lock_data wins
     - `TestExtractConstraints_GemExecEmptyLockData` - empty lock_data handling
     - `TestHasGemLockConstraint` - helper function tests

4. **Primitive action parameters**:
   - `gem_exec` primitive stores `lock_data` parameter (verified in jekyll golden file)
   - Bundler uses `install_gem_direct` primitive (special case, no lock_data)

## Gap Analysis

### Minor Gaps
None identified. The pattern is clear and well-established:
- `GemLock` field already exists in `EvalConstraints` (decomposable.go line 80)
- `gem_exec` primitive already stores `lock_data` parameter (verified in jekyll/v4.4.1 golden file)
- Sibling implementations provide exact template to follow

### Moderate Gaps
None identified.

### Major Gaps
None identified.

## Implementation Details

Based on jekyll golden file analysis, the `gem_exec` step stores:
- `gem` (string): gem name
- `version` (string): gem version
- `executables` ([]string): list of executables
- `lock_data` (string): full Gemfile.lock content
- `ruby_version` (string, optional): Ruby version metadata

The extraction should:
1. Look for `gem_exec` action steps
2. Extract `lock_data` parameter (string)
3. Store in `constraints.GemLock` (first one wins, like other ecosystems)

## Recommendation
**Proceed**

The issue spec is complete. The GemLock field is already defined, the gem_exec primitive already stores lock_data, and the pattern from sibling issues (#921-#924) provides a clear implementation template. No user input needed.

## Proposed Amendments
None needed. The issue can be implemented following the exact pattern established by sibling issues.

# Architecture Review: install_binaries Parameter Semantics

## Executive Summary

The proposed architecture is **implementable and sound**, with minor clarifications needed around backward compatibility handling and edge case documentation. The design successfully balances semantic clarity with minimal migration burden.

**Key Findings:**
- Architecture is clear and complete for implementation
- No critical missing components or interfaces identified
- Implementation phases are correctly sequenced
- A simpler pure-inference approach exists but trades flexibility for marginal complexity reduction
- Current recipe count (112 files with `binaries=`) is significantly higher than design document states (35)

**Recommendation:** Proceed with implementation after addressing backward compatibility clarifications and recipe migration scope updates.

---

## 1. Architecture Clarity Assessment

### 1.1 Is the architecture clear enough to implement?

**Yes, with minor clarifications needed.**

The proposed architecture provides:
- Clear parameter renaming strategy (`binaries` → `outputs`)
- Explicit executability determination logic (`DetermineExecutables()`)
- Well-defined backward compatibility approach
- Concrete code examples in pseudocode form

**Clarifications needed:**

1. **Backward compatibility execution path**: The design shows deprecation warning logic in `Preflight()` but doesn't specify how `Execute()` should handle both parameter names. Should it:
   - Prefer `outputs` if both exist?
   - Error if both are present?
   - Transparently alias `binaries` to `outputs`?

2. **`executables` parameter interaction**: When explicit `executables` is provided:
   - Are paths relative to WorkDir or InstallDir?
   - Must they be a subset of `outputs`?
   - What happens if an executable is listed that isn't in `outputs`?

3. **Integration with existing code**: The design references `BinaryMapping` struct but current implementation shows:
   ```go
   type BinaryMapping struct {
       Src  string `toml:"src"`
       Dest string `toml:"dest"`
   }
   ```
   This struct is used in `parseBinaries()` to handle both string shorthand and explicit mappings. The design should clarify whether this parsing logic changes or if `outputs` uses the same dual format.

### 1.2 Implementation Pseudocode Validation

The proposed `DetermineExecutables()` function is clear but could be optimized:

```go
// Proposed (from design doc)
func DetermineExecutables(outputs []string, explicitExecutables []string) []string {
    if len(explicitExecutables) > 0 {
        return explicitExecutables
    }

    var result []string
    for _, path := range outputs {
        if strings.HasPrefix(path, "bin/") {
            result = append(result, path)
        }
    }
    return result
}
```

**Issue:** This returns paths as-is, but the existing `installDirectoryWithSymlinks()` expects `[]recipe.BinaryMapping` with both `Src` and `Dest` fields. The architecture needs to clarify whether:
- `DetermineExecutables()` returns string paths that are later converted
- The function should return `[]BinaryMapping` directly
- Both src and dest are needed for executability determination

---

## 2. Missing Components and Interfaces

### 2.1 Identified Gaps

**Critical:**
1. **ExtractBinaries() function update**: The design doc mentions updating `internal/recipe/recipe.go` for `BinaryMapping` but doesn't address the `ExtractBinaries()` function (lines 689-787 in types.go), which:
   - Currently searches for `binaries` parameter in step params
   - Determines symlink targets for `$TSUKU_HOME/bin/`
   - Distinguishes between `install_mode = "directory"` and default mode

   This function needs updating to search for `outputs` instead of `binaries`.

2. **Metadata.Binaries field**: The `MetadataSection` struct has:
   ```go
   Binaries []string `toml:"binaries,omitempty"` // Explicit binary paths for homebrew recipes
   ```
   This field serves a different purpose (metadata declaration for homebrew recipes) but shares the same name. The design should clarify:
   - Is this field also renamed to `Outputs`?
   - Or kept as `Binaries` since it specifically means "executables" in this context?
   - How does this interact with the action parameter rename?

**Minor:**
3. **Preflight validation enhancement**: The design shows adding a deprecation warning but doesn't specify validation for the new `executables` parameter:
   - Should it validate that paths don't contain `..` (security)?
   - Should it warn if `executables` contains paths not in `outputs`?
   - Should it error if `executables` contains library paths (`.so`, `.dylib`)?

4. **Test coverage**: The architecture doesn't specify which test cases need updating. Current `install_binaries_test.go` has comprehensive coverage (702 lines) including:
   - Security validation tests
   - Mode routing tests
   - Parse binaries tests

   These need parallel test cases for `outputs` parameter.

### 2.2 Interface Dependencies

**Well-defined:**
- `PreflightResult` interface usage is clear
- `ExecutionContext` structure is established
- `BinaryMapping` type is documented

**Needs documentation:**
- How does this change affect composite actions (`github_archive`, `download_archive`) that accept `binaries` parameter and pass it through to `install_binaries`?
- The design doc states "Update `BinaryMapping` struct field names if needed" but doesn't specify what changes are needed (or if any are needed).

---

## 3. Implementation Phase Sequencing

### 3.1 Proposed Phases

The design proposes:
1. **Phase 1: Core Changes** - Add `outputs` support, implement `DetermineExecutables()`, add deprecation warning
2. **Phase 2: Recipe Migration** - Automated script to rename, manual review, update testdata
3. **Phase 3: Documentation and Cleanup** - Update docs, remove deprecated support

**Assessment:** **Correctly sequenced** with one optimization opportunity.

### 3.2 Recommended Sequencing Adjustment

**Issue:** Phase 1 adds deprecation warning but Phase 2 migrates recipes. This means:
- During Phase 1 testing, all tests will show deprecation warnings
- CI will be noisy with warnings until Phase 2 completes
- Developers working during transition will see warnings for valid recipes

**Better sequencing:**

**Phase 1: Core Implementation (no deprecation yet)**
- Add `outputs` parameter support alongside `binaries` (both work silently)
- Implement `DetermineExecutables()` with path inference
- Add `executables` override parameter
- Update unit tests to cover both parameter names
- **Do not add deprecation warning yet**

**Phase 2: Recipe Migration**
- Automated script to rename `binaries` → `outputs` across all recipes
- Manual review of edge cases (current design states 35 files, but grep shows 112 files)
- Update testdata recipes
- **Commit recipe changes**

**Phase 3: Deprecation and Enforcement**
- Add deprecation warning for `binaries` parameter
- Update CONTRIBUTING.md with new parameter guidance
- Monitor for external recipe usage (if any)

**Phase 4: Cleanup (future)**
- Remove deprecated `binaries` support entirely
- Update inline code documentation

**Benefits:**
- Clean CI output during development
- Atomic migration (recipes work before and after)
- Clear deprecation timeline for external users (if recipes published)

### 3.3 Missing Phase Details

**Migration script specification:** Phase 2 mentions "automated script" but doesn't specify:
- Language (Go, Shell, Python)?
- Validation approach (parse TOML, regex replace)?
- How to handle composite actions that pass `binaries` through?
- Edge case handling (what if a recipe has both `binaries` and `outputs`)?

**Suggested approach:**
```go
// Tool: cmd/migrate-binaries/main.go
// 1. Parse each recipe TOML
// 2. For each [[steps]] block:
//    - If params["binaries"] exists and params["outputs"] doesn't exist:
//      - Rename "binaries" key to "outputs"
//    - If both exist: error (manual review needed)
// 3. Write updated TOML with formatting preservation
// 4. Report statistics (files changed, steps migrated)
```

---

## 4. Simpler Alternatives Analysis

### 4.1 Pure Path Inference (Option 2A alone)

**What was overlooked:**

The design chose hybrid approach (2D) but admits in the validation results:
> Audit of 35 directory-mode recipes: All executable paths use bin/ prefix, no executables found outside standard paths

**Current reality check:**
- Grep shows 127 occurrences of `binaries =` across 112 recipe files
- All examined recipes (liberica, cmake, curl, openssl) use `bin/` for executables
- Zero recipes in current codebase need `executables` override

**Simpler alternative:**
```toml
# Option 2A: Pure inference, no executables parameter
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/tool", "lib/libfoo.so"]
# Executability always inferred from bin/ prefix - no override needed
```

**Trade-offs:**

| Aspect | Hybrid (2D) | Pure Inference (2A) |
|--------|-------------|---------------------|
| Code complexity | +1 parameter, +1 logic path | Minimal |
| Recipe verbosity | Same (override rarely used) | Same |
| Edge case handling | Can handle via `executables` | Must use `bin/` or can't install |
| Current recipe compatibility | 100% (0 need override) | 100% (0 need override) |
| Future flexibility | High | Medium (constrained by convention) |
| Conceptual overhead | "Why two ways?" | "Why so rigid?" |

**Recommendation:** The hybrid approach (2D) is justified **if and only if** there's evidence that future recipes will need executables outside `bin/`. Given:
- Current: 0/112 recipes need it
- Unix convention: `bin/` is standard for 50+ years
- Workaround exists: recipes can create `bin/` symlinks to executables elsewhere

The **simpler pure inference approach (2A) is viable** and reduces implementation complexity. However, the hybrid approach (2D) has minimal cost since the override is optional.

**Verdict:** Hybrid (2D) is acceptable but not strictly necessary. If implementation complexity becomes an issue, 2A is a valid simplification.

### 4.2 Alternative Naming: `artifacts`

**Not considered in design doc:**

Industry terminology analysis missed "artifacts" which is common in CI/CD systems:
- GitHub Actions: `upload-artifact`, `download-artifact`
- GitLab CI: `artifacts:` keyword
- Jenkins: artifact archiving

**Comparison:**
```toml
# Proposed: outputs
outputs = ["bin/tool", "lib/libfoo.so"]

# Alternative: artifacts
artifacts = ["bin/tool", "lib/libfoo.so"]
```

**Pro:**
- Widely understood in DevOps context
- Conveys "things produced by a build"
- Matches CI/CD terminology users already know

**Con:**
- Could be confused with CI artifacts (build logs, test reports)
- Doesn't align with Nix `outputs` terminology
- Not used elsewhere in tsuku codebase

**Verdict:** `outputs` is still the better choice for consistency with Nix and existing tsuku vocabulary, but `artifacts` would be a reasonable alternative if `outputs` proves confusing in practice.

---

## 5. Recipe Migration Scope Discrepancy

### 5.1 Design Document Claims

The design document states:
> Impact: 35 recipes use install_mode = "directory"

And in validation results:
> Audit of 35 directory-mode recipes

### 5.2 Actual Codebase Analysis

Grep results show:
```
Found 127 total occurrences across 112 files.
```

**Discrepancy:** The design scope is significantly underestimated.

**Investigation needed:**
1. Are there 112 recipes with `binaries =` parameter total?
2. How many use `install_mode = "directory"` vs default mode?
3. Does the `binaries` parameter appear in other actions besides `install_binaries`?

**Sample check (from grep results):**
- `cmake.toml`: 3 occurrences of `binaries =`
  - Line 6: `binaries = ["bin/cmake", ...]` (metadata)
  - Line 14: `binaries = [...]` (set_rpath action)
  - Line 20: `binaries = [...]` (install_binaries action)

**Insight:** The parameter appears in:
1. `[metadata]` section (different purpose - declares what binaries recipe provides)
2. `set_rpath` action (specifies which binaries need RPATH modification)
3. `install_binaries` action (the target of this design)

### 5.3 Migration Scope Correction

**Required changes:**
1. **Metadata section:** Keep as `binaries` (semantically correct - it IS a list of executables)
2. **set_rpath action:** Keep as `binaries` (semantically correct - modifying executable RPATH)
3. **install_binaries action:** Rename to `outputs` (semantically incorrect currently - includes libraries)

**Updated migration count estimate:**
- `install_binaries` occurrences: ~50-70 (subset of 127 total)
- Composite actions (`github_archive`, `download_archive`) passing `binaries` through: ~40-50
- Other actions using `binaries` correctly: ~30-40

**Action required:** Design document should update scope estimates with accurate counts.

---

## 6. Backward Compatibility Considerations

### 6.1 Deprecation Timeline

Design proposes:
> During a transition period, both binaries and outputs will be accepted

But doesn't specify:
- How long is the transition period?
- What triggers the end of transition (all in-tree recipes migrated? calendar date?)
- Are there external recipe authors to consider?

### 6.2 Composite Action Propagation

Actions like `github_archive` accept a `binaries` parameter and pass it to `install_binaries`:

```toml
# Current usage
[[steps]]
action = "github_archive"
binaries = ["bin/tool"]
install_mode = "directory"
```

**Question:** Do composite actions also need to accept `outputs` parameter?

**Investigation needed:**
- Review composite action implementations
- Determine if they pass through parameters or transform them
- Decide if they need parallel migration

### 6.3 Version Compatibility

If tsuku CLI version N supports both `binaries` and `outputs`, but version N-1 only supports `binaries`:
- Can recipes target older CLI versions?
- Is there a recipe format version field?
- Should recipes specify minimum tsuku version?

**Current observation:** No recipe format versioning seen in examined files.

**Recommendation:** Document minimum tsuku version that supports `outputs` parameter in migration guide.

---

## 7. Security and Safety Review

### 7.1 Security Improvements

Design correctly identifies:
> Only files in bin/ receive chmod 0755, libraries in lib/ retain original permissions

**Validation:** This is a genuine security improvement. Current behavior:
```go
// Current: chmod ALL files to 0755
for _, binary := range binaries {
    os.Chmod(destPath, 0755)  // Even libraries!
}
```

**Proposed:**
```go
// Proposed: chmod only executables
executables := DetermineExecutables(outputs, explicitExecutables)
for _, exe := range executables {
    os.Chmod(exePath, 0755)
}
```

**Impact:** Reduces attack surface slightly (libraries shouldn't be directly executable).

### 7.2 Path Traversal Validation

Existing code has security validation:
```go
func (a *InstallBinariesAction) validateBinaryPath(binaryPath string) error {
    if strings.Contains(binaryPath, "..") {
        return fmt.Errorf("binary path cannot contain '..': %s", binaryPath)
    }
    if filepath.IsAbs(binaryPath) {
        return fmt.Errorf("binary path must be relative, not absolute: %s", binaryPath)
    }
    return nil
}
```

**Question:** Does this validation apply to:
- `outputs` parameter? (Yes, should)
- `executables` parameter? (Yes, should)

**Recommendation:** Design should explicitly state that existing security validation applies to new parameters.

### 7.3 Edge Case: Executable Not in Outputs

**Scenario:**
```toml
outputs = ["lib/libfoo.so"]
executables = ["bin/tool"]  # Not in outputs!
```

**What should happen?**
1. Error: "executable must be in outputs list"
2. Warning: "executable not in outputs, will not be installed"
3. Silently succeed: chmod bin/tool even though it's not tracked

**Current design:** Doesn't address this case.

**Recommendation:** Validation should error if `executables` contains paths not present in `outputs`.

---

## 8. Testing Strategy Gaps

### 8.1 Required Test Coverage

Design says "Update unit tests" but doesn't specify:

**Required test cases:**
1. **Parameter acceptance:**
   - `outputs` parameter alone (should succeed)
   - `binaries` parameter alone (should succeed with warning during transition)
   - Both parameters present (should error or prefer `outputs`)
   - Neither parameter present (should error)

2. **Executability inference:**
   - `outputs = ["bin/tool", "lib/lib.so"]` → only `bin/tool` is executable
   - `outputs = ["bin/tool"]` → `bin/tool` is executable
   - `outputs = ["lib/lib.so"]` → no executables

3. **Explicit executables override:**
   - `outputs = ["libexec/tool"], executables = ["libexec/tool"]` → works
   - `outputs = ["bin/tool"], executables = ["bin/other"]` → error (not in outputs)
   - `outputs = ["bin/a", "bin/b"], executables = ["bin/a"]` → only a is executable

4. **Backward compatibility:**
   - Old recipes with `binaries` still work
   - Deprecation warning is emitted
   - Functionality is identical between `binaries` and `outputs`

5. **Edge cases:**
   - Empty `outputs` array (should error per existing validation)
   - `outputs` with mixed slashes (`bin\tool` on Windows?)
   - Duplicate entries in `outputs`

### 8.2 Integration Testing

**Missing from design:** How to validate the migration worked correctly?

**Suggested approach:**
1. Before migration: Record checksums of installed files for all recipes
2. Run migration script
3. After migration: Re-install all recipes and verify checksums match
4. Verify symlinks in `$TSUKU_HOME/bin/` are identical

---

## 9. Documentation Requirements

### 9.1 Recipe Author Guide

Design mentions updating CONTRIBUTING.md but doesn't specify content. Required sections:

**1. When to use `outputs`:**
```markdown
The `outputs` parameter lists all files that should be tracked by the installation:
- Executables in `bin/`
- Libraries in `lib/`
- Any other files needed at runtime

Files in `bin/` are automatically made executable. For executables outside `bin/`,
use the `executables` parameter.
```

**2. When to use `executables` override:**
```markdown
Rarely needed. Use only if you need to mark files outside `bin/` as executable:

[[steps]]
action = "install_binaries"
outputs = ["libexec/git-core/git-remote-http", "lib/libgit.so"]
executables = ["libexec/git-core/git-remote-http"]
```

**3. Migration guide for existing recipes:**
```markdown
If you have an existing recipe using `binaries`, rename it to `outputs`:

- binaries = ["bin/tool", "lib/lib.so"]
+ outputs = ["bin/tool", "lib/lib.so"]

No other changes needed - executability is inferred from the `bin/` prefix.
```

### 9.2 Inline Code Documentation

The `DetermineExecutables()` function needs comprehensive godoc:

```go
// DetermineExecutables returns the list of files that should be made executable (chmod 0755).
//
// Executability is determined by one of two methods:
//  1. Explicit list: If explicitExecutables is non-empty, it is returned as-is.
//     The caller is responsible for validating that all paths exist in outputs.
//  2. Path inference: Files with a "bin/" prefix are considered executable.
//     This follows Unix convention where executables live in bin/ directories.
//
// Examples:
//   DetermineExecutables(["bin/tool", "lib/lib.so"], nil) → ["bin/tool"]
//   DetermineExecutables(["bin/tool", "lib/lib.so"], ["bin/tool"]) → ["bin/tool"]
//   DetermineExecutables(["libexec/helper"], ["libexec/helper"]) → ["libexec/helper"]
//
// Parameters:
//   outputs: All files being installed (executables, libraries, etc.)
//   explicitExecutables: Optional override for executability (rarely needed)
//
// Returns:
//   List of paths (relative to installation directory) that should receive execute permission
func DetermineExecutables(outputs []string, explicitExecutables []string) []string
```

---

## 10. Open Questions for Design Resolution

Before proceeding with implementation, resolve:

1. **Composite actions:** Do `github_archive` and `download_archive` need to accept `outputs` parameter in addition to `binaries`?

2. **Metadata.Binaries field:** Is `[metadata] binaries = [...]` renamed or kept as-is? (Recommendation: keep it - semantically correct as list of executables)

3. **Both parameters present:** If a recipe has both `binaries` and `outputs`, what happens?
   - Error (safer, prevents accidents)
   - Prefer `outputs` (more forgiving)
   - Merge them (complex, probably wrong)

4. **Executables validation:** Should `executables` paths be validated as a subset of `outputs`? (Recommendation: yes, error if mismatch)

5. **Migration timeline:** When does `binaries` support get removed entirely? (Recommendation: after 1-2 major versions)

6. **Recipe format versioning:** Should recipes specify minimum tsuku version? (Recommendation: add `min_tsuku_version` to metadata)

---

## 11. Recommendations Summary

### 11.1 Critical (Must Address Before Implementation)

1. **Update recipe count:** Design doc claims 35 recipes, grep shows 112 files with `binaries =`. Clarify actual scope.
2. **Specify backward compatibility execution:** How does `Execute()` handle both parameter names?
3. **Define executables validation:** Must they be subset of outputs?
4. **Address ExtractBinaries() function:** Update to search for `outputs` parameter.

### 11.2 Important (Should Address)

5. **Revise implementation phases:** Move deprecation warning to Phase 3 (after migration).
6. **Document composite action changes:** How do `github_archive`, etc. handle the rename?
7. **Add test coverage spec:** Enumerate required test cases.
8. **Write migration script spec:** Language, validation approach, edge cases.

### 11.3 Nice to Have

9. **Consider pure inference (2A):** Simpler implementation, 0 current recipes need override.
10. **Add recipe format versioning:** Future-proof for parameter changes.
11. **Document security validation:** Explicitly state path traversal checks apply to new params.

---

## 12. Final Verdict

**Architecture Status: APPROVED WITH REVISIONS**

The proposed architecture is fundamentally sound and implementable. The design successfully balances:
- Semantic clarity (accurate parameter names)
- Migration burden (minimal with path inference)
- Future flexibility (optional executables override)

**Required revisions:**
1. Correct recipe migration scope (112 files, not 35)
2. Specify backward compatibility execution path
3. Define executables parameter validation
4. Update ExtractBinaries() function handling

**Optional improvements:**
5. Consider pure path inference (2A) for simplicity
6. Add recipe format versioning
7. Revise phase sequencing to delay deprecation warning

**Implementation risk: LOW**

With the above clarifications, this design is ready for Phase 1 implementation.

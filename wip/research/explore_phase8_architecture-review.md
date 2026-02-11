# Phase 8 Architecture Review: Homebrew Relocation Fix

## Executive Summary

The proposed solution architecture is **implementable and correctly sequenced**. The design addresses the core bugs with minimal invasiveness. This review identifies a few clarifications and edge cases to address before implementation, but no fundamental redesign is needed.

---

## Question 1: Is the architecture clear enough to implement?

**Verdict: Yes, with minor clarifications needed**

### Clarity Assessment

| Component | Clarity | Implementation Ready? |
|-----------|---------|----------------------|
| Component 1: extractBottlePrefixes() | High | Yes |
| Component 2: relocatePlaceholders() | High | Yes |
| Component 3: CopyDirectoryExcluding | Medium | Needs detail |

### Component 1: extractBottlePrefixes()

The design provides clear pseudocode:

```go
// After finding /tmp/action-validator- and extracting full path:
installIdx := strings.Index(pathStr, "/.install/")
if installIdx == -1 {
    continue
}

afterInstall := pathStr[installIdx+len("/.install/"):]
parts := strings.SplitN(afterInstall, "/", 3)  // formula, version, rest
if len(parts) < 2 {
    continue
}

prefix := pathStr[:installIdx] + "/.install/" + parts[0] + "/" + parts[1]
prefixMap[pathStr] = prefix
```

This is directly implementable. The current function signature:
```go
func (a *HomebrewRelocateAction) extractBottlePrefixes(content []byte, prefixes map[string]bool)
```

Becomes:
```go
func (a *HomebrewRelocateAction) extractBottlePrefixes(content []byte, prefixMap map[string]string)
```

**Clarification needed**: The current code at line 127 initializes `bottlePrefixes := make(map[string]bool)`. This initialization point and its callers at lines 184 and 215-218 need to be updated together.

### Component 2: relocatePlaceholders()

Current code (lines 215-218):
```go
for prefix := range bottlePrefixes {
    newContent = bytes.ReplaceAll(newContent, []byte(prefix), prefixReplacement)
}
```

New code:
```go
for fullPath, prefix := range bottlePrefixes {
    suffix := fullPath[len(prefix):]
    replacement := prefixPath + suffix
    newContent = bytes.ReplaceAll(newContent, []byte(fullPath), []byte(replacement))
}
```

This is clear and directly implementable. The key insight is using `fullPath` for matching and constructing the replacement by appending `suffix` to `prefixPath`.

### Component 3: CopyDirectoryExcluding

The design mentions adding this function but doesn't provide implementation details. The current `CopyDirectory` in utils.go (lines 12-51) uses `filepath.Walk`.

**Missing detail**: How should exclusion work?

**Recommended implementation**:
```go
func CopyDirectoryExcluding(src, dst string, excludePattern string) error {
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        relPath, err := filepath.Rel(src, path)
        if err != nil {
            return err
        }

        // Skip excluded directory and its contents
        if relPath == excludePattern || strings.HasPrefix(relPath, excludePattern+string(filepath.Separator)) {
            if info.IsDir() {
                return filepath.SkipDir
            }
            return nil
        }

        // ... rest identical to CopyDirectory
    })
}
```

The critical detail: use `filepath.SkipDir` when encountering the excluded directory to avoid walking into it.

---

## Question 2: Are there missing components or interfaces?

**Verdict: One minor gap identified**

### Gap: Debug statement cleanup

The current code has extensive debug statements (lines 84, 168-183, 231, 243-250, 731-732). The Phase 4 review flagged these as tech debt. The implementation phases don't include cleanup.

**Recommendation**: Add a Phase 0 or incorporate into Phase 1:
- Convert `fmt.Printf("   Debug: ...")` statements to conditional logging
- Use `ctx.Log().Debug(...)` pattern already established in the codebase

### No missing interfaces

The design correctly identifies all touch points:
1. `extractBottlePrefixes()` - signature change from `map[string]bool` to `map[string]string`
2. `relocatePlaceholders()` - loop iteration change
3. `installDirectoryWithSymlinks()` - call site change to use exclusion

The only caller of `extractBottlePrefixes()` is `relocatePlaceholders()`, so the signature change is contained.

---

## Question 3: Are the implementation phases correctly sequenced?

**Verdict: Yes, the phases are correctly ordered**

### Phase Dependencies

```
Phase 1: Fix path extraction
    |
    +-- Modifies extractBottlePrefixes() signature
    +-- Modifies relocatePlaceholders() loop
    +-- Updates caller (single location)
    |
    v
Phase 2: Fix directory copy
    |
    +-- Adds CopyDirectoryExcluding()
    +-- Updates installDirectoryWithSymlinks() call site
    |
    v
Phase 3: Validation
    |
    +-- Integration tests
    +-- Cross-platform verification
```

**Correct sequencing rationale**:
- Phase 1 can be implemented and tested independently on any Homebrew recipe
- Phase 2 only affects `install_mode=directory` recipes (5 affected: make, cmake, ninja, pkg-config, patchelf)
- Phase 3 validates both fixes together

The phases could be implemented in parallel since they modify different code paths, but sequential implementation reduces cognitive load and makes bisection easier if issues arise.

### Suggested refinement

Phase 1 could be split for safer rollout:
- Phase 1a: Add new function, keep old one, add feature flag
- Phase 1b: Migrate callers
- Phase 1c: Remove old function

However, given the contained scope (single caller), this may be overengineering.

---

## Question 4: Are there simpler alternatives we overlooked?

**Verdict: One simpler alternative deserves consideration**

### Alternative: String suffix extraction without parsing

Instead of parsing the `/.install/<formula>/<version>` structure, we could:

1. Find all unique bottle path prefixes by looking for common prefix across all extracted paths
2. The longest common prefix that ends with a version-like pattern is the bottle prefix

**Example**:
```
Found paths:
  /tmp/action-validator-123/.install/pod/1.16.2/libexec/bin/pod
  /tmp/action-validator-123/.install/pod/1.16.2/libexec/lib/ruby.rb

Common prefix: /tmp/action-validator-123/.install/pod/1.16.2
```

**Advantages**:
- Doesn't require parsing formula/version from path
- Works even if path structure changes
- Simpler edge case handling

**Disadvantages**:
- Requires multiple paths to find common prefix (single-path files won't work)
- May fail if bottle contains paths from dependencies (different prefixes)

**Verdict**: The marker-based approach in the design is more robust. It works with single paths and handles the common case correctly. This alternative is noted but not recommended.

### Alternative: Replace all temp paths with install path (no suffix handling)

Current behavior: `/tmp/action-validator-123/.install/pod/1.16.2` -> `/home/user/.tsuku/tools/pod-1.16.2`

The bug is that full paths like `/tmp/.../libexec/bin/pod` become `/home/.../pod-1.16.2` (losing `libexec/bin/pod`).

What if we detect this pattern and just don't replace paths that extend beyond the version?

**Verdict**: This doesn't solve the problem - we need those extended paths to work correctly. The suffix must be preserved.

---

## Additional Observations

### Edge Case: Paths without `/.install/` marker

The design assumes all bottle paths contain `/.install/`. Line 735 of current code:
```go
if strings.Contains(pathStr, "/.install/") {
    prefixes[pathStr] = true
}
```

**Question**: Are there bottle paths without this marker?

**Finding**: The `/.install/` directory is created by `homebrew_relocate` action when extracting bottles. All Homebrew bottle paths processed by this code should contain it. The check is defensive and correct.

### Edge Case: Version strings with slashes

The parsing logic uses `strings.SplitN(afterInstall, "/", 3)` to get formula/version/rest.

**Question**: What if a version contains a slash?

**Finding**: Homebrew versions don't contain slashes. Versions follow patterns like `1.16.2`, `8.17.0_1`, `2.4.52`. The SplitN approach is safe.

### Edge Case: Empty suffix

If a path exactly matches the bottle prefix (no suffix), the code should still work:

```go
suffix := fullPath[len(prefix):]  // suffix = ""
replacement := prefixPath + suffix  // replacement = prefixPath
```

This is correct - a path like `/tmp/.../pod/1.16.2` should become `/home/.../pod-1.16.2`.

### Binary vs Text file handling

The design focuses on text file replacement. Binary files (ELF, Mach-O) are handled differently - they use patchelf/install_name_tool for RPATH manipulation, not string replacement.

**Question**: Does the bug affect binary files?

**Finding**: No. Binary relocation (lines 254-258, 263-410) uses RPATH which is relative (`$ORIGIN`, `@loader_path`). The suffix preservation bug only affects text file replacement where absolute paths are embedded.

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Parsing fails on unexpected path format | Low | Medium | Debug logging shows parsed results; falls back to current behavior if no marker found |
| Exclusion pattern too broad | Low | Low | Explicit pattern `.install` won't match other directories |
| Breaking working recipes | Low | High | Phase 3 validation runs full test suite |
| Performance regression from map type change | Very Low | Low | Map operations are O(1); file I/O dominates |

---

## Recommendations

### Before implementation

1. **Add concrete test case**: Create a unit test with a sample file containing `/tmp/action-validator-12345/.install/pod/1.16.2/libexec/bin/pod` and verify the replacement produces the correct output.

2. **Clarify CopyDirectoryExcluding implementation**: Add the implementation sketch to the design document or rely on implementer to follow the pattern in this review.

3. **Document debug statement cleanup**: Either add to Phase 1 scope or create a follow-up task.

### During implementation

1. **Test suffix edge cases**: Empty suffix, single-level suffix, deep suffix (4+ levels).

2. **Verify backward compatibility**: Run the existing recipe test suite before and after changes.

3. **Log parsed prefixes**: Add debug logging showing `fullPath -> prefix` mappings for troubleshooting.

### After implementation

1. **Monitor CI metrics**: Track how many recipes move from failing to passing.

2. **Clean up debug statements**: Remove or guard the `fmt.Printf("   Debug: ...")` lines.

---

## Conclusion

The solution architecture is well-designed and ready for implementation. The three components are clearly specified, the phases are correctly sequenced, and no simpler alternatives provide equivalent robustness.

Key implementation notes:
- The map type change from `map[string]bool` to `map[string]string` is the core API change
- The replacement loop modification is straightforward once the map structure is updated
- `CopyDirectoryExcluding` should use `filepath.SkipDir` to efficiently skip the excluded subtree

Estimated implementation effort: 2-3 hours for a developer familiar with the codebase.

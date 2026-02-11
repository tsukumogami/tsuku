# Security Review: Homebrew Bottle Relocation Fix

**Date**: 2026-02-10
**Scope**: DESIGN-homebrew-relocation-fix.md
**Status**: Review Complete

## Executive Summary

The security analysis in the design document is **mostly accurate** but has some gaps. The primary concern is that the mitigations rely on assumptions that could be violated. There is one attack vector not fully addressed (prefix injection via suffix manipulation) and one "not applicable" classification that deserves scrutiny (supply chain). Overall, residual risk is **low** given existing defense-in-depth, but two recommendations warrant consideration.

---

## Question 1: Attack Vectors Not Considered

### 1.1 Suffix Injection via Crafted Binary Content

**New Attack Vector Identified**: An attacker with control over bottle content could embed paths like:

```
/tmp/action-validator-XXX/.install/formula/version/../../../etc/passwd
```

The current path extraction logic (lines 699-742 in `homebrew_relocate.go`) extracts everything after the marker up to a delimiter. If the suffix portion contains `..` sequences, the final replacement path could escape `$TSUKU_HOME`.

**Current State**: The design claims "path validation blocks `..` traversal" but:
- `extractBottlePrefixes()` does **not** validate extracted paths for `..`
- `relocatePlaceholders()` performs straight substitution without validation
- The "existing path validation in `install_binaries`" only applies to output mappings, not to embedded binary paths being rewritten

**Severity**: Medium. Requires attacker control of bottle content (mitigated by checksums), but if achieved, could write to arbitrary locations.

**Recommendation**: Add explicit `..` validation in `extractBottlePrefixes()` or `relocatePlaceholders()` before performing replacement.

### 1.2 Null Byte Injection

The path extraction uses `strings.IndexAny(remaining, " \t\n\r'\"<>;:|")` to find delimiters. Null bytes are not in this set. A crafted binary could embed:

```
/tmp/action-validator-XXX/.install/formula/version\x00/malicious/suffix
```

The Go `strings.Index` would extract up to the null, but when written back to a binary, the null byte could truncate the path differently at runtime.

**Severity**: Low. The attack is speculative and depends on how the target binary parses its embedded paths at runtime.

### 1.3 Unicode/Encoding Attacks

Paths are processed as UTF-8 strings. Malformed UTF-8 sequences or lookalike characters could potentially bypass validation or cause unexpected behavior.

**Severity**: Very Low. Go's string handling is generally robust, and Homebrew bottle paths are ASCII.

---

## Question 2: Mitigation Sufficiency

### 2.1 "Replacement path is always under $TSUKU_HOME"

**Assessment**: Partially sufficient.

The code sets `prefixPath` from `ctx.ToolInstallDir` or `ctx.InstallDir`, which are controlled by tsuku. However, the **suffix** comes from binary content. If the suffix contains path escape sequences (`../`), the final path escapes the intended directory.

```go
// Current vulnerable pattern
for prefix := range bottlePrefixes {
    newContent = bytes.ReplaceAll(newContent, []byte(prefix), prefixReplacement)
}
```

The proposed fix (returning map[fullPath]prefix) would change this to:
```go
suffix := fullPath[len(prefix):]
replacement := prefixPath + suffix  // suffix could contain ../
```

**Gap**: Suffix is not validated.

### 2.2 "Suffix extracted from binary content, not user input"

**Assessment**: Misleading framing.

Binary content from Homebrew bottles is external input. The distinction between "user input" and "binary content" is irrelevant for security purposes. The bottle content should be treated as untrusted.

**Mitigation by Checksum**: SHA256 verification (lines 106-108 in `homebrew.go`) ensures the bottle content matches what Homebrew published. This shifts trust to Homebrew's build infrastructure.

**Gap**: If Homebrew's build infrastructure is compromised, or if a formula maintainer is malicious, the checksum won't help.

### 2.3 "Path validation in install_binaries rejects `..` traversal"

**Assessment**: Correct but not applicable here.

The `validateBinaryPath()` function (lines 262-273 in `install_binaries.go`) validates output paths specified in recipe TOML. It does **not** validate paths embedded in binary files during relocation.

**Gap**: This mitigation applies to a different code path.

### 2.4 Recursive Copy Exclusion

**Assessment**: Sufficient.

The proposed `CopyDirectoryExcluding()` is a straightforward fix. The exclusion pattern is a literal string match, not a pattern that could be manipulated.

---

## Question 3: Residual Risk Assessment

### Risk Matrix

| Risk | Likelihood | Impact | Residual Risk | Escalation? |
|------|------------|--------|---------------|-------------|
| Crafted bottle with malicious paths | Very Low | High | Low | No |
| Homebrew supply chain compromise | Very Low | Critical | Low | No |
| Path suffix injection via `..` | Low | Medium | **Medium** | **Yes** |
| Null byte injection | Very Low | Low | Very Low | No |

### Escalation Recommendation

**Path suffix injection** should be escalated to engineering before the fix is merged. Adding suffix validation is a small code change:

```go
if strings.Contains(suffix, "..") {
    continue // Skip paths with traversal attempts
}
```

This is defense-in-depth. The primary protection (checksum verification) is solid, but adding this check costs little and prevents a class of attacks if the checksum protection is ever bypassed.

---

## Question 4: "Not Applicable" Justification Review

### 4.1 Download Verification: "Not applicable"

**Assessment**: Accurate.

The relocation fix operates on already-downloaded bottles. The download and checksum verification happen in `homebrew.go` lines 100-108, which is unchanged by this fix.

### 4.2 User Data Exposure: "Not applicable"

**Assessment**: Accurate.

The relocation only reads/writes binary files in the work directory. No user data is accessed.

### 4.3 Supply Chain Risks: "Unchanged"

**Assessment**: Technically accurate but incomplete framing.

While true that the fix doesn't change the supply chain (same Homebrew bottles), the design should acknowledge that the relocation logic **trusts** the content of those bottles in a new way.

Previously, if a bottle contained malicious embedded paths, the bug would cause installation to fail (wrong paths). The fix makes the embedded paths work correctly - including potentially malicious ones.

**Observation**: The fix increases the attack surface slightly by making embedded paths functional. Before: broken installation. After: potentially exploitable installation. This is a valid trade-off (making things work correctly), but should be acknowledged.

---

## Specific Code Review Notes

### homebrew_relocate.go

**Line 699-742 (extractBottlePrefixes)**:
- Path parsing is robust for the happy path
- Missing validation for `..` in extracted paths
- Delimiter set doesn't include null byte

**Line 215-218 (relocatePlaceholders)**:
- Straight `bytes.ReplaceAll` without output validation
- No length check on replacement string (decompression bomb variant?)

### homebrew.go

**Line 140-157 (validateFormulaName)**:
- Good: Blocks `..`, `/`, `\`, and special characters
- This only validates formula name, not embedded paths

### extract.go

**Line 19-35 (isPathWithinDirectory)**:
- Excellent: Uses `filepath.Abs` and proper prefix checking
- This pattern should be applied to relocation output paths

### install_binaries.go

**Line 262-274 (validateBinaryPath)**:
- Good pattern but only for recipe output paths
- Not used during relocation

---

## Recommendations

### Priority 1 (Before Merge)

Add path validation in `relocatePlaceholders()`:

```go
for fullPath, prefix := range bottlePrefixes {
    suffix := fullPath[len(prefix):]

    // Security: Reject paths with traversal attempts
    if strings.Contains(suffix, "..") {
        fmt.Printf("   Warning: Skipping path with traversal attempt: %s\n", fullPath)
        continue
    }

    replacement := prefixPath + suffix
    newContent = bytes.ReplaceAll(newContent, []byte(fullPath), []byte(replacement))
}
```

### Priority 2 (Before Merge)

Add a unit test that verifies `..` paths in bottle content are rejected:

```go
func TestRelocatePlaceholders_RejectsTraversal(t *testing.T) {
    content := []byte("/tmp/action-validator-123/.install/formula/1.0/../../../etc/passwd")
    // ... verify this path is not replaced or causes error
}
```

### Priority 3 (Future)

Consider using `isPathWithinDirectory()` from `extract.go` to validate the final replacement path is within `$TSUKU_HOME`. This is defense-in-depth against edge cases.

---

## Conclusion

The security analysis in the design document is reasonable but has one notable gap: path suffix injection via `..` sequences. The mitigations are mostly sufficient due to checksum verification shifting trust to Homebrew, but adding explicit suffix validation is low-cost and provides defense-in-depth.

**Verdict**: Approve with minor changes (add suffix validation before merge).

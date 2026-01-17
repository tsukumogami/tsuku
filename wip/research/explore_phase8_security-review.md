# Security Review: Tier 2 Dependency Resolution Design

**Document:** `docs/designs/DESIGN-library-verify-deps.md`
**Reviewer:** Security Analysis Agent
**Date:** 2026-01-16

---

## Executive Summary

The Tier 2 dependency resolution design is **generally sound** with appropriate scoping of security considerations. However, this review identifies several attack vectors that warrant additional mitigation, one "not applicable" justification that needs qualification, and residual risks that should be explicitly documented.

**Risk Assessment:** Medium-Low
- The feature is read-only and local-only, limiting attack surface
- Main risks involve malicious input parsing and path validation edge cases
- No privilege escalation or network exposure vectors

---

## 1. Attack Vectors Not Considered

### 1.1 Symlink-Based Path Traversal

**Risk:** Medium

The design states paths are validated to stay within `$TSUKU_HOME/libs/` or system paths. However, symlinks within the libs directory could be exploited:

```
$TSUKU_HOME/libs/evil-1.0/lib -> /etc
```

If a dependency path resolves to `$TSUKU_HOME/libs/evil-1.0/lib/passwd`, it passes the prefix check but actually reads from `/etc/passwd`.

**Attack Scenario:**
1. Attacker creates a malicious recipe that installs a library with symlinks
2. Library's dependency list references the symlink path
3. Tier 2 validation follows the symlink during resolution
4. Could potentially validate or report on files outside tsuku's control

**Mitigation Recommendation:**
- Use `filepath.EvalSymlinks()` on resolved paths before validation
- Compare the evaluated path against allowed directories
- Reference: `set_rpath.go:328-334` already has symlink checking for wrapper creation

### 1.2 Race Condition (TOCTOU)

**Risk:** Low

Between path validation and file reading for Tier 1 header validation, the file could be replaced (Time-of-Check-to-Time-of-Use):

1. Validate path is within `$TSUKU_HOME/libs/`
2. Attacker replaces file with symlink to sensitive file
3. Tier 1 validation reads the wrong file

**Mitigation Recommendation:**
- Document as accepted residual risk (user controls their own filesystem)
- Consider opening file once and passing the file descriptor to both checks
- Low priority since attacker needs local filesystem access

### 1.3 Billion Laughs / ZIP Bomb Equivalent for Dependencies

**Risk:** Low

A malicious binary could have an extremely large number of `DT_NEEDED`/`LC_LOAD_DYLIB` entries, causing memory exhaustion during dependency extraction.

**Current State:**
- Go's `debug/elf` and `debug/macho` packages have reasonable limits
- Tier 1 already extracts dependencies (`HeaderInfo.Dependencies`)

**Mitigation Recommendation:**
- Add a sanity check: reject libraries with >1000 dependencies
- Document expected limit in design

### 1.4 Pattern Bypass via Path Normalization Differences

**Risk:** Medium

System library patterns could be bypassed through path variations:

```
Pattern: "/usr/lib/"
Bypass:  "/usr//lib/"  or  "/usr/lib/../lib/"
```

**Mitigation Recommendation:**
- Apply `filepath.Clean()` to all dependency paths before pattern matching
- The design mentions this for RPATH validation but should be explicit for dependency paths

### 1.5 Environment Variable Injection via Unexpanded Variables

**Risk:** Low

If path expansion is incomplete, unexpanded variables could be logged or passed to other functions:

```
Dependency: "$MALICIOUS_VAR/../../../etc/passwd"
```

**Current State:**
- Design specifies only known prefixes are expanded
- Unknown variables should remain literal

**Mitigation Recommendation:**
- After expansion, reject any paths still containing `$` or `@` prefixes
- Treat unexpanded variables as errors, not warnings

---

## 2. Evaluation of Existing Mitigations

### 2.1 Path Traversal Mitigation

**Current:** "Validate resolved paths stay within `$TSUKU_HOME/libs/` or system paths"

**Assessment:** Partially Sufficient

**Gaps:**
- Does not specify symlink handling (see 1.1)
- Does not specify normalization before comparison
- Does not specify what happens with "../" after variable expansion

**Recommendation:**
Strengthen to: "Validate resolved paths using filepath.Clean() and filepath.EvalSymlinks(), then verify the canonical path starts with `$TSUKU_HOME/libs/` or matches system path patterns."

### 2.2 Parser Vulnerability Mitigation

**Current:** "Use Go's standard library parsers with panic recovery (already in Tier 1)"

**Assessment:** Good

**Observations:**
- Tier 1 (`header.go:103-112, 161-170, 218-227`) implements panic recovery
- Go's standard library is well-maintained
- Design correctly identifies theoretical bugs as residual risk

**Recommendation:**
No changes needed. Consider adding a comment that parser crashes should be logged for security monitoring.

### 2.3 False Sense of Security Mitigation

**Current:** "Document clearly that Tier 2 validates dependency presence, not integrity or symbols"

**Assessment:** Sufficient

**Observations:**
- Design explicitly states this limitation in multiple places
- Verification outcome levels table is clear

**Recommendation:**
Ensure the CLI output and user-facing documentation reinforce this message.

---

## 3. Residual Risk Assessment

### 3.1 Residual Risks Requiring Escalation

**None identified.** All residual risks are acceptable given the threat model.

### 3.2 Residual Risks Requiring Documentation

| Risk | Likelihood | Impact | Current Documentation | Recommendation |
|------|------------|--------|----------------------|----------------|
| Go parser 0-day in debug/elf or debug/macho | Very Low | High | Mentioned | Sufficient |
| Malformed path variables escaping | Low | Medium | Mentioned ("bounded by file existence check") | Add symlink handling |
| Pattern list staleness | Medium | Low | Mentioned | Add update process documentation |
| Local TOCTOU attack | Very Low | Low | Not mentioned | Document as accepted |
| Excessive dependencies causing slowdown | Low | Low | Not mentioned | Add limit and document |

### 3.3 Accepted Residual Risks

The following risks are explicitly accepted:

1. **Theoretical Go parser bugs** - Mitigated by panic recovery and Go's security practices
2. **Pattern list incompleteness** - Addressed by maintainable pattern lists with reference implementation
3. **Symbol-level issues** - Explicitly deferred to Tier 3
4. **dlopen dependencies** - Explicitly out of scope

---

## 4. Evaluation of "Not Applicable" Justifications

### 4.1 Download Verification: "Not Applicable"

**Original Justification:**
> This feature does not download external artifacts. Tier 2 validation operates on already-installed libraries within `$TSUKU_HOME/libs/`.

**Assessment:** Needs Qualification

**Issue:** While Tier 2 itself does not download, it validates artifacts that were downloaded. The justification implies trust in the installation process, but should acknowledge:

1. If installation verification fails (checksums disabled, bypassed, or compromised), Tier 2 operates on potentially malicious binaries
2. Tier 2's parser could be targeted by crafted malicious binaries designed to exploit Go's ELF/Mach-O parsers

**Recommendation:** Revise to:

> **Defense in Depth Context** - Tier 2 operates on already-installed libraries within `$TSUKU_HOME/libs/`. The primary download verification (checksums) occurs during installation. Tier 2 assumes libraries have passed installation verification but provides additional defense via panic recovery in binary parsing, which limits exposure to malformed inputs.

### 4.2 Execution Isolation: Correctly Documented

The claims are accurate:
- Reads only from trusted directories
- Uses standard library parsing (no code execution)
- Read-only operation

### 4.3 Network Access: Correctly Documented

Tier 2 has no network access requirements.

### 4.4 Privilege Escalation: Correctly Documented

No setuid or privilege elevation.

### 4.5 User Data Exposure: Correctly Documented

Only technical metadata is accessed.

---

## 5. Additional Security Observations

### 5.1 Missing: Input Size Limits

The design should specify limits on:
- Maximum number of dependencies to process per library
- Maximum dependency path length
- Maximum RPATH entry count

These prevent resource exhaustion attacks.

### 5.2 Missing: Error Message Information Leakage

Error messages revealing full paths could leak system information:

```
Dependency missing: /home/user/private/secret-project/lib/libfoo.so
```

**Recommendation:** In warning/error output, consider redacting or abbreviating paths outside `$TSUKU_HOME`.

### 5.3 Good Practice: Compiled-in Pattern Lists

The design specifies:
> Pattern lists for system libraries are compiled into the binary, not fetched externally

This is good security practice - prevents remote pattern injection.

### 5.4 Opportunity: Audit Logging

Consider adding audit logging for:
- Libraries with unusual dependency counts
- Dependencies that trigger warnings
- Validation failures

This aids security monitoring without adding complexity.

---

## 6. Recommendations Summary

### Critical (Address Before Implementation)

None identified.

### High Priority

1. **Add symlink resolution** - Use `filepath.EvalSymlinks()` before path validation
2. **Add path normalization** - Apply `filepath.Clean()` to dependency paths before pattern matching
3. **Qualify download verification N/A** - Acknowledge defense-in-depth assumptions

### Medium Priority

4. **Add dependency count limit** - Reject libraries with >1000 dependencies
5. **Add path length limits** - Reject individual dependency paths exceeding reasonable length (e.g., 4096 chars)
6. **Reject unexpanded variables** - Treat `$` or `@` prefixes remaining after expansion as errors

### Low Priority (Documentation Only)

7. **Document TOCTOU as accepted risk** - User controls their filesystem
8. **Add pattern update process** - Document how to update system library patterns
9. **Consider path redaction in errors** - For paths outside `$TSUKU_HOME`

---

## 7. Conclusion

The Tier 2 dependency resolution design demonstrates good security awareness with appropriate scoping. The identified gaps are addressable with minor additions to the implementation approach. No escalation to external security review is warranted.

The main improvement opportunity is strengthening the path validation to handle symlinks and ensure normalization before pattern matching. These are straightforward additions that align with patterns already present in the codebase (see `set_rpath.go` for symlink checking precedent).

**Overall Assessment:** Proceed with implementation after addressing High Priority items.

---

## Appendix: Code References

| File | Relevance |
|------|-----------|
| `internal/verify/header.go` | Tier 1 implementation with panic recovery |
| `internal/verify/types.go` | Error categories and data structures |
| `internal/actions/set_rpath.go` | Path validation and symlink handling precedent |
| `test/scripts/verify-no-system-deps.sh` | Reference pattern lists |

# Design Review: Tier 2 Dependency Resolution

**Design Document:** `docs/designs/DESIGN-library-verify-deps.md`
**Review Date:** 2026-01-16
**Reviewer:** Options analysis review

---

## Executive Summary

The design document is well-structured and addresses a real problem. The problem statement is specific enough to evaluate solutions, though some edge cases need clarification. Option 2 appears to be a strawman due to the fundamental macOS dyld cache limitation. One potentially valuable alternative is missing. The recommendation for Option 3 (Hybrid) is sound, but some refinements would strengthen the design.

---

## 1. Problem Statement Specificity

### Strengths

The problem statement is concrete and actionable:
- Clear categorization of the three dependency types (system, tsuku-managed, missing)
- Specific examples of runtime errors users would encounter
- Well-defined scope with explicit in/out boundaries
- Platform-specific challenges clearly enumerated

### Gaps to Address

**Gap 1: What constitutes a "verification failure"?**

The design should clarify the distinction between:
- **Hard failures**: Dependency definitely missing (should fail verification)
- **Soft warnings**: Dependency unresolvable but may exist (should warn but not fail)
- **Unknown**: Cannot determine status (should log for debugging)

For example, an absolute path like `/opt/custom/lib/libfoo.so` is neither system nor tsuku-managed. Should this:
- Fail verification?
- Warn and continue?
- Be checked for existence on disk?

**Recommendation:** Add a section defining failure severity levels and how each dependency category maps to them.

**Gap 2: What about weak dependencies?**

ELF binaries can have `DT_NEEDED` (hard) and also reference optional libraries via `dlopen()` in code (not encoded in headers). The scope says "only direct dependencies" but should explicitly note that dynamically loaded libraries are out of scope.

**Recommendation:** Add to Out of Scope: "Dynamically loaded libraries (dlopen calls encoded in code, not headers)"

**Gap 3: Versioned library matching**

The scope excludes "Version compatibility checking (e.g., SONAME version matching)" but the examples use versioned names like `libc.so.6`. Should `libc.so.6` match a pattern for `libc.so.*`? What about symlink resolution (`libstdc++.so` -> `libstdc++.so.6` -> `libstdc++.so.6.0.33`)?

**Recommendation:** Clarify that pattern matching handles version suffixes and symlinks are followed for tsuku-managed libraries.

---

## 2. Missing Alternatives

### Option 4: RPATH-Aware Resolution (Missing)

There is a fourth approach not considered that sits between Option 2 and Option 3:

**Approach:**
- Extract RPATH/RUNPATH from the binary being verified
- For each dependency:
  - If it matches system library patterns: skip
  - If it uses `$ORIGIN`, `@rpath`, etc.: resolve against extracted RPATH and verify file exists
  - If absolute path: check if in tsuku paths or report
- Do not check file existence for system paths (avoiding dyld cache problem)

**Pros:**
- More accurate than pure pattern matching (Option 1)
- Avoids dyld cache problem (unlike Option 2)
- Simpler than two-phase hybrid (Option 3)
- Uses RPATH information that's already needed for resolution

**Cons:**
- Requires RPATH extraction (additional parsing, though Go stdlib supports this)
- Still needs pattern matching for system library identification

**Analysis:** This is essentially a refined version of Option 3 where the "selective resolution" phase is RPATH-driven rather than ad-hoc. The distinction may be subtle, but explicitly using RPATH makes the resolution logic more principled and testable.

**Recommendation:** Either add this as Option 4 or incorporate RPATH-driven resolution into Option 3's description.

### Consideration: Lazy Resolution

Another approach would be to defer dependency resolution entirely to Tier 3 (dlopen). If dlopen succeeds, all dependencies were resolved. If it fails, the error message tells us which dependency is missing.

**Pros:**
- Zero false positives (system decides what's missing)
- No pattern lists to maintain
- Simple implementation (skip Tier 2 entirely)

**Cons:**
- Less informative error messages (dlopen gives one error, not a list)
- Executes library code (same security concern as Tier 3)
- Makes Tier 2 essentially optional/redundant

**Analysis:** This is a reasonable simplification but loses the "verify without executing" benefit. Mentioned for completeness but probably not worth adding as a formal option.

---

## 3. Pros/Cons Evaluation

### Option 1: Pattern-Based System Library Detection

**Pros assessment:** Fair and complete.

**Missing cons:**
- **Overly permissive risk**: If a pattern is too broad (e.g., `/usr/lib/*`), it could skip verification of libraries that should be checked. A tsuku library accidentally installed to `/usr/lib/` would be missed.
- **Glob vs regex ambiguity**: The document mentions "patterns" but doesn't specify the matching mechanism. Glob patterns (`libc.so*`) behave differently from regexes.

**Recommendation:** Add note about overly permissive patterns and specify matching semantics.

### Option 2: Path Resolution with File Existence Check

**Strawman assessment:** This option has a fundamental flaw (macOS dyld cache) that makes it unusable on a major supported platform. The cons adequately describe this, but the option feels included mainly to be rejected.

**Is this a strawman?** Arguably yes. The "Poor" ratings in the evaluation table and the explicit statement that it "Fails on macOS Big Sur+" make it clear this option is non-viable. However, including it serves the educational purpose of explaining why pure resolution doesn't work.

**Recommendation:** Consider reframing as "Why pure resolution doesn't work" rather than a full "Considered Option" to avoid the appearance of padding the options list. Alternatively, acknowledge in the intro that this is included for completeness despite known limitations.

### Option 3: Hybrid Approach

**Pros assessment:** Fair and complete.

**Missing cons:**
- **Boundary definition complexity**: The "unknown absolute paths: report as warning" rule is vague. What paths are "unknown"? This could generate noise for legitimate configurations.
- **Testing complexity**: Two-phase logic is harder to test exhaustively than single-phase.

**Additional consideration:**
The "first pass / unknown / tsuku paths" classification is not fully specified. What are the exact rules? This matters for implementation.

**Recommendation:** Add pseudo-code or flowchart for the classification logic.

---

## 4. Unstated Assumptions

### Assumption 1: System library lists are knowable

The design assumes we can enumerate "most" system libraries. This is reasonable for Linux (glibc/musl core libraries) and macOS (libSystem, frameworks), but edge cases exist:
- Third-party system-wide libraries (e.g., `libssl` from Homebrew on macOS)
- Enterprise environments with custom library paths
- Container environments with different base images

**Impact:** May generate false warnings for legitimate system-wide libraries not in the pattern list.

**Recommendation:** Add an escape hatch for users to extend the system library patterns via configuration (e.g., `$TSUKU_HOME/config.toml` with `[verify] system_library_patterns = [...]`).

### Assumption 2: Dependencies extracted from headers are complete

`DT_NEEDED` and `LC_LOAD_DYLIB` list direct dependencies, but:
- Libraries loaded via `dlopen()` at runtime are not listed
- Weak symbols may reference optional libraries
- Some libraries use `@rpath` in ways that require knowing the full load chain

**Impact:** Tier 2 verification may pass but Tier 3 (dlopen) or actual usage may fail.

**Recommendation:** Acknowledged in scope as "direct dependencies only" but could note this limitation more explicitly in trade-offs.

### Assumption 3: RPATH values are well-formed

The design assumes `$ORIGIN`, `@rpath`, etc. can be resolved. But:
- `$ORIGIN` depends on knowing the binary's actual installed location
- `@rpath` may have multiple entries that need to be searched in order
- Some binaries have broken RPATH from incorrect build configurations

**Impact:** Resolution may fail for edge cases with complex RPATH configurations.

**Recommendation:** The existing `set_rpath.go` code shows RPATH handling patterns. Reference this and note that Tier 2 reuses those patterns.

### Assumption 4: File existence check is sufficient

For tsuku-managed libraries, the design checks if the file exists. But:
- File could exist but be empty (0 bytes)
- File could be a dangling symlink
- File could be the wrong architecture (different library with same name)

**Impact:** Could miss certain classes of installation failures.

**Recommendation:** For tsuku-managed libraries, consider doing a quick header validation (reuse Tier 1) rather than just existence check. This adds minimal overhead and catches more issues.

### Assumption 5: Alpine/musl compatibility

The uncertainties section mentions Alpine Linux but doesn't address it. Key differences:
- musl uses `libc.musl-x86_64.so.1` instead of `libc.so.6`
- Different loader name (`ld-musl-x86_64.so.1` vs `ld-linux-x86-64.so.2`)
- Some libraries have different names or don't exist

**Impact:** Pattern lists may need musl-specific entries.

**Recommendation:** Add musl patterns to the allowlist or document that Alpine is not yet fully supported.

---

## 5. Evaluation Table Assessment

The evaluation table is useful but has some issues:

### Rating Subjectivity

- "Good" vs "Fair" for maintainability isn't clearly justified
- "Poor" for Option 2 cross-platform is accurate but severe (it's effectively "broken")

### Missing Criteria

Consider adding:
- **Debuggability**: How easy is it to diagnose verification failures?
- **Extensibility**: How easy is it to add new platforms or library patterns?
- **Predictability**: Will the same binary give the same result on different machines?

### Recommendation

Add a brief justification below the table for each rating, especially where options differ (e.g., why is Option 1 "Fair" and Option 3 "Good" for false positives?).

---

## 6. Recommendations Summary

### Critical (Must Address)

1. **Clarify failure severity levels** - Define what causes hard failures vs warnings
2. **Add RPATH resolution to Option 3** - The "selective resolution" should be RPATH-driven, not ad-hoc
3. **Reframe Option 2** - Either remove or explicitly label as "for completeness"

### Important (Should Address)

4. **Add extensibility escape hatch** - Let users extend system library patterns
5. **Add musl/Alpine patterns** - Address the stated uncertainty
6. **Validate tsuku-managed deps with Tier 1** - Not just existence, header validation too

### Nice to Have

7. **Add classification flowchart** - Visual aid for the two-phase logic
8. **Expand evaluation table** - Add debuggability, extensibility criteria
9. **Note dlopen-only alternative** - Mention that skipping Tier 2 entirely is an option (simplicity vs granularity trade-off)

---

## 7. Conclusion

The design is solid and the Hybrid approach (Option 3) is the right choice. The main improvements needed are:

1. Making the classification logic more explicit (possibly with pseudo-code)
2. Acknowledging that Option 2 is non-viable and restructuring accordingly
3. Addressing the Alpine/musl uncertainty with concrete patterns
4. Adding an extensibility mechanism for enterprise/custom environments

With these refinements, the design will be ready for implementation.

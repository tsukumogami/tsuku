# Phase 4 Review: Libtool Zig CC Design

## Problem Statement Review

### Specificity Assessment

The problem statement is **well-defined and specific enough** to evaluate solutions against. It clearly identifies:

1. **The symptom**: Libtool-based builds fail with zig cc, specifically with "no acceptable ld found in $PATH"
2. **The root cause**: Libtool queries `$CC -print-prog-name=ld` which zig cc does not support
3. **The scope**: Making zig cc work with libtool detection, not full GCC compatibility
4. **The test case**: gdbm-source in the No-GCC Container test (currently disabled)

The problem statement includes concrete evidence (the error message from configure) and links to the specific issue (#856 and disabled CI test in #747).

### Missing Context

A few minor items could strengthen the document:

1. **Libtool version scope**: The document mentions "Some libtool versions may use different detection mechanisms" as an uncertainty but doesn't specify which libtool versions were tested. Since the CI uses Ubuntu 22.04 container, documenting the libtool version there (2.4.6 likely) would help scope the solution.

2. **Other `-print-prog-name` values**: The document correctly focuses on `ld` but libtool may also query `-print-prog-name=ar` or `-print-prog-name=ranlib`. Since tsuku already creates wrappers for these tools, documenting whether the solution should handle them proactively (or explicitly defer) would be helpful.

3. **Clang precedent**: Zig's cc mode is clang-based. Documenting how clang handles `-print-prog-name=ld` (if at all) could inform whether this is truly a zig-specific gap or an inherent clang limitation.

## Options Analysis Review

### Completeness

The options are **reasonably complete**. The document considers four approaches spanning different layers (wrapper enhancement, environment variables, test change, combined).

One **missing alternative worth considering**:

**Option 5: Libtool M4 Macro Override**

Many autotools projects include a `m4/` directory with custom macros. An approach could inject a custom `LT_PATH_LD` macro that short-circuits the detection. However, this would require modifying source trees during build, which violates tsuku's design principle of non-invasive installation. The document could mention this as a rejected alternative to demonstrate thoroughness.

Another approach worth brief mention:

**Option 6: Patch Libtool at Runtime**

Dynamically patch the generated `libtool` script after configure runs (similar to how Homebrew handles relocations). This is fragile and version-dependent but is used by some package managers. Rejecting this with rationale would strengthen the options section.

### Balance

The pros/cons for each option are **generally fair and balanced**:

**Option 1 (Wrapper Enhancement)**
- Pros are accurate: directly fixes the issue, minimal change, returns correct path
- Cons are appropriate: adds complexity (though "6 lines" is reasonable), partial coverage
- The "Fair" rating for maintainability is justified given the shell logic addition

**Option 2 (Cache Variables)**
- The con "may break with libtool updates" is valid but perhaps overstated. The `lt_cv_path_LD` variable is a long-standing libtool convention unlikely to change.
- The existing use of `lt_cv_sys_lib_dlsearch_path_spec=` in configure_make.go (line 345) demonstrates the pattern is already established in tsuku. This precedent weakens the "requires changes to configure_make action" con since the codebase already uses this technique.
- The con "Only works when tsuku controls the environment" is less relevant since tsuku always controls the configure environment for recipes.

**Option 3 (Different Test Recipe)**
- Correctly identified as avoiding rather than solving the problem
- The "Poor" correctness rating is appropriate

**Option 4 (Combined)**
- The "over-engineered" characterization is fair given the specific, narrow problem

### Strawman Detection

**No strawmen detected.** All options appear genuinely viable:

- **Option 1**: Proposed solution, clearly favored
- **Option 2**: Viable alternative with documented trade-offs; the existing `lt_cv_` usage in the codebase suggests this is a proven pattern
- **Option 3**: Valid if the goal were purely "re-enable CI test" rather than "fix libtool compatibility"
- **Option 4**: Would work but rightly deemed excessive for a narrow issue

The evaluation matrix gives reasonable ratings across decision drivers. Option 3 appropriately scores "Poor" on correctness and test coverage since it doesn't fix the actual problem.

## Unstated Assumptions

The following assumptions should be made explicit:

1. **`zig ld.lld` is libtool-compatible**: The document assumes that once libtool detects the linker path, `zig ld.lld` will work correctly with libtool's generated linker commands. The Uncertainties section partially addresses this ("We haven't validated whether lld's behavior is compatible with what libtool expects after detection") but this should be elevated to an explicit assumption with mitigation (the existing test will validate this).

2. **Single argument checking is sufficient**: The shell script checks `for arg in "$@"` but `-print-prog-name=ld` could theoretically appear as two arguments (`-print-prog-name ld`). GCC accepts the `=` form only, but this assumption should be documented.

3. **Only one `-print-prog-name` call per configure run**: The solution assumes libtool won't query multiple programs in a single cc invocation (e.g., `-print-prog-name=ld -print-prog-name=ar`). This is correct for libtool but worth noting.

4. **The ld wrapper location is stable**: The solution hardcodes the ld wrapper path at wrapper generation time. If tsuku ever moves wrapper directories or supports multiple zig versions simultaneously, this path could become stale. The current architecture suggests this is safe, but the assumption should be documented.

5. **Shell compatibility**: The wrapper uses POSIX shell (`#!/bin/sh`) with `for`/`case` constructs. These are portable, but the assumption that all target systems have a POSIX-compliant `/bin/sh` should be explicit (likely satisfied given zig cc's target environments).

## Recommendations

### Critical

1. **Validate Option 2's existing pattern**: The codebase already uses `lt_cv_sys_lib_dlsearch_path_spec=` in `configure_make.go:345`. Update Option 2's analysis to acknowledge this precedent, which strengthens its viability as an alternative. Consider whether Option 2's "Poor" maintainability rating should be upgraded to "Good" given the existing pattern.

### High Priority

2. **Add explicit assumptions section**: Move the implicit assumptions listed above into the document, either in the Uncertainties section or a new "Assumptions" section.

3. **Document `-print-prog-name` argument form**: Note that GCC only accepts `-print-prog-name=<prog>` (not `-print-prog-name <prog>` as separate arguments), validating the shell script approach.

4. **Consider proactive handling of ar/ranlib**: Since tsuku already creates `ar` and `ranlib` wrappers, consider whether the wrapper enhancement should handle `-print-prog-name=ar` and `-print-prog-name=ranlib` proactively to avoid future issues.

### Medium Priority

5. **Add rejected alternatives briefly**: Mention libtool macro override and runtime patching as explicitly rejected approaches to demonstrate thoroughness in option exploration.

6. **Document tested libtool version**: Specify the libtool version in Ubuntu 22.04 (the CI container) to scope the solution's validation.

### Low Priority

7. **Note clang behavior**: A brief note on whether clang (which zig cc wraps) handles `-print-prog-name` would provide useful context. Testing `clang -print-prog-name=ld` locally would clarify whether this is a zig-specific omission or clang behavior that zig inherits.

## Summary

The design document is well-structured and the analysis is sound. The chosen Option 1 (Wrapper Enhancement) is appropriate for the problem. The main improvement areas are:

1. **Acknowledge existing `lt_cv_` usage** in the codebase when evaluating Option 2
2. **Document implicit assumptions** about linker compatibility, argument forms, and shell portability
3. **Consider proactive handling** of `ar`/`ranlib` in addition to `ld`

The problem statement is specific, the options are genuine (no strawmen), and the trade-offs are fairly represented. The decision rationale is clear and defensible.

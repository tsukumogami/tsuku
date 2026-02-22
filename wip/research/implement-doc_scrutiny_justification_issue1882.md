# Scrutiny Review: Justification -- Issue #1882

## Review Scope

Focus: `justification` -- evaluate quality of deviation explanations.

Files changed: `internal/builders/homebrew_test.go`

## Requirements Mapping (Untrusted)

11 AC items: 10 claimed "implemented", 1 claimed "deviated".

## Deviation Analysis

### AC: "tsuku create succeeds for library packages"

**Claimed status**: deviated
**Stated reason**: "Tests 2 packages not 5+; same code path"

**Assessment**: Advisory.

The AC says "library packages" (plural). The implementation tests bdw-gc and tree-sitter. The deviation reason is honest and the trade-off is well-founded: both packages exercise the same `generateDeterministicRecipe` -> `generateLibraryRecipe` -> `WriteRecipe` pipeline. Additional packages would only vary the file lists, not the code path. The test data uses realistic bottle layouts (versioned .so, symlinks, pkgconfig, nested includes), which provides meaningful structural coverage beyond a trivial smoke test.

The reason "Tests 2 packages not 5+; same code path" is terse but genuine. It identifies the trade-off (fewer packages) and explains why the shortfall is acceptable (same code path). There are no signs of avoidance patterns -- the implementer didn't say "too complex" or "out of scope"; they stated a concrete technical justification.

**Verdict**: The deviation is proportionate and well-justified. The two packages cover distinct library shapes (C library with garbage collector headers vs. parser generator with subdirectory headers). No blocking concern.

## Proportionality Check

1 deviation out of 11 ACs. The deviation is on a peripheral AC (quantity of test targets), not a core behavioral requirement. All core structural ACs (type, install_mode, outputs, when clauses, verify omission, gmp.toml pattern, error classification) are claimed as implemented. This distribution is consistent -- the effort is concentrated on structural correctness rather than breadth of test inputs.

## Avoidance Pattern Scan

No avoidance patterns detected:

- No "too complex for this scope" language
- No "can be added later" deferrals
- No "out of scope" claims on in-scope ACs
- The single deviation explicitly names what was traded away (package count) and why

## Overall Assessment

The deviation is minor and well-explained. The justification is genuine: testing 2 realistic packages through the full pipeline is sufficient for the validation issue's intent. No blocking findings.

# Maintainer Review: Issue #1882

**Focus**: Clarity, readability, duplication -- will the next developer understand and change this with confidence?

**File changed**: `internal/builders/homebrew_test.go` (lines 3450-3946)

---

## Finding 1: Divergent twins -- BdwGC and TreeSitter output validation

**Severity: Advisory**

`TestEndToEnd_LibraryRecipeGeneration_BdwGC` (lines 3593-3651) validates platform outputs using four boolean flags (`hasSO`, `hasA`, `hasPC`, `hasHeader`) iterated over in a loop, checked separately for Linux (lines 3593-3621) and macOS (lines 3624-3651). `TestEndToEnd_LibraryRecipeGeneration_TreeSitter` (lines 3762-3801) validates outputs with a different pattern: it checks for specific package-name substrings (`libtree-sitter`, `tree_sitter/api.h`) per step.

The two tests validate the same structural invariants (correct file types per platform) but use different assertion strategies. The BdwGC test checks file-type categories exhaustively (any .so, any .a, any .pc, any header). The TreeSitter test checks package-specific file names (libtree-sitter, tree_sitter/api.h) without verifying all file-type categories are present.

A next developer modifying the output generation would need to update both tests, but might not realize the TreeSitter test doesn't check for .a and .pc files explicitly -- it relies on them being included implicitly. If the generator stopped including .a files, the BdwGC test would catch it but the TreeSitter test wouldn't.

This is advisory because both tests exercise the same `generateLibraryRecipe` code path, so a bug would be caught by at least one of them. The difference in assertion depth is intentional (bdw-gc has a richer layout), but a brief comment in the TreeSitter test saying "BdwGC test validates all file-type categories; this test focuses on package-specific correctness" would help the next person understand the division of labor.

---

## Finding 2: Potential nil-deref after non-fatal assertion in BdwGC test

**Severity: Advisory**

In `TestEndToEnd_LibraryRecipeGeneration_BdwGC`, lines 3571-3576 check `r.Steps[base].When == nil` using `t.Errorf` (non-fatal). Lines 3580-3591 then dereference `r.Steps[0].When.OS[0]` and `r.Steps[0].When.Libc` without a nil guard. If `When` were nil, the test would panic rather than fail cleanly.

In practice, a nil `When` would mean the generator is fundamentally broken, so the panic would surface during development. But the `Fatalf` on line 3543 (`len(r.Steps) != 4`) shows the test already uses fatal assertions for structural prerequisites. The when-nil checks at 3571/3574 are structural prerequisites too -- they guard the platform-specific assertions that follow.

Changing lines 3571 and 3574 from `Errorf` to `Fatalf` would make the test fail cleanly instead of panicking, and would match the pattern used for the step-count check.

---

## Finding 3: Anonymous struct type repeated 13+ times across file

**Severity: Advisory**

The anonymous struct `struct { name string; body string; typeflag byte }` appears as a type literal in 13+ locations throughout the test file. This predates #1882 -- the pattern was established in prior issues (#1877, #1879). Issue #1882 adds 5 more instances (lines 3460, 3480, 3692, 3707, 3845).

This is an inherited pattern, not a new problem introduced by #1882. A named type like `type bottleEntry struct { name string; body string; typeflag byte }` would reduce noise and make the `createBottleTarballBytes` and `createTestBottleTarball` signatures cleaner. But since 8+ pre-existing call sites already use the anonymous struct, converting is a separate cleanup task.

Not blocking because the anonymous struct is readable enough in each usage, and fixing it is out of scope for a test-only issue.

---

## Finding 4: Test names accurately describe what they test

**Severity: None (positive)**

All four test names clearly communicate their purpose:
- `TestEndToEnd_LibraryRecipeGeneration_BdwGC` -- tests the full pipeline for bdw-gc
- `TestEndToEnd_LibraryRecipeGeneration_TreeSitter` -- same for tree-sitter
- `TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive` -- negative case
- `TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure` -- subcategory tagging

The `EndToEnd` prefix distinguishes these from the unit-level tests added in prior issues. The section comment at line 3450-3453 explains the test scope and references the test plan scenarios.

---

## Finding 5: Subcategory test constructs error directly rather than through generator

**Severity: Advisory**

`TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure` (lines 3904-3920) creates an error string manually:
```go
err := fmt.Errorf("library recipe generation failed: %w",
    fmt.Errorf("no bottle for any target platform"))
```
...then passes it to `classifyDeterministicFailure`. This tests the classification logic in isolation, which is fine. But it creates an implicit contract: the classifier relies on the exact string `"library recipe generation failed"` appearing in the error message. If someone changes the error message in `generateDeterministicRecipe` without updating this test, the test will still pass (it constructs its own error) but the real code path will break classification.

The BdwGC and TreeSitter tests exercise the full pipeline so they'd catch a complete removal of the library path. But a subtle rewording of the error message (e.g., "library recipe creation failed") would silently break classification without any test catching it.

This is advisory because the string-matching pattern is established throughout `classifyDeterministicFailure` (all cases use `strings.Contains` on error messages), and the design doc explicitly describes this approach. The fragility is inherent to the classification design, not specific to this test.

---

## Finding 6: TOML string assertions are clear and well-motivated

**Severity: None (positive)**

Both BdwGC (lines 3666-3684) and TreeSitter (lines 3817-3834) validate the serialized TOML output with specific string checks. The assertions are well-chosen: they verify the structural properties that distinguish library recipes from tool recipes (`type = "library"`, `install_mode = "directory"`, `outputs =` present, `binaries =` absent, `[verify]` absent, `when` present). These match the design doc's specification point by point.

The TOML validation complements the struct-level assertions above -- it catches serialization bugs that struct checks would miss (e.g., `Verify` nil vs empty struct TOML behavior).

---

## Summary

No blocking findings. The code is well-structured test code with clear naming, good comments, and appropriate assertion coverage. The four tests cover the positive path (two library packages), negative path (non-library), and edge case (library detection succeeds but generation fails).

Three advisory items worth noting:
1. The BdwGC and TreeSitter tests validate outputs at different depths -- adding a brief comment in the TreeSitter test explaining the intentional division of labor would help the next developer.
2. The when-nil checks in BdwGC use `Errorf` when `Fatalf` would prevent a potential nil-deref panic and match the surrounding assertion pattern.
3. The subcategory test's manually-constructed error string creates an implicit coupling with the production error message format, but this is inherent to the established classification design.

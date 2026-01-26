# Issue 1115 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-platform-compatibility-verification.md`
- Sibling issues reviewed:
  - #1109 (libc detection) - Closed
  - #1110 (libc filter) - Closed, PR #1123
  - #1111 (step-level deps) - Closed
  - #1112 (enhanced *_install) - Closed
  - #1113 (supported_libc constraint) - Closed, PR #1132
  - #1114 (recipe migration) - Closed, PR #1134
  - #1116 (container tests) - Closed
- Prior patterns identified:
  - `SupportedLibc []string` field in `MetadataSection` (types.go:166)
  - `WhenClause.Libc` matching in `Matches()` method (types.go:302-315)
  - `ValidatePlatformConstraints()` validates `supported_libc` values (platform.go:209-218)
  - Recipe pattern: hybrid approach with glibc/musl/darwin steps (see zlib.toml, openssl.toml)
  - Step-level `Dependencies []string` field already exists (types.go:328)

## Gap Analysis

### Minor Gaps

1. **File location established**: The issue spec mentions `internal/recipe/coverage.go` which aligns with existing file organization patterns. No coverage.go file exists yet - this is expected as it's the implementation target.

2. **Testing patterns from siblings**: Looking at the test file patterns (types_test.go, platform_test.go, when_test.go), the coverage tests should follow similar structure.

3. **No existing `validate-recipes` command**: The issue references `--check-libc-coverage` flag on a `validate-recipes` command that doesn't exist. The current codebase only has a `validate` command for single recipes (cmd/tsuku/validate.go). The CI workflow (test.yml:255-317) manually loops through recipes calling `./tsuku validate --strict`. The implementation should either:
   - Add a new `validate-recipes` subcommand with the `--check-libc-coverage` flag, OR
   - Add the flag to the existing `validate` command and document the batch workflow

### Moderate Gaps

None identified. The spec is complete with respect to what prior issues implemented.

### Major Gaps

None identified.

## Recommendation

**Proceed**

The issue spec is complete and aligns well with the implemented patterns from prior sibling issues. The only minor adjustment is clarifying that a `validate-recipes` command needs to be created (not just a flag added), since no such command exists in the current CLI.

## Proposed Amendments

None required. The spec's acceptance criteria clearly indicate:
- New `CoverageReport` struct in `internal/recipe/coverage.go`
- New `AnalyzeRecipeCoverage()` function
- New `TestTransitiveDepsHaveMuslSupport()` test
- New `--check-libc-coverage` flag

The existing patterns provide clear implementation guidance:
- Use `WhenClause.Matches()` for platform matching
- Use `Recipe.Metadata.SupportedLibc` for constraint checking
- Follow existing test patterns in the package

## Additional Implementation Notes

The `validate-recipes` command mentioned in the acceptance criteria will need to be created. Looking at the CI workflow, it iterates over recipe files manually. A new `validate-recipes` command would:

1. Accept the `--check-libc-coverage` flag
2. Iterate over all embedded and registry recipes
3. Apply coverage validation to each recipe
4. Return errors for libraries missing musl, warnings for tools

This aligns with the design doc's Testing Infrastructure section (Layer 1: Recipe Validation).

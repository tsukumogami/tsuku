---
summary:
  constraints:
    - Library recipes must have musl support OR explicit supported_libc constraint
    - Tool recipes with library deps get warnings (non-blocking), not errors
    - CoverageReport must handle unconditional steps (count for all platforms)
    - Must detect explicit supported_libc constraints to allow documented exceptions
  integration_points:
    - internal/recipe/coverage.go (new file)
    - cmd/tsuku/main.go (add --check-libc-coverage flag to validate-recipes)
    - internal/recipe/types.go (uses existing MetadataSection.SupportedLibc)
    - internal/recipe/when.go (uses existing WhenClause for libc filtering)
  risks:
    - Matching libc conditions requires careful logic (unconditional steps, empty when clauses)
    - Transitive dependency checking must walk the full tree
    - Error vs warning distinction must be clear for libraries vs tools
  approach_notes: |
    Create CoverageReport struct and AnalyzeRecipeCoverage function that:
    1. Checks step when clauses for glibc/musl/darwin coverage
    2. Handles unconditional steps (count for all platforms)
    3. Detects explicit supported_libc constraints (allow opt-out)
    4. Generates errors for libraries missing musl (blocks merge)
    5. Generates warnings for tools with library deps missing musl (visible, non-blocking)

    Add TestTransitiveDepsHaveMuslSupport test to walk dependency tree.
    Add --check-libc-coverage flag to validate-recipes command.
---

# Implementation Context: Issue #1115

**Source**: docs/designs/DESIGN-platform-compatibility-verification.md (None)

## Design Excerpt

The key section from the design doc is "Layer 1: Recipe Validation" which describes:

1. **CoverageReport struct** with fields: Recipe, HasGlibc, HasMusl, HasDarwin, SupportedLibc, Warnings, Errors

2. **AnalyzeRecipeCoverage function** that:
   - Checks step when clauses for platform coverage
   - Handles unconditional steps (count for all platforms)
   - Detects explicit supported_libc constraints
   - Generates errors for libraries missing musl
   - Generates warnings for tools with library deps missing musl

3. **Validation behavior**:
   | Recipe Type | Has musl path | Has constraint | CI Result |
   |-------------|---------------|----------------|-----------|
   | Library | Yes | - | Pass |
   | Library | No | No | **Error** (blocks merge) |
   | Library | No | `supported_libc = ["glibc"]` | Pass (with note) |
   | Tool | Yes | - | Pass |
   | Tool | No | No | Warning (visible, non-blocking) |
   | Tool | No | Any constraint | Pass |

4. **Transitive dependency test**: TestTransitiveDepsHaveMuslSupport to ensure all transitive deps have musl support

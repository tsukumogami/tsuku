# Issue #793 Introspection

## Context Reviewed

- **Design doc**: `docs/DESIGN-system-dependency-actions.md`
- **Sibling issues reviewed**: All 11 prior issues in M30 (#754-766) - all closed/merged
- **Prior patterns identified**:
  - Testdata recipes live in `testdata/recipes/*.toml`
  - Action implementations in `internal/actions/*_actions.go` with corresponding tests
  - Recipe validation via `tsuku validate <recipe-file>` command
  - Test files follow naming pattern `<action>_test.go` with table-driven tests

## Gap Analysis

### Minor Gaps

1. **Testdata location assumption**: Issue assumes `testdata/` but doesn't specify the exact subdirectory. Based on existing patterns, should be `testdata/recipes/`.

2. **Naming convention not specified**: Existing testdata recipes use kebab-case with descriptive suffixes (e.g., `bash-source.toml`, `tool-a.toml`). Issue should follow this pattern with names like:
   - `sysdep-package-managers.toml` (for package manager actions)
   - `sysdep-configuration.toml` (for configuration actions)
   - `sysdep-verification.toml` (for verification actions)

3. **Action coverage detail**: The design doc defines 16 action types across categories:
   - **Package installation**: `apt_install`, `apt_repo`, `apt_ppa`, `dnf_install`, `dnf_repo`, `brew_install`, `brew_cask`, `pacman_install`, `apk_install`, `zypper_install`
   - **Configuration**: `group_add`, `service_enable`, `service_start`
   - **Verification**: `require_command`
   - **Fallback**: `manual`

   The acceptance criteria mention three categories but don't enumerate which specific actions from each category should be included. Recommending comprehensive coverage of all implemented actions.

4. **Recipe metadata requirements**: Based on existing testdata patterns, recipes need:
   - `[metadata]` section with `name` and `description`
   - `[[steps]]` sections with new M30 actions
   - `[verify]` section (though may not apply to all system dependency recipes)

   The issue doesn't specify metadata values to use.

5. **Implicit constraints validation**: The acceptance criteria state "Recipes use implicit constraints (no explicit `when` clauses for PM actions)" but don't clarify whether:
   - Configuration actions (`group_add`, `service_enable`) should have explicit `when` clauses (they should, per design doc examples)
   - Whether mixing PM actions from different families in one recipe is desired for testing

6. **Integration with existing test infrastructure**: The issue doesn't mention whether these fixtures should be integrated into existing test suites or remain standalone validation examples.

### Moderate Gaps

None identified. All gaps are resolvable from prior work and design doc context.

### Major Gaps

None identified. The issue spec is implementable given the completed M30 infrastructure.

## Recommendation

**Proceed** with minor clarifications incorporated into implementation.

## Proposed Implementation Details

Based on gap analysis, recommend creating three testdata recipes:

1. **`testdata/recipes/sysdep-package-managers.toml`**
   - Exercises: `apt_install`, `dnf_install`, `brew_install`, `pacman_install`, `apk_install`, `zypper_install`
   - Tests implicit constraints (no explicit `when` clauses on PM actions)
   - Includes `fallback` field example
   - Includes `unless_command` field example

2. **`testdata/recipes/sysdep-configuration.toml`**
   - Exercises: `group_add`, `service_enable`, `service_start`
   - Tests explicit `when` clauses (these actions don't have implicit constraints)
   - Realistic example: Docker group/service setup

3. **`testdata/recipes/sysdep-verification.toml`**
   - Exercises: `require_command`, `manual`
   - Tests verification flow after package installation
   - Tests manual action for unsupported platforms

All recipes should:
- Pass `tsuku validate` without errors
- Use kebab-case naming convention
- Include descriptive metadata
- Follow TOML formatting conventions from existing testdata
- Exercise the implicit constraint system established in #760

## Notes

- The design doc was updated on 2026-01-01 to add issues #793 and #794 to the implementation table
- All dependency issues (#754-766) are complete
- The action infrastructure, implicit constraints, validation, and description generation are all implemented
- These fixtures will serve dual purposes:
  1. Test coverage for M30 action types
  2. Reference examples for recipe authors

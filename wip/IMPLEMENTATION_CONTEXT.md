## Goal

Extend exclusion validation to check `code-validation-exclusions.json` for stale (closed) issue references.

## Context

Commit 5c32bc4 merged and showed green in CI despite 12 exclusions in `code-validation-exclusions.json` referencing closed issue #953. The exclusion system was supposed to catch this, but `validate-golden-exclusions.sh` only validates `testdata/golden/exclusions.json` (hardcoded at line 19).

**Two exclusion files exist:**
- `exclusions.json` - platform-specific exclusions (OS/arch), **validated**
- `code-validation-exclusions.json` - recipe-level code validation exclusions, **NOT validated**

## Acceptance Criteria

- [ ] `validate-golden-exclusions.sh` accepts `--file <path>` argument (defaults to `exclusions.json` for backward compatibility)
- [ ] `validate-golden-code.yml` validates `code-validation-exclusions.json` for stale issues
- [ ] 12 stale exclusions removed (those referencing closed #953)
- [ ] CI fails when any exclusion references a closed issue

## Dependencies

None

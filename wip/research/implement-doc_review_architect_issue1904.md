# Architect Review: Issue #1904 (chore: add container-images.json to CODEOWNERS)

## Review Focus: Architecture (design patterns, separation of concerns)

## Change Summary

The change adds a CODEOWNERS entry for `/container-images.json` at `.github/CODEOWNERS:12-16`, assigning `@tsukumogami/core-team` and `@tsukumogami/security-team` as required reviewers. This matches the protection level already applied to workflow files on line 10.

## Files Changed

- `.github/CODEOWNERS` -- Added 5 lines (comment block + rule) for `container-images.json`

## Findings

### Advisory: Embedded copy not covered by CODEOWNERS

**File:** `.github/CODEOWNERS`
**Severity:** Advisory

The CODEOWNERS rule covers `/container-images.json` (root) but not `internal/containerimages/container-images.json` (embedded copy). In theory, a PR modifying only the embedded copy could bypass the required review.

However, this is adequately mitigated by the CI drift-check job in `.github/workflows/drift-check.yml:22-42`. That job runs `go generate ./internal/containerimages/...` and fails if the embedded copy differs from the root. Any direct modification of the embedded copy that doesn't match the root file will be caught. And any modification of the root file triggers the CODEOWNERS rule.

Adding a CODEOWNERS rule for the embedded copy would be redundant (since it's a generated file) and slightly misleading (suggesting it should be edited directly). The current layered approach -- CODEOWNERS on the source file, CI check on the generated file -- is the right pattern.

**Verdict:** No action needed. Noting this for completeness.

## Design Alignment

The implementation matches the design doc's intent precisely:

- **Design doc (Security Considerations section):** "Add container-images.json to CODEOWNERS with the same review requirements as workflow files."
- **Implementation:** Uses the same `@tsukumogami/core-team @tsukumogami/security-team` teams as the workflow rule on line 10.

The comment block (lines 12-15) accurately describes the supply chain risk that motivates the rule, matching the design doc's rationale about centralization amplifying the impact of a single edit.

## Pattern Consistency

The CODEOWNERS file follows a consistent pattern:
1. Comment explaining what the rule protects and why
2. Path pattern with team assignments
3. Blank line separator

The new entry follows this pattern exactly. The file remains organized by sensitivity level (workflow + container config get both teams; scripts get core team only).

## Structural Assessment

This is a clean, minimal change that fits the existing CODEOWNERS structure. No architectural concerns.

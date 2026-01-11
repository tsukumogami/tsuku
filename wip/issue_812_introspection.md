# Issue 812 Introspection

## Context Reviewed
- Design doc: none (standalone CI fix)
- Sibling issues reviewed: #829 (golden script family support), #830 (workflow family support), #831 (contributing docs)
- Prior patterns identified: `--os` flag convention for platform filtering in golden scripts

## Gap Analysis

### Major Gaps

**1. Root Cause Analysis is Incorrect**

The issue states:
> "GitHub Actions is triggering it on `push` events, which causes immediate failure since `push` is not a valid trigger for this workflow."

This analysis is **wrong**. The actual root cause is:

```yaml
# INVALID: Cannot use both paths and paths-ignore on same trigger
paths:
  - 'internal/version/*.go'
  # ...
paths-ignore:
  - '**/*_test.go'
```

GitHub Actions does not allow both `paths:` and `paths-ignore:` on the same trigger event. The error message "This run likely failed because of a workflow file issue" is GitHub's generic message for invalid workflow YAML, not specifically about push events.

**Evidence**: All workflow runs show 0s duration failures with "workflow file issue" - this happens at YAML parsing time, not execution time.

**2. Missing Requirement: Platform-Specific Validation**

Since issues #829-831 were completed, golden files now include:
- Family-aware files (e.g., `v1.0.0-linux-debian-amd64.json`)
- Platform-specific pip hashes that differ between Linux and macOS

The workflow runs on `ubuntu-latest` only, but was attempting to validate ALL golden files including darwin ones. This causes failures because:
- pip hash resolution is host-dependent
- Some darwin golden files may be missing

The fix requires:
1. Adding `--os` flag to `validate-golden.sh` (new requirement)
2. Adding `--os` flag to `validate-all-golden.sh` (new requirement)
3. Using `--os linux` in the workflow since it runs on ubuntu-latest

### Minor Gaps

- The issue's "Suggested Fix" mentions adding `push` trigger, but this is not needed
- The workflow only validates linux files; darwin validation would need a macOS runner (out of scope)

## Recommendation

**Amend** - The issue spec's root cause is wrong but the symptom (workflow failing) is correct. The fix direction needs correction.

## Proposed Amendments

The issue should be amended to:

1. **Correct the root cause**: "The workflow YAML has both `paths:` and `paths-ignore:` on the same trigger event, which is invalid GitHub Actions syntax."

2. **Correct the solution**: "Replace `paths-ignore` with negation patterns in `paths` (e.g., `!internal/version/*_test.go`)"

3. **Add new requirement**: "Add `--os` flag to validation scripts and use `--os linux` in the workflow to avoid cross-platform validation failures from host-dependent pip hashes."

## Impact on Current PR

The PR #815 implementation is correct despite the wrong issue analysis:
- Removes `paths-ignore:` and uses negation pattern in `paths:`
- Adds `--os` flag to both validation scripts
- Uses `--os linux` in the workflow

The implementation fixes the actual problem, just not the problem described in the issue.

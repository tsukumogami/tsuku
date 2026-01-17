# Issue 974 Implementation Plan

## Summary

The `validate-golden-exclusions.sh` script only validates `testdata/golden/exclusions.json` (hardcoded at line 19), leaving `code-validation-exclusions.json` unvalidated for stale issue references. This allowed 12 exclusions referencing closed issue #953 to remain undetected. The fix adds a `--file` argument to the validation script and updates `validate-golden-code.yml` to validate both exclusion files.

## Files to Modify

- `scripts/validate-golden-exclusions.sh`: Add `--file <path>` argument to make the exclusion file configurable
- `.github/workflows/validate-golden-code.yml`: Add step to validate `code-validation-exclusions.json` for stale issues
- `testdata/golden/code-validation-exclusions.json`: Remove 12 stale exclusions referencing closed issue #953

## Implementation Steps

### Step 1: Update `validate-golden-exclusions.sh` to accept `--file` argument [DONE]

**Lines to modify: 3, 19, 23-36**

1. Update usage comment (line 3):
   ```bash
   # Usage: ./scripts/validate-golden-exclusions.sh [--file <path>] [--check-issues]
   ```

2. Change line 19 to set a default value only:
   ```bash
   EXCLUSIONS_FILE=""
   DEFAULT_FILE="$REPO_ROOT/testdata/golden/exclusions.json"
   ```

3. Extend the argument parsing loop (lines 23-36) to handle `--file`:
   ```bash
   while [[ $# -gt 0 ]]; do
       case "$1" in
           --file)
               if [[ -z "${2:-}" ]]; then
                   echo "Error: --file requires a path argument" >&2
                   exit 2
               fi
               EXCLUSIONS_FILE="$2"
               shift 2
               ;;
           --check-issues) CHECK_ISSUES=true; shift ;;
           -h|--help)
               echo "Usage: $0 [--file <path>] [--check-issues]"
               echo ""
               echo "Validate golden file exclusions."
               echo ""
               echo "Options:"
               echo "  --file <path>   Exclusion file to validate (default: testdata/golden/exclusions.json)"
               echo "  --check-issues  Verify linked issues are still open (requires GITHUB_TOKEN)"
               exit 0
               ;;
           *) echo "Unknown flag: $1" >&2; exit 2 ;;
       esac
   done
   ```

4. Add default file assignment after the loop:
   ```bash
   # Use default file if not specified
   if [[ -z "$EXCLUSIONS_FILE" ]]; then
       EXCLUSIONS_FILE="$DEFAULT_FILE"
   fi
   ```

### Step 2: Update `validate-golden-code.yml` to validate code-validation-exclusions [DONE]

**Add new step after line 71 (after "Build tsuku" step)**

Insert a new job step to validate `code-validation-exclusions.json`:

```yaml
      - name: Validate code-validation exclusions
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./scripts/validate-golden-exclusions.sh --file testdata/golden/code-validation-exclusions.json --check-issues
```

This step should run **before** the "Validate linux golden files" step since stale exclusions should fail fast.

**Recommended insertion point:** After "Build tsuku" (line 88), before "Validate linux golden files" (line 90).

### Step 3: Remove stale exclusions from `code-validation-exclusions.json` [DONE]

**Lines to remove:** All exclusions referencing issue #953 (12 total)

Recipes to remove:
- `ack` (line 5-9)
- `cobra-cli` (line 15-19)
- `curl` (line 20-24)
- `dlv` (line 25-29)
- `fpm` (line 30-34)
- `gofumpt` (line 35-39)
- `goimports` (line 40-44)
- `gopls` (line 45-49)
- `jekyll` (line 55-59)
- `ninja` (line 65-69)
- `readline` (line 75-79)
- `sqlite` (line 85-89)

After removal, the file should only contain 5 exclusions (those referencing open issue #961):
- `black`
- `httpie`
- `meson`
- `poetry`
- `ruff`

## Testing Strategy

### Local Testing

1. **Test argument parsing:**
   ```bash
   # Should show help with --file documented
   ./scripts/validate-golden-exclusions.sh --help

   # Should validate default file (exclusions.json)
   ./scripts/validate-golden-exclusions.sh

   # Should validate code-validation-exclusions.json
   ./scripts/validate-golden-exclusions.sh --file testdata/golden/code-validation-exclusions.json

   # Test with non-existent file
   ./scripts/validate-golden-exclusions.sh --file /nonexistent.json
   ```

2. **Test stale issue detection (before removing stale exclusions):**
   ```bash
   # This should FAIL with exit code 1 (12 stale exclusions)
   GITHUB_TOKEN=$(gh auth token) ./scripts/validate-golden-exclusions.sh \
       --file testdata/golden/code-validation-exclusions.json --check-issues
   ```

3. **Test after removing stale exclusions:**
   ```bash
   # This should PASS (only open issue #961 exclusions remain)
   GITHUB_TOKEN=$(gh auth token) ./scripts/validate-golden-exclusions.sh \
       --file testdata/golden/code-validation-exclusions.json --check-issues
   ```

4. **Verify backward compatibility:**
   ```bash
   # Original invocation should still work
   GITHUB_TOKEN=$(gh auth token) ./scripts/validate-golden-exclusions.sh --check-issues
   ```

### CI Verification

After PR is created:
1. The `validate-golden-code.yml` workflow should run and validate both exclusion files
2. The `validate-golden-recipes.yml` workflow should continue to work (unchanged invocation)

## Risks

### Low Risk: Argument parsing edge cases
- **Risk:** Script may not handle edge cases like `--file` without argument
- **Mitigation:** Added explicit check for missing argument with clear error message

### Low Risk: Different JSON schemas
- **Risk:** `code-validation-exclusions.json` has simpler schema (no `platform` field)
- **Mitigation:** The validation script only uses common fields (`recipe`, `issue`, `reason`) and ignores `platform`, so it works with both schemas

### Medium Risk: Stale exclusion cleanup may reveal real issues
- **Risk:** Removing the 12 exclusions for #953 may cause CI failures if those recipes have genuine validation issues
- **Mitigation:** Issue #953 (toolchain pinning) is closed as resolved. If recipes still fail, they need new exclusions with tracking issues

### Low Risk: GitHub API rate limiting
- **Risk:** Adding another validation step doubles API calls for issue status checks
- **Mitigation:** Each exclusion file has relatively few entries (17 in exclusions.json, 5 after cleanup in code-validation-exclusions.json). Well under rate limits.

## Order of Operations

1. First modify `validate-golden-exclusions.sh` (add --file support)
2. Then update `.github/workflows/validate-golden-code.yml` (add validation step)
3. Finally clean up `code-validation-exclusions.json` (remove stale exclusions)

This order ensures the validation infrastructure is in place before cleanup, and allows testing the detection of stale exclusions before removing them.

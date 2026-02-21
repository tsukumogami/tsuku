## Goal

Add homepage URL validation to the Go recipe validator so invalid URLs are caught at development time, not during website deployment.

## Context

The website deploy failed because `ansifilter.toml` had an `http://` homepage URL. The Python registry generator (`scripts/generate-registry.py`) requires HTTPS and rejects dangerous schemes, but the Go recipe validator (`internal/recipe/validator.go`) doesn't check homepage URLs at all.

This creates a validation gap: recipes pass CI validation, get merged, then break the website deploy pipeline. The fix should move this check upstream into the Go validator where `tsuku validate` catches it during PR review.

## Acceptance Criteria

- [ ] `validateMetadata()` rejects homepage URLs that don't start with `https://`
- [ ] `validateMetadata()` rejects homepage URLs containing dangerous schemes (`javascript:`, `data:`, `vbscript:`)
- [ ] Clear error messages match the existing validator style
- [ ] Test coverage for valid HTTPS URLs, HTTP URLs, and dangerous scheme URLs
- [ ] All existing recipes still pass validation (no regressions)

## Dependencies

None

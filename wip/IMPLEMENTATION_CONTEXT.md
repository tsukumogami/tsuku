## Summary

The sqlite-source build essentials test fails when resolving the `make` dependency from Homebrew registry with MANIFEST_UNKNOWN error from GHCR.

## Error

```
Failed to generate plan: failed to generate dependency plans: failed to generate plan for dependency make: failed to generate plan for make: failed to resolve step homebrew: failed to decompose homebrew: failed to decompose "homebrew": failed to get blob SHA: manifest request returned 404: {"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}
```

## Context

- Test installs `testdata/recipes/sqlite-source.toml`
- sqlite-source depends on make via Homebrew decomposition
- GHCR returns 404 MANIFEST_UNKNOWN for the make bottle

## Investigation Needed

- Check if make bottle was updated/removed from Homebrew
- May need to update make recipe or pin version

## Acceptance Criteria

- [ ] sqlite-source test passes reliably

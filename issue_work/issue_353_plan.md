# Issue 353 Implementation Plan

## Summary

Consolidate website and recipe registry deployments into a single Cloudflare Pages deployment by generating `recipes.json` as part of the website build and updating the frontend to fetch from same-origin.

## Approach

Modify the website deployment workflow to run the registry generation script before deployment, placing `recipes.json` in the website directory. Update the frontend `API_URL` to use a relative path.

### Alternatives Considered

- **Keep separate deployments with redirect**: Adds complexity without solving the core issues
- **Move script into website directory**: Unnecessary - can run from existing location with adjusted output path

## Files to Modify

- `.github/workflows/deploy-website.yml` - Add Python setup and registry generation step
- `website/recipes/index.html` - Change `API_URL` from external to same-origin path
- `.github/workflows/deploy-recipes.yml` - Rename and repurpose as deprecated/redirect (or delete)

## Files to Create

None

## Implementation Steps

- [ ] Update `deploy-website.yml` to generate `recipes.json` before deployment
- [ ] Update `website/recipes/index.html` to use `/recipes.json` instead of external URL
- [ ] Update path triggers to include recipe TOML files
- [ ] Deprecate `deploy-recipes.yml` (add note that registry.tsuku.dev is deprecated)
- [ ] Update design doc references (optional, can be separate PR)

## Testing Strategy

- Manual verification: Deploy to preview, verify `recipes.json` is accessible
- Visual inspection: Verify recipes page loads and displays correctly
- Cross-origin removed: Confirm no CORS headers needed

## Risks and Mitigations

- **Risk**: External consumers using `registry.tsuku.dev` directly
- **Mitigation**: Keep GitHub Pages deployment as fallback/redirect for a transition period

- **Risk**: Build may fail if Python or script fails
- **Mitigation**: Script is well-tested, Python 3.11 readily available in GitHub Actions

## Success Criteria

- [ ] `recipes.json` is generated and deployed with website
- [ ] Website fetches `recipes.json` from same origin (`/recipes.json`)
- [ ] Preview deployments work fully (can test recipe detail pages)
- [ ] `registry.tsuku.dev` documented as deprecated
- [ ] No breaking changes for existing API consumers

## Open Questions

None - approach is straightforward.

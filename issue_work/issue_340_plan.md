# Issue 340 Implementation Plan

## Summary

Add client-side routing to `recipes/index.html` using the History API to switch between grid and detail views based on URL path.

## Approach

Extend the existing JavaScript with a minimal router that:
1. Parses the URL to determine the current view (grid vs detail)
2. Provides a `navigateTo()` function for SPA-style navigation
3. Handles browser back/forward with `popstate`
4. Dispatches rendering to the appropriate view function

The router is intentionally minimal - it only handles two routes (`/recipes/` and `/recipes/<name>/`) without a generic routing framework.

### Alternatives Considered

- **External routing library**: Not chosen - overkill for 2 routes, adds dependency
- **Hash-based routing**: Not chosen - less clean URLs, worse for linking

## Files to Modify

- `website/recipes/index.html` - Add router functions, modify initialization
- `website/_redirects` - Add catch-all redirect for SPA (per design doc, but noted as #343)

## Files to Create

None - all changes in existing files

## Implementation Steps

- [ ] 1. Add `getViewFromURL()` function to parse URL and return view state
- [ ] 2. Add `navigateTo(path)` function using History API
- [ ] 3. Add `renderCurrentView()` dispatcher function
- [ ] 4. Add placeholder `renderDetailView()` and `render404()` stubs
- [ ] 5. Add `popstate` event listener for browser navigation
- [ ] 6. Modify `DOMContentLoaded` handler to use router
- [ ] 7. Update grid rendering to preserve search state across view switches

Note: The redirect rule (`/recipes/*` -> `/recipes/index.html`) is covered by #343, but I'll add it here since it's needed to test the router with direct URLs.

## Testing Strategy

- **Manual verification**:
  - Navigate to `/recipes/` - should show grid
  - Navigate to `/recipes/k9s/` - should show detail placeholder (or 404 if recipe not found)
  - Use browser back/forward - should switch views correctly
  - Click card (when #342 is done) - should navigate without reload

## Risks and Mitigations

- **Breaking existing grid**: Mitigation - router defaults to grid view, existing `renderRecipes()` unchanged
- **Direct URL 404**: Mitigation - add redirect rule; stub `render404()` shows friendly message

## Success Criteria

- [ ] `getViewFromURL()` correctly parses `/recipes/` as grid view
- [ ] `getViewFromURL()` correctly parses `/recipes/k9s/` as detail view with recipe name
- [ ] `navigateTo()` updates URL without page reload
- [ ] Browser back/forward buttons switch between views
- [ ] `renderCurrentView()` dispatches to grid or detail renderer

## Open Questions

None - design doc provides clear specifications.

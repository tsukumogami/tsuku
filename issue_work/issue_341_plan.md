# Issue 341 Implementation Plan

## Summary

Enhance the placeholder `renderDetailView` function to display complete recipe metadata including homepage link, install command with copy functionality, and dependencies grouped by type.

## Approach

The existing router and placeholder detail view are already in place from issue #340. This issue extends `renderDetailView()` to:
1. Add homepage link (external, secure)
2. Add install command with copy button (matching landing page pattern)
3. Add dependencies section (install and runtime dependencies)
4. Ensure dependency names link to their detail pages

All rendering uses safe DOM APIs (`textContent`, `createElement`) as required.

### Alternatives Considered
- None - the design document specifies the exact approach

## Files to Modify
- `website/recipes/index.html` - Enhance `renderDetailView()` function, add helper functions
- `website/assets/style.css` - Add styles for install command, dependencies section

## Files to Create
- None

## Implementation Steps
- [x] Add `renderInstallCommand()` helper function with copy button
- [x] Add `renderDependencies()` helper function with grouped display
- [x] Enhance `renderDetailView()` to call helper functions
- [x] Add CSS styles for install command box and dependency lists
- [ ] Test manually on preview deployment

Mark each step [x] after it is implemented and committed.

## Testing Strategy
- Unit tests: N/A (vanilla JavaScript, no test framework)
- Manual verification:
  - Visit `/recipes/` and click a recipe card
  - Verify name, description, homepage displayed
  - Verify install command shown with copy button
  - Test copy button copies command to clipboard
  - If recipe has dependencies, verify they display grouped
  - Verify dependency links navigate to detail pages
  - Verify "Back to Recipes" navigation works
  - Visit `/recipes/nonexistent/` and verify 404 view

## Risks and Mitigations
- **Risk**: No recipes currently have dependencies in production JSON
  - **Mitigation**: Dependencies section only renders when data present; code handles empty arrays gracefully
- **Risk**: Copy to clipboard may not work in all browsers
  - **Mitigation**: Use same pattern as landing page which already works

## Success Criteria
- [ ] Recipe name, description, and homepage link displayed
- [ ] Install command shown with copy functionality
- [ ] Dependencies displayed grouped by type (install vs runtime)
- [ ] Dependency names link to their detail pages
- [ ] "Back to Recipes" navigation works
- [ ] 404 view shown for unknown recipe names
- [ ] All content rendered via safe DOM APIs

## Open Questions
None - all requirements are clear from the design document.

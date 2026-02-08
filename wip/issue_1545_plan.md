# Issue 1545 Implementation Plan

## Summary

Add JavaScript to existing coverage dashboard HTML that fetches `/coverage/coverage.json` and renders an interactive coverage matrix with sort/filter capabilities, following the established pattern from `website/pipeline/index.html`.

## Approach

This implementation builds on the HTML structure created in #1544 by adding vanilla JavaScript that loads coverage data and populates the three dashboard views. The approach follows the proven pipeline dashboard pattern: fetch JSON, handle loading/error states, render views with template literals, and manage state with simple global variables.

The coverage matrix view (primary focus of this issue) will display all recipes in a table with columns for name, type, and platform support (glibc, musl, darwin). Sort controls allow ordering by name or type, and filter controls show recipes missing specific platform support. Visual indicators (✓/✗ with green/red colors) make support status immediately clear.

Since the HTML structure already exists with styled container divs (#coverage-matrix, #gap-list, #category-breakdown), this implementation only needs to add the `<script>` tag with data loading and rendering logic. The coverage.json format is well-defined by cmd/coverage-analytics and includes all necessary data: recipe metadata, platform support booleans, gaps arrays, and aggregate statistics.

### Alternatives Considered

- **Client-side framework (React/Vue)**: Would provide more sophisticated state management and reactivity, but breaks the "no frameworks" constraint established in the website component. The vanilla JS approach keeps dependencies minimal and page load fast.

- **Server-side rendering**: Could generate static HTML pages for each view, eliminating need for client-side JavaScript. However, this would require build infrastructure and complicate the deployment process. The static JSON + client-side rendering pattern matches existing website architecture.

- **Virtual scrolling for large dataset**: With 265 recipes, initial concern was rendering performance. However, pipeline dashboard demonstrates that this size renders quickly without optimization. Can revisit if performance issues arise.

## Files to Modify

- `website/coverage/index.html` - Add JavaScript implementation in existing `<script>` tag (lines 349-358), add CSS for interactive controls in existing `<style>` tag (lines 9-259)

## Implementation Steps

- [x] Add data loading function with fetch API and error handling (loadCoverageData)
- [x] Add state management variables (currentData, currentSort, currentFilter)
- [x] Implement coverage matrix rendering function (renderMatrix) with table generation
- [x] Add sort functionality (by name ascending/descending, by type)
- [x] Add filter functionality (show recipes missing specific platforms)
- [x] Implement platform cell rendering with visual indicators (✓ green, ✗ red)
- [x] Add sort controls UI (clickable table headers or dedicated buttons)
- [x] Add filter controls UI (dropdown or checkboxes above table)
- [x] Wire up event handlers for sort and filter controls
- [x] Add empty state handling ("No recipes found" when filters return nothing)
- [x] Add ARIA labels and accessibility attributes for screen readers
- [ ] Test responsive behavior at 320px, 768px, and 1280px widths
- [ ] Verify integration with existing HTML structure from #1544

## Testing Strategy

### Unit Testing

Not applicable - vanilla JavaScript with no build step means no unit test infrastructure. Testing will be manual and integration-focused.

### Manual Verification

**Data Loading**:
- Generate coverage.json with `go run ./cmd/coverage-analytics`
- Start local server: `python3 -m http.server 8000` from website/ directory
- Navigate to http://localhost:8000/coverage/
- Verify loading spinner appears briefly
- Verify matrix loads within 2 seconds
- Rename coverage.json temporarily to test error state
- Verify error message displays with retry button
- Verify retry button reloads data successfully

**Matrix Rendering**:
- Check table shows all recipes (count should match total_recipes in JSON)
- Verify recipe names are alphabetically sorted by default
- Verify platform cells show ✓ for supported platforms
- Verify platform cells show ✗ for unsupported platforms
- Verify colors: green for supported, red for unsupported
- Compare sample recipe entries against source JSON to verify accuracy

**Sorting**:
- Click recipe name header, verify A-Z sort
- Click again, verify Z-A sort
- Click type header, verify library/tool grouping
- Verify sort indicator shows current column and direction
- Apply filter, then change sort, verify both persist correctly

**Filtering**:
- Select "Missing musl" filter
- Verify only recipes with platforms.musl = false appear
- Verify count matches number of recipes shown
- Repeat for "Missing glibc" and "Missing darwin"
- Clear filter, verify all recipes return
- Apply filter that yields no results, verify "No recipes found" message

**Responsive Design**:
- Resize browser to 320px width (mobile)
- Verify table displays without breaking layout
- Check if horizontal scroll appears (acceptable if needed)
- Verify controls remain accessible and functional
- Test at 768px (tablet) and 1280px (desktop) widths
- Verify no layout issues at any width

**Accessibility**:
- Tab through controls, verify keyboard navigation works
- Press Enter on sort controls, verify sorting activates
- Use screen reader (VoiceOver/NVDA) to verify ARIA labels
- Verify table structure is semantic (thead/tbody)
- Verify platform cells announce support status ("glibc supported", "musl unsupported")

**Browser Compatibility**:
- Test in Chrome/Edge (latest)
- Test in Firefox (latest)
- Test in Safari (latest)
- Verify fetch API works in all browsers
- Verify template literals render correctly

**Integration**:
- Verify matrix renders in correct container (#coverage-matrix)
- Verify no JavaScript errors in console
- Check if loading/error state transitions work cleanly
- Verify CSS styles don't conflict with main stylesheet

## Risks and Mitigations

- **Risk**: coverage.json may not exist if cmd/coverage-analytics hasn't been run
  - **Mitigation**: Error handling already planned. Error state shows clear message with retry button. Include instructions in PR description for generating coverage.json.

- **Risk**: 265 recipes could cause performance issues during rendering
  - **Mitigation**: Pipeline dashboard handles similar dataset sizes without issue. If performance problems arise, can add pagination or virtual scrolling in follow-up issue. Start with full table render.

- **Risk**: Mobile table layout may be difficult to use with 5+ columns
  - **Mitigation**: Allow horizontal scroll on mobile. CSS already includes overflow-x: auto on matrix-container. Alternative would be to hide columns on mobile, but this reduces data visibility.

- **Risk**: Sort/filter state management could cause bugs with multiple interactions
  - **Mitigation**: Keep state management simple with global variables. Re-render entire matrix on each state change rather than trying to optimize updates. Follow pipeline dashboard pattern exactly.

- **Risk**: ARIA labels and accessibility might be insufficient for screen readers
  - **Mitigation**: Follow HTML5 semantic table structure. Add aria-label to platform cells describing support status. Test with actual screen reader before merging.

## Success Criteria

- [ ] Coverage matrix loads data from `/coverage/coverage.json` successfully
- [ ] All 265 recipes display in table (count matches JSON total_recipes)
- [ ] Platform support indicators (✓/✗) accurately reflect JSON data
- [ ] Sort by name (A-Z, Z-A) works correctly
- [ ] Sort by type (library/tool) groups recipes correctly
- [ ] Filter "Missing musl" shows only recipes without musl support
- [ ] Filter "Missing glibc" shows only recipes without glibc support
- [ ] Filter "Missing darwin" shows only recipes without darwin support
- [ ] Empty filter results show "No recipes found" message
- [ ] Loading state appears before data loads
- [ ] Error state appears if fetch fails, with working retry button
- [ ] Table is responsive at 320px, 768px, and 1280px widths
- [ ] Keyboard navigation works for all controls
- [ ] Screen reader announces sort changes and platform support status
- [ ] No JavaScript errors in browser console
- [ ] Table integrates correctly with HTML from #1544

## Open Questions

None - implementation path is clear based on existing pipeline pattern and well-defined coverage.json schema.

# Issue 1546 Implementation Plan

## Summary

Add JavaScript to populate the gap list and category breakdown views in the coverage dashboard. These views provide additional perspectives on the coverage data by highlighting recipes with missing platform support and showing statistics grouped by recipe type (libraries vs tools).

## Approach

This implementation extends the coverage dashboard created in #1544 and #1545 by adding two new views. Since #1545 already handles data loading (loadCoverageData, currentData), I can reuse that infrastructure and add rendering functions for gap list and category breakdown.

The gap list will show recipes with missing platforms by analyzing the `gaps` array in coverage.json. It will also display recipes with errors (particularly M47 libraries missing musl) and any execution exclusions from the `exclusions` array.

The category breakdown will count recipes by type (library vs tool), calculate percentages, and display the data using progress bars matching the existing CSS styles from the pipeline dashboard pattern.

### Alternatives Considered

- **Separate data load for each view**: Could fetch coverage.json independently for gap list and category breakdown. Rejected because #1545 already loads the data globally, and redundant fetches waste bandwidth.

- **Server-side gap detection**: Could generate gap statistics in cmd/coverage-analytics and add to coverage.json. Rejected because the gaps field already exists in coverage.json, and client-side rendering keeps the tool simple.

- **Interactive filtering across views**: Could link gap list clicks to matrix filters. Deferred to future enhancement - keeping views independent is simpler for initial implementation.

## Files to Modify

- `website/coverage/index.html` - Add JavaScript for renderGapList() and renderCategoryBreakdown() functions (reusing existing script tag from #1545)

## Implementation Steps

- [ ] Add renderGapList() function to populate #gap-list container
- [ ] Detect recipes with missing platforms from gaps array
- [ ] Detect recipes with errors (M47 musl gaps from errors array)
- [ ] Display execution exclusions from exclusions array
- [ ] Add renderCategoryBreakdown() function to populate #category-breakdown container
- [ ] Count recipes by type (library vs tool)
- [ ] Calculate platform support percentages per category
- [ ] Render progress bars for category statistics
- [ ] Call both rendering functions from renderDashboard()
- [ ] Test gap list shows all three gap types correctly
- [ ] Test category breakdown shows accurate statistics
- [ ] Verify integration with existing coverage matrix view

## Testing Strategy

### Manual Verification

**Gap List**:
- Start local server: `python3 -m http.server 8000` from website/ directory
- Navigate to http://localhost:8000/coverage/
- Verify gap list shows recipes with missing platforms
- Check that library recipes without musl support are highlighted
- Verify execution exclusions are displayed (if any in coverage.json)
- Confirm gap counts match the data

**Category Breakdown**:
- Verify total recipe count matches coverage.json total_recipes field
- Check library count and tool count sum to total
- Verify percentages are calculated correctly (library% + tool% = 100%)
- Check progress bars render with correct widths
- Verify platform support stats per category match the data

**Integration**:
- Verify all three views (matrix, gap list, category breakdown) display correctly
- Check that navigation between views works if implemented
- Test responsive layout at different widths
- Verify no JavaScript errors in console

## Risks and Mitigations

- **Risk**: coverage.json gaps field structure may differ from expectations
  - **Mitigation**: Check actual coverage.json structure before implementing. The coverage-analytics tool already generates this, so structure is known.

- **Risk**: No execution exclusions in coverage.json yet (exclusions array may be empty)
  - **Mitigation**: Handle empty exclusions array gracefully. Show "No execution exclusions" message if array is empty.

- **Risk**: Library vs tool classification may be inconsistent in coverage.json
  - **Mitigation**: Rely on type field from coverage-analytics. If missing, default to "tool" type.

- **Risk**: M47 detection relies on errors array content format
  - **Mitigation**: Parse errors strings looking for "musl" keyword and "library" type recipes. This matches coverage.go error format.

## Success Criteria

- [ ] Gap list renders showing recipes with missing platforms
- [ ] Gap list highlights library recipes without musl support
- [ ] Gap list displays execution exclusions (if any exist in data)
- [ ] Gap list shows accurate counts for each gap category
- [ ] Category breakdown shows correct library vs tool split
- [ ] Category breakdown calculates percentages correctly
- [ ] Category breakdown displays progress bars for each category
- [ ] Category breakdown shows platform support stats per type
- [ ] Both views integrate seamlessly with coverage matrix from #1545
- [ ] No JavaScript errors in browser console
- [ ] Views render correctly at mobile, tablet, and desktop widths

## Open Questions

None - implementation path is clear based on existing coverage.json schema and #1545 pattern.

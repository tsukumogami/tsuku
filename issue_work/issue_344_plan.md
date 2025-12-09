# Issue 344 Implementation Plan

## Summary

Add mobile-responsive CSS for recipe detail pages. Most styling was already implemented in #341; only mobile breakpoint adjustments remain.

## Approach

The recipe detail view styling was largely completed as part of #341 (recipe detail view renderer). This issue focuses on the remaining gap: mobile responsiveness at the 600px breakpoint.

### Alternatives Considered

- **Complete restyle**: Not needed - existing desktop styles are consistent with site theme
- **Separate mobile-first rewrite**: Overkill - just need to add responsive adjustments to existing styles

## Files to Modify

- `website/assets/style.css` - Add mobile breakpoint styles for recipe detail components

## Files to Create

None

## Implementation Steps

- [ ] Add mobile styles for `.recipe-detail` (smaller h1, reduced padding)
- [ ] Add mobile styles for `.dependencies-section` (full width, reduced padding)
- [ ] Add mobile styles for `.install-section` (stack layout for install box)
- [ ] Verify all acceptance criteria are met

## Testing Strategy

- Manual verification: Test recipe detail pages at mobile breakpoints (<600px)
- Visual inspection: Verify styles match existing site patterns
- Cross-check: Compare with how other sections handle mobile (`.recipe-card`, `.install-box`)

## Risks and Mitigations

- **Risk**: Mobile styles may conflict with existing install-box styles
- **Mitigation**: Reuse existing `.install-box` mobile styles, only add container adjustments

## Success Criteria

- [ ] `.recipe-detail` component styled consistently with grid (already done)
- [ ] Dependency lists styled with grouped sections (already done)
- [ ] Install command block styled with copy button (already done)
- [ ] Responsive layout works at mobile breakpoints
- [ ] Dark theme colors match existing site (already done)

## Open Questions

None - scope is clear.

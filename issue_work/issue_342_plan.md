# Issue 342 Implementation Plan

## Summary

Make recipe cards clickable to navigate to detail pages by adding a click handler that calls `navigateTo()` instead of page reload.

## Approach

Modify `createRecipeCard()` to:
1. Add a click event listener to the card element
2. Navigate to `/recipes/<name>/` using `navigateTo()`
3. Add `cursor: pointer` style to indicate clickability
4. Keep the homepage link functional (with `stopPropagation` to prevent navigation)

### Alternatives Considered

- **Wrap card in anchor tag**: Would require more CSS changes and complicate the existing structure
- **Only make name clickable**: Less intuitive UX - users expect entire card to be clickable

## Files to Modify

- `website/recipes/index.html` - Add click handler to `createRecipeCard()` function
- `website/assets/style.css` - Add cursor style for clickable cards

## Files to Create

None

## Implementation Steps

- [x] Add click event listener to recipe cards in `createRecipeCard()`
- [x] Add `stopPropagation()` to homepage link to prevent double navigation
- [x] Add `cursor: pointer` style to `.recipe-card` in CSS
- [x] Test navigation from grid to detail and back

## Testing Strategy

- Manual verification:
  - Click a recipe card: should navigate to `/recipes/<name>/`
  - Click "Homepage" link on a card: should open homepage in new tab, NOT navigate
  - Click browser back button from detail: should return to grid
  - Search, then click a result: should navigate to detail page
  - Verify search query is preserved after navigating back

## Risks and Mitigations

- **Risk**: Homepage link also triggers card navigation
  - **Mitigation**: Use `stopPropagation()` on the link click event

- **Risk**: Click handler interferes with text selection
  - **Mitigation**: Not a significant UX issue since cards have minimal selectable text

## Success Criteria

- [x] Recipe cards are clickable (entire card)
- [x] Clicking navigates via `navigateTo()` (no page reload)
- [x] Existing search/filter functionality preserved
- [x] Cards have appropriate hover states (already exists)
- [x] Homepage link still works correctly

## Open Questions

None

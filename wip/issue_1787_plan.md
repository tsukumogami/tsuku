# Issue 1787 Implementation Plan

## Summary

Fix two bugs in the pipeline dashboard: add the missing `requires_manual` status to the Queue Status bar (Bug 1), and add a "has dependencies" annotation to the blocked count so users understand the distinction between queue status and dependency-blocked packages (Bug 2).

## Approach

**Bug 1** is straightforward: add `requires_manual` to the `statusOrder` array in `index.html`, add a CSS color for it, and create a new `requires_manual.html` listing page following the pattern of `pending.html`.

**Bug 2** needs more thought. The Top Blockers widget counts packages with `blocked_by` entries in the *failure records* (which span all statuses), while Queue Status counts packages whose *queue status* is literally `blocked`. These are fundamentally different metrics from different data sources. Changing the Queue Status "blocked" count to include all dependency-blocked packages would misrepresent the queue status (a package can be `pending` yet have a `blocked_by` entry from a past failure). Instead, we'll add a subtitle/annotation below the Queue Status "blocked" row showing the total dependency-blocked count, linking it conceptually to the Top Blockers widget. This communicates the distinction without conflating two different meanings.

### Alternatives Considered

- **Change blocked count to include all packages with `blocked_by` entries**: Rejected. The Queue Status shows queue statuses; inserting a cross-cutting concern into one row would break the invariant that displayed counts sum to the total. A package with status `pending` that has a `blocked_by` entry would be double-counted (once in pending, once in blocked).
- **Add tooltip on the blocked row**: Rejected. Tooltips are invisible on mobile and easy to miss on desktop. An inline annotation is always visible.
- **Rename "blocked" to "blocked-only"**: Rejected. Confusing to users who don't know what "only" distinguishes from.
- **Compute dependency-blocked count server-side (in dashboard.go)**: Considered but unnecessary. The `blockers` data already contains this info (sum of direct_count values). The frontend can derive the total from existing data without backend changes.

## Files to Modify

- `website/pipeline/index.html` - Add `requires_manual` to `statusOrder`, add CSS color for the new status, add annotation below blocked row showing total dependency-blocked count derived from `data.blockers`

## Files to Create

- `website/pipeline/requires_manual.html` - New listing page for packages with `requires_manual` status, modeled on `pending.html` with minor copy adjustments (title, description)

## Implementation Steps

- [ ] Add `requires_manual` CSS class to the status bar styles in `index.html` (pick a distinct color, e.g., purple/violet `#bc8cff` to differentiate from pending gray, failed red, blocked amber, and success green)
- [ ] Add `requires_manual` to the `statusOrder` array in the `renderDashboard` function
- [ ] Widen the `.status-label` width from 80px to accommodate the longer `requires_manual` label (or use a shorter display label like `manual`)
- [ ] Add a dependency-blocked annotation after the Queue Status bars: compute the total dependency-blocked count from `data.blockers` (sum all `total_count` values) and render it as a small note below the status bars, e.g., "N packages have unresolved dependencies (see Top Blockers)"
- [ ] Create `website/pipeline/requires_manual.html` by copying `pending.html` and changing `STATUS = 'pending'` to `STATUS = 'requires_manual'`, updating the title to "Requires Manual Review", and updating the tagline
- [ ] Verify the dashboard renders correctly with all five statuses and the counts sum to the total
- [ ] Verify the `requires_manual.html` page loads and displays the correct packages

## Testing Strategy

- **Manual verification**: Open `index.html` in a browser (served via `python3 -m http.server 8000` from `website/`), confirm all five statuses render, counts sum to total, and the dependency annotation appears below the status bars.
- **Manual verification**: Click the `requires_manual` row to confirm it navigates to `requires_manual.html` and displays the correct package list.
- **No Go test changes**: The backend (`dashboard.go`) already emits all five statuses in `by_status` and includes `requires_manual` packages in `packages`. The bug is purely frontend.

## Risks and Mitigations

- **Label width overflow**: The `requires_manual` label (16 chars) is longer than existing labels. Mitigation: use a shorter display label like "manual" or increase `.status-label` width. Test at mobile breakpoint (600px).
- **Color accessibility**: The new status color must be distinguishable from existing colors for colorblind users. Mitigation: use a purple/violet that's perceptually distinct from the existing gray/red/amber/green palette.
- **Dependency-blocked count may be confusing**: Users might not understand the annotation. Mitigation: link the annotation text to the `blocked.html` page for context, and keep the wording simple.

## Success Criteria

- [ ] All five queue statuses (pending, success, failed, blocked, requires_manual) are visible in the status bar
- [ ] Displayed counts in the status bar sum to the total shown in the panel header
- [ ] The relationship between Queue Status "blocked" and Top Blockers "N blocked" is communicated via the dependency annotation
- [ ] Clicking `requires_manual` in the status bar navigates to a working listing page
- [ ] Page renders correctly at both desktop and mobile (600px) widths

## Open Questions

None -- both bugs have clear fixes and the implementation is frontend-only.

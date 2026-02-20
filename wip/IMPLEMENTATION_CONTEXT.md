## Goal

Fix two inconsistencies in the pipeline dashboard's Queue Status section that make the numbers misleading.

## Context

The Queue Status bar on `tsuku.dev/pipeline/` shows four statuses: pending, success, failed, blocked. The data has five statuses including `requires_manual`, and the "blocked" count doesn't align with the Top Blockers widget.

## Bug 1: `requires_manual` status not displayed

The frontend hardcodes four statuses:

```javascript
const statusOrder = ['pending', 'success', 'failed', 'blocked'];
```

But the data contains a fifth status, `requires_manual`, with 2,009 packages. The status bar shows a total of 5,275 but only renders 3,266 — the rest are invisible. Anyone doing arithmetic on the displayed numbers will notice they don't add up.

**Fix:** Add `requires_manual` to `statusOrder` and create a corresponding page (or fold it into an existing category if that makes more sense).

## Bug 2: "blocked" count doesn't match Top Blockers

The Queue Status shows "blocked: 5", but the Top Blockers widget shows openssl@3 alone blocking 13 packages. The disconnect: only 5 packages have `status=blocked`, but 124 packages have `blocked_by` entries across pending (29), requires_manual (90), and blocked (5).

The two widgets use "blocked" to mean different things:
- **Queue Status**: packages whose queue status is literally `blocked`
- **Top Blockers**: all packages with unresolved `blocked_by` dependencies, regardless of queue status

A user sees "5 blocked" next to "openssl@3: 13 blocked" and reasonably concludes something is wrong.

**Fix options:**
- Rename the Queue Status entry to something narrower (e.g., "blocked-only") — probably confusing
- Change the Queue Status "blocked" count to include all packages with `blocked_by` entries regardless of status — aligns with the Top Blockers widget
- Add a label or tooltip clarifying the difference

## Acceptance Criteria

- [ ] All five queue statuses are visible in the status bar, and the displayed counts sum to the total
- [ ] The "blocked" count in Queue Status is consistent with what the Top Blockers widget reports (or the distinction is clearly communicated)

## Dependencies

None

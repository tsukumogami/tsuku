# Issue 1787 Summary

## Changes

### Bug 1: Missing `requires_manual` status
- Added `requires_manual` to `statusOrder` in `index.html`
- Added purple CSS class (`.status-bar.requires_manual { background: #bc8cff }`)
- Display label mapped to "manual" to fit 80px label width
- Created `requires_manual.html` listing page (modeled on `pending.html`)

### Bug 2: Inconsistent blocked count
- Added annotation below status bars showing total dependency-blocked packages
- Counts all packages with non-empty `blocked_by` arrays across all queue statuses
- Links to `#blockers` anchor on the Top Blockers panel

## Files Modified
- `website/pipeline/index.html` (3 edits: CSS, statusOrder, annotation)

## Files Created
- `website/pipeline/requires_manual.html` (new listing page)

## No backend changes needed
Both bugs were purely frontend rendering issues.

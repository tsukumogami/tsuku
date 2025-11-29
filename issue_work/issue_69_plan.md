# Issue 69 Implementation Plan

## Summary
Update README installation instructions to use get.tsuku.dev and remove the now-obsolete install.sh file.

## Approach
Simple documentation update and file deletion. The install script is now managed externally at tsuku-dev/tsuku.dev.

### Alternatives Considered
None - this is a straightforward change with no alternatives.

## Files to Modify
- `README.md` - Update Installation section with curl command

## Files to Delete
- `install.sh` - Now managed in tsuku-dev/tsuku.dev repo

## Implementation Steps
- [ ] Update README.md Installation section with new curl command
- [ ] Delete install.sh

## Testing Strategy
- Manual verification: Confirm README renders correctly on GitHub
- No code changes, so no unit tests needed

## Risks and Mitigations
- None - simple documentation change

## Success Criteria
- [ ] README Installation section shows `curl -fsSL https://get.tsuku.dev/now | bash`
- [ ] install.sh is deleted from repository

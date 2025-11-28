# Issue 10 Implementation Plan

## Summary

Expand existing CONTRIBUTING.md to cover all sections specified in the issue, using googleapis/librarian's format as inspiration for issue/PR workflow documentation.

## Approach

The existing CONTRIBUTING.md already covers basic CI/CD and testing. Rather than replace it, expand it with missing sections:

1. Add Development Setup at the beginning
2. Expand Testing section to include integration tests
3. Add Branch Naming conventions
4. Expand PR Process with description requirements
5. Add Recipe contribution guidance (pointing to tsuku-registry)

### Alternatives Considered
- Create new CONTRIBUTING.md from scratch: Rejected - existing content is useful, just incomplete
- Add separate docs/CONTRIBUTING.md: Rejected - standard practice is root-level CONTRIBUTING.md

## Files to Modify
- `CONTRIBUTING.md` - Expand with missing sections

## Files to Create
- None

## Implementation Steps
- [x] Add Development Setup section (Go version, prerequisites, build)
- [x] Expand Testing section (unit tests, integration tests, coverage, race detection)
- [x] Add Code Style section (formatting, linting)
- [x] Expand commit conventions with conventional commit format
- [x] Add Branch Naming section
- [x] Expand PR Process section
- [x] Add Adding Recipes section (pointing to tsuku-registry)

## Testing Strategy
- Manual verification: Review CONTRIBUTING.md renders correctly on GitHub
- Content accuracy: Verify all commands mentioned actually work

## Risks and Mitigations
- Risk: Outdated information in new content
  - Mitigation: Cross-reference with actual project files (go.mod, CI workflows)

## Success Criteria
- [ ] CONTRIBUTING.md covers all 5 sections from issue #10
- [ ] All commands in the document work correctly
- [ ] Document follows librarian-style clarity and organization

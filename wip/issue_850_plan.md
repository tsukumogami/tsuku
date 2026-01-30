# Issue 850 Plan

## Classification
Simplified plan (docs task).

## Approach
Consolidate unique content from the superseded DESIGN-sandbox-implicit-dependencies.md into the parent DESIGN-sandbox-dependencies.md.

## Unique Content in Superseded Doc

After comparing both documents, the superseded doc contains these sections not present in the parent:

1. **Plan Structure (Format v3)** - Go struct definitions for InstallationPlan, Platform, DependencyPlan
2. **Platform Detection and Population** - LinuxFamily detection logic, when/how platform values are determined
3. **Dependency Inheritance** - How platform constraints propagate to nested dependencies
4. **Circular Dependency Detection** - Detection mechanism, error messages, platform-specific cycles
5. **Version Conflict Resolution** - First-encountered-wins strategy, rationale, failure modes
6. **Dependency Deduplication** - How dedup works during generation vs execution
7. **User-Facing Error Messages** - Comprehensive error message catalog
8. **Debugging Plan Generation** - Verbose logging, diagnostic commands
9. **Resource Limits** - Max depth (5), max total (100), validateResourceLimits()
10. **Migration Path for Pre-GA Users** - Old plan handling, user actions needed
11. **Plan Tampering** security consideration (not in parent's Security section)

The parent doc already covers:
- Context and problem statement (more focused on code duplication)
- Decision drivers and options (similar but scoped differently)
- Solution architecture (high-level, missing the details above)
- Implementation approach (step-by-step refactoring)
- Security considerations (4 dimensions covered, but missing plan tampering)

## Steps

1. Add unique subsections from superseded doc into parent doc's existing sections:
   - Plan Structure → under Solution Architecture
   - Platform Detection → under Solution Architecture
   - Circular Dependency Detection → under Solution Architecture
   - Version Conflict Resolution → under Solution Architecture
   - Dependency Deduplication → under Solution Architecture
   - User-Facing Error Messages → under Solution Architecture
   - Debugging → under Solution Architecture
   - Resource Limits → under Implementation Approach (new phase)
   - Migration Path → new section before Consequences
   - Plan Tampering → add to Security Considerations

2. Update frontmatter to reflect expanded scope

3. Verify no cross-references need updating (already confirmed: none)

## Files Modified
- `docs/designs/current/DESIGN-sandbox-dependencies.md` (parent - expanded)

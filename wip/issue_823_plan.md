# Issue 823 Implementation Plan

## Goal

Extend WhenClause with LinuxFamily and Arch fields, and implement mergeWhenClause() to combine explicit when clause constraints with implicit action constraints.

## Files to Modify

- `internal/recipe/types.go` - WhenClause struct, IsEmpty(), UnmarshalTOML, ToMap, add mergeWhenClause()
- `internal/recipe/types_test.go` - Unit tests for new functionality

## Files to Create

None - all changes go into existing files.

## Implementation Steps

- [ ] Add `Arch` field to WhenClause struct (line ~195)
- [ ] Add `LinuxFamily` field to WhenClause struct (line ~195)
- [ ] Update `IsEmpty()` to include new fields (line ~201)
- [ ] Update `UnmarshalTOML` to parse `arch` from when clause (after line 298)
- [ ] Update `UnmarshalTOML` to parse `linux_family` from when clause (after line 298)
- [ ] Update `ToMap` to serialize `Arch` when present (after line 350)
- [ ] Update `ToMap` to serialize `LinuxFamily` when present (after line 350)
- [ ] Implement `mergeWhenClause()` function
- [ ] Add unit tests for WhenClause.IsEmpty() with new fields
- [ ] Add unit tests for mergeWhenClause() - nil implicit, nil when
- [ ] Add unit tests for mergeWhenClause() - platform array conflict
- [ ] Add unit tests for mergeWhenClause() - OS conflict
- [ ] Add unit tests for mergeWhenClause() - LinuxFamily conflict
- [ ] Add unit tests for mergeWhenClause() - Arch conflict
- [ ] Add unit tests for mergeWhenClause() - extends implicit with explicit arch
- [ ] Add unit tests for mergeWhenClause() - compatible (redundant) constraints
- [ ] Run go build, go vet, go test

## Testing Strategy

### Unit Tests

**WhenClause.IsEmpty tests:**
- Empty struct returns true
- Struct with only Arch returns false
- Struct with only LinuxFamily returns false

**mergeWhenClause tests:**
1. `TestMergeWhenClause_NilImplicit_NilWhen`: nil implicit, nil when → empty constraint
2. `TestMergeWhenClause_NilImplicit_WithArch`: nil implicit, when.arch="amd64" → arch=amd64
3. `TestMergeWhenClause_NilImplicit_WithFamily`: nil implicit, when.linux_family="debian" → family=debian
4. `TestMergeWhenClause_PlatformConflict`: implicit.OS="linux", when.Platform=["darwin/arm64"] → error
5. `TestMergeWhenClause_OSConflict`: implicit.OS="linux", when.OS=["darwin"] → error
6. `TestMergeWhenClause_FamilyConflict`: implicit.LinuxFamily="debian", when.LinuxFamily="rhel" → error
7. `TestMergeWhenClause_ArchConflict`: implicit.Arch="amd64", when.Arch="arm64" → error
8. `TestMergeWhenClause_ExtendsWithArch`: implicit.OS="linux"/family="debian", when.Arch="amd64" → linux/debian/amd64
9. `TestMergeWhenClause_RedundantFamily`: implicit.family="debian", when.family="debian" → debian (valid)
10. `TestMergeWhenClause_InvalidFinalConstraint`: result would be darwin+debian → error from Validate()

## Alternatives Considered

1. **Separate file for mergeWhenClause**: Rejected - the function is closely tied to WhenClause and Constraint types in types.go

2. **Add mergeWhenClause as a method on WhenClause**: Rejected - it takes two arguments (implicit constraint, when clause) and produces a Constraint, so a free function is cleaner

## Dependencies

Uses from #822:
- `Constraint` type with OS, Arch, LinuxFamily fields
- `Constraint.Clone()` method (nil-safe)
- `Constraint.Validate()` method

## Design Reference

From `docs/DESIGN-golden-family-support.md`, section "Merge Semantics":

| Implicit (action) | Explicit (when) | Result |
|-------------------|-----------------|--------|
| apt_install (linux/debian) | when.linux_family: rhel | ERROR |
| apt_install (linux/debian) | when.os: darwin | ERROR |
| apt_install (linux/debian) | when.arch: amd64 | OK (extends) |
| apt_install (linux/debian) | when.linux_family: debian | OK (redundant) |

## Goal

Extend WhenClause with LinuxFamily and Arch fields, and implement mergeWhenClause() to combine explicit when clause constraints with implicit action constraints, detecting conflicts.

## Context

Recipe steps can have both implicit constraints (from action types like `apt_install` which implies debian) and explicit constraints (from `when` clauses). This issue adds the fields needed for explicit family/arch targeting and the merge logic that combines them with conflict detection.

Design: `docs/DESIGN-golden-family-support.md` (Phase 2: Extend WhenClause)

## Key Implementation Points

From design document section "Merge Semantics":

1. **Compatible constraints (AND semantics)**: If both specify the same dimension, they must match.
   - `apt_install` (implicit: linux/debian) + `when: linux_family: debian` → linux/debian (valid, redundant)

2. **Conflicting constraints (validation error)**: If implicit and explicit contradict on any dimension, the recipe is invalid.
   - `apt_install` (implicit: linux/debian) + `when: linux_family: rhel` → **ERROR** (family conflict)
   - `apt_install` (implicit: linux/debian) + `when: os: darwin` → **ERROR** (OS conflict)

3. **Explicit extends implicit**: Explicit constraints can add dimensions not covered by implicit.
   - `apt_install` (implicit: linux/debian) + `when: arch: amd64` → linux/debian/amd64

## Acceptance Criteria

- WhenClause has Arch string field
- WhenClause has LinuxFamily string field
- IsEmpty() includes new fields
- UnmarshalTOML parses arch and linux_family from when clause
- ToMap serializes new fields when present
- mergeWhenClause() returns merged constraint when compatible
- mergeWhenClause() returns error on OS conflict
- mergeWhenClause() returns error on LinuxFamily conflict
- mergeWhenClause() returns error on Arch conflict
- mergeWhenClause() extends implicit with explicit arch (no conflict)
- Unit tests for all conflict scenarios

## Dependencies

Depends on #822 (Constraint type with Clone() and Validate() methods) - **DONE**

## Downstream Dependencies

This issue unblocks:
- #824: feat(recipe): add step analysis computation logic
- #827: feat(recipe): add Matchable interface for platform matching

## Goal

Add the `Matchable` interface to enable platform matching with Linux family support. This provides a uniform interface for `WhenClause.Matches()` to accept both the lightweight `MatchTarget` struct and the runtime `platform.Target` type.

## Context

The current `WhenClause.Matches(os, arch string)` signature cannot support Linux family filtering needed for family-aware recipes. Rather than adding more parameters, an interface-based approach allows:
1. Adding new dimensions (e.g., libc variant) without signature changes
2. Eliminating conversions between `platform.Target` and recipe-level structs
3. Both types implementing the interface directly

Design: `docs/DESIGN-golden-family-support.md`

## Key Implementation Points

1. Define `Matchable` interface in recipe package with `OS()`, `Arch()`, `LinuxFamily()` methods
2. Add `MatchTarget` struct with constructor `NewMatchTarget(os, arch, linuxFamily string)`
3. Add `LinuxFamily()` method to `platform.Target` (making it implement Matchable)
4. Update `WhenClause.Matches()` to accept `Matchable` parameter
5. Update all call sites to use `NewMatchTarget()` or `platform.Target` directly

## Acceptance Criteria

- Matchable interface defined with OS(), Arch(), LinuxFamily() methods
- MatchTarget struct implements Matchable with constructor
- platform.Target has LinuxFamily() method and implements Matchable
- WhenClause.Matches() accepts Matchable parameter
- All existing tests pass with updated call sites
- No breaking changes to external API

## Dependencies

Depends on #823 (WhenClause with LinuxFamily field) - **DONE**

## Downstream Dependencies

This issue is marked "simple" tier - no downstream dependencies in the current milestone.

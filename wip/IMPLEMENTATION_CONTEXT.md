---
summary:
  constraints:
    - Use Go's debug/elf package only - no external tools
    - ErrABIMismatch must be explicit value 10 (design decision #2)
    - Static binaries (no PT_INTERP) should return nil (pass)
    - macOS has no PT_INTERP equivalent - skip validation
  integration_points:
    - internal/verify/abi.go (new file to create)
    - internal/verify/types.go (add ErrABIMismatch constant)
    - Consumed by #989 (recursive validation) as first validation step
  risks:
    - PT_INTERP segment reading must handle null-terminated strings correctly
    - Need to distinguish static binaries (pass) from missing interpreter (fail)
    - Error message must be clear about glibc/musl mismatch cause
  approach_notes: |
    Create ValidateABI(path string) error function that:
    1. Opens file with debug/elf (return nil for non-ELF, i.e., macOS)
    2. Iterates program headers looking for PT_INTERP
    3. If no PT_INTERP -> static binary -> return nil
    4. If PT_INTERP found -> read interpreter path -> check if exists
    5. If interpreter missing -> return ErrABIMismatch with clear message
---

# Implementation Context: Issue #981

**Source**: docs/designs/DESIGN-library-verify-deps.md (None)

## Design Excerpt

# Dependency Resolution for Library Verification (Tier 2)

**Status:** Planned

## Implementation Issues

### Milestone: [Tier 2 Dependency Validation](https://github.com/tsukumogami/tsuku/milestone/38)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [#978](https://github.com/tsukumogami/tsuku/issues/978) | Add Sonames field to LibraryVersionState | None | simple |
| [#979](https://github.com/tsukumogami/tsuku/issues/979) | Add IsExternallyManaged() to SystemAction | None | testable |
| [#980](https://github.com/tsukumogami/tsuku/issues/980) | Implement system library pattern matching | None | testable |
| [#981](https://github.com/tsukumogami/tsuku/issues/981) | Implement PT_INTERP ABI validation | None | testable |
| [#982](https://github.com/tsukumogami/tsuku/issues/982) | Implement RPATH and path variable expansion | None | critical |
| [#983](https://github.com/tsukumogami/tsuku/issues/983) | Implement soname extraction for ELF/Mach-O | [#978](https://github.com/tsukumogami/tsuku/issues/978) | testable |
| [#984](https://github.com/tsukumogami/tsuku/issues/984) | Add IsExternallyManagedFor() method | [#979](https://github.com/tsukumogami/tsuku/issues/979) | testable |
| [#985](https://github.com/tsukumogami/tsuku/issues/985) | Extract and store sonames during install | [#983](https://github.com/tsukumogami/tsuku/issues/983) | testable |
| [#986](https://github.com/tsukumogami/tsuku/issues/986) | Implement SonameIndex and classification | [#978](https://github.com/tsukumogami/tsuku/issues/978), [#980](https://github.com/tsukumogami/tsuku/issues/980) | testable |
| [#989](https://github.com/tsukumogami/tsuku/issues/989) | Implement recursive dependency validation | [#981](https://github.com/tsukumogami/tsuku/issues/981), [#982](https://github.com/tsukumogami/tsuku/issues/982), [#984](https://github.com/tsukumogami/tsuku/issues/984), [#986](https://github.com/tsukumogami/tsuku/issues/986) | critical |
| [#990](https://github.com/tsukumogami/tsuku/issues/990) | Integrate Tier 2 dependency validation | [#989](https://github.com/tsukumogami/tsuku/issues/989) | testable |
| [#991](https://github.com/tsukumogami/tsuku/issues/991) | Add Tier 2 integration tests | [#990](https://github.com/tsukumogami/tsuku/issues/990) | testable |

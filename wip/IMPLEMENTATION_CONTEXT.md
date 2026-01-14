# Implementation Context for Issue #862

## Summary

Add a minimal end-to-end walking skeleton for Homebrew Cask support. This establishes the foundational interfaces and creates a working flow from recipe to installed application, enabling parallel development of full implementations in subsequent issues.

The skeleton includes:
- Stub `CaskProvider` returning hardcoded metadata for iTerm2
- `CaskVersionInfo` type with Version, URL, Checksum fields
- Basic template substitution for `{{version.url}}` and `{{version.checksum}}`
- `AppsDir` field in `ExecutionContext`
- Basic `AppBundleAction` supporting ZIP extraction and .app installation
- Integration test demonstrating the complete flow

## Design Reference

This issue implements portions of DESIGN-cask-support.md (not yet created):
- Solution Architecture (Overview) - partial
- Implementation Approach (Slices 1-2) - stub implementations

## Key Acceptance Criteria

1. **Stub Cask Version Provider** - CaskProvider with hardcoded iTerm2 metadata
2. **Template Substitution** - Support `{{version.url}}` and `{{version.checksum}}`
3. **ExecutionContext Extension** - Add `AppsDir` field
4. **App Bundle Action** - ZIP extraction and .app installation
5. **Integration Test** - Full flow test (macOS only)

## Downstream Dependencies

- #863 - Needs `CaskProvider` interface and `CaskVersionInfo` type
- #864 - Needs `AppBundleAction` structure to add DMG support
- #865 - Needs `AppBundleAction` structure for binary symlinks

## Warnings

- Design doc not found - using issue body only

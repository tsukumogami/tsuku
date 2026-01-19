---
summary:
  constraints:
    - Must preserve existing platform matrix and ldflags
    - Variable pinnedDltestVersion must exist for ldflag injection to work
    - goreleaser check must pass
  integration_points:
    - .goreleaser.yaml (modify release.draft and ldflags)
    - internal/verify/version.go (create for pinnedDltestVersion variable)
  risks:
    - If pinnedDltestVersion variable doesn't exist, ldflag has no effect
  approach_notes: |
    1. Create internal/verify/version.go with pinnedDltestVersion variable
    2. Update .goreleaser.yaml:
       - Change draft: false to draft: true
       - Add pinnedDltestVersion ldflag
    3. Verify with goreleaser check and goreleaser build --snapshot
---

# Implementation Context: Issue #1026

**Source**: docs/designs/DESIGN-release-workflow-native.md (Step 2)

## Current goreleaser Config

```yaml
release:
  github:
    owner: tsukumogami
    name: tsuku
  draft: false  # Change to true
  prerelease: auto

ldflags:
  - -s -w -X github.com/tsukumogami/tsuku/internal/buildinfo.version={{.Version}} -X github.com/tsukumogami/tsuku/internal/buildinfo.commit={{.Commit}}
```

## Required Changes

1. **draft: true** - goreleaser shouldn't publish, finalize-release handles that
2. **Add pinnedDltestVersion ldflag** - inject version at build time

## pinnedDltestVersion

The variable needs to exist in the codebase for the ldflag to inject into. Create:

```go
// internal/verify/version.go
package verify

// pinnedDltestVersion is the expected tsuku-dltest version.
// Injected at build time via ldflags.
var pinnedDltestVersion = "dev"
```

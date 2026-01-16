# Implementation Context: Issue #866

**Source**: docs/designs/DESIGN-cask-support.md

## Design Overview

Issue #866 implements **Slice 5: CaskBuilder (Optional)** from the Homebrew Cask Support design. This is the final slice of the cask support feature.

## Key Implementation Points

1. **CaskBuilder Pattern**: Follow `SessionBuilder` interface pattern used by `HomebrewBuilder`
2. **Artifact Detection**: Parse cask API `artifacts` array to detect `app` vs `binary` artifacts
3. **Recipe Generation**: Generate TOML with `cask:` version provider and `app_bundle` action
4. **No LLM Required**: Deterministic generation based on cask metadata

## Dependencies (All Completed)

- #863: Cask version provider - provides metadata resolution
- #864: DMG extraction - enables DMG-based cask recipes
- #865: Binary symlinks - enables `binaries` parameter in recipes

## API Response Structure

```json
{
  "token": "visual-studio-code",
  "version": "1.85.1",
  "sha256": "...",
  "url": "https://update.code.visualstudio.com/...",
  "artifacts": [
    {"app": ["Visual Studio Code.app"]},
    {"binary": ["{{appdir}}/Contents/Resources/app/bin/code"]}
  ]
}
```

## Generated Recipe Format

```toml
[metadata]
name = "vscode"
description = "Code editing. Redefined."
homepage = "https://code.visualstudio.com/"

[version]
provider = "cask:visual-studio-code"

[[steps]]
action = "app_bundle"
url = "{{version.url}}"
checksum = "{{version.checksum}}"
app_name = "Visual Studio Code.app"
binaries = ["Contents/Resources/app/bin/code"]
symlink_applications = true
```

## Acceptance Criteria

- CaskBuilder implements SessionBuilder interface
- `tsuku create --from cask:<name>` generates valid recipe
- App artifacts detected and app_name populated correctly
- Binary artifacts detected and binaries array populated
- Casks with unsupported artifacts (pkg, preflight) return clear error
- Unit tests cover artifact detection edge cases

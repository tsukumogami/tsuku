# Implementation Context: Issue #863

**Source**: docs/designs/DESIGN-cask-support.md

## Key Design Decisions

1. **Hybrid Approach**: Cask version provider for metadata + generic `app_bundle` action for installation
2. **API Endpoint**: `https://formulae.brew.sh/api/cask/{name}.json`
3. **Template Syntax**: `{version.url}` and `{version.checksum}` for metadata injection
4. **Architecture Handling**: Provider selects URL based on `runtime.GOARCH` (arm64 → Apple Silicon, amd64 → Intel)

## Implementation Focus for This Issue

**File**: `internal/version/provider_cask.go` (replace hardcoded stub)

**Key Interfaces:**
- `CaskProvider` implements `VersionResolver` and `VersionLister`
- Returns `VersionInfo` with `Metadata` map containing `url`, `checksum`
- `CaskSourceStrategy` already registered (from #862)

**API Response Structure:**
```json
{
  "token": "visual-studio-code",
  "version": "1.96.4",
  "sha256": "abc123...",
  "url": "https://update.code.visualstudio.com/1.96.4/darwin-arm64/stable",
  "url_specs": {
    "arm64": "https://update.code.visualstudio.com/1.96.4/darwin-arm64/stable",
    "x86_64": "https://update.code.visualstudio.com/1.96.4/darwin/stable"
  }
}
```

**Architecture Selection Logic:**
- `arm64` → Use ARM64/Apple Silicon URL
- `amd64` → Use Intel/Universal URL

**Missing Checksum Handling:**
- Some casks use `:no_check` (empty checksum)
- Should warn but not error

## Dependencies

- #862 (walking skeleton) - COMPLETED
- Provides: CaskProvider stub, template expansion, app_bundle action

## Downstream Dependencies

- #866 (CaskBuilder) - needs working cask API integration

# Implementation Context: Issue #865

**Source**: docs/designs/DESIGN-cask-support.md

## Issue Title
feat(cask): add binary symlinks and applications integration

## Design Context

This issue is part of the Homebrew Cask Support milestone, implementing **Slice 4** of the design.

### What This Issue Implements
- Create symlinks in `$TSUKU_HOME/bin` for CLI tools specified in recipe `binaries` array
- Create optional symlink in `~/Applications` when `symlink_applications` is true (default)
- Handle version switching: updating to new version updates symlinks
- Handle removal: `tsuku remove` cleans up all symlinks
- Add `tsuku list --apps` flag

### Dependencies
- **Blocked by #862** (walking skeleton) - CLOSED
- **Downstream**: #866 (CaskBuilder) depends on this

### Key Design Decisions from Design Doc
1. **Binary symlinks**: Paths within .app bundle symlinked to `$TSUKU_HOME/bin`
2. **Applications symlink**: Optional symlink in `~/Applications` for Launchpad/Spotlight integration
3. **Installation target**: `$TSUKU_HOME/apps/<name>-<version>.app`

### Action Parameters (from design)
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | Download URL |
| `checksum` | string | Yes | SHA256 checksum |
| `app_name` | string | Yes | Name of `.app` bundle |
| `binaries` | []string | No | Paths to CLI tools within `.app` to symlink |
| `symlink_applications` | bool | No | Create `~/Applications` symlink (default: true) |

### Architecture from Design Doc
```
1. download_file: Fetch DMG/ZIP with checksum verification
2. extract: DMG via hdiutil, ZIP via standard extraction
3. copy: Move .app to $TSUKU_HOME/apps/<name>-<version>.app
4. symlink: Create ~/Applications/<name>.app (optional)
5. binaries: Symlink CLI tools to $TSUKU_HOME/bin (if any)
```

### Related Files to Review
- `internal/actions/app_bundle.go` - Existing app_bundle action from #862
- `internal/install/manager.go` - For removal handling
- `internal/install/list.go` - For `--apps` flag
- `internal/config/config.go` - For AppsDir configuration

# Issue 705 Completion Summary

## Implementation Status

**Issue**: #705 - Extend `tsuku info` with `--recipe` and `--metadata-only` flags
**Status**: ✅ Complete
**Branch**: docs/metadata-command
**PR**: #706

## What Was Implemented

Extended the `tsuku info` command with two new flags following patterns from `eval.go` and `install.go`:

### 1. `--recipe <path>` Flag
- Loads recipe from local file path instead of registry
- Enables testing uncommitted recipe changes
- Validates mutual exclusivity with tool name argument
- Uses existing `loadLocalRecipe()` helper

### 2. `--metadata-only` Flag
- Skips dependency resolution (faster, no network calls)
- Skips installation state checks
- Returns only static recipe metadata
- Can be combined with `--recipe` flag

### 3. Expanded JSON Output Schema
Added new static fields (backward compatible, additive-only):
- `version_source`: Version provider source (e.g., "github_releases")
- `supported_platforms`: Computed OS/arch tuples (e.g., ["linux/amd64", "darwin/arm64"])
- `tier`: Installation tier (1=binary, 2=package manager, 3=nix)
- `type`: Recipe type ("tool" or "library")

Made installation-related fields use `omitempty` for metadata-only mode:
- `status`
- `installed_version`
- `location`

### 4. Updated Human-Readable Output
Added new fields to console output:
- Version Source
- Tier (if > 0)
- Type (if non-empty)

Conditionally hide in `--metadata-only` mode:
- Status/Location
- Install/Runtime Dependencies

## Files Modified

- `cmd/tsuku/info.go` - Primary implementation file
  - Added flag definitions (lines 190-192)
  - Changed Args from `ExactArgs(1)` to `MaximumNArgs(1)` (line 19)
  - Added mutual exclusivity validation (lines 26-33)
  - Added conditional recipe loading (lines 40-54)
  - Wrapped dependency resolution in `!metadataOnly` check (lines 62-102)
  - Expanded JSON output struct with 4 new fields (lines 106-124)
  - Updated human-readable output with new fields (lines 154-160)
  - Conditionally hide status and dependencies in metadata-only mode (lines 171-178, 186-199)

- `docs/DESIGN-info-enhancements.md` - Updated status from "Proposed" to "Current"

## Testing Performed

### Build Verification
- ✅ `go build -o tsuku ./cmd/tsuku` - success
- ✅ `go vet ./...` - no issues
- ✅ `go test -v -test.short ./...` - same pre-existing failures as baseline (unrelated to this change)

### Manual Testing
1. **Registry tool with JSON**: `./tsuku info ack --json`
   - ✅ Returns expanded schema with all new fields
   - ✅ `supported_platforms` computed correctly
   - ✅ `tier` and `type` default to 0/"" when not set

2. **Metadata-only mode**: `./tsuku info ack --metadata-only`
   - ✅ Skips status display
   - ✅ Skips dependency display
   - ✅ Shows static metadata only

3. **Local recipe file**: `./tsuku info --recipe test-recipe.toml --json`
   - ✅ Loads from file successfully
   - ✅ Shows all new fields with actual values
   - ✅ `version_source`, `tier`, `type` populated

4. **Combined flags**: `./tsuku info --recipe test-recipe.toml --metadata-only`
   - ✅ Works correctly
   - ✅ Fast (no dependency resolution)

5. **Error validation**:
   - ✅ `./tsuku info ack --recipe file.toml` → "cannot specify both --recipe and a tool name"
   - ✅ `./tsuku info` → "must specify either a tool name or --recipe flag"

## Backward Compatibility

✅ Fully backward compatible:
- All existing `tsuku info <tool>` calls work unchanged
- JSON schema expanded additively (new fields only)
- Existing parsers ignore unknown fields (JSON standard behavior)
- No breaking changes to output format

## Performance Impact

`--metadata-only` flag provides measurable performance improvement:
- Skips `actions.ResolveDependencies()`
- Skips `actions.ResolveTransitive()` (network calls)
- Skips installation state manager checks
- Ideal for bulk recipe queries (e.g., golden plan testing)

## Design Alignment

Implementation follows approved design (DESIGN-info-enhancements.md):
- ✅ Decision 1: Extend existing `info` command (not new command)
- ✅ Decision 2: Both `--recipe` and `--metadata-only` flags
- ✅ Decision 3: Additive JSON schema expansion
- ✅ Decision 4: Platform tuples via `GetSupportedPlatforms()`

All 4 key decisions from the design document were implemented as specified.

## Success Criteria (from plan)

- ✅ `tsuku info <tool>` works exactly as before (backward compatibility)
- ✅ `tsuku info --recipe <path>` loads and displays metadata from local file
- ✅ `tsuku info <tool> --metadata-only` skips dependency resolution
- ✅ JSON output includes all new static fields
- ✅ `--recipe` and `--metadata-only` can be combined
- ✅ Platform computation produces correct "os/arch" tuple arrays
- ✅ Error messages are clear for invalid flag combinations
- ✅ Go vet and go build pass
- ✅ Command help text updated (via flag definitions)

## Notes

- Test failures in baseline are unrelated (Docker architecture, network flakes)
- Design document renamed from `DESIGN-metadata-command.md` to `DESIGN-info-enhancements.md` after pivot
- Implementation is single-file change (cmd/tsuku/info.go) as predicted

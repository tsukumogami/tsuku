# Implementation Plan: Issue #714 - Golden File Regeneration and Validation Scripts

## Overview

Create two shell scripts for managing golden files for individual recipes:
- `scripts/regenerate-golden.sh <recipe>` - regenerates golden files for a recipe
- `scripts/validate-golden.sh <recipe>` - validates current golden files match generated output

## Analysis Summary

### Available Commands

The following commands are available and working:

1. **`tsuku info --recipe <path> --metadata-only --json`**
   - Returns JSON with `supported_platforms` array (format: `linux/amd64`, `darwin/arm64`)
   - Fast execution (no dependency resolution)

2. **`tsuku eval --recipe <path> --version <ver> --os <os> --arch <arch>`**
   - Generates JSON plan for specific platform and version
   - Downloads artifacts to compute checksums
   - Output includes `format_version`, `tool`, `version`, `platform`, `steps`, etc.

3. **`tsuku versions <tool>`**
   - Returns available versions with latest first

### Directory Structure

Per design doc, golden files use first-letter subdirectories:
```
testdata/golden/plans/{first-letter}/{recipe}/{version}-{os}-{arch}.json
```

Example: `testdata/golden/plans/f/fzf/v0.60.0-linux-amd64.json`

### Platform Handling

- Recipe platforms use `/` separator: `linux/amd64`, `darwin/arm64`
- Golden file names use `-` separator: `linux-amd64`, `darwin-arm64`
- Scripts must convert between formats
- `linux-arm64` is excluded (no CI runner available)

### Version Format

- Recipes may report versions with or without `v` prefix
- Golden files use `v` prefix in filename: `v0.60.0-linux-amd64.json`
- Script normalizes versions to include `v` prefix

## Implementation Steps

### Step 1: Create scripts directory structure

Ensure `scripts/` directory exists with appropriate structure.

**Files:**
- Verify `scripts/` exists (already does)

### Step 2: Create `scripts/regenerate-golden.sh`

Create the regeneration script with the following functionality:

**Features:**
- Bash strict mode (`set -euo pipefail`)
- Argument parsing for `--version`, `--os`, `--arch` filters
- Query supported platforms via `tsuku info --recipe --metadata-only --json`
- Platform format conversion (`linux/amd64` -> `linux-amd64`)
- Exclude `linux-arm64` from generation
- Discover existing versions or resolve latest
- Generate golden files for each platform/version combination
- Clean up unsupported platform files when no filters applied

**Arguments:**
```
./scripts/regenerate-golden.sh <recipe> [--version <ver>] [--os <os>] [--arch <arch>]
```

**Exit codes:**
- 0: Success
- 1: Invalid arguments or recipe not found
- 2: No platforms match filters

### Step 3: Create `scripts/validate-golden.sh`

Create the validation script with the following functionality:

**Features:**
- Bash strict mode (`set -euo pipefail`)
- Regenerate plans to temp directory
- Fast SHA256 hash comparison first
- Show unified diff on mismatch
- Report actionable error message

**Arguments:**
```
./scripts/validate-golden.sh <recipe>
```

**Exit codes:**
- 0: All golden files match
- 1: Mismatch detected (with diff output)
- 2: Error (missing files, invalid recipe, etc.)

### Step 4: Verify scripts are executable and functional

Test both scripts manually with a sample recipe.

**Test commands:**
```bash
# Make executable
chmod +x scripts/regenerate-golden.sh scripts/validate-golden.sh

# Test regeneration
./scripts/regenerate-golden.sh fzf --version 0.60.0 --os linux --arch amd64

# Verify file created
ls -la testdata/golden/plans/f/fzf/

# Test validation (should pass)
./scripts/validate-golden.sh fzf

# Modify a golden file and test (should fail with diff)
# ... manual test
```

## File Changes

| File | Change Type | Description |
|------|-------------|-------------|
| `scripts/regenerate-golden.sh` | Create | Golden file regeneration script |
| `scripts/validate-golden.sh` | Create | Golden file validation script |

## Dependencies

This issue depends on:
- #712: Cross-platform eval support (merged)
- #713: `--version` flag for recipe mode (merged)

## Test Plan

1. **Unit test: Regeneration creates correct file structure**
   - Run `regenerate-golden.sh fzf --version 0.60.0`
   - Verify files created at `testdata/golden/plans/f/fzf/v0.60.0-{platform}.json`
   - Verify `linux-arm64` is excluded

2. **Unit test: Validation passes for fresh generation**
   - Run `regenerate-golden.sh fzf --version 0.60.0`
   - Run `validate-golden.sh fzf`
   - Verify exit code 0

3. **Unit test: Validation fails with diff on mismatch**
   - Run `regenerate-golden.sh fzf --version 0.60.0`
   - Modify a golden file
   - Run `validate-golden.sh fzf`
   - Verify exit code 1 and diff output

4. **Unit test: Filter flags work correctly**
   - Run `regenerate-golden.sh fzf --os linux`
   - Verify only linux platforms generated
   - Run `regenerate-golden.sh fzf --arch amd64`
   - Verify only amd64 platforms generated

5. **Unit test: Unsupported platform cleanup**
   - Create a fake `testdata/golden/plans/f/fzf/v0.60.0-fakeos-fakeach.json`
   - Run `regenerate-golden.sh fzf` (no filters)
   - Verify fake file is removed

## Script Implementation Details

### regenerate-golden.sh

```bash
#!/usr/bin/env bash
# Regenerate golden files for a single recipe
# Usage: ./scripts/regenerate-golden.sh <recipe> [--version <ver>] [--os <os>] [--arch <arch>]

set -euo pipefail

# Script location for relative paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Paths
RECIPE_BASE="$REPO_ROOT/internal/recipe/recipes"
GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"
TSUKU="$REPO_ROOT/tsuku"

# Parse arguments
RECIPE=""
FILTER_VERSION=""
FILTER_OS=""
FILTER_ARCH=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) FILTER_VERSION="$2"; shift 2 ;;
        --os)      FILTER_OS="$2"; shift 2 ;;
        --arch)    FILTER_ARCH="$2"; shift 2 ;;
        -*)        echo "Unknown flag: $1" >&2; exit 1 ;;
        *)         RECIPE="$1"; shift ;;
    esac
done

# Validate arguments
if [[ -z "$RECIPE" ]]; then
    echo "Usage: $0 <recipe> [--version <ver>] [--os <os>] [--arch <arch>]" >&2
    exit 1
fi

# Compute paths
FIRST_LETTER="${RECIPE:0:1}"
RECIPE_PATH="$RECIPE_BASE/$FIRST_LETTER/$RECIPE.toml"
GOLDEN_DIR="$GOLDEN_BASE/$FIRST_LETTER/$RECIPE"

# Validate recipe exists
if [[ ! -f "$RECIPE_PATH" ]]; then
    echo "Recipe not found: $RECIPE_PATH" >&2
    exit 1
fi

# Create golden directory
mkdir -p "$GOLDEN_DIR"

# Get supported platforms (format: linux/amd64)
# Exclude linux-arm64 (no CI runner available)
ALL_PLATFORMS=$("$TSUKU" info --recipe "$RECIPE_PATH" --metadata-only --json | \
    jq -r '.supported_platforms[]' | tr '/' '-' | grep -v '^linux-arm64$' || true)

if [[ -z "$ALL_PLATFORMS" ]]; then
    echo "No supported platforms found for $RECIPE (excluding linux-arm64)"
    exit 0
fi

# Apply platform filters
PLATFORMS=""
for platform in $ALL_PLATFORMS; do
    os="${platform%-*}"
    arch="${platform#*-}"

    if [[ -n "$FILTER_OS" && "$os" != "$FILTER_OS" ]]; then
        continue
    fi

    if [[ -n "$FILTER_ARCH" && "$arch" != "$FILTER_ARCH" ]]; then
        continue
    fi

    PLATFORMS="$PLATFORMS $platform"
done

PLATFORMS=$(echo "$PLATFORMS" | xargs)

if [[ -z "$PLATFORMS" ]]; then
    echo "No platforms match filters (--os=$FILTER_OS, --arch=$FILTER_ARCH)" >&2
    exit 2
fi

# Determine versions to regenerate
if [[ -n "$FILTER_VERSION" ]]; then
    # Normalize version (add v prefix if missing)
    if [[ "$FILTER_VERSION" != v* ]]; then
        FILTER_VERSION="v$FILTER_VERSION"
    fi
    VERSIONS="$FILTER_VERSION"
elif [[ -d "$GOLDEN_DIR" ]] && ls "$GOLDEN_DIR"/*.json >/dev/null 2>&1; then
    # Extract versions from existing files (with v prefix)
    VERSIONS=$(ls "$GOLDEN_DIR"/*.json | sed 's/.*\/\(v[^-]*\)-.*/\1/' | sort -u)
else
    # Get latest version
    LATEST=$("$TSUKU" versions "$RECIPE" 2>/dev/null | grep -E '^\s+v' | head -1 | xargs)
    if [[ -z "$LATEST" ]]; then
        echo "Could not resolve latest version for $RECIPE" >&2
        exit 1
    fi
    VERSIONS="$LATEST"
fi

# Regenerate for each version/platform combination
for VERSION in $VERSIONS; do
    # Remove v prefix for tsuku eval (it expects version without v)
    VERSION_NO_V="${VERSION#v}"

    echo "Regenerating $RECIPE@$VERSION..."

    for platform in $PLATFORMS; do
        os="${platform%-*}"
        arch="${platform#*-}"
        OUTPUT="$GOLDEN_DIR/${VERSION}-${platform}.json"

        if "$TSUKU" eval --recipe "$RECIPE_PATH" --os "$os" --arch "$arch" \
            --version "$VERSION_NO_V" > "$OUTPUT.tmp" 2>/dev/null; then
            mv "$OUTPUT.tmp" "$OUTPUT"
            echo "  Generated: $OUTPUT"
        else
            rm -f "$OUTPUT.tmp"
            echo "  Failed: $OUTPUT" >&2
        fi
    done
done

# Clean up files for unsupported platforms (only when no filters applied)
if [[ -z "$FILTER_OS" && -z "$FILTER_ARCH" && -z "$FILTER_VERSION" ]]; then
    if [[ -d "$GOLDEN_DIR" ]]; then
        find "$GOLDEN_DIR" -name "*.json" | while read -r file; do
            # Extract platform from filename (e.g., v0.60.0-linux-amd64.json -> linux-amd64)
            filename=$(basename "$file")
            platform=$(echo "$filename" | sed 's/v[^-]*-//' | sed 's/\.json$//')

            if ! echo "$ALL_PLATFORMS" | grep -qw "$platform"; then
                echo "  Removing unsupported: $file"
                rm -f "$file"
            fi
        done
    fi
fi
```

### validate-golden.sh

```bash
#!/usr/bin/env bash
# Validate golden files for a single recipe
# Usage: ./scripts/validate-golden.sh <recipe>
# Exit codes: 0 = match, 1 = mismatch, 2 = error

set -euo pipefail

# Script location for relative paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Paths
RECIPE_BASE="$REPO_ROOT/internal/recipe/recipes"
GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"
TSUKU="$REPO_ROOT/tsuku"

# Validate arguments
RECIPE="${1:-}"
if [[ -z "$RECIPE" ]]; then
    echo "Usage: $0 <recipe>" >&2
    exit 2
fi

# Compute paths
FIRST_LETTER="${RECIPE:0:1}"
RECIPE_PATH="$RECIPE_BASE/$FIRST_LETTER/$RECIPE.toml"
GOLDEN_DIR="$GOLDEN_BASE/$FIRST_LETTER/$RECIPE"

# Validate recipe exists
if [[ ! -f "$RECIPE_PATH" ]]; then
    echo "Recipe not found: $RECIPE_PATH" >&2
    exit 2
fi

# Validate golden directory exists
if [[ ! -d "$GOLDEN_DIR" ]]; then
    echo "No golden files found for $RECIPE" >&2
    exit 2
fi

# Create temp directory for generated files
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Get supported platforms (exclude linux-arm64)
PLATFORMS=$("$TSUKU" info --recipe "$RECIPE_PATH" --metadata-only --json | \
    jq -r '.supported_platforms[]' | tr '/' '-' | grep -v '^linux-arm64$' || true)

# Extract versions from existing golden files
VERSIONS=$(ls "$GOLDEN_DIR"/*.json 2>/dev/null | sed 's/.*\/\(v[^-]*\)-.*/\1/' | sort -u)

if [[ -z "$VERSIONS" ]]; then
    echo "No golden files found in $GOLDEN_DIR" >&2
    exit 2
fi

MISMATCH=0

for VERSION in $VERSIONS; do
    VERSION_NO_V="${VERSION#v}"

    for platform in $PLATFORMS; do
        os="${platform%-*}"
        arch="${platform#*-}"
        GOLDEN="$GOLDEN_DIR/${VERSION}-${platform}.json"
        ACTUAL="$TEMP_DIR/${VERSION}-${platform}.json"

        # Skip if golden file doesn't exist for this platform
        if [[ ! -f "$GOLDEN" ]]; then
            continue
        fi

        # Generate current plan
        if ! "$TSUKU" eval --recipe "$RECIPE_PATH" --os "$os" --arch "$arch" \
            --version "$VERSION_NO_V" > "$ACTUAL" 2>/dev/null; then
            echo "Failed to generate plan for $RECIPE@$VERSION ($platform)" >&2
            continue
        fi

        # Fast hash comparison
        GOLDEN_HASH=$(sha256sum "$GOLDEN" | cut -d' ' -f1)
        ACTUAL_HASH=$(sha256sum "$ACTUAL" | cut -d' ' -f1)

        if [[ "$GOLDEN_HASH" != "$ACTUAL_HASH" ]]; then
            MISMATCH=1
            echo "MISMATCH: $GOLDEN"
            echo "--- Expected (golden)"
            echo "+++ Actual (generated)"
            diff -u "$GOLDEN" "$ACTUAL" || true
            echo ""
        fi
    done
done

if [[ $MISMATCH -eq 1 ]]; then
    echo ""
    echo "Golden file validation failed."
    echo "Run './scripts/regenerate-golden.sh $RECIPE' to update."
    exit 1
fi

echo "Golden files for $RECIPE are up to date."
exit 0
```

## Acceptance Criteria Verification

| Criteria | Implementation |
|----------|----------------|
| `regenerate-golden.sh <recipe>` generates golden files for all supported platforms | Uses `tsuku info --metadata-only --json` to get platforms, iterates and generates |
| Script supports `--version`, `--os`, `--arch` constraint flags | Argument parsing with filter logic |
| Script queries platform support via `tsuku info --recipe --metadata-only --json` | Direct command usage |
| Platform format conversion (`linux/amd64` -> `linux-amd64`) | `tr '/' '-'` in pipeline |
| Script excludes linux-arm64 (no CI runner available) | `grep -v '^linux-arm64$'` filter |
| Script removes files for unsupported platforms when no filters applied | Cleanup loop at end of regenerate script |
| `validate-golden.sh <recipe>` compares generated plans against golden files | Temp directory generation and comparison |
| Validation uses fast hash comparison, shows diff on mismatch | SHA256 comparison first, diff on mismatch |
| Exit codes: 0 = match, 1 = mismatch, 2 = error | Implemented in validate script |
| Scripts are executable and use bash strict mode | `#!/usr/bin/env bash` and `set -euo pipefail` |

# Issue 1574 Implementation Plan

## Summary

Create a shell script that installs system packages based on recipe declarations using `tsuku info --deps-only --system --family`.

## Files to Create

1. `.github/scripts/install-recipe-deps.sh`

## Implementation Steps

### Step 1: Create the helper script

**File**: `.github/scripts/install-recipe-deps.sh`

Script structure:
1. Header comment with usage and description
2. `set -euo pipefail` for strict error handling
3. Parse arguments: `FAMILY` (required), `RECIPE` (required), `TSUKU` (optional, default `./tsuku`)
4. Call `tsuku info --deps-only --system --family "$FAMILY" "$RECIPE"` to get packages
5. Exit cleanly if no packages returned
6. Install packages using family-appropriate package manager

**Package manager commands per family**:
- alpine: `apk add --no-cache`
- debian: `apt-get install -y --no-install-recommends`
- rhel: `dnf install -y --setopt=install_weak_deps=False`
- arch: `pacman -S --noconfirm`
- suse: `zypper -n install`

### Step 2: Make script executable

```bash
chmod +x .github/scripts/install-recipe-deps.sh
```

### Step 3: Test locally

Build tsuku and verify the script works:
```bash
go build -o tsuku ./cmd/tsuku
.github/scripts/install-recipe-deps.sh alpine zlib
# Should output: zlib-dev
```

## Validation

Per issue acceptance criteria:
- [x] Script exists at `.github/scripts/install-recipe-deps.sh`
- [x] Script is executable
- [x] Accepts `<family>` and `<recipe>` as required positional arguments
- [x] Accepts optional third argument for tsuku binary path
- [x] Calls `tsuku info --deps-only --system --family`
- [x] Exits cleanly on empty output
- [x] Installs with correct package manager per family
- [x] Handles all five families
- [x] Uses `set -e` for error propagation

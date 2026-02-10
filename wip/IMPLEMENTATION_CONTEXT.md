---
summary:
  constraints:
    - Script must work with all five Linux families (alpine, debian, rhel, arch, suse)
    - Must use correct package manager command per family
    - Must handle empty package list gracefully (no error)
    - Uses tsuku info --deps-only --system --family (from #1573)
  integration_points:
    - .github/scripts/install-recipe-deps.sh (new script)
    - cmd/tsuku/info.go (consumer - uses its output)
    - Future workflows: integration-tests.yml, platform-integration.yml, validate-golden-execution.yml
  risks:
    - Package manager commands vary between families
    - Tsuku binary path might need to be configurable
  approach_notes: |
    Simple shell script that:
    1. Accepts family, recipe, and optional tsuku binary path
    2. Calls tsuku info --deps-only --system --family to get packages
    3. Installs packages with family-appropriate package manager
    The script is a thin wrapper - the real logic is in tsuku info.
---

# Implementation Context: Issue #1574

This issue creates a workflow helper script that uses the `tsuku info --deps-only --system --family` command (implemented in #1573) to install recipe-declared system dependencies.

## Key Details

**Script location**: `.github/scripts/install-recipe-deps.sh`

**Arguments**:
1. `family` (required) - alpine, debian, rhel, arch, suse
2. `recipe` (required) - recipe name to extract deps for
3. `tsuku` (optional) - path to tsuku binary, defaults to `./tsuku`

**Package manager mapping**:
| Family | Command |
|--------|---------|
| alpine | `apk add --no-cache` |
| debian | `apt-get install -y --no-install-recommends` |
| rhel | `dnf install -y --setopt=install_weak_deps=False` |
| arch | `pacman -S --noconfirm` |
| suse | `zypper -n install` |

## Acceptance Criteria Summary

- Script exists and is executable
- Handles all five families
- Exits cleanly on empty output
- Uses `set -e` for error propagation

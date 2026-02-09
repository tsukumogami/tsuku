---
status: Proposed
problem: |
  The musl library dlopen tests fail in CI because the workflow doesn't install
  the system packages that recipes require. The `apk_install` action only checks
  if packages exist - it doesn't install them. The workflow hardcodes build tools
  but not library-specific packages like `zlib-dev` and `yaml-dev`, causing tests
  to fail with "missing system packages" errors.
decision: |
  Create a test helper script that extracts required packages from recipes and
  installs them before running tests. The script will use tsuku's plan generation
  to identify `apk_install` actions and extract their package lists, then run
  the appropriate package manager command. This aligns with how sandbox mode
  already handles system requirements via `ExtractSystemRequirements()`.
rationale: |
  This approach treats recipes as the source of truth for dependencies, matching
  the sandbox verification model. It eliminates the need to hardcode packages in
  workflows and ensures new library recipes work without CI changes. The existing
  `sandbox/packages.go` logic provides a proven pattern to follow.
---

# Design: Recipe-Driven Library Test Setup

## Status

Proposed

## Context and Problem Statement

The musl library dlopen integration tests in CI are failing. When the test script runs `tsuku install zlib` on Alpine Linux (musl), the `apk_install` action checks whether `zlib-dev` is installed and fails with:

```
missing system packages: [zlib-dev] (install with: apk add zlib-dev)
```

The fundamental issue is an asymmetry in how different test environments handle library installation:

| Environment | Installation Method | Package Source | Status |
|-------------|---------------------|----------------|--------|
| glibc (Linux) | Homebrew bottles | Downloaded directly | Works |
| musl (Alpine) | System packages | Must be pre-installed | Fails |
| macOS | Homebrew bottles | Downloaded directly | Works |
| Sandbox | Container setup | Extracted from recipe | Works |

For musl, the `apk_install` action is designed to verify that packages are already installed - it doesn't install them. This is correct behavior for tsuku (which shouldn't require root), but means CI must pre-install these packages.

Currently, the workflow hardcodes build dependencies:
```yaml
- name: Install build dependencies
  run: |
    apk add --no-cache curl gcc musl-dev bash git
```

But it doesn't install library-specific packages like `zlib-dev` or `yaml-dev`. These packages are specified in recipes but never extracted for pre-installation.

The sandbox mode already solves this problem correctly via `ExtractSystemRequirements()` in `internal/sandbox/packages.go`, which parses installation plans and extracts package lists from `apk_install`, `apt_install`, and similar actions.

### Scope

**In scope:**
- Extracting required packages from library recipes for test setup
- Installing those packages before running dlopen tests on musl
- Making the test infrastructure recipe-driven rather than hardcoded

**Out of scope:**
- Changing how `apk_install` action works (it correctly only verifies)
- Adding package installation capabilities to tsuku itself
- Modifying glibc/macOS test paths (they work via Homebrew)

## Decision Drivers

- **Recipes as source of truth**: Package requirements should come from recipe files, not hardcoded in workflows
- **Consistency with sandbox model**: The solution should mirror how sandbox mode extracts and installs packages
- **No workflow changes for new recipes**: Adding a new library recipe shouldn't require editing CI configuration
- **Maintainability**: Reduce duplication between how sandbox and CI tests handle system packages
- **Simplicity**: Prefer using existing tsuku infrastructure over building new extraction logic

## Considered Options

### Decision 1: How to Extract Required Packages

The core question is how the test script should determine which system packages a recipe needs. This needs to work for any library recipe, not just the current test matrix.

#### Chosen: Use tsuku plan output

Have the test script run `tsuku plan <library> --json` to generate an installation plan, then extract packages from `apk_install` steps. This reuses tsuku's existing platform filtering and step selection logic.

The test script would:
1. Run `tsuku plan zlib --json` to get the filtered plan for the current platform
2. Parse the JSON to find steps with `action: "apk_install"`
3. Extract the `packages` array from those steps
4. Run `apk add` with the collected packages before the actual test

This approach:
- Uses tsuku's existing constraint evaluation (os, libc, family filtering)
- Works automatically for any recipe with `apk_install` steps
- Matches how sandbox mode conceptually works (extract from plan, then install)

#### Alternatives Considered

**Parse recipe TOML directly**: Read the recipe file and extract packages from `apk_install` steps using shell tools (grep, jq after toml-to-json conversion).
Rejected because it would duplicate constraint evaluation logic that tsuku already implements. Recipe steps have `when` clauses that filter by OS/libc/family - reimplementing this in shell would be error-prone and diverge from tsuku's behavior.

**Add a tsuku subcommand**: Create `tsuku deps --json <recipe>` that outputs required system packages for the current platform.
Rejected because it adds permanent CLI surface for a CI-only use case. The plan output already contains this information; adding a dedicated command isn't justified.

**Hardcode packages per library in workflow**: Expand the matrix to include required packages for each library.
Rejected because it defeats the goal of recipe-driven testing. Every new library recipe would require workflow edits.

### Decision 2: Where to Implement the Extraction Logic

#### Chosen: Test helper script with jq parsing

Create a test helper script (`test/scripts/install-recipe-deps.sh`) that:
1. Takes a recipe name and package manager as arguments
2. Runs `tsuku plan <recipe> --json`
3. Uses jq to extract packages from matching action steps
4. Runs the appropriate install command

The dlopen test script calls this helper before running the actual test. This keeps the logic reusable across different test types.

```bash
# Example usage in test-library-dlopen.sh
./test/scripts/install-recipe-deps.sh "$LIBRARY_NAME" apk
```

The helper script handles the JSON parsing:
```bash
PACKAGES=$(./tsuku plan "$RECIPE" --json | jq -r '
  .steps[] |
  select(.action == "'"$MANAGER"'_install") |
  .params.packages[]
' | sort -u | xargs)

if [ -n "$PACKAGES" ]; then
  case "$MANAGER" in
    apk) apk add --no-cache $PACKAGES ;;
    apt) apt-get install -y $PACKAGES ;;
    # ... other managers
  esac
fi
```

#### Alternatives Considered

**Inline in test-library-dlopen.sh**: Put the extraction logic directly in the test script.
Rejected because other tests (library-integrity, future library tests) would need the same logic. A helper script enables reuse.

**Go helper binary**: Write a Go tool that uses the sandbox package extraction code.
Rejected as overengineered for this use case. The jq approach is simpler, requires no compilation, and the JSON structure is stable.

## Decision Outcome

**Chosen: Plan-based extraction with helper script**

### Summary

Create a helper script that extracts required system packages from recipe plans and installs them. The script runs `tsuku plan <recipe> --json`, parses the output with jq to find package manager actions (apk_install, apt_install, etc.), collects their package lists, and runs the appropriate install command.

The dlopen test script will call this helper early in its execution, before attempting to install the library via tsuku. This ensures the externally-managed packages are available when tsuku's verification actions check for them.

The helper script takes two arguments: recipe name and package manager type (apk, apt, dnf, etc.). It outputs the packages being installed for visibility, and exits cleanly if no packages are needed (allowing glibc/macOS tests to call it harmlessly).

Error handling: if `tsuku plan` fails, the script exits with an error. If jq finds no matching packages, the script logs this and exits successfully (the recipe may not have system package requirements for this platform).

### Rationale

This approach mirrors the sandbox model conceptually - both extract package requirements from plans and install them before verification. Using `tsuku plan` output means we inherit tsuku's constraint evaluation without reimplementing it. The jq-based parsing is sufficient for CI scripts and avoids adding permanent CLI surface.

The helper script pattern enables reuse across test types. When we add more library tests or expand to other package managers, the same script handles extraction. The workflow only needs to specify which recipe to test; the script figures out what to install.

## Solution Architecture

### Component Overview

```
test/scripts/
├── install-recipe-deps.sh   # NEW: Extracts and installs recipe dependencies
├── test-library-dlopen.sh   # MODIFIED: Calls install-recipe-deps.sh
└── ...

.github/workflows/
└── integration-tests.yml    # MODIFIED: Remove continue-on-error once fixed
```

### Helper Script Interface

```bash
# install-recipe-deps.sh <recipe-name> <package-manager>
#
# Extracts system packages from a recipe's installation plan and installs them.
#
# Arguments:
#   recipe-name     Name of the recipe (e.g., "zlib", "libyaml")
#   package-manager Package manager to use: apk, apt, dnf, pacman, zypper
#
# Environment:
#   TSUKU_HOME      Optional. If set, uses this for plan generation.
#
# Exit codes:
#   0  Success (packages installed or none needed)
#   1  Error (plan generation failed, install failed)
#
# Examples:
#   ./install-recipe-deps.sh zlib apk      # Alpine: installs zlib-dev
#   ./install-recipe-deps.sh libyaml apt   # Debian: installs libyaml-dev
#   ./install-recipe-deps.sh gcc-libs apk  # Alpine: installs libstdc++
```

### Data Flow

```
1. test-library-dlopen.sh called with (library, family)
              |
              v
2. install-recipe-deps.sh called with (library, apk)
              |
              v
3. tsuku plan <library> --json
              |
              v
4. jq extracts packages from apk_install steps
              |
              v
5. apk add --no-cache <packages>
              |
              v
6. Back to test-library-dlopen.sh
              |
              v
7. tsuku install <library> (now succeeds)
              |
              v
8. tsuku verify <library> (dlopen test)
```

### JSON Parsing Logic

The plan output structure:
```json
{
  "steps": [
    {
      "action": "apk_install",
      "when": {"os": ["linux"], "libc": ["musl"]},
      "params": {
        "packages": ["zlib-dev"]
      }
    }
  ]
}
```

jq filter to extract packages:
```bash
jq -r '.steps[] | select(.action == "apk_install") | .params.packages[]'
```

## Implementation Approach

### Phase 1: Create Helper Script

1. Create `test/scripts/install-recipe-deps.sh`:
   - Accept recipe name and package manager arguments
   - Build tsuku if not present
   - Run `tsuku plan <recipe> --json`
   - Parse with jq to extract packages for the specified manager
   - Run appropriate install command with collected packages
   - Log actions for CI visibility

2. Make script executable and add to git

### Phase 2: Integrate with Test Scripts

1. Modify `test/scripts/test-library-dlopen.sh`:
   - Call `install-recipe-deps.sh` before `tsuku install`
   - Map family to package manager (alpine -> apk, debian -> apt, etc.)
   - Keep existing logic unchanged after package installation

2. Test locally with Docker:
   ```bash
   docker run --rm -it golang:1.23-alpine /bin/sh
   # Inside container: run test script
   ```

### Phase 3: Update CI Workflow

1. Remove `continue-on-error: true` from `library-dlopen-musl` job
2. Optionally extend to other library tests (integrity tests)
3. Verify all musl tests pass

### Verification

- musl tests for zlib, libyaml, gcc-libs all pass
- glibc tests unaffected (helper exits cleanly with no apk packages)
- macOS tests unaffected (same reason)
- Adding a new library recipe works without workflow changes

## Security Considerations

### Download Verification
Not directly affected. This design adds a pre-installation step for system packages but doesn't change how tsuku verifies downloaded content. System packages are verified by the package manager's signature checking.

### Execution Isolation
The helper script runs package manager commands that require root. In CI, this is acceptable (containers run as root). The script doesn't grant any new permissions - it just automates what would otherwise be a manual `apk add` step.

### Supply Chain Risks
System packages come from Alpine's official repositories, which are signed and verified. This is the same trust model as the existing `apk add curl gcc...` line in the workflow. No new package sources are introduced.

### User Data Exposure
Not applicable. The helper script only installs packages and doesn't access or transmit any user data.

## Consequences

### Positive

- **Recipe-driven testing**: Package requirements come from recipes, not hardcoded lists
- **Lower maintenance**: New library recipes work without workflow changes
- **Consistency**: Test setup mirrors sandbox mode's approach to system requirements
- **Visibility**: Helper script logs which packages it installs for debugging

### Negative

- **Additional script**: One more file to maintain in test infrastructure
- **jq dependency**: CI containers need jq installed (already available in golang:alpine)
- **Slower tests**: Extra `tsuku plan` invocation adds a few seconds per test

### Neutral

- **Plan output stability**: The helper depends on plan JSON structure remaining stable (it has been stable since introduction)

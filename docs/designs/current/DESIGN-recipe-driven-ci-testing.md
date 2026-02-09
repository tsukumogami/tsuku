---
status: Current
problem: |
  GHA integration tests for musl/Alpine use hardcoded package lists instead of
  deriving requirements from recipes. Tests pass when packages are pre-installed
  even if recipes don't declare them, causing users on Alpine to encounter
  failures that CI never caught. Issue #1570 exposed this when musl dlopen tests
  failed for zlib and libyaml.
decision: |
  Add a composable CLI command `tsuku deps` that extracts system package names
  from recipes for a target platform. GHA workflows use this to install only
  recipe-declared packages, making under-declaration cause natural test failures.
  This validates recipes without complex orchestration or nested containers.
rationale: |
  Validation comes from workflow structure, not tooling complexity. Starting from
  a minimal container and installing only declared packages means under-declaration
  fails naturally. This is simpler than extending sandbox mode (avoids container-in-
  container) and more composable than a dedicated CI command (works with any CI
  system). The ~200 LOC implementation reuses existing filtering and extraction code.
---

# Recipe-Driven CI Testing

## Status

**Current**

## Context and Problem Statement

GHA integration tests for musl/Alpine use hardcoded package lists:

```yaml
run: apk add --no-cache curl gcc musl-dev bash git
```

But recipes declare different packages via `apk_install` actions:

```toml
[[steps]]
action = "apk_install"
packages = ["zlib-dev"]
when = { os = ["linux"], libc = ["musl"] }
```

This creates two problems:

1. **False positives**: Tests pass when packages are pre-installed, even if recipes don't declare them
2. **User failures**: Users on Alpine get errors because recipes under-declare dependencies

Issue #1570 exposed this gap when the `library-dlopen-musl` job failed for `zlib` and `libyaml` with "missing system packages" errors. The recipes correctly declared `zlib-dev` and `yaml-dev`, but the workflow only installed generic build dependencies.

The root cause is that CI doesn't validate recipes correctly declare their system dependencies. The existing `--sandbox` mode derives container configuration from recipes, but GHA workflows bypass this entirely with manual package lists.

### Scope

**In scope:**
- CLI mechanism to extract system package requirements from recipes
- GHA workflow changes to use recipe-driven package installation
- Validation that recipes don't under-declare dependencies

**Out of scope:**
- Changes to sandbox mode internals
- New testing frameworks
- Changes to recipe format

## Decision Drivers

- **Validate declarations**: Tests must fail if recipes under-declare dependencies
- **Simplicity**: Prefer composable CLI tools over complex integrated systems
- **No nested containers**: Avoid container-in-container complexity in GHA
- **Reuse existing code**: Leverage existing package extraction and filtering logic
- **CI agnostic**: Solution should work with any CI system, not just GHA

## Considered Options

### Decision 1: Architectural Approach

The central question is how to make GHA tests derive package requirements from recipes instead of using hardcoded lists. Three approaches were considered, each with different tradeoffs around complexity, validation strength, and reuse of existing infrastructure.

#### Chosen: CLI Query Only

Add a composable CLI command that extracts system package requirements from recipes. Workflows use this to install only declared packages.

**How it works:**

```bash
# Get system packages declared for Alpine
tsuku deps --system --family alpine zlib
# Output: zlib-dev

# JSON format for scripting
tsuku deps --system --family alpine --format json zlib
# Output: {"packages":["zlib-dev"],"family":"alpine"}

# Use in CI workflows
apk add $(tsuku deps --system --family alpine zlib)
```

The command:
1. Loads the recipe
2. Builds a target from `--family` (derives libc: alpine → musl, others → glibc)
3. Filters steps using existing `FilterStepsByTarget()` function
4. Extracts package names from system dependency actions (`apk_install`, `apt_install`, etc.)
5. Outputs as text (one per line) or JSON

**Trade-offs accepted:**
- Workflows must be structured correctly (start minimal, install only declared packages)
- Each workflow needs manual updates to use the CLI
- Validation is implicit (natural failure) rather than explicit (error message)

#### Alternatives Considered

**Extend Sandbox Mode**: Use `tsuku install --sandbox` in GHA workflows. The sandbox already derives container configuration from plans via `ComputeSandboxRequirements()`.

Rejected because:
- **Container-in-container complexity**: GHA runners would need to spawn Docker containers inside containers
- **Static binary required**: tsuku must be built with `CGO_ENABLED=0` for Alpine execution
- **Test overhead**: Container build adds time and image caching complexity
- **Scope creep**: Sandbox is designed for local dev testing, not CI validation

The sandbox approach would work, but it solves a different problem (isolated execution) and brings unnecessary complexity for what's fundamentally a data extraction task.

**Parallel CI System (`tsuku ci-test`)**: Create a new command specifically for CI validation that extracts, validates, installs, and tests in the current environment.

Rejected because:
- **New subsystem**: ~500 LOC for orchestration logic vs ~200 LOC for query-only
- **Requires root/sudo**: Must install packages in CI, coupling tsuku to privilege escalation
- **Coupled responsibilities**: Mixes extraction, validation, and installation in one command
- **Less composable**: Only works for the specific CI pattern it's designed for

This approach is more "complete" but less flexible. The query-only approach can be composed with any workflow structure.

### Decision 2: Output Format

How should the CLI output package requirements for workflow consumption?

#### Chosen: Machine-readable JSON with shell-friendly default

```bash
# Plain text for shell scripting (default)
tsuku deps --system --family alpine zlib
zlib-dev

# JSON for programmatic use
tsuku deps --system --family alpine --format json zlib
{"packages": ["zlib-dev"], "family": "alpine"}
```

The plain text output can be used directly in shell:
```bash
apk add $(tsuku deps --system --family alpine zlib)
```

#### Alternatives Considered

**Only JSON**: Would require `jq` for shell usage, adding a dependency and complexity for the common case.

**Only text**: Would lose structure for programmatic consumers that want to parse the family or handle empty results.

## Decision Outcome

**Chosen: CLI Query Only with dual output format**

### Summary

Add a `tsuku deps` command that extracts system package requirements for a recipe on a target platform. The command reuses existing logic from `sysdeps.go` (`systemActionNames` map) and `filter.go` (`FilterStepsByTarget()`). It filters recipe steps by target platform, extracts package names from system dependency actions, and outputs them.

GHA workflows change from hardcoded package lists to recipe-derived installation:

```yaml
# Before
run: apk add --no-cache curl gcc musl-dev bash git

# After
run: |
  DEPS=$(./tsuku deps --system --family alpine ${{ matrix.library }})
  if [ -n "$DEPS" ]; then
    apk add --no-cache $DEPS
  fi
```

Validation happens through workflow structure: start with a minimal container (only bootstrap dependencies), install only what recipes declare, run the test. If a recipe under-declares, the test fails because the required package isn't installed. No explicit validation logic needed—the natural consequence of the workflow structure is that under-declaration causes failures.

### Rationale

This approach validates recipe declarations through **workflow structure** rather than complex tooling:

1. **Minimal base**: Start with only bootstrap dependencies (curl, gcc for Rust build)
2. **Recipe-driven install**: Install only what the recipe declares
3. **Natural failure**: Under-declaration causes missing package errors

The CLI query approach fits the Unix philosophy of small, composable tools. It does one thing (extract package names) and does it well. Workflows can compose it however they need—the validation logic is in the workflow structure, not baked into the tool.

Sandbox mode (Option A) adds container-in-container complexity for a problem that doesn't require containers. The CI-test command (Option B) creates an orchestration layer when simple data extraction suffices. Both are valid approaches but bring complexity that isn't justified by the problem.

## Solution Architecture

### Component: `tsuku deps` Command

**Location**: `cmd/tsuku/deps.go`

**Interface**:
```
tsuku deps [--system] [--family <family>] [--format <text|json>] <recipe>
```

**Flags:**
- `--system`: Filter to system dependency actions only (apk_install, apt_install, etc.)
- `--family`: Target Linux family (alpine, debian, rhel, arch, suse)
- `--format`: Output format (default: text, one package per line)
- `--recipe`: Path to local recipe file (alternative to registry lookup)

**Data flow:**

```
Recipe (zlib.toml)
    │
    ├─ steps with when = { libc = ["musl"] }
    │     └─ apk_install packages = ["zlib-dev"]
    │
    ▼
tsuku deps --system --family alpine zlib
    │
    ├─ FilterStepsByTarget(steps, alpine/musl)
    │     └─ matches apk_install step (implicit alpine constraint)
    │
    ├─ Extract packages from params
    │     └─ ["zlib-dev"]
    │
    ▼
Output: zlib-dev
```

**Reused code:**
- `internal/executor/filter.go`: `FilterStepsByTarget()` for platform filtering
- `cmd/tsuku/sysdeps.go`: `systemActionNames` map for identifying system actions
- `internal/actions/util.go`: `GetStringSlice()` for extracting package arrays
- `internal/platform/libc.go`: `LibcForFamily()` for deriving libc from family

### Workflow Integration

**Before** (`.github/workflows/integration-tests.yml`):
```yaml
library-dlopen-musl:
  container:
    image: golang:1.23-alpine
  steps:
    - name: Install build dependencies
      run: |
        apk add --no-cache curl gcc musl-dev bash git  # HARDCODED
```

**After**:
```yaml
library-dlopen-musl:
  container:
    image: golang:1.23-alpine
  steps:
    - name: Install bootstrap dependencies
      run: |
        apk add --no-cache curl gcc musl-dev bash git
        # Rust toolchain for dltest binary
        curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y

    - name: Build tsuku
      run: go build -o tsuku ./cmd/tsuku

    - name: Install recipe-declared dependencies
      run: |
        DEPS=$(./tsuku deps --system --family alpine ${{ matrix.library }})
        if [ -n "$DEPS" ]; then
          apk add --no-cache $DEPS
        fi
```

The key insight: bootstrap dependencies (Go, Rust, curl) are still hardcoded because they're needed to build tsuku and the test harness. But library-specific dependencies (zlib-dev, yaml-dev) come from the recipe.

## Implementation Approach

### Phase 1: Add `tsuku deps` command

**Files created:**
- `cmd/tsuku/deps.go` (~150 LOC) - Command implementation
- `cmd/tsuku/deps_test.go` (~180 LOC) - Unit tests

**Files modified:**
- `cmd/tsuku/main.go` - Register command

**Key functions:**
- `buildTargetFromFlags(family string) platform.Target` - Constructs target with correct libc
- `extractSystemPackages(r *recipe.Recipe, target platform.Target) []string` - Extracts package names

### Phase 2: Update GHA workflow

**Files modified:**
- `.github/workflows/integration-tests.yml` - Update `library-dlopen-musl` job

**Changes:**
1. Add step to build tsuku before dependency extraction
2. Add step to install recipe-declared packages using `tsuku deps`
3. Remove `continue-on-error: true` (tests should now pass)

## Security Considerations

### Download Verification

Not applicable. The `deps` command only extracts package names from recipes—it doesn't download anything. Package installation happens via the system package manager (apk), which has its own verification.

### Execution Isolation

The `tsuku deps` command is read-only. It:
1. Loads a recipe from the registry or a local file
2. Filters steps by target platform
3. Extracts string values from step parameters
4. Outputs text or JSON

No execution, no privilege escalation, no system modification.

### Supply Chain Risks

Package installation happens via `apk add` in the workflow, not via tsuku. Alpine's package manager has GPG-signed packages from official mirrors. This design doesn't change the trust model—it just determines *which* packages get installed.

One consideration: a malicious recipe could declare packages that install malware. But this is no different from the current situation where recipes can declare arbitrary packages. The workflow runs in CI containers with limited blast radius.

### User Data Exposure

No data access or transmission. The command reads recipe files and outputs package names. No telemetry, no network calls, no file system modifications.

## Consequences

### Positive

- **Validates recipe declarations**: Under-declaring packages causes CI failures
- **Simple**: ~200 LOC, reuses existing code
- **Composable**: Works with any CI system, any workflow structure
- **No nested containers**: Avoids sandbox complexity in CI
- **Self-documenting**: Workflows show exactly which packages recipes need
- **Debuggable**: Users can run `tsuku deps` locally to see what packages a recipe needs

### Negative

- **Manual workflow updates**: Each workflow needs modification to use the new pattern
- **Workflow discipline required**: Teams must structure workflows correctly (minimal base, recipe-driven install)
- **Bootstrap dependency**: Need Go installed to build tsuku before using `deps`
- **Two-phase package install**: Bootstrap deps (hardcoded) + library deps (recipe-driven) is more complex than single hardcoded list

### Uncertainties

- **Other workflows**: The `validate-all-recipes.yml` workflow also has hardcoded package lists. This design applies there too, but migration wasn't included in the initial implementation.
- **Non-Alpine families**: The pattern works for any family (debian, rhel, arch, suse), but only Alpine was tested in this implementation.

---
status: Proposed
problem: |
  GHA integration tests for musl/Alpine use hardcoded package lists instead of
  deriving requirements from recipes. Tests pass when packages are pre-installed
  even if recipes don't declare them, causing users on Alpine to encounter
  failures that CI never caught. Issue #1570 exposed this when musl dlopen tests
  failed for zlib and libyaml.
decision: |
  Add a `tsuku deps` command that recursively extracts system package names from
  recipes and their transitive dependencies for a target platform. The command
  shares extraction logic with sandbox mode via a new internal/executor/system_deps.go
  library. GHA workflows use this to install only recipe-declared packages, making
  under-declaration cause natural test failures. All five Linux families are supported.
rationale: |
  Validation comes from workflow structure, not tooling complexity. Recursive
  extraction ensures complete dependency coverage. Shared extraction code eliminates
  duplication between deps command and sandbox mode (~66 LOC saved). Keeping deps
  as a separate command (vs extending info or check-deps) matches Unix philosophy
  and the shell scripting use case.
---

# Recipe-Driven CI Testing

## Status

**Proposed**

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
- CLI mechanism to extract system package requirements from recipes (including transitive dependencies)
- Shared extraction library for deps command and sandbox mode
- GHA workflow changes to use recipe-driven package installation
- Multi-family support (alpine, debian, rhel, arch, suse)
- Validation that recipes don't under-declare dependencies

**Out of scope:**
- Changes to sandbox mode internals (beyond sharing extraction code)
- New testing frameworks
- Changes to recipe format

## Decision Drivers

- **Validate declarations**: Tests must fail if recipes under-declare dependencies
- **Complete coverage**: Transitive dependencies must be included
- **Code reuse**: Share logic between deps command and sandbox mode
- **Multi-family**: Support all five Linux families consistently
- **Simplicity**: Prefer composable CLI tools over complex integrated systems
- **No nested containers**: Avoid container-in-container complexity in GHA
- **CI agnostic**: Solution should work with any CI system, not just GHA

## Considered Options

### Decision 1: Architectural Approach

The central question is how to make GHA tests derive package requirements from recipes instead of using hardcoded lists.

#### Chosen: CLI Query with Shared Extraction Library

Add a composable CLI command that extracts system package requirements from recipes and their transitive dependencies. The extraction logic is shared with sandbox mode via a new internal library.

**How it works:**

```bash
# Get system packages declared for Alpine (includes transitive deps)
tsuku deps --system --family alpine zlib
# Output: zlib-dev

# For a recipe with dependencies
tsuku deps --system --family alpine curl
# Output: zlib-dev openssl-dev (from curl's dependencies)

# JSON format for scripting
tsuku deps --system --family alpine --format json zlib
# Output: {"packages":["zlib-dev"],"family":"alpine"}

# Use in CI workflows
apk add $(tsuku deps --system --family alpine zlib)
```

The command:
1. Loads the recipe
2. Resolves transitive tsuku dependencies using existing `ResolveTransitive()` machinery
3. Builds a target from `--family` (derives libc: alpine → musl, others → glibc)
4. For each recipe in the dependency tree, filters steps and extracts packages
5. Deduplicates and outputs as text or JSON

#### Alternatives Considered

**Extend Sandbox Mode**: Use `tsuku install --sandbox` in GHA workflows.

Rejected because:
- **Container-in-container complexity**: GHA runners would need to spawn Docker containers inside containers
- **Static binary required**: tsuku must be built with `CGO_ENABLED=0` for Alpine execution
- **Scope creep**: Sandbox is designed for local dev testing, not CI validation

**Parallel CI System (`tsuku ci-test`)**: Create a new command for CI validation that orchestrates extraction, installation, and testing.

Rejected because:
- **New subsystem**: ~500 LOC for orchestration logic vs ~250 LOC for query-only
- **Requires root/sudo**: Must install packages in CI
- **Less composable**: Only works for specific CI patterns

### Decision 2: Transitive Dependency Resolution

Should the command extract packages from just the target recipe, or recursively from all dependencies?

#### Chosen: Recursive by Default

The command recursively resolves all tsuku dependencies and extracts system packages from each.

**Rationale:**
- Recipes like `curl` depend on `openssl` and `zlib`, which have their own system packages
- Non-recursive would require each recipe to redeclare all transitive system deps
- The infrastructure already exists (`ResolveTransitive()` in resolver.go)
- Implementation cost is ~55 additional LOC with battle-tested cycle detection

**Trade-offs accepted:**
- Slightly higher latency (multiple recipe loads)
- More complex implementation than direct extraction

#### Alternatives Considered

**Non-recursive**: Only extract from the target recipe.

Rejected because current test recipes (zlib, libyaml, gcc-libs) don't expose the problem, but future recipes with dependency chains would fail silently.

**Opt-in flag**: Add `--recursive` flag, default to non-recursive.

Rejected because the feature hasn't shipped yet—now is the time to design it correctly. Backward compatibility isn't a concern for unreleased code.

### Decision 3: Code Reuse with Sandbox

Both `tsuku deps` and sandbox mode extract system packages from recipes. How should they share code?

#### Chosen: Shared Extraction Library in internal/executor

Create `internal/executor/system_deps.go` with extraction functions that both consumers use.

```go
// Core extraction from steps (used by both)
func ExtractSystemPackagesFromSteps(steps []recipe.Step) (
    packages map[string][]string,
    repositories []SystemRepository,
)

// High-level wrapper for CLI
func ExtractSystemPackagesList(r *recipe.Recipe, target platform.Target) []string

// Helper using SystemAction interface
func IsSystemAction(step recipe.Step) bool
```

**Benefits:**
- Single source of truth for extraction logic
- ~66 LOC reduction from eliminating duplication
- Leverages existing `SystemAction` interface for type-safe detection
- Places code alongside related `FilterStepsByTarget()` function

#### Alternatives Considered

**internal/actions/system_helpers.go**: Place in actions package.

Not chosen because executor is the right abstraction level (between actions and consumers).

**Keep separate**: Document duplication, don't consolidate.

Rejected because maintenance burden grows and bugs must be fixed twice.

### Decision 4: UX - Command Structure

Should `tsuku deps` be a new command, or should functionality be added to existing commands like `info` or `check-deps`?

#### Chosen: Separate Top-Level Command

Keep `tsuku deps` as a distinct command focused on package extraction for scripting.

**Rationale:**

Tsuku has four dependency-related commands, each with distinct purposes:

| Command | Purpose | Output | Use Case |
|---------|---------|--------|----------|
| `tsuku info` | Introspection | Human-readable metadata | "Tell me about this tool" |
| `tsuku check-deps` | Validation | Pass/fail with report | Pre-install validation |
| `tsuku verify-deps` | Verification | Pass/fail for require_command | System setup checks |
| `tsuku deps` | Extraction | Shell-friendly package list | CI automation |

The `deps` command exists for shell scripting: `apk add $(tsuku deps --system --family alpine zlib)`. This is cleaner than `apk add $(tsuku info --deps-only --system --family alpine zlib)`.

#### Alternatives Considered

**Extend `tsuku info`**: Add `--deps-only --system --family` flags.

Rejected because it makes the common scripting case verbose and mixes introspection with extraction.

**Subcommand structure**: `tsuku deps show`, `tsuku deps tree`.

Rejected as over-architected for current scope. Can be added later if needed.

### Decision 5: Output Format

How should the CLI output package requirements?

#### Chosen: Shell-friendly text default with JSON option

```bash
# Plain text (default) - one package per line
tsuku deps --system --family alpine zlib
zlib-dev

# JSON for programmatic use
tsuku deps --system --family alpine --format json zlib
{"packages": ["zlib-dev"], "family": "alpine"}
```

Plain text can be used directly in shell substitution without dependencies like `jq`.

## Decision Outcome

**Chosen: Recursive CLI query with shared extraction library**

### Summary

Add a `tsuku deps` command that recursively extracts system package requirements for a recipe and all its transitive dependencies on a target platform. The command shares extraction logic with sandbox mode via `internal/executor/system_deps.go`, eliminating code duplication.

The command:
1. Loads the target recipe
2. Resolves transitive dependencies using existing `ResolveTransitive()`
3. For each recipe, filters steps by target platform
4. Extracts package names from system actions
5. Deduplicates and outputs as text or JSON

GHA workflows change from hardcoded package lists to recipe-derived installation:

```yaml
# Before
run: apk add --no-cache curl gcc musl-dev bash git zlib-dev yaml-dev

# After
run: |
  DEPS=$(./tsuku deps --system --family alpine ${{ matrix.library }})
  if [ -n "$DEPS" ]; then
    apk add --no-cache $DEPS
  fi
```

Validation happens through workflow structure: start with minimal bootstrap dependencies, install only what recipes declare (including transitive deps), run the test. Under-declaration at any level causes failure.

### Rationale

This approach validates recipe declarations through **workflow structure** while providing **complete coverage** of transitive dependencies:

1. **Recursive extraction**: Catches under-declaration in any recipe in the dependency tree
2. **Shared code**: Single extraction implementation for deps and sandbox
3. **Multi-family**: All five Linux families use the same pattern
4. **Composable**: Works with any CI system via shell scripting

## Solution Architecture

### Component 1: Shared Extraction Library

**Location**: `internal/executor/system_deps.go`

```go
// SystemDeps holds extracted system dependencies
type SystemDeps struct {
    Packages     map[string][]string // package manager → packages
    Repositories []RepositoryConfig  // apt_repo, brew_tap, etc.
}

// ExtractSystemPackagesFromSteps extracts system deps from filtered steps
func ExtractSystemPackagesFromSteps(steps []recipe.Step) *SystemDeps

// ExtractSystemPackagesRecursive extracts from recipe and all transitive deps
func ExtractSystemPackagesRecursive(
    ctx context.Context,
    loader recipe.Loader,
    r *recipe.Recipe,
    target platform.Target,
) ([]string, error)

// IsSystemAction checks if a step is a system dependency action
func IsSystemAction(step recipe.Step) bool
```

**Consumers:**
- `cmd/tsuku/deps.go` - CLI command
- `internal/sandbox/packages.go` - Container configuration

### Component 2: `tsuku deps` Command

**Location**: `cmd/tsuku/deps.go`

**Interface**:
```
tsuku deps [--system] [--family <family>] [--format <text|json>] <recipe>
```

**Flags:**
- `--system`: Filter to system dependency actions only
- `--family`: Target Linux family (alpine, debian, rhel, arch, suse)
- `--format`: Output format (default: text)
- `--recipe`: Path to local recipe file

**Data flow:**

```
Recipe (curl.toml)
    │
    ├─ dependencies = ["openssl", "zlib"]
    │
    ▼
tsuku deps --system --family alpine curl
    │
    ├─ ResolveTransitive() → [curl, openssl, zlib]
    │
    ├─ For each recipe:
    │     ├─ FilterStepsByTarget(steps, alpine/musl)
    │     └─ ExtractSystemPackagesFromSteps()
    │
    ├─ Deduplicate packages
    │     └─ ["openssl-dev", "zlib-dev"]
    │
    ▼
Output:
openssl-dev
zlib-dev
```

### Component 3: Workflow Helper Script

**Location**: `.github/scripts/install-recipe-deps.sh`

Encapsulates the `tsuku deps` pattern for all five families:

```bash
#!/bin/bash
set -e

FAMILY="${1:?Family required}"
RECIPE="${2:?Recipe required}"
TSUKU="${3:-./tsuku}"

DEPS=$("$TSUKU" deps --system --family "$FAMILY" "$RECIPE")
[ -z "$DEPS" ] && exit 0

case "$FAMILY" in
  alpine)  apk add --no-cache $DEPS ;;
  debian)  apt-get install -y --no-install-recommends $DEPS ;;
  rhel)    dnf install -y --setopt=install_weak_deps=False $DEPS ;;
  arch)    pacman -S --noconfirm $DEPS ;;
  suse)    zypper -n install $DEPS ;;
esac
```

### Workflow Integration

**integration-tests.yml** (library-dlopen-musl):
```yaml
- name: Install recipe-declared dependencies
  run: ./.github/scripts/install-recipe-deps.sh alpine ${{ matrix.library }}
```

**Other workflows to update:**
- `validate-golden-execution.yml` - Replace hardcoded Alpine packages
- `platform-integration.yml` - Replace hardcoded zlib-dev, yaml-dev, libgcc

## Implementation Approach

### Phase 1: Shared Extraction Library

**Files created:**
- `internal/executor/system_deps.go` (~80 LOC)
- `internal/executor/system_deps_test.go` (~100 LOC)

**Files modified:**
- `internal/sandbox/packages.go` - Use shared extraction

### Phase 2: Update `tsuku deps` Command

**Files modified:**
- `cmd/tsuku/deps.go` - Add recursive extraction, use shared library
- `cmd/tsuku/deps_test.go` - Add transitive tests

### Phase 3: Workflow Helper Script

**Files created:**
- `.github/scripts/install-recipe-deps.sh` (~30 LOC)

**Files modified:**
- `.github/workflows/integration-tests.yml` - Use helper script
- `.github/workflows/validate-golden-execution.yml` - Use helper script
- `.github/workflows/platform-integration.yml` - Use helper script

## Security Considerations

### Download Verification

Not applicable. The `deps` command only extracts package names from recipes—it doesn't download anything. Package installation happens via system package managers with their own verification.

### Execution Isolation

The `tsuku deps` command is read-only. It loads recipes, filters steps, and outputs text. No execution, no privilege escalation, no system modification.

### Supply Chain Risks

Package installation happens via `apk add`/`apt install`/etc., not via tsuku. System package managers have GPG-signed packages. This design doesn't change the trust model.

### User Data Exposure

No data access or transmission. The command reads recipe files and outputs package names.

## Consequences

### Positive

- **Complete validation**: Recursive extraction catches under-declaration at any level
- **Code reuse**: ~66 LOC saved by sharing extraction with sandbox
- **Multi-family**: All five Linux families supported consistently
- **Composable**: Works with any CI system via shell scripting
- **Debuggable**: Users can run `tsuku deps` locally to see complete package requirements

### Negative

- **Implementation complexity**: Recursive resolution adds ~55 LOC over simple extraction
- **Workflow updates needed**: Multiple workflows need migration to new pattern
- **Bootstrap dependency**: Need Go installed to build tsuku before using `deps`

### Resolved Uncertainties

- **Transitive deps**: Recursive by default ensures complete coverage
- **Code duplication**: Shared library eliminates it
- **Multi-family**: All families work via shared helper script
- **UX**: Separate command is the right design for scripting use case

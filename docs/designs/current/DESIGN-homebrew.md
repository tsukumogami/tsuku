---
status: Current
problem: Homebrew is the dominant package manager for developer tools on macOS with 6,000+ formulas, but requires a self-contained, reliable integration to provide tsuku users access without system dependencies.
decision: Implement a `homebrew` action that downloads and installs pre-built Homebrew bottles with platform-specific binary patching and relocation, plus a HomebrewBuilder for deterministic recipe generation with LLM fallback.
rationale: Research shows 99.94% of Homebrew formulas have bottles available, making them the only viable production option. This design uses deterministic inspection (~85-90% success) first for cost efficiency, with LLM analysis and repair loops for edge cases, enabling automatic recipe generation for complex tool ecosystems.
---

# Design: Homebrew Integration

## Status

Current

## Context and Problem Statement

Homebrew is the dominant package manager for developer tools on macOS, with over 6,000 formulas in homebrew-core. Many valuable developer tools are distributed exclusively through Homebrew, making it an essential source for any comprehensive package manager.

Research into Homebrew's actual behavior revealed key findings:

1. **99.94% of Homebrew formulas have bottles** (8,096 out of 8,101)
2. **The 5 formulas without bottles** are internal bootstrapping formulas (`portable-libffi`, `portable-libxcrypt`, `portable-libyaml`, `portable-openssl`, `portable-zlib`) that users should never install
3. **Homebrew's default behavior** no longer falls back to source builds - it fails with "no bottle available!" and requires explicit `--build-from-source`
4. **The "source fallback" scenario is rare and transient** - mainly affects `brew upgrade` immediately after a version bump, before bottles are built

This means bottles are the only viable option for production recipes. Tsuku's Homebrew integration focuses exclusively on bottle installation through the `homebrew` action.

### Why Tsuku Cannot Provide Everything

Some dependencies fundamentally cannot be relocated or installed without system privileges. Tsuku uses the `require_system` action for these unprovisionable dependencies.

## Decision Drivers

1. **User demand for Homebrew tools**: Many developer tools are distributed exclusively through Homebrew, making it essential for comprehensive coverage
2. **Self-contained installation**: Tsuku's core promise is installing tools without sudo or system dependencies
3. **Cross-platform support**: Solutions must work on both macOS and Linux
4. **Reliability over flexibility**: 99.94% of formulas have bottles; focusing on this path maximizes success rate
5. **Recipe authoring cost**: Manual recipe creation is time-consuming; automation enables scaling to thousands of tools
6. **Cost efficiency**: LLM-based generation has per-recipe costs; deterministic approaches should handle the common case

## Considered Options

### Option 1: Source Build Integration

Build formulas from source using Homebrew's Ruby DSL and build instructions.

**Pros:**
- Works for all formulas, including the 0.06% without bottles
- Matches Homebrew's exact build process

**Cons:**
- Requires Ruby interpreter and Homebrew infrastructure
- Build times range from seconds to hours
- System dependencies often required (compilers, SDKs)
- Non-deterministic: different machines may produce different results
- Contradicts tsuku's "no system dependencies" principle

### Option 2: Wrap Homebrew CLI

Shell out to `brew install` and copy installed files to tsuku's directory.

**Pros:**
- Simple implementation
- Leverages Homebrew's mature dependency resolution

**Cons:**
- Requires Homebrew to be installed (circular dependency)
- Cannot relocate binaries with hardcoded paths
- Homebrew's global state conflicts with tsuku's version isolation
- Not available on Linux without Linuxbrew setup

### Option 3: Bottle-Only with Direct GHCR Access (Selected)

Download pre-built bottles directly from GitHub Container Registry, applying binary relocation patches.

**Pros:**
- No Homebrew installation required
- Sub-second downloads vs minutes for source builds
- Deterministic: same bottle on all machines
- Bottles are built by Homebrew's trusted CI
- Works on both macOS and Linux

**Cons:**
- Cannot support the 5 formulas without bottles (internal bootstrapping formulas)
- Requires binary patching for relocation
- Depends on Homebrew's bottle infrastructure availability

### Option 4: LLM-Only Recipe Generation

Use LLM analysis for all recipe generation from Homebrew formulas.

**Pros:**
- Handles complex edge cases well
- Can interpret build instructions and infer behavior

**Cons:**
- Per-recipe cost (~$0.02-0.10)
- Slower than deterministic inspection
- Overkill for simple formulas with obvious structure

## Decision Outcome

**Selected: Option 3 (Bottle-Only) with deterministic-first recipe generation and LLM fallback**

This hybrid approach combines:
- Direct GHCR bottle access for installation
- Deterministic bottle inspection for ~85-90% of recipe generation
- LLM fallback with repair loops for complex formulas

The decision prioritizes reliability and user experience. The 5 formulas without bottles are internal Homebrew bootstrapping tools that users should never install directly. For the remaining 99.94%, bottles provide fast, verified, pre-built binaries.

Recipe generation uses deterministic inspection first because:
- Simple formulas (single binary, obvious verify command) don't need LLM analysis
- Cost savings compound when generating thousands of recipes
- LLM fallback ensures complex formulas still get handled

## Solution Architecture

### The `homebrew` Action

The `homebrew` action (renamed from `homebrew_bottle` in issue #580) downloads and installs pre-built Homebrew bottles from GitHub Container Registry (GHCR).

**Key capabilities:**
1. **GHCR authentication**: Anonymous token acquisition
2. **Manifest query**: Platform-specific blob SHA extraction
3. **Download with verification**: SHA256 from manifest annotations
4. **Placeholder relocation**: `@@HOMEBREW_PREFIX@@`, `@@HOMEBREW_CELLAR@@` replacement
5. **Binary patching**: RPATH fixup via `patchelf` (Linux) or `install_name_tool` (macOS)

**Platform tag mapping:**
- `darwin/arm64` → `arm64_sonoma`
- `darwin/amd64` → `sonoma`
- `linux/arm64` → `arm64_linux`
- `linux/amd64` → `x86_64_linux`

**Example recipe:**
```toml
# jq.toml
[metadata]
name = "jq"
description = "Lightweight and flexible command-line JSON processor"

[version]
provider = "homebrew:jq"

[[steps]]
action = "homebrew"
formula = "jq"

[[steps]]
action = "install_binaries"
binaries = ["bin/jq"]

[verify]
command = "jq --version"
```

### Platform Support

Generated recipes are **platform-agnostic**. The `homebrew` action handles platform detection at runtime:

| Host Platform | Bottle Tag |
|---------------|------------|
| macOS ARM64 | `arm64_sonoma` (or latest available) |
| macOS x86_64 | `sonoma` (or latest available) |
| Linux ARM64 | `arm64_linux` |
| Linux x86_64 | `x86_64_linux` |

This enables:
- **Local testing on Linux** - Validate recipes in containers before CI
- **macOS CI validation** - GitHub Actions runners test macOS-specific behavior
- **Single recipe per formula** - No platform-specific recipe variants needed

### HomebrewBuilder (Deterministic + LLM Recipe Generation)

The HomebrewBuilder generates tsuku recipes from Homebrew formulas, eliminating manual recipe authoring for Homebrew-only tools. It uses a deterministic-first approach for speed and cost efficiency, falling back to LLM analysis only when needed.

**Implementation milestone:** [Homebrew Builder](https://github.com/tsukumogami/tsuku/milestone/17)

**Architecture:**

```
User: tsuku create mylib --from homebrew:libyaml
                    │
                    ▼
            ┌───────────────┐
            │  CanBuild()   │
            │               │
            │ Query formula │
            │ JSON API      │
            └───────┬───────┘
                    │ Formula exists, has bottles
                    ▼
            ┌───────────────────────┐
            │ generateDeterministic │
            │                       │
            │ Inspect bottle:       │
            │ - Download bottle     │
            │ - List executables    │
            │ - Infer verify cmd    │
            └───────┬───────────────┘
                    │
                    ▼
            ┌───────────────┐
            │  Validate()   │
            │               │
            │ Container run │
            │ with bottle   │
            └───────┬───────┘
                    │
            ┌───────┴───────┐
            ▼               ▼
        [PASS]          [FAIL]
            │               │
            │               ▼
            │       ┌───────────────┐
            │       │ Start LLM     │
            │       │               │
            │       │ LLM convo:    │
            │       │ - Formula JSON│
            │       │ - Failure ctx │
            │       └───────┬───────┘
            │               │
            │       ┌───────┴───────────┐
            │       ▼                   ▼
            │  ┌─────────────┐   ┌─────────────┐
            │  │fetch_formula│   │inspect_bottle│
            │  └─────┬───────┘   └─────┬───────┘
            │        └───────┬───────────┘
            │                ▼
            │        ┌───────────────┐
            │        │extract_recipe │
            │        └───────┬───────┘
            │                │
            │                ▼
            │        ┌───────────────┐
            │        │  Validate()   │
            │        └───────┬───────┘
            │                │
            │        ┌───────┴───────┐
            │        ▼               ▼
            │    [PASS]          [FAIL]
            │        │               │
            │        │       ┌───────┴───────┐
            │        │       │ Repair Loop   │
            │        │       │ (max 2)       │
            │        │       └───────┬───────┘
            │        │               │
            └────────┴───────────────┘
                    ▼
            ┌───────────────┐
            │ Return Result │
            │               │
            │ Recipe +      │
            │ Warnings +    │
            │ Cost          │
            └───────────────┘
```

**Generation approaches:**

1. **Deterministic (fast path):**
   - Inspects bottle contents directly
   - Infers executables and verify command
   - No LLM cost, ~1-2 second generation
   - Success rate: ~85-90% for simple formulas

2. **LLM fallback (when deterministic fails validation):**
   - LLM analyzes formula metadata
   - Uses tools: `fetch_formula_json`, `inspect_bottle`, `extract_recipe`
   - Costs ~$0.02-0.10 per recipe
   - Success rate: ~75-85% with repair loop (max 2 attempts)

**Container validation:**
All generated recipes are validated in isolated containers with `--network=none` and resource limits (2GB RAM, 2 CPU, 5min timeout)

### Dependency Discovery

Before generating recipes, the builder traverses the dependency tree using Homebrew's JSON API. This provides full visibility of what needs to be generated and estimated costs.

**User confirmation flow:**

```
$ tsuku create neovim --from homebrew:neovim

Discovering dependencies...

Dependency tree for neovim:
  neovim (needs recipe)
  ├── gettext (needs recipe)
  ├── libuv (has recipe ✓)
  ├── lpeg (needs recipe)
  │   └── lua (needs recipe)
  ├── luajit (needs recipe)
  ├── luv (needs recipe)
  │   ├── libuv (has recipe ✓)
  │   └── luajit (needs recipe) [duplicate]
  ├── msgpack (needs recipe)
  ├── tree-sitter (needs recipe)
  └── unibilium (needs recipe)

Recipes to generate: 8
Estimated cost: ~$0.45

Proceed? [y/N]
```

Recipes are generated in topological order (dependencies first) to ensure deps are ready when parent is validated.

### Alternative: Manual Recipe Authoring

For formulas without bottles or complex source builds, tsuku provides build system primitives for manual recipe authoring:
- `configure_make` - Autotools builds
- `cmake_build` - CMake projects
- `meson_build` - Meson build system
- `apply_patch` - Patch application

These actions can be composed in manually authored recipes for edge cases not covered by HomebrewBuilder's bottle-only approach.

## Implementation Approach

### Phase 1: Core `homebrew` Action (M15)

1. **GHCR client implementation**
   - Anonymous token acquisition from `ghcr.io/token`
   - Manifest fetching with platform-specific tag resolution
   - Blob download with SHA256 verification from annotations

2. **Bottle extraction and relocation**
   - Tar/gzip extraction to tool directory
   - Placeholder replacement: `@@HOMEBREW_PREFIX@@` → `$TSUKU_HOME/tools/<name>-<version>`
   - Text file scanning for path references

3. **Binary patching**
   - Linux: `patchelf --set-rpath` for ELF binaries
   - macOS: `install_name_tool -change` for Mach-O binaries
   - Dependency resolution for bundled libraries

### Phase 2: HomebrewBuilder (M17)

1. **Deterministic generator**
   - Download and inspect bottle contents
   - Identify executables in `bin/` directory
   - Infer verify command from binary name or `--version` pattern
   - Generate recipe without LLM involvement

2. **Container validation harness**
   - Docker-based validation with `--network=none`
   - Resource limits: 2GB RAM, 2 CPU, 5 minute timeout
   - Pre-downloaded bottle mounted read-only
   - Exit code and output verification

3. **LLM fallback pipeline**
   - Tool definitions: `fetch_formula_json`, `inspect_bottle`, `extract_recipe`
   - Context assembly: formula JSON, dependency tree, failure messages
   - Repair loop: up to 2 attempts with validation feedback

### Phase 3: Dependency Resolution (M18)

1. **Dependency tree traversal**
   - Query Homebrew JSON API for formula dependencies
   - Build directed graph of transitive dependencies
   - Detect existing tsuku recipes to avoid regeneration

2. **Topological generation**
   - Generate recipes in dependency order
   - Validate each before proceeding to dependents
   - Aggregate costs and warnings for user confirmation

3. **Batch operations**
   - `tsuku create --from homebrew:<formula> --batch` for automated pipelines
   - Cost estimation before execution
   - Partial success handling with clear status reporting

## Security Considerations

### Download Verification

Bottles are verified through multiple mechanisms:

1. **SHA256 from GHCR manifest**: Each bottle blob's SHA256 is recorded in the OCI manifest annotations. The `homebrew` action verifies downloaded content matches this hash before extraction.

2. **Manifest signature**: GHCR manifests are signed by Homebrew's CI infrastructure. While tsuku doesn't independently verify signatures (that would require Homebrew's signing keys), the SHA256 chain ensures content integrity.

3. **HTTPS transport**: All downloads use HTTPS to prevent man-in-the-middle attacks during transit.

### Execution Isolation

The `homebrew` action operates with minimal privileges:

1. **No sudo required**: All files are written to `$TSUKU_HOME` (default `~/.tsuku`), which is user-writable
2. **No network at runtime**: Installed tools don't require network access for basic operation
3. **User-level PATH**: Binaries are symlinked to `$TSUKU_HOME/bin`, which users add to their PATH voluntarily

**HomebrewBuilder validation** runs in isolated containers:
- `--network=none`: No network access during validation
- Resource limits: 2GB RAM, 2 CPU cores, 5 minute timeout
- Read-only bottle mount: Validation cannot modify source artifacts
- Ephemeral containers: Destroyed after each validation run

### Supply Chain Risks

**Trusted sources:**
- Bottles originate from Homebrew's official CI (`ghcr.io/homebrew/core/*`)
- Homebrew-core formulas are reviewed by maintainers before merge
- CI builds are reproducible and auditable

**Mitigation measures:**
1. **URL allowlist**: Generated recipes can only reference `ghcr.io/homebrew/core/*` and `formulae.brew.sh/*`
2. **homebrew-core only**: Third-party taps require explicit opt-in with security warnings
3. **No arbitrary shell**: LLM-generated recipes have limited schema; no arbitrary command execution beyond verify commands
4. **Schema enforcement**: LLM output cannot include checksums (obtained from GHCR) or arbitrary URLs (constructed programmatically)

**Residual risks:**
- Compromise of Homebrew's CI infrastructure would affect all bottles
- Homebrew maintainer accounts could be compromised
- These risks are inherent to using pre-built binaries and apply equally to direct Homebrew usage

### User Data Exposure

**Data accessed:**
- `$TSUKU_HOME/state.json`: Installation state (tool names, versions, timestamps)
- Recipe files: TOML definitions downloaded from tsuku registry

**Data transmitted:**
- Anonymous telemetry (if enabled): Install counts, tool names, platform
- GHCR API requests: Formula names, platform tags (no user identification)

**Data NOT transmitted:**
- File contents from user's system
- Environment variables or credentials
- Usage patterns of installed tools

## Consequences

### Positive

1. **Massive tool catalog**: Access to 8,000+ Homebrew formulas without manual recipe authoring
2. **Fast installation**: Bottle downloads complete in seconds vs minutes for source builds
3. **Cross-platform consistency**: Same recipe works on macOS and Linux
4. **No Homebrew dependency**: Users don't need Homebrew installed to use Homebrew-sourced tools
5. **Automated recipe generation**: HomebrewBuilder reduces recipe authoring from hours to seconds
6. **Cost-efficient generation**: Deterministic-first approach minimizes LLM costs

### Negative

1. **Binary patching complexity**: RPATH and install_name_tool modifications can fail for unusual binaries
2. **Platform tag churn**: Homebrew's macOS version tags (sonoma, ventura) require ongoing maintenance
3. **GHCR dependency**: Homebrew's bottle hosting infrastructure must remain available
4. **Relocation limitations**: Some tools hardcode absolute paths that cannot be patched
5. **No source fallback**: The 0.06% of formulas without bottles cannot be supported

### Neutral

1. **Manual recipes still needed**: Complex tools with unprovisionable dependencies require hand-authored recipes
2. **Version lag**: New Homebrew releases appear before bottles are built; tsuku users may experience brief delays
3. **Dependency duplication**: Homebrew's shared dependencies become per-tool copies in tsuku's isolated model

## References

- **Action documentation**: `docs/GUIDE-actions-and-primitives.md`
- **Dependency provisioning**: `docs/DESIGN-dependency-provisioning.md`
- **Relocatable libraries**: `docs/DESIGN-relocatable-library-deps.md`
- **Homebrew documentation**: https://docs.brew.sh/Formula-Cookbook
- **Homebrew JSON API**: https://formulae.brew.sh/docs/api/
- **Homebrew Bottles Documentation**: https://docs.brew.sh/Bottles
- **Homebrew Discussion #305**: Source fallback behavior change

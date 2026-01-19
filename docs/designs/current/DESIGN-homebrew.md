---
status: Current
problem: Homebrew is the dominant package manager for developer tools on macOS with 6,000+ formulas, but requires a self-contained, reliable integration to provide tsuku users access without system dependencies.
decision: Implement a `homebrew` action that downloads and installs pre-built Homebrew bottles with platform-specific binary patching and relocation, plus a HomebrewBuilder for deterministic recipe generation with LLM fallback.
rationale: Research shows 99.94% of Homebrew formulas have bottles available, making them the only viable production option. This design uses deterministic inspection (~85-90% success) first for cost efficiency, with LLM analysis and repair loops for edge cases, enabling automatic recipe generation for complex tool ecosystems.
---

# Design: Homebrew Integration

**Status**: Current

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

## Homebrew Integration Architecture

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

**Implementation milestone:** [M17: Homebrew Builder](https://github.com/tsukumogami/tsuku/milestone/17)

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

### Security Considerations

**Trusted Binary Sources:**
- Bottles use pre-built binaries from Homebrew's trusted CI
- SHA256 verification via GHCR manifest annotations
- Bottles signed by Homebrew infrastructure
- No build logic executed

**URL Allowlist:**
Generated recipes can only reference:
- `ghcr.io/homebrew/core/*` - Homebrew bottles
- `formulae.brew.sh/*` - Homebrew API

**Schema Enforcement:**
LLM output schema does NOT include:
- Checksums (obtained from GHCR at runtime)
- Arbitrary URLs (constructed programmatically)
- Shell commands beyond verification

**Container Validation:**
All generated recipes are validated in isolated containers:
- `--network=none` - No network access
- Resource limits (2GB RAM, 2 CPU, 5min timeout)
- Pre-downloaded bottles mounted read-only
- Verification command executed in isolation

**homebrew-core Only:**
MVP restricts to official homebrew-core tap:
- Vetted by Homebrew maintainers
- Required CI checks before merge
- Open source, auditable

Third-party tap support requires explicit opt-in with security warnings.

## Alternative: Manual Recipe Authoring

For formulas without bottles or complex source builds, tsuku provides build system primitives for manual recipe authoring:
- `configure_make` - Autotools builds
- `cmake_build` - CMake projects
- `meson_build` - Meson build system
- `apply_patch` - Patch application

These actions can be composed in manually authored recipes for edge cases not covered by HomebrewBuilder's bottle-only approach.

## References

- **Action documentation**: `docs/GUIDE-actions-and-primitives.md`
- **Dependency provisioning**: `docs/DESIGN-dependency-provisioning.md`
- **Relocatable libraries**: `docs/DESIGN-relocatable-library-deps.md`
- **Homebrew documentation**: https://docs.brew.sh/Formula-Cookbook
- **Homebrew JSON API**: https://formulae.brew.sh/docs/api/
- **Homebrew Bottles Documentation**: https://docs.brew.sh/Bottles
- **Homebrew Discussion #305**: Source fallback behavior change

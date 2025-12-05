# Design Document: Ecosystem-Specific Recipe Builders

**Status**: Planned

## Context and Problem Statement

Tsuku includes embedded recipes at `internal/recipe/recipes/`. This works well for the tools in the registry, but creates a coverage gap: thousands of CLI tools exist across package ecosystems (crates.io, RubyGems, PyPI, npm) that are not included.

Users who need these tools must either wait for someone to write a recipe or write one themselves. This friction prevents tsuku from being a complete solution for developer tooling.

### The Opportunity

Each major package ecosystem exposes metadata APIs that contain enough information to generate valid tsuku recipes automatically:

- **crates.io**: Crate metadata, repository links for Cargo.toml parsing
- **RubyGems**: Gem metadata including `executables` field
- **PyPI**: Package metadata with `console_scripts` entry points
- **npm**: Package.json with `bin` field

By implementing "builders" that understand these ecosystems, tsuku can generate recipes on-demand for any package in these registries.

### Why Now

1. **Actions exist**: `cargo_install`, `gem_install`, `pipx_install`, `npm_install` are implemented
2. **Version providers exist**: crates.io, RubyGems, PyPI, npm providers can resolve versions
3. **Registry architecture is stable**: The external registry model is established; builders complement this as a fallback
4. **The pattern is clear**: We have enough experience with different ecosystems to see what's deterministic vs. what needs heuristics

### Scope

**In scope:**
- Define the Builder interface that all ecosystem builders implement
- Implement builders for: Cargo (crates.io), Gem (RubyGems), PyPI, npm
- Local recipe storage for builder-generated recipes
- Integration with existing actions and version providers
- `tsuku create` command for explicit recipe generation

**Out of scope:**
- AI/LLM orchestration for automatic ecosystem selection (future Layer 1)
- Homebrew builder (requires complex Ruby DSL interpretation)
- Nix builder (medium complexity, defer to later iteration)
- GitHub releases builder (requires heuristics for asset pattern detection)
- Automatic registry contribution workflow

### Success Criteria

- Majority of CLI packages in target ecosystems can generate valid recipes (executable discovery works)
- Generated recipes pass verification on first attempt for well-behaved packages
- Clear error messages when generation fails with actionable next steps
- User can inspect and edit generated recipe before installation
- Generated recipes use version-agnostic format (suitable for registry contribution)

## Decision Drivers

- **Determinism first**: Start with ecosystems where API metadata maps cleanly to recipes, proving the pattern before adding complexity
- **Leverage existing infrastructure**: Builders should generate recipes that use existing actions (`cargo_install`, `gem_install`, etc.) and version providers
- **Consistent interface**: All builders must implement the same interface, regardless of internal complexity
- **Extensibility**: The framework must support adding LLM capabilities to individual builders without changing the interface
- **Security**: Generated recipes must be validated before execution; builders from trusted registries only
- **Transparency**: Users should be able to inspect generated recipes before installation

## External Research

### asdf / mise Plugin Architecture

**Approach**: Both asdf and mise use a plugin-per-tool model where each plugin is a Git repository containing shell scripts (`bin/install`, `bin/list-all`, etc.). When installing a tool, the system clones the plugin repo and executes these scripts.

**How it works**:
- Plugins receive environment variables: `ASDF_INSTALL_VERSION`, `ASDF_INSTALL_PATH`
- Each plugin is responsible for downloading, extracting, and placing binaries
- Modern mise also supports "backends" (aqua, ubi, cargo, npm) that bypass the plugin system entirely

**Trade-offs**:
- Pro: Extremely flexible - plugins can do anything
- Pro: Community can contribute plugins without core changes
- Con: No consistency - each plugin implements logic differently
- Con: Shell scripts are fragile and hard to validate for security
- Con: Requires cloning a Git repo before first use

**Relevance to tsuku**: The plugin model is too unstructured for tsuku's goals. However, mise's newer "backend" concept (where cargo/npm are first-class citizens with standardized behavior) aligns well with tsuku's builder concept.

**Sources**: [asdf Plugin Creation](https://asdf-vm.com/plugins/create.html), [mise Dev Tools](https://mise.jdx.dev/dev-tools/)

### Nix/Nixpkgs Derivation System

**Approach**: Nix uses a functional language to describe package builds. Each package is a "derivation" that declares inputs, build steps, and outputs. Language-specific builders (buildRustPackage, buildPythonPackage, etc.) encode ecosystem knowledge.

**How it works**:
- Derivations are deterministic: same inputs always produce same outputs
- Build isolation via sandboxing (no network during build, controlled inputs)
- Each language ecosystem has specialized builders that know how to invoke cargo, pip, etc.

**Trade-offs**:
- Pro: Extreme reproducibility and isolation
- Pro: Language-specific builders encode ecosystem knowledge
- Con: Nix language has steep learning curve
- Con: Derivations must be written manually (no auto-generation)
- Con: Source builds are slow without binary cache

**Relevance to tsuku**: Nix's language-specific builders validate the concept of ecosystem-specific builders. Tsuku doesn't need Nix's reproducibility guarantees, but the builder pattern applies.

**Sources**: [Nixpkgs Manual](https://nixos.org/nixpkgs/manual/)

### cargo-binstall

**Approach**: cargo-binstall downloads pre-built binaries for Rust crates instead of compiling from source. It uses crate metadata to locate release artifacts.

**How it works**:
1. Fetch crate info from crates.io
2. Check `[package.metadata.binstall]` in Cargo.toml for custom URL patterns
3. If not specified, use defaults: `{repo}/releases/download/v{version}/{name}-{target}.{format}`
4. Fall back to QuickInstall (third-party binary cache), then cargo install

**Trade-offs**:
- Pro: Zero-config for projects following conventions
- Pro: Graceful fallback chain (metadata -> conventions -> compilation)
- Con: Relies on maintainers publishing binaries
- Con: Template system is specific to GitHub releases pattern

**Relevance to tsuku**: cargo-binstall demonstrates that Rust ecosystem can work with pre-built binaries when available. The fallback chain pattern (try metadata, then conventions, then compile) is worth considering.

**Sources**: [cargo-binstall README](https://github.com/cargo-bins/cargo-binstall)

### pipx Entry Point Discovery

**Approach**: pipx installs Python CLI tools in isolated virtual environments and discovers executables via package entry points.

**How it works**:
1. Create isolated venv at `~/.local/share/pipx/venvs/{package}/`
2. `pip install` the package into that venv
3. Discover console_scripts from package metadata (pyproject.toml `[project.scripts]`)
4. Create symlinks in `~/.local/bin/` for each entry point

**Trade-offs**:
- Pro: Entry points are standardized - packages declare their CLIs
- Pro: Perfect isolation between tools
- Con: Not all packages define entry points correctly
- Con: Requires Python runtime per installation

**Relevance to tsuku**: PyPI entry points (`console_scripts`) provide a reliable way to discover which executables a package provides. Tsuku's pipx_install already uses this pattern; a builder can query PyPI metadata to pre-populate the executables list.

**Sources**: [How pipx works](https://pipx.pypa.io/stable/how-pipx-works/), [Entry Points Specification](https://packaging.python.org/en/latest/specifications/entry-points/)

### Research Summary

**Common patterns:**
1. **Ecosystem-specific knowledge is essential**: Every system (Nix, cargo-binstall, pipx) encodes ecosystem-specific logic rather than trying to be generic
2. **Metadata APIs are the key**: crates.io, PyPI, RubyGems all provide JSON APIs with version lists, dependencies, and (sometimes) executable info
3. **Convention over configuration**: cargo-binstall's default URL patterns work for most projects; exceptions can override
4. **Fallback chains**: cargo-binstall tries multiple sources before giving up

**Key differences:**
- asdf/mise: Shell scripts, maximum flexibility, minimum consistency
- Nix: Functional language, maximum reproducibility, steep learning curve
- cargo-binstall: Single ecosystem, binary-first with compile fallback
- pipx: Single ecosystem, always installs from source (wheels are pre-compiled but still "installed")

**Implications for tsuku:**
1. **Builder interface should be narrow**: Builders take a package name and return a recipe - nothing more
2. **Leverage existing version providers**: Don't duplicate API logic - use `CratesIOProvider`, `RubyGemsProvider`, etc.
3. **Leverage existing actions**: Builders generate recipes that use `cargo_install`, `gem_install`, etc.
4. **Start with high-determinism ecosystems**: crates.io and RubyGems have clean APIs; PyPI and npm require more heuristics

## Considered Options

### Option 1: Thin Builder Layer (Recipe Generation Only)

Builders are minimal adapters that query ecosystem APIs and generate recipe structs. They have no knowledge of execution - they simply return a `*recipe.Recipe` that the existing executor handles.

```go
type Builder interface {
    // Name returns the builder identifier (e.g., "crates_io", "rubygems")
    Name() string

    // CanBuild checks if this builder can handle the package
    CanBuild(ctx context.Context, packageName string) (bool, error)

    // Build generates a recipe for the package
    Build(ctx context.Context, packageName string, version string) (*BuildResult, error)
}

type BuildResult struct {
    Recipe   *recipe.Recipe
    Warnings []string  // Human-readable messages about generation uncertainty
    Source   string    // Where the metadata came from (e.g., "crates.io:ripgrep")
}
```

**Example flow:**
```
User: tsuku create ripgrep --from crates.io

1. CargoBuilder.CanBuild("ripgrep") -> true
2. CargoBuilder.Build("ripgrep", ""):
   a. Query crates.io API for metadata
   b. Get version from CratesIOProvider
   c. Construct Recipe{steps: [{action: "cargo_install", crate: "ripgrep", ...}]}
3. Return recipe, write to ~/.tsuku/recipes/ripgrep.toml
4. User runs `tsuku install ripgrep`, executes via normal path
```

**Pros:**
- Simple interface - builders do one thing
- Leverages existing infrastructure completely (actions, version providers, executor)
- Easy to test - builders return data, not side effects
- Clear separation of concerns
- Generated recipes can be inspected, edited, cached, shared

**Cons:**
- Two-step UX for users (`create` then `install`) unless we auto-chain
- Builder has no visibility into execution failures - no feedback loop if generated recipe fails
- Cached recipes may become stale as upstream packages change
- Some ecosystems may need execution-time decisions

### Option 2: Builder as Extended Action

Builders are a new type of action that can generate recipes internally and execute them. They're invoked like actions but have special privileges to create recipe steps dynamically.

```go
type BuilderAction interface {
    Action

    // CanHandle returns true if this builder can handle the package
    CanHandle(ctx context.Context, packageName string) bool

    // Execute both generates and runs the installation
    Execute(ctx *ExecutionContext, params map[string]interface{}) error
}
```

**Example flow:**
```
User: tsuku install ripgrep --from crates.io

1. No recipe found in registry
2. CargoBuilderAction.CanHandle("ripgrep") -> true
3. CargoBuilderAction.Execute():
   a. Query crates.io API
   b. Generate internal recipe
   c. Execute cargo_install action
   d. Cache recipe for future use
```

**Pros:**
- Single command UX (`install` works for everything)
- Builder can react to execution failures and retry with different params
- Runtime context awareness - can check system resources and adjust recipe
- Consistent with existing action pattern

**Cons:**
- Conflates generation and execution - harder to test
- Less transparent - user doesn't see recipe before it runs
- Actions are not supposed to generate other actions
- Harder to cache/share recipes if generation is implicit

### Option 3: External Builders as CLI Subcommands

Instead of Go interfaces, builders could be separate executables (`tsuku-build-cargo`) that tsuku discovers and invokes. This enables community contributions in any language.

**Pros:**
- Community can contribute builders in Python, Ruby, etc.
- Decoupled release cycle for builders
- Language-appropriate tooling for each ecosystem
- Security isolation - external builders can't corrupt tsuku state

**Cons:**
- Violates tsuku's self-contained philosophy (single binary, no dependencies)
- Shelling out adds complexity and security surface for IPC
- Harder to test and debug as integrated system
- No shared infrastructure (HTTP clients, validation, rate limiting)

**Note**: While tsuku already shells out to package managers (cargo, gem, npm) for *installation*, those are trusted tools the user has installed. External *builders* would be a new dependency category that must be acquired and trusted separately, which is a different concern than runtime package manager execution.

### Evaluation Against Decision Drivers

| Driver | Option 1 (Thin) | Option 2 (Extended Action) | Option 3 (External) |
|--------|-----------------|---------------------------|---------------------|
| Determinism first | Good - generation is pure | Fair - execution mixed in | Fair - depends on impl |
| Leverage existing infra | Good - reuses everything | Fair - new action type | Poor - no sharing |
| Consistent interface | Good - simple interface | Poor - special action type | Poor - varies by builder |
| Extensibility for LLM | Good - just implement interface | Fair - awkward for LLM results | Good - any language |
| Security | Good - validate before execute | Poor - executes immediately | Poor - shells out |
| Transparency | Good - recipe visible | Poor - recipe hidden | Fair - could output recipe |

### Uncertainties

- **Executable discovery accuracy**: We believe crates.io and RubyGems have reliable executable metadata, but we haven't validated PyPI's entry points coverage. Some packages may not declare their CLIs correctly.
- **Performance of API queries**: Each builder will make HTTP requests to registry APIs. Latency impact is unknown; caching strategy may be needed.
- **User expectation management**: If a generated recipe fails, will users understand why? Do we need a "suggest edits" flow?
- **Recipe stability**: If we cache a generated recipe and the package updates, the recipe may become stale.

### Assumptions Requiring Validation

These assumptions should be validated during initial implementation:

1. **Ecosystem APIs provide reliable executable metadata**: crates.io Cargo.toml parsing and RubyGems `executables` field should work for majority of CLI packages. PyPI entry points may be less reliable.

2. **Existing version providers have adequate rate limits**: The version providers already query these APIs; builders will add additional metadata queries. We assume this won't cause rate limiting issues.

3. **Generated recipes are functionally equivalent to hand-written recipes**: The same `Recipe` struct is used; actions don't distinguish between sources. We assume no semantic differences.

4. **Local-only storage is acceptable**: Generated recipes stored in `~/.tsuku/recipes/` without cross-machine sync or team sharing is sufficient for initial implementation.

5. **Toolchains are pre-installed**: Builders generate recipes that require `cargo`, `gem`, `pip`, or `npm`. If these are missing, the recipe will fail at execution time with a clear error.

## Decision Outcome

**Chosen option: Option 1 (Thin Builder Layer)**

Builders are pure functions that take a package name and return a recipe struct. They include warnings for known risks. The recipe can be inspected before execution via `tsuku create`, and the explicit command makes recipe generation intentional rather than automatic.

### Rationale

This option was chosen because:

1. **Determinism first**: Option 1's pure-function approach makes builders easy to test and reason about. Generation is decoupled from execution, so we can validate recipes before running them.

2. **Leverage existing infrastructure**: Builders generate `*recipe.Recipe` structs that use existing actions (`cargo_install`, `gem_install`). No new action types or execution paths needed.

3. **Transparency**: Users can inspect generated recipes with `tsuku create <package> --from <ecosystem>` before committing to installation. The `Warnings` field communicates uncertainty without hiding it.

4. **Security**: Recipe validation happens before execution, allowing security checks on generated recipes. Users explicitly opt into builder usage.

5. **Extensibility**: The interface is simple enough that an LLM-assisted builder (e.g., for Homebrew) would implement the same interface. The `BuildResult.Warnings` field lets builders communicate uncertainty without requiring numeric confidence calibration.

### Alternatives Rejected

- **Option 2 (Builder as Extended Action)**: Conflating generation and execution makes testing harder and hides the recipe from users. Actions generating other actions violates the current action model.

- **Option 3 (External Builders)**: Introduces a new dependency category users must acquire and trust. While the flexibility is appealing, it undermines tsuku's single-binary philosophy and adds IPC complexity.

### Trade-offs Accepted

By choosing this option, we accept:

1. **Two-step UX by default**: Users run `tsuku create` then `tsuku install`. This is intentional for transparency but adds friction.

2. **No automatic learning**: If installation fails, the builder doesn't learn. Users must manually edit recipes if generated executables are wrong.

3. **Recipe staleness**: Cached recipes don't automatically update when upstream packages change. Users must re-run `tsuku create` to refresh.

These are acceptable because:
- The two-step UX ensures users know what they're installing
- Manual editing is infrequent and keeps users in control
- Recipe staleness is the same problem hand-written recipes have

## Solution Architecture

### Overview

Recipe builders are invoked via `tsuku create` to generate recipes that are written to the user's local recipes directory (`~/.tsuku/recipes/`). They are completely decoupled from the install/lookup flow.

```
User: tsuku create bat --from crates.io
         |
         v
+------------------+
| Builder Registry |
+------------------+
| CargoBuilder     |
| GemBuilder       |
| PyPIBuilder      |
| NpmBuilder       |
+------------------+
         |
         v
+------------------+
| BuildResult      |
| - Recipe         |
| - Warnings       |
| - Source         |
+------------------+
         |
         v
~/.tsuku/recipes/bat.toml
(Now available for tsuku install)
```

### Components

**Builder Interface** (`internal/builders/builder.go`):
```go
type Builder interface {
    Name() string
    CanBuild(ctx context.Context, packageName string) (bool, error)
    Build(ctx context.Context, packageName string, version string) (*BuildResult, error)
}

type BuildResult struct {
    Recipe   *recipe.Recipe
    Warnings []string
    Source   string
}
```

**Builder Registry** (`internal/builders/registry.go`):
```go
type Registry struct {
    builders map[string]Builder
}

func (r *Registry) Get(name string) (Builder, bool)
func (r *Registry) Register(b Builder)
```

**Concrete Builders**:

| Builder | Ecosystem | Action Used | Executable Discovery |
|---------|-----------|-------------|---------------------|
| `CargoBuilder` | crates.io | `cargo_install` | Parse Cargo.toml `[[bin]]` sections |
| `GemBuilder` | RubyGems | `gem_install` | `executables` field from API |
| `PyPIBuilder` | PyPI | `pipx_install` | `console_scripts` from metadata |
| `NpmBuilder` | npm | `npm_install` | `bin` field from package.json |

**Builder Constructor Pattern**:

All builders receive dependencies via constructor:
```go
func NewCargoBuilder(resolver *version.Resolver) *CargoBuilder {
    return &CargoBuilder{
        resolver: resolver,
    }
}
```

**Recipe Writer** (`internal/recipe/writer.go`):
```go
type Writer struct {
    recipesDir string
}

func NewWriter(recipesDir string) *Writer
func (w *Writer) Write(name string, recipe *Recipe) error  // Atomic write (temp + rename)
```

### Recipe Storage and Lookup

```
~/.tsuku/recipes/           # User's recipes (highest priority, writable)
    bat.toml                # Created by builder or manually
    my-custom-tool.toml

~/.tsuku/registry/          # Registry cache (read-only, official recipes)
    k/
        kubectl.toml
    t/
        terraform.toml
```

**Lookup order in recipe Loader**:
1. Local recipes (`~/.tsuku/recipes/{name}.toml`)
2. Registry cache (`~/.tsuku/registry/{letter}/{name}.toml`)
3. Remote registry fetch (GitHub raw files)

Builders do NOT participate in lookup. They are invoked explicitly via `tsuku create` to generate recipes that land in the local recipes directory.

### Data Flow

**Creating a recipe:**
```
1. User: tsuku create bat --from crates.io
2. CLI parses: tool="bat", ecosystem="crates.io"
3. Registry.Get("crates_io") -> CargoBuilder
4. CargoBuilder.Build(ctx, "bat", ""):
   a. Fetch metadata from crates.io API
   b. Parse Cargo.toml from repository for [[bin]] names
   c. Construct Recipe struct with cargo_install action
5. Write to ~/.tsuku/recipes/bat.toml
6. Display: "Recipe created. Run: tsuku install bat"
```

**Installing after creation:**
```
1. User: tsuku install bat
2. Loader.Get("bat"):
   a. Check ~/.tsuku/recipes/bat.toml -> found
   b. Return recipe
3. Executor runs recipe (same as any other recipe)
```

### Executable Discovery Strategy

The critical challenge is determining which executables a package provides. Each ecosystem uses a different approach:

**Cargo (crates.io)**:
1. Get repository URL from crates.io API response
2. Construct raw file URL (GitHub: `{repo}/raw/main/Cargo.toml`)
3. Fetch with timeout (10s) and size limit (1MB)
4. Parse `[[bin]]` sections to extract executable names
5. Fallback: Assume crate name == executable name if fetch fails
6. Warning when using fallback

Failure modes handled:
- Repository field empty: Fallback + warning
- Repository is non-GitHub: Fallback + warning (initially)
- HTTP timeout/404: Fallback + warning
- No `[[bin]]` sections: Use crate name

**Gem (RubyGems)**:
1. RubyGems API includes `executables` field
2. Fallback to gem name if empty

**PyPI**:
1. Parse `console_scripts` from package metadata
2. Fallback to package name
3. Higher warning rate expected (PyPI metadata less reliable)

**npm**:
1. Fetch package.json, parse `bin` field
2. Fallback to package name

### Example Generated Recipe

A builder-generated recipe for `ripgrep`:

```toml
# Generated by tsuku CargoBuilder
# Source: crates.io:ripgrep

[metadata]
name = "ripgrep"
description = "ripgrep recursively searches directories for a regex pattern"
homepage = "https://github.com/BurntSushi/ripgrep"

[version]
provider = "crates_io:ripgrep"

[[steps]]
action = "cargo_install"
crate = "ripgrep"
executables = ["rg"]

[verify]
command = "rg --version"
```

Key characteristics:
- **Version-agnostic**: Uses `provider` for runtime resolution, not pinned version
- **Same format as registry recipes**: Can be contributed back without modification
- **Provenance comments**: Documents where recipe came from (comments only)

## Implementation Approach

### Phase 1: Local Recipe Support + Cargo Builder

**Deliverables**:
- `internal/recipe/writer.go` - Recipe serialization to TOML with atomic writes
- Modify `Loader.Get()` to check `~/.tsuku/recipes/` before registry
- `internal/builders/builder.go` - Interface and BuildResult types
- `internal/builders/registry.go` - Builder registry
- `internal/builders/cargo.go` - CargoBuilder implementation (crate name as executable initially)
- `internal/builders/cargo_parser.go` - Cargo.toml `[[bin]]` parsing for executable discovery
- `tsuku create <tool> --from crates.io` command

**Validation**: `tsuku create ripgrep --from crates.io` generates valid recipe, `tsuku install ripgrep` works.

### Phase 2: Gem Builder

**Deliverables**:
- `internal/builders/gem.go` - GemBuilder implementation
- Tests

**Validation**: `tsuku create jekyll --from rubygems` works.

### Phase 3: PyPI and npm Builders

**Deliverables**:
- `internal/builders/pypi.go` - PyPIBuilder
- `internal/builders/npm.go` - NpmBuilder

**Validation**: `tsuku create ruff --from pypi` and `tsuku create prettier --from npm` work.

### Phase 4: UX Polish

**Deliverables**:
- `tsuku recipes` - List local recipes
- Improved error message when tool not found (suggests available ecosystems)
- `--force` flag on `tsuku create` to overwrite existing local recipe

### Phase 5: Toolchain Bootstrapping (Future)

**Deliverables**:
- Detect when cargo/gem/pip/npm is missing
- Suggest installing via tsuku or provide helpful error

## Security Considerations

### Download Verification

**How are downloaded artifacts validated?**

Builders themselves do not download binaries - they only query metadata APIs and generate recipes. The actual downloads happen when recipes are executed via existing actions (`cargo_install`, `gem_install`, etc.).

For metadata API queries:
- **HTTPS only**: All registry APIs (crates.io, RubyGems, PyPI, npm) are accessed over HTTPS
- **Response validation**: JSON responses are validated against expected structure before parsing
- **Size limits**: Response bodies should be limited to prevent memory exhaustion attacks
- **Content-Type verification**: Responses must have `application/json` content type

**What happens if verification fails?**: Build fails with clear error message; no recipe is generated.

### Execution Isolation

**What permissions does this feature require?**

- **Network access**: Required to query registry APIs (crates.io, rubygems.org, pypi.org, registry.npmjs.org, raw.githubusercontent.com for Cargo.toml parsing)
- **File system access**: Write to `~/.tsuku/recipes/` for storing generated recipes; read from existing cache
- **No elevated privileges**: Builders run with normal user permissions; no sudo required
- **No code execution**: Builders do not execute any code from packages; they only read metadata

**Privilege escalation risks**: None. Builders generate TOML data structures. Actual installation (which could involve running package manager commands) happens through existing actions that already have security controls.

### Supply Chain Risks

**Where do artifacts come from?**

Builders query metadata from official package registries:

| Registry | Trust Model |
|----------|-------------|
| crates.io | Rust Foundation operated, packages verified by crate owners |
| rubygems.org | Ruby Central operated, package signing optional |
| pypi.org | Python Software Foundation, package signing optional (PEP 458) |
| registry.npmjs.org | npm, Inc. / GitHub, package signing optional |

**Source authenticity verification**:
- Builders only generate recipes that use official package managers (`cargo install`, `gem install`, `pipx install`, `npm install`)
- These package managers handle artifact verification according to their own security models
- Builders do NOT download arbitrary URLs - they delegate to trusted package managers

**What if upstream is compromised?**

If a package registry is compromised:
1. Builders would generate recipes pointing to malicious packages
2. Users would install compromised packages via standard package managers

This is the same risk as using `cargo install`, `gem install`, etc. directly. Tsuku does not add additional attack surface; it delegates to the same package managers users would use anyway.

**Typosquatting risk**: Users could accidentally request a malicious package with a similar name to a legitimate one. Mitigation: Display package metadata (description, homepage, download count if available) for user review before writing recipe.

### User Data Exposure

**What user data does this feature access or transmit?**

**Data accessed locally**:
- Package name (provided by user)
- Version (provided by user or resolved)
- Existing cached recipes (read-only)

**Data transmitted externally**:
- Package name (sent to registry APIs as part of URL)
- User-Agent header identifying tsuku (e.g., `tsuku/1.0`)

**Privacy implications**:
- Package registries see which packages users query (same as using `cargo search`, `gem search`, etc.)
- No telemetry or analytics are collected by builders
- No user identifiers are transmitted (IP address is visible to registries, as with any HTTP request)

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious metadata response | Validate JSON structure, limit response size | Sophisticated attacks exploiting JSON parser bugs |
| Registry compromise | Delegate to official package managers for downloads | Registry-wide compromises affect all users of that ecosystem |
| Typosquatting attacks | Display package metadata for user review | User may not notice subtle differences |
| Executable name injection | Validate executable names against shell metacharacter patterns | Novel injection patterns not yet known |
| Man-in-the-middle attacks | HTTPS only for all API requests | Compromised CA certificates |

### Cargo.toml Fetch Security

When fetching Cargo.toml for executable discovery:

1. **URL validation**: Only fetch from github.com/raw.githubusercontent.com domains initially
2. **Size limit**: Max 1MB per file (typical Cargo.toml is <50KB)
3. **Timeout**: 10 second timeout for fetch
4. **TOML parsing limits**: Reject files with excessive nesting
5. **Field validation**: Only parse `[[bin]]` sections, ignore other content

### Trust Boundaries

**Explicit trust boundary**: Builders delegate to system package managers (`cargo`, `gem`, `pip`, `npm`). These binaries are trusted as installed by the user. If a user's package manager binary is compromised, tsuku cannot detect this - this is the same trust model as using package managers directly.

### Security Best Practices for Implementation

1. **Input validation**: All package names must be validated using the same patterns as existing version providers (alphanumeric, hyphens, underscores only)
2. **Output validation**: Generated recipes should be validated using the same schema validation as registry recipes before writing to disk
3. **Executable name validation**: Builders must validate executable names before including them in recipes:
   - Must match pattern: `^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,254}$`
   - Reject shell metacharacters: `; | & $ \` \n < > ( ) { } [ ] * ? ~ \`
4. **Atomic writes**: Use write-to-temp-then-rename pattern to prevent TOCTOU races
5. **Fail secure**: If any step fails (API error, validation failure), no recipe is generated
6. **Transparency**: Show users the generated recipe content and any warnings before writing to disk
7. **Output sanitization**: Strip ANSI escape sequences from displayed metadata to prevent terminal injection

## Consequences

### Positive

1. **Expanded coverage**: Users can create recipes for any tool in supported ecosystems.
2. **Foundation for AI**: The builder interface establishes the pattern for LLM-assisted builders.
3. **Clean separation**: Recipe creation is completely decoupled from installation.
4. **Contribution-ready**: Generated recipes use the same format as registry recipes.
5. **User control**: Local recipes take precedence; users can customize.
6. **Leverages existing infrastructure**: No changes to actions, version providers, or executor.

### Negative

1. **Executable discovery is imperfect**: Some generated recipes will have wrong executable names.
2. **API dependencies**: Builders depend on external APIs; outages affect generation.
3. **Explicit creation step**: Users must run `tsuku create` before `tsuku install` for new tools.
4. **Ecosystem knowledge required**: Until LLM integration, users must know which ecosystem provides their tool.

### Mitigations

1. **Executable discovery**: Clear warnings when using fallback inference. Users can edit recipes.
2. **API dependencies**: Helpful error messages on API failure.
3. **Explicit creation**: Clear error message when tool not found suggests `tsuku create` options.
4. **Ecosystem knowledge**: Error message lists available ecosystems.

## Implementation Issues

Milestone: [Recipe Builders](https://github.com/tsukumogami/tsuku/milestone/4)

| Issue | Phase | Description |
|-------|-------|-------------|
| #93 | Phase 1a | Recipe writer with atomic file operations |
| #40 | Phase 1b | Local recipe support + Cargo builder |
| #41 | Phase 2 | Gem builder for RubyGems |
| #42 | Phase 3 | PyPI builder for Python packages |
| #43 | Phase 3 | npm builder for Node.js packages |
| #44 | Phase 4 | Recipe management UX improvements |
| #45 | Phase 5 | Toolchain bootstrapping |

### Dependency Graph

```
Phase 1a: Recipe Writer (#93)
    │
    v
Phase 1b: Local Recipe Support + Cargo Builder (#40)
    │
    ├──────────┬──────────┐
    v          v          v
Phase 2    Phase 3a   Phase 3b
Gem (#41)  PyPI (#42) npm (#43)
    │          │          │
    └──────────┴──────────┘
               │
               v
         Phase 4: UX Polish (#44)
               │
               v
         Phase 5: Toolchain (#45)
```

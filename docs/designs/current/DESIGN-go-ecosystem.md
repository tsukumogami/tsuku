# Design Document: Go Ecosystem Support

**Status**: Current

## Context and Problem Statement

Tsuku provides self-contained CLI tool installations without requiring system package managers or sudo access. The current ecosystem coverage includes Rust (cargo_install), Python (pipx_install), Ruby (gem_install), and Node.js (npm_install). However, Go dominates the DevOps, Kubernetes, and cloud-native CLI space with hundreds of popular tools like kubectl, terraform, gh, and k9s.

Users who want to install Go-based tools with tsuku face two paths:
1. **Pre-built binaries**: For tools that publish GitHub releases, users can write recipes using `github_archive` or `download_archive` actions. This works but requires a recipe for every tool.
2. **Build from source**: Not currently supported. Users cannot leverage `go install` to build tools directly from module paths.

This gap limits tsuku's utility for developers working in cloud-native environments where Go tools are ubiquitous. Installation time should be comparable to other ecosystem installers (cargo_install, pipx_install), typically completing within 1-2 minutes for average-sized tools.

### Why Now

1. **Actions exist for other ecosystems**: cargo_install, gem_install, pipx_install, and npm_install demonstrate the pattern. Go is the obvious gap.
2. **Builder infrastructure is designed**: The recipe builders design (DESIGN-recipe-builders.md) establishes patterns for ecosystem-specific builders. Go support should follow this pattern.
3. **Version provider gap**: Unlike crates.io, RubyGems, PyPI, and npm, there is no Go version provider. The Go module proxy provides version information that could fill this gap.
4. **User demand**: Go tools are extremely common in the target user base (DevOps, platform engineers, SREs).

### Scope

**In scope:**
- Go version provider (query Go proxy for module version resolution)
- `go_install` action (execute `go install` with GOBIN/GOMODCACHE isolation)
- Go toolchain bootstrap (auto-install Go as hidden dependency when needed)
- Go builder (generate recipes for Go modules, following builder pattern from DESIGN-recipe-builders.md)
- Support for at least 10 popular Go CLI tools

**Out of scope:**
- Cross-compilation support (go install limitation when GOBIN is set)
- cgo-dependent tools (require system libraries; CGO_ENABLED=0 is a hard constraint)
- Private module support (requires authentication)
- Go workspace management (go.work files)
- Build caching across versions
- Custom GOPROXY configurations (requires network access to proxy.golang.org)
- Docker/container-based builds (violates self-contained principle)

## Decision Drivers

- **Self-contained philosophy**: Users should only need tsuku to install Go tools. If Go is not installed, tsuku should bootstrap it automatically.
- **Isolation**: Go tools must be installed in isolation from the user's system Go installation (if any). GOBIN and GOMODCACHE must be controlled.
- **Consistency with existing patterns**: The go_install action should behave like cargo_install, gem_install, etc.
- **Registry API availability**: The Go module proxy (proxy.golang.org) provides version lists and module info, enabling a version provider.
- **Executable discovery challenge**: Unlike crates.io (Cargo.toml [[bin]]) or npm (package.json bin), Go modules don't have standardized metadata for executable names. Heuristics are required.

### Assumptions

These assumptions underpin the design and should be validated during implementation:

1. **Semantic versioning**: Target Go tools follow Go module versioning best practices (semantic version tags). Tools using pseudo-versions are out of scope for v1.
2. **One binary per module**: Each Go module provides a single primary executable matching the module's base name. Multi-binary modules require explicit configuration in recipes.
3. **Network access**: The standard Go module proxy (proxy.golang.org) is accessible. Restricted network environments are out of scope.
4. **Pure Go binaries**: Tools are built with CGO_ENABLED=0. This is a hard constraint, not a future consideration.
5. **Build cache sharing**: Go's build cache isolation (via GOMODCACHE) prevents cross-tool contamination. Intentional sharing improves performance.

## External Research

### mise Go Backend

**Approach**: mise treats Go as a "core tool" backend that wraps `go install`. Users can install any Go package via `mise use go:github.com/user/repo`.

**How it works**:
1. Requires Go to be installed (via mise or externally)
2. Runs `go install` with the module path
3. Binaries land in a managed location

**Trade-offs**:
- Pro: Zero configuration for most tools
- Pro: Works with any public Go module
- Con: Requires Go toolchain to be pre-installed
- Con: No executable discovery - uses module name as binary name

**Relevance to tsuku**: Validates the approach of wrapping `go install`. Tsuku should auto-bootstrap Go to maintain the self-contained philosophy.

**Source**: [mise Go Backend](https://mise.jdx.dev/dev-tools/backends/go.html)

### asdf-golang Plugin

**Approach**: asdf-golang manages Go toolchain versions. Users install Go versions, then use `go install` manually. The plugin provides shims and environment management.

**How it works**:
1. Downloads Go toolchain archives from go.dev/dl
2. Installs to `~/.asdf/installs/golang/{version}/`
3. Creates shims for `go`, `gofmt`, etc.
4. Requires `asdf reshim golang` after `go install` to update shims

**Trade-offs**:
- Pro: Full Go version management
- Pro: Supports .tool-versions for per-project Go versions
- Con: Two-step process (install Go, then install tools)
- Con: Reshim step is easy to forget

**Relevance to tsuku**: The download pattern is useful (go.dev/dl archives). The reshim problem suggests tsuku should manage binaries directly rather than relying on Go's default paths.

**Source**: [asdf-golang](https://github.com/asdf-community/asdf-golang)

### Go Module Proxy (proxy.golang.org)

**Approach**: Official Go infrastructure for module caching and version resolution. Provides HTTP API for listing versions and downloading modules.

**API Endpoints**:
- `/<module>/@v/list` - List all versions
- `/<module>/@latest` - Get latest version info
- `/<module>/@v/<version>.info` - Get version metadata
- `/<module>/@v/<version>.mod` - Get go.mod file

**Trade-offs**:
- Pro: Official, reliable, well-maintained
- Pro: Covers all public modules
- Con: No executable metadata (unlike npm registry)
- Con: Module path encoding rules are complex

**Relevance to tsuku**: The proxy provides the API needed for a Go version provider. The `@v/list` endpoint is the primary source for version resolution.

**Source**: [Go Module Proxy](https://proxy.golang.org/)

### go install Isolation

**Approach**: Go supports isolation via environment variables:
- `GOBIN`: Where binaries are installed (absolute path required)
- `GOMODCACHE`: Where module downloads are cached
- `GOPATH`: Legacy, but affects default GOBIN location

**How it works**:
```bash
GOBIN=/path/to/bin GOMODCACHE=/path/to/cache go install github.com/user/tool@version
```

**Trade-offs**:
- Pro: Complete isolation from system Go
- Pro: GOBIN can be any directory
- Con: GOBIN must be absolute path
- Con: Cross-compilation fails with GOBIN set

**Relevance to tsuku**: This is the isolation mechanism tsuku should use. Set GOBIN to install directory and GOMODCACHE to a shared cache.

**Source**: [Go Environment Variables](https://pkg.go.dev/cmd/go#hdr-Environment_variables)

### Research Summary

**Common patterns:**
1. **Wrapping go install**: All tools (mise, asdf) delegate to `go install` for package installation
2. **Toolchain bootstrap**: Required for self-contained operation; Go must be available
3. **Environment isolation**: GOBIN and GOMODCACHE control where things land
4. **Version resolution via proxy**: proxy.golang.org provides version lists for any public module

**Key differences:**
- mise: Requires pre-installed Go, uses go install directly
- asdf: Manages Go versions, separate from tool installation
- tsuku: Should combine both - bootstrap Go and install tools atomically

**Implications for tsuku:**
1. **Bootstrap Go as hidden dependency**: When go_install is used and Go is not available, install it automatically
2. **Use GOBIN isolation**: Set GOBIN to the tool's install directory
3. **Version provider from proxy**: Implement a provider that queries proxy.golang.org
4. **Executable discovery heuristics**: Since Go has no metadata for executables, use module path as default (last path segment)

## Considered Options

### Option 1: go_install Action with Explicit Toolchain Dependency

A new `go_install` action that requires Go to be installed as an explicit dependency. Recipes would declare `dependencies = ["go"]`, and the Go toolchain recipe would be a standard tsuku recipe using `download_archive`.

```toml
[metadata]
name = "lazygit"
dependencies = ["go"]

[[steps]]
action = "go_install"
module = "github.com/jesseduffield/lazygit"
```

**Pros:**
- Follows established pattern (npm_install depends on nodejs)
- Explicit dependency makes toolchain version controllable
- User can see Go is being installed
- Simple implementation - just add go_install action and Go recipe
- Debuggability: Users can inspect, verify, and even manually use the installed Go toolchain
- Security transparency: Users can audit which Go version is being used

**Cons:**
- Requires Go recipe to exist and be maintained
- Two recipes needed for each Go tool (or one shared Go recipe)
- User must manage Go updates separately from tool updates
- Toolchain visible in `tsuku list` (though this is consistent with how Node.js works)
- Dependency resolution complexity: If Tool A needs Go 1.21+ and Tool B needs Go 1.20, version conflicts become visible

### Option 2: go_install Action with Hidden Bootstrap

A `go_install` action that automatically bootstraps a hidden Go toolchain when needed. The toolchain is stored in a special location (e.g., `$TSUKU_HOME/toolchains/go/`) and is not visible in `tsuku list`.

```toml
[metadata]
name = "lazygit"

[[steps]]
action = "go_install"
module = "github.com/jesseduffield/lazygit"
```

**Pros:**
- Cleanest user experience - just install the tool
- Go is an implementation detail, not a visible dependency
- No toolchain clutter in installed tools list
- Automatic Go version management
- Faster recipe creation: No need to think about dependencies when writing recipes

**Cons:**
- Hidden state that user can't easily inspect or control
- How to update Go? Needs separate mechanism (e.g., `tsuku update-toolchain go`)
- What Go version to use? Needs policy decisions
- More complex implementation
- Disk space: Hidden Go toolchain may surprise users ("why is my disk space growing?")
- Upgrade path unclear: If a vulnerability is found in Go, how does tsuku upgrade all tools that used it?

### Option 3: Pre-built Binaries Only (No go_install)

Skip the go_install action entirely. Focus on GitHub releases and pre-built binaries. A Go builder would generate recipes that use `github_archive` to download pre-compiled binaries.

```toml
[metadata]
name = "lazygit"

[[steps]]
action = "github_archive"
repo = "jesseduffield/lazygit"
asset_pattern = "lazygit_{version}_{os}_{arch}.tar.gz"
```

**Pros:**
- No toolchain management at all
- Fast installations (no compilation)
- Works for most popular Go tools (many publish binaries)
- Consistent with existing tsuku patterns
- Reduced attack surface: No compiler toolchain needed = fewer supply chain risks

**Cons:**
- Only works for tools that publish releases
- Some Go tools don't publish pre-built binaries (coverage gap)
- Asset patterns vary wildly between projects (complex builder logic)
- Doesn't leverage Go's strength (easy cross-platform builds)
- Version availability: GitHub releases might not have every version, limiting `tsuku install tool@version`
- Platform coverage gaps: Some projects only build for common OS/arch combinations

**Note:** This option could be a v1 implementation, with go_install added in v2 if coverage gaps prove significant.

### Evaluation Against Decision Drivers

| Driver | Option 1 (Explicit) | Option 2 (Hidden) | Option 3 (Binaries) |
|--------|---------------------|-------------------|---------------------|
| Self-contained | Good | Excellent | Excellent |
| Isolation | Good | Good | N/A |
| Consistency | Good | Fair | Good |
| Registry API | Good | Good | N/A (uses GitHub) |
| Executable discovery | Fair | Fair | Poor |

### Uncertainties

- **Go version pinning**: We haven't determined how to choose which Go version to bootstrap, or how to update it. Most Go tools don't specify a minimum Go version in their go.mod. A reasonable policy would be "latest stable Go" with user override.
- **Module cache sharing**: Should GOMODCACHE be shared across all go_install invocations? This improves performance but may cause version conflicts. Go's module system is designed for sharing, so this is likely safe.
- **Compilation time**: Some Go tools take significant time to compile. We haven't measured this for typical tools. Initial estimates: small tools (1-2 min), large tools like k9s (3-5 min).
- **Binary asset patterns**: For Option 3, we haven't validated how consistently Go projects name their release assets. Anecdotal evidence suggests most popular tools follow conventions, but a builder would need heuristics.
- **User Go preference**: DevOps engineers often have Go installed. Should tsuku prefer a user's existing Go installation via environment variable (e.g., `TSUKU_GO_PATH`)? This could reduce disk usage and respect user preferences.

## Decision Outcome

**Chosen option: Option 1 (go_install Action with Explicit Toolchain Dependency)**

This option best balances self-contained operation with consistency, transparency, and implementation simplicity. The explicit dependency model follows established patterns (npm_install depends on nodejs) and provides clear visibility into what's installed.

### Rationale

This option was chosen because:

1. **Consistency with existing patterns**: npm_install already depends on nodejs as an explicit dependency. Following the same pattern for Go maintains conceptual consistency and reduces cognitive load for users and contributors.

2. **Transparency**: Users can see that Go is installed, inspect its version, and update it independently. This addresses the "hidden state" concerns of Option 2 while supporting the self-contained philosophy (Go is auto-installed as a dependency).

3. **Implementation simplicity**: Adding a go_install action and a Go toolchain recipe is straightforward. The executor already handles dependency resolution. No new infrastructure needed for toolchain management.

4. **Debuggability**: When something goes wrong, users can verify the Go installation, check its version, and even use it directly for debugging. This is valuable for the DevOps/SRE target audience.

5. **Security transparency**: Users can audit which Go version is being used, important for environments with compliance requirements.

### Alternatives Rejected

- **Option 2 (Hidden Bootstrap)**: While cleaner UX, the hidden state creates maintenance burden (separate upgrade mechanism), user confusion (unexpected disk usage), and security audit challenges. The complexity isn't justified.

- **Option 3 (Binaries Only)**: Viable as a v1 approach, but doesn't solve the core problem (users cannot leverage `go install`). Many Go tools don't publish binaries, limiting coverage. Could be considered for high-priority tools while go_install is developed.

### Trade-offs Accepted

By choosing this option, we accept:

1. **Toolchain visibility**: Go appears in `tsuku list`. Users may perceive this as clutter.

2. **Update responsibility**: Users must run `tsuku update go` separately from updating Go tools. The Go toolchain doesn't auto-update when tools are updated.

3. **Disk usage**: Go toolchain (~500MB) is installed once and shared. Users who already have Go via other means won't benefit from sharing.

These are acceptable because:
- The same trade-offs apply to Node.js, which is already accepted
- Explicit updates are safer than automatic toolchain upgrades
- 500MB is modest for the value provided; most target users have ample disk space

## Solution Architecture

### Overview

Go ecosystem support consists of four components that work together:

1. **Go Toolchain Recipe**: A standard tsuku recipe that downloads and installs the Go compiler
2. **Go Version Provider**: Queries proxy.golang.org to resolve Go module versions
3. **go_install Action**: Executes `go install` with isolated GOBIN/GOMODCACHE
4. **Go Builder**: Generates recipes for Go modules (follows DESIGN-recipe-builders.md pattern)

```
User: tsuku install lazygit
         |
         v
+-------------------+
| Recipe Loader     |
+-------------------+
         |
         v (lazygit recipe has dependencies = ["go"])
+-------------------+
| Dependency Resolver|
+-------------------+
         |
         +-------------------+
         | (if Go not installed)
         v
+-------------------+       +-------------------+
| Install Go        | ----> | Go Toolchain      |
| (download_archive)|       | $TSUKU_HOME/tools/go-1.23.0/
+-------------------+       +-------------------+
         |
         v
+-------------------+       +-------------------+
| go_install action | ----> | GOBIN, GOMODCACHE |
| lazygit module    |       | env var isolation |
+-------------------+       +-------------------+
         |
         v
+-------------------+
| lazygit binary    |
| $TSUKU_HOME/bin/  |
+-------------------+
```

### Components

#### Go Toolchain Recipe (`go.toml`)

Standard tsuku recipe using `download_archive` action:

```toml
[metadata]
name = "go"
description = "The Go programming language toolchain"
homepage = "https://go.dev"
tier = 1

[version]
source = "custom"
url = "https://go.dev/dl/?mode=json"
# Parse JSON to extract latest stable version

[[steps]]
action = "download_archive"
url = "https://go.dev/dl/go{version}.{os}-{arch}.tar.gz"
archive_format = "tar.gz"
strip_dirs = 1
binaries = ["bin/go", "bin/gofmt"]
install_mode = "directory"
os_mapping = { linux = "linux", darwin = "darwin" }
arch_mapping = { amd64 = "amd64", arm64 = "arm64" }

[verify]
command = "{install_dir}/bin/go version"
pattern = "go{version}"
```

#### Go Toolchain Version Provider

Queries the go.dev/dl JSON API for Go toolchain releases (distinct from module versions):

```go
type GoToolchainProvider struct {
    client *http.Client
}

func (p *GoToolchainProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
    // GET https://go.dev/dl/?mode=json
    // Parse JSON array, find first stable release
    // Returns version like "1.23.4" (no "v" prefix for toolchain)
}
```

#### Go Module Version Provider (`internal/version/provider_goproxy.go`)

Queries the Go module proxy to resolve versions for Go modules (not toolchain):

```go
type GoProxyProvider struct {
    client     *http.Client
    modulePath string
}

func (p *GoProxyProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
    // GET https://proxy.golang.org/{module}/@latest
    // Returns JSON: {"Version":"v1.2.3","Time":"2024-01-15T..."}
}

func (p *GoProxyProvider) ListVersions(ctx context.Context) ([]string, error) {
    // GET https://proxy.golang.org/{module}/@v/list
    // Returns newline-separated version list
    // Version format: "v1.2.3" (with "v" prefix per go.mod convention)
}

// Module path encoding (e.g., github.com/User/Repo -> github.com/!user/!repo)
func encodeModulePath(path string) string

// isValidGoModule validates module paths match expected patterns
// Prevents command injection via malformed paths
func isValidGoModule(path string) bool {
    // Pattern: alphanumeric, slashes, hyphens, dots, underscores
    // Must start with domain-like prefix
}
```

**Version format distinction:**
- Go Toolchain versions: `1.23.4` (no "v" prefix)
- Go Module versions: `v1.2.3` (with "v" prefix, per go.mod convention)

#### go_install Action (`internal/actions/go_install.go`)

Executes `go install` with environment isolation:

```go
type GoInstallAction struct{}

func (a *GoInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    module := params["module"].(string)      // e.g., "github.com/jesseduffield/lazygit"
    version := params["version"].(string)    // e.g., "v0.40.0" or "" for latest
    executables := params["executables"]     // optional: explicit binary names

    // Find Go binary from dependency
    goBin := findGoBinary(ctx)

    // Validate module path to prevent injection
    if !isValidGoModule(module) {
        return fmt.Errorf("invalid module path: %s", module)
    }

    // Set isolated environment with explicit secure defaults
    env := []string{
        "GOBIN=" + ctx.InstallDir + "/bin",
        "GOMODCACHE=" + filepath.Join(ctx.ToolsDir, ".gomodcache"),
        "CGO_ENABLED=0",
        "GOPROXY=https://proxy.golang.org,direct",  // Explicit secure default
        "GOSUMDB=sum.golang.org",                    // Explicit checksum database
    }

    // Build install target
    target := module
    if version != "" {
        target = module + "@" + version
    } else {
        target = module + "@latest"
    }

    // Execute go install
    cmd := exec.CommandContext(ctx.Context, goBin, "install", target)
    cmd.Env = append(os.Environ(), env...)
    return cmd.Run()
}
```

#### Go Builder (`internal/builders/go.go`)

Generates recipes for Go modules (follows DESIGN-recipe-builders.md):

```go
type GoBuilder struct {
    resolver *version.Resolver
}

func (b *GoBuilder) Build(ctx context.Context, modulePath string, version string) (*BuildResult, error) {
    // 1. Validate module exists via proxy
    // 2. Resolve version if empty
    // 3. Infer executable name (last path segment, or from cmd/ directory)
    // 4. Construct recipe

    executable := inferExecutable(modulePath)

    recipe := &recipe.Recipe{
        Metadata: recipe.Metadata{
            Name:         executable,
            Description:  "Go CLI tool from " + modulePath,
            Dependencies: []string{"go"},
        },
        Version: recipe.VersionConfig{
            Source: "goproxy:" + modulePath,
        },
        Steps: []recipe.Step{{
            Action: "go_install",
            Params: map[string]interface{}{
                "module":      modulePath,
                "executables": []string{executable},
            },
        }},
        Verify: recipe.VerifyConfig{
            Command: executable + " --version",
        },
    }

    return &BuildResult{
        Recipe:   recipe,
        Warnings: warnings,
        Source:   "goproxy:" + modulePath,
    }, nil
}

func inferExecutable(modulePath string) string {
    // github.com/jesseduffield/lazygit -> lazygit
    // github.com/golangci/golangci-lint/cmd/golangci-lint -> golangci-lint
    parts := strings.Split(modulePath, "/")
    return parts[len(parts)-1]
}
```

### Key Interfaces

**Version Provider Registration:**
```go
// In provider_factory.go, add strategy for goproxy source
strategies = append(strategies, &GoProxyStrategy{priority: PriorityKnownRegistry})
```

**Action Registration:**
```go
// In action.go registry init
registry.Register(&GoInstallAction{})
```

**Builder Registration:**
```go
// In builders/registry.go
registry.Register(NewGoBuilder(resolver))
```

### Data Flow

**Installing a Go tool:**
```
1. tsuku install lazygit
2. Loader.Get("lazygit") -> Recipe with dependencies=["go"]
3. DependencyResolver checks if "go" is installed
4. If not: Install "go" recipe (download_archive from go.dev/dl)
5. Executor runs go_install action:
   a. Find go binary at $TSUKU_HOME/tools/go-{version}/bin/go
   b. Set GOBIN=$TSUKU_HOME/tools/lazygit-{version}/bin
   c. Set GOMODCACHE=$TSUKU_HOME/.gomodcache
   d. Run: go install github.com/jesseduffield/lazygit@{version}
6. Verify: lazygit --version
7. Create symlink: $TSUKU_HOME/bin/lazygit -> ../tools/lazygit-{version}/bin/lazygit
```

**Creating a recipe for a Go module:**
```
1. tsuku create gofumpt --from go
2. GoBuilder.CanBuild("gofumpt") -> checks if it looks like a module path, returns true
3. GoBuilder.Build("mvdan.cc/gofumpt", ""):
   a. Query proxy.golang.org/mvdan.cc/gofumpt/@latest for version
   b. Infer executable: "gofumpt"
   c. Generate recipe with go_install action
4. Write to $TSUKU_HOME/recipes/gofumpt.toml
5. User runs: tsuku install gofumpt
```

## Implementation Approach

### Phase 1a: Go Toolchain Version Provider

**Deliverables:**
- `internal/version/provider_go_toolchain.go`
- Queries go.dev/dl JSON API for stable releases
- Returns versions without "v" prefix (e.g., "1.23.4")
- Tests for version resolution

**Validation:** Provider correctly identifies latest stable Go version.

### Phase 1b: Go Toolchain Recipe

**Deliverables:**
- `go.toml` recipe in `internal/recipe/recipes/g/`
- Uses Phase 1a provider for version resolution
- Checksum verification from go.dev
- Tests for Go installation

**Validation:** `tsuku install go` downloads and installs Go, `go version` works.

### Phase 2: go_install Action

**Deliverables:**
- `internal/actions/go_install.go`
- Environment isolation (GOBIN, GOMODCACHE, CGO_ENABLED, GOPROXY, GOSUMDB)
- Module path validation (isValidGoModule)
- Find Go binary from installed dependency
- Action registration

**Validation:** Manual recipe with `go_install` action successfully builds and installs a Go tool.

### Phase 3: Go Module Version Provider

**Deliverables:**
- `internal/version/provider_goproxy.go`
- Module path encoding
- `@latest` and `@v/list` endpoint support
- Factory integration

**Validation:** `tsuku versions lazygit` lists versions from proxy.golang.org.

### Phase 4: Go Builder

**Deliverables:**
- `internal/builders/go.go`
- Executable name inference
- Recipe generation with dependencies
- Integration with `tsuku create`

**Validation:** `tsuku create lazygit --from go` generates working recipe.

### Phase 5: Popular Tool Recipes

**Deliverables:**
- Recipes for 10+ popular Go tools in `internal/recipe/recipes/`
- Tools: lazygit, k9s, gh, golangci-lint, gofumpt, air, cobra-cli, gore, dlv, staticcheck

**Validation:** All tools install and verify successfully.

## Consequences

### Positive

1. **Complete Go ecosystem coverage**: Any public Go module can be installed via `go install`.
2. **Self-contained**: Users don't need Go pre-installed; tsuku handles bootstrapping.
3. **Consistent patterns**: Follows existing npm_install/nodejs pattern.
4. **Version resolution**: Go proxy integration enables `tsuku versions` and `@version` syntax.
5. **Builder compatibility**: Go builder follows recipe-builders design, enabling future `tsuku create` UX.
6. **Shared toolchain**: Single Go installation serves all Go tools.

### Negative

1. **Compilation time**: First install of a Go tool requires compilation (1-5 minutes depending on tool size).
2. **Disk usage**: Go toolchain is ~500MB plus module cache growth over time.
3. **Go version coupling**: All Go tools share one Go version; can't have tool-specific Go versions.
4. **CGO limitation**: Tools requiring cgo won't work (hard constraint).

### Mitigations

1. **Compilation time**: Display progress during compilation. Future LLM layer could prefer pre-built binaries when available.
2. **Disk usage**: Document in user guide. Add `tsuku gc` command to clean module cache (future work).
3. **Go version coupling**: Document limitation. In practice, Go has excellent backward compatibility, so this rarely matters.
4. **CGO limitation**: Document clearly. Suggest users install cgo-dependent tools via system package manager.

## Security Considerations

### Download Verification

**Go Toolchain Downloads:**
- **Source**: Official Go downloads from go.dev/dl
- **Verification**: Go provides SHA256 checksums alongside downloads
- **Implementation**: The `download_archive` action should verify checksums. The go.dev/dl JSON API includes checksums for each release.
- **Failure behavior**: If checksum verification fails, installation aborts with clear error message.

**Go Module Downloads (via go install):**
- **Source**: proxy.golang.org (official Go infrastructure)
- **Verification**: Go's built-in checksum database (sum.golang.org) automatically verifies module integrity
- **Implementation**: The `go install` command handles verification internally; tsuku doesn't need to implement additional checks
- **Failure behavior**: Go itself aborts if checksums don't match; user sees Go's error message

**Version Provider Queries:**
- **Source**: proxy.golang.org API
- **Verification**: HTTPS ensures transport security; response format is validated before parsing
- **Failure behavior**: Invalid responses are rejected; version resolution fails with error

### Execution Isolation

**File System Access:**
- **Scope**: Write access to `$TSUKU_HOME/tools/`, `$TSUKU_HOME/bin/`, `$TSUKU_HOME/.gomodcache/`
- **Controlled via**: GOBIN and GOMODCACHE environment variables
- **Outside scope**: No access to user's system Go installation or global GOPATH

**Network Access:**
- **Required**: proxy.golang.org for module downloads, sum.golang.org for checksums, go.dev for toolchain
- **Controlled via**: Standard Go proxy settings apply (GOPROXY, GOSUMDB)
- **No elevated privileges**: All operations run as the current user

**Process Isolation:**
- **go install execution**: Runs as subprocess with controlled environment
- **Build isolation**: GOMODCACHE and GOBIN prevent contamination of system Go
- **No privilege escalation**: All operations are unprivileged

### Supply Chain Risks

**Go Toolchain Supply Chain:**
- **Trust model**: Trust golang.org/go.dev as the official source
- **Authenticity**: Checksums from go.dev; HTTPS transport
- **Compromise scenario**: If go.dev is compromised, malicious Go toolchain could be distributed
- **Mitigation**: Go is widely used; compromises would likely be detected quickly. Users can pin Go versions.

**Go Module Supply Chain:**
- **Trust model**: Same as any use of `go install` - trust the module author
- **Authenticity**: Go's checksum database (sum.golang.org) provides tamper detection
- **Compromise scenario**: A malicious module could execute arbitrary code during `go install`
- **Mitigation**: This is inherent to `go install` - same risk as users running it directly. Tsuku doesn't add risk.

**Typosquatting:**
- **Risk**: User requests `github.com/user/lazygti` instead of `lazygit`
- **Mitigation**: Display module metadata before installation (name, description, download count if available)
- **Residual risk**: Sophisticated typosquatting attacks may not be obvious

### User Data Exposure

**Data Accessed Locally:**
- Module paths (from recipe or user input)
- Version preferences (from user input or recipe)
- No access to user source code, credentials, or personal files

**Data Transmitted Externally:**
- Module paths sent to proxy.golang.org as URL path components
- User-Agent header identifying tsuku version
- IP address visible to Go infrastructure (same as any HTTP request)

**Privacy Implications:**
- Package registries see which modules users install (same as using `go install` directly)
- No telemetry or analytics beyond what Go itself sends
- No user identifiers transmitted beyond IP address

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious Go toolchain | Checksum verification from go.dev | Compromise of go.dev itself |
| Malicious Go module | Go's sum.golang.org verification | Malicious code in legitimate module |
| Typosquatting | Display module metadata for review | Sophisticated attacks may fool users |
| Build-time code execution | Inherent to `go install`; same risk as direct use | Cannot prevent malicious build scripts |
| Network eavesdropping | HTTPS for all connections | Compromised CA certificates |
| Module path injection | Validate module paths before use | Novel injection patterns |

### Security Best Practices for Implementation

1. **Input validation**: Validate module paths match expected patterns (alphanumeric, slashes, hyphens, dots). Reject paths with shell metacharacters.
2. **Environment isolation**: Always set GOBIN, GOMODCACHE, CGO_ENABLED=0, GOPROXY, GOSUMDB explicitly. Don't inherit potentially malicious user environment for these variables.
3. **Checksum verification**: Enable and verify checksums for Go toolchain downloads from go.dev.
4. **Error transparency**: Surface Go's security errors to users (don't suppress checksum failures).
5. **HTTPS only**: All network requests use HTTPS; reject HTTP redirects to non-HTTPS.
6. **Proxy hardening**: Explicitly set `GOPROXY=https://proxy.golang.org,direct` and `GOSUMDB=sum.golang.org` to prevent environment manipulation attacks.

### Additional Security Considerations

**Shared GOMODCACHE:**
- The module cache at `$TSUKU_HOME/.gomodcache/` is shared across all Go tool installations for performance
- If a compromised module is cached, subsequent installs of the same module@version use the cached copy
- Go's checksum database prevents tampering but not malicious content in legitimate modules
- Future work: Add `tsuku gc` command to clear module cache; document cache location in user guide

**Recipe Integrity:**
- Recipes are fetched from tsuku-registry via HTTPS
- Recipe integrity relies on GitHub's security model
- This is a general tsuku concern, not specific to Go support
- Future work: Consider recipe signing for defense-in-depth (out of scope for this design)

**GOPROXY Environment Inheritance:**
- Users in corporate environments may have custom GOPROXY settings
- The go_install action explicitly sets GOPROXY to the official proxy
- Users who need custom proxies must configure at the recipe level (future work)

## Implementation Issues

### Milestone: [Go Ecosystem Support](https://github.com/tsukumogami/tsuku/milestone/5)

**Completed:**
- [#117](https://github.com/tsukumogami/tsuku/issues/117): feat(version): add Go toolchain version provider
- [#118](https://github.com/tsukumogami/tsuku/issues/118): feat(version): add Go module version provider
- [#120](https://github.com/tsukumogami/tsuku/issues/120): feat(actions): add go_install action
- [#121](https://github.com/tsukumogami/tsuku/issues/121): feat(builders): add Go builder
- [#123](https://github.com/tsukumogami/tsuku/issues/123): test: add integration test for Go tool installation

**Remaining:**
- Go toolchain recipe exists at `internal/recipe/recipes/g/go.toml`
- Popular Go tool recipes needed (tracked in milestone issue [#110](https://github.com/tsukumogami/tsuku/issues/110))

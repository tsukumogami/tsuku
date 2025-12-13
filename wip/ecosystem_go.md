# Go Ecosystem: Deterministic Execution Investigation

## Executive Summary

Go provides strong reproducibility guarantees through its module system (go.mod/go.sum) and checksum database, achieving bit-for-bit identical builds when CGO is disabled. The lock mechanism uses MVS (Minimal Version Selection) to deterministically resolve dependencies at build time, with go.sum providing cryptographic verification. Residual non-determinism primarily stems from compiler version differences, CGO-enabled builds, and build-time timestamps, though these can be largely controlled through environment variables and build flags.

## Lock Mechanism

### go.mod - Dependency Declaration

The `go.mod` file declares the module's dependencies using semantic versioning. It specifies:
- Direct dependencies (explicitly imported by the module)
- Go version requirement
- Module path and version

**Important distinction**: go.mod is NOT a lock file. It declares minimum version requirements, which Go's MVS algorithm uses to compute the actual build list deterministically.

### go.sum - Checksum Verification

The `go.sum` file is often misunderstood as a lock file, but it serves a different purpose: **cryptographic verification of dependency integrity**. It contains SHA-256 checksums for:
- The source code of each dependency version
- The go.mod file of each dependency version

When Go fetches dependencies, it verifies checksums against go.sum entries. Mismatches cause build failures, preventing tampered or modified dependencies from being used.

### MVS - Minimal Version Selection

Go uses MVS to deterministically compute the build list from go.mod requirements:

1. **Deterministic by design**: MVS always selects the same versions given the same go.mod files
2. **Minimum satisfying versions**: Selects the minimum version that satisfies all requirements (not the latest available)
3. **Predictable**: New versions are never incorporated automatically; upgrades only occur when developers explicitly request them
4. **Simple algorithm**: Can be implemented in ~50 lines of code

**Key property for tsuku**: Given identical go.mod files, MVS will produce identical build lists every time, regardless of when or where the build occurs.

## Eval-Time Capture

### Extracting the Complete Dependency Graph

**Primary command**:
```bash
go list -m -json all
```

This outputs the complete dependency graph as JSON, including:
- Module path and version for all direct and transitive dependencies
- Replace directives
- Whether each module is indirect

**Alternative for graph visualization**:
```bash
go mod graph
```

Outputs edge list format: `module version -> dependency version`

### Capturing Lock Information

To capture all lock information at evaluation time:

```bash
# 1. Ensure go.mod/go.sum are up to date
go mod tidy

# 2. Download all dependencies and populate go.sum
go mod download all

# 3. Extract dependency list with versions
go list -m -json all > dependencies.json

# 4. Capture go.sum content (checksums)
cat go.sum > checksums.txt

# 5. Verify integrity
go mod verify
```

### Efficient Eval-Time Resolution

**Recommended approach**: Use `go mod download` for efficient dependency resolution without full build:

```bash
# Download dependencies without building
go mod download -json <module>@<version>
```

This:
- Downloads module source to GOMODCACHE
- Verifies checksums via GOSUMDB
- Updates go.sum if needed
- Returns JSON with download metadata
- Does NOT compile code (much faster than build)

**For tsuku's use case**: During evaluation phase, run `go mod download` to:
1. Resolve the complete dependency graph
2. Populate go.sum with checksums
3. Verify all dependencies against checksum database
4. Capture go.sum content for plan storage

## Locked Execution

### Environment Variables for Deterministic Builds

**Critical isolation variables**:

```bash
# Module cache location (isolate from user cache)
GOMODCACHE=/path/to/isolated/cache

# Module proxy (enforce specific source)
GOPROXY=https://proxy.golang.org,direct

# Checksum database (enforce verification)
GOSUMDB=sum.golang.org

# Disable CGO for pure Go binaries
CGO_ENABLED=0

# Binary install location
GOBIN=/path/to/install/dir
```

**Additional determinism variables**:

```bash
# Target platform (explicit cross-compilation)
GOOS=linux
GOARCH=amd64

# Build mode flags
GOFLAGS="-trimpath -buildvcs=false"
```

### Build Flags for Reproducibility

**Essential flags**:

- **`-trimpath`**: Removes all file system paths from the compiled executable, replacing them with package import paths. Critical for builds across different machines.

- **`-buildvcs=false`**: Prevents VCS information (Git commit hashes, timestamps) from being embedded. Without this, builds from the same source at different times/commits produce different binaries.

- **`-ldflags="-s -w"`** (optional): Strips debug information and symbol tables for smaller, more reproducible binaries.

### Complete Locked Build Command

```bash
CGO_ENABLED=0 \
GOOS=linux \
GOARCH=amd64 \
GOMODCACHE=/path/to/cache \
GOPROXY=https://proxy.golang.org,direct \
GOSUMDB=sum.golang.org \
go install -trimpath -buildvcs=false <module>@<version>
```

### Verification During Execution

```bash
# Before build: verify all dependencies match go.sum
go mod verify

# Build with locked environment
go install [flags] <module>@<version>

# After build: verify installed binary
sha256sum /path/to/binary
```

## Reproducibility Guarantees

### Official Go Toolchain Guarantees (Go 1.21+)

As of Go 1.21, the Go team achieved **perfect reproducibility** for the toolchain itself:

- **Bit-for-bit identical builds**: Same source + same Go version = identical binaries
- **Verified nightly**: Go team runs reproducibility tests daily (results at go.dev/rebuild)
- **No hidden non-determinism**: All sources of randomness eliminated from compiler/linker
- **Cross-platform consistency**: Linux builds are identical across Debian, Arch, Alpine, etc.

### User Code Reproducibility

For user code (non-CGO):

**Fully reproducible when**:
- CGO_ENABLED=0 (pure Go)
- -trimpath flag used
- -buildvcs=false flag used
- Same Go compiler version
- Same GOOS/GOARCH
- go.sum present and verified

**Go's guarantees**:
- Module immutability: Once published to proxy, versions never change
- Checksum database: Global verification prevents tampering
- MVS determinism: Same go.mod = same build list always

### The Checksum Database (sumdb)

Go's checksum database (sum.golang.org) provides stronger guarantees than typical lock files:

- **Global consistency**: Every developer building the same module version gets the same checksums
- **Cryptographic verification**: Checksums are cryptographically signed and verifiable
- **Append-only ledger**: Checksums can never be removed or modified
- **Transparency**: Any attempt to serve different code for the same version is detectable

This means:
- If a module author modifies a published version, builds will fail globally
- Tampered proxies cannot serve different code without detection
- go.sum files provide defense-in-depth on top of sumdb

## Residual Non-Determinism

### 1. Compiler Version Differences

**Issue**: Different Go compiler versions may produce different binaries even from identical source.

**Mitigation**: Pin Go version in recipe, use tsuku-installed Go toolchain.

**Recommended capture**: Store Go version in plan:
```json
"go_version": "1.21.5"
```

### 2. CGO-Enabled Builds

**Issue**: CGO invokes host C toolchain, introducing multiple sources of non-determinism:
- Different C compilers (gcc vs clang)
- Different C library versions (glibc vs musl)
- Different compiler flags and optimizations
- Host-specific paths in debug information

**Mitigation**:
- Prefer CGO_ENABLED=0 when possible
- For required CGO: Document C toolchain requirements
- Use -fdebug-prefix-map for path normalization

**Tsuku recommendation**: Mark CGO builds as non-deterministic in plans.

### 3. Build-Time Timestamps

**Issue**: Default behavior injects build timestamp via ldflags.

**Mitigation**: Use -buildvcs=false or explicitly set timestamp to fixed value:
```bash
-ldflags="-X main.date=2025-01-01T00:00:00Z"
```

### 4. Platform-Specific Code

**Issue**: Different GOOS/GOARCH may use different source files via build tags.

**Mitigation**: Explicitly set GOOS/GOARCH in locked execution. Not actually a problem - it's expected variation.

### 5. Module Proxy Availability

**Issue**: If proxy is unreachable and go.sum is incomplete, build may fail or use alternate sources.

**Mitigation**:
- Ensure go.sum is complete during evaluation
- Use GOPROXY=https://proxy.golang.org,direct for fallback
- Consider GOPROXY=off for maximum isolation (requires complete go.sum)

### 6. Non-Reproducible Dependencies

**Issue**: If a dependency itself uses CGO or has non-deterministic build steps, transitive non-determinism occurs.

**Mitigation**: Limited control. Document that reproducibility depends on dependency practices.

### Summary of Control

| Factor | Controllable? | How |
|--------|--------------|-----|
| Compiler version | Yes | Pin Go version |
| Source code | Yes | Version pinning + checksums |
| Dependencies | Yes | go.sum verification |
| CGO usage | Partial | Disable when possible |
| Build flags | Yes | Standardize flags |
| Timestamps | Yes | -buildvcs=false |
| Platform | Yes | Explicit GOOS/GOARCH |

## Recommended Primitive Interface

```go
// GoBuildParams defines parameters for the go_build primitive.
// This primitive represents the decomposition barrier for Go module installations.
type GoBuildParams struct {
    // Module is the Go module path (e.g., "github.com/user/tool")
    Module string `json:"module"`

    // Version is the module version (e.g., "v1.2.3")
    // Must include "v" prefix for Go compatibility
    Version string `json:"version"`

    // Executables lists the binary names expected after build
    Executables []string `json:"executables"`

    // GoVersion specifies the Go toolchain version to use
    // Example: "1.21.5"
    GoVersion string `json:"go_version"`

    // GoSum contains the complete go.sum content for verification
    // This is the cryptographic lock captured at eval time
    GoSum string `json:"go_sum"`

    // GoMod contains the go.mod content (for debugging/transparency)
    // Not strictly required if GoSum is present, but useful for auditing
    GoMod string `json:"go_mod,omitempty"`

    // BuildFlags are additional flags passed to go install
    // Default: ["-trimpath", "-buildvcs=false"]
    BuildFlags []string `json:"build_flags,omitempty"`

    // Env specifies additional environment variables
    // Standard isolation variables (GOPROXY, GOSUMDB, etc.) are always set
    Env map[string]string `json:"env,omitempty"`

    // CGOEnabled indicates whether CGO is required
    // Default: false (pure Go builds)
    // If true, plan is marked as non-deterministic
    CGOEnabled bool `json:"cgo_enabled,omitempty"`

    // Platform specifies target GOOS/GOARCH
    // If omitted, uses host platform
    Platform *Platform `json:"platform,omitempty"`
}

// Platform specifies target OS and architecture
type Platform struct {
    OS   string `json:"os"`   // linux, darwin, windows
    Arch string `json:"arch"` // amd64, arm64, etc.
}

// GoBuildLock represents the lock information captured at eval time
type GoBuildLock struct {
    // Dependencies is the output of `go list -m -json all`
    Dependencies []GoModule `json:"dependencies"`

    // GoSum is the complete go.sum file content
    GoSum string `json:"go_sum"`

    // GoVersion is the Go toolchain version used for resolution
    GoVersion string `json:"go_version"`
}

// GoModule represents a single module from the dependency graph
type GoModule struct {
    Path    string `json:"Path"`
    Version string `json:"Version"`

    // Indirect indicates this is a transitive dependency
    Indirect bool `json:"Indirect,omitempty"`

    // Replace indicates this module is replaced
    Replace *GoModule `json:"Replace,omitempty"`
}
```

### Plan Representation

```json
{
  "action": "go_build",
  "params": {
    "module": "github.com/jesseduffield/lazygit",
    "version": "v0.40.2",
    "executables": ["lazygit"],
    "go_version": "1.21.5",
    "build_flags": ["-trimpath", "-buildvcs=false"],
    "cgo_enabled": false,
    "platform": {
      "os": "linux",
      "arch": "amd64"
    }
  },
  "locks": {
    "go_sum": "github.com/jesseduffield/lazygit v0.40.2 h1:abc123...\ngithub.com/foo/bar v1.0.0 h1:xyz456...\n...",
    "dependencies": [
      {
        "Path": "github.com/jesseduffield/lazygit",
        "Version": "v0.40.2"
      },
      {
        "Path": "github.com/foo/bar",
        "Version": "v1.0.0",
        "Indirect": true
      }
    ]
  },
  "deterministic": true
}
```

### Execution Pseudocode

```go
func ExecuteGoBuild(ctx *ExecutionContext, params GoBuildParams) error {
    // 1. Resolve Go toolchain binary
    goPath := resolveGo(params.GoVersion)

    // 2. Create isolated module cache
    modCache := filepath.Join(ctx.TempDir, ".gomodcache")

    // 3. Create temporary module directory
    moduleDir := filepath.Join(ctx.TempDir, "module")
    os.MkdirAll(moduleDir, 0755)

    // 4. Write go.mod and go.sum to temp directory
    ioutil.WriteFile(
        filepath.Join(moduleDir, "go.mod"),
        []byte(params.GoMod),
        0644,
    )
    ioutil.WriteFile(
        filepath.Join(moduleDir, "go.sum"),
        []byte(params.GoSum),
        0644,
    )

    // 5. Set isolated environment
    env := []string{
        "GOBIN=" + filepath.Join(ctx.InstallDir, "bin"),
        "GOMODCACHE=" + modCache,
        "GOPROXY=https://proxy.golang.org,direct",
        "GOSUMDB=sum.golang.org",
        "CGO_ENABLED=" + cgoValue(params.CGOEnabled),
    }

    if params.Platform != nil {
        env = append(env,
            "GOOS=" + params.Platform.OS,
            "GOARCH=" + params.Platform.Arch,
        )
    }

    // 6. Verify dependencies against go.sum
    cmd := exec.Command(goPath, "mod", "verify")
    cmd.Dir = moduleDir
    cmd.Env = env
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("go.sum verification failed: %w", err)
    }

    // 7. Install with locked dependencies
    target := params.Module + "@" + params.Version
    flags := params.BuildFlags
    if len(flags) == 0 {
        flags = []string{"-trimpath", "-buildvcs=false"}
    }

    args := append([]string{"install"}, flags...)
    args = append(args, target)

    cmd = exec.Command(goPath, args...)
    cmd.Dir = moduleDir
    cmd.Env = env

    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("go install failed: %w\n%s", err, output)
    }

    // 8. Verify expected executables exist
    for _, exe := range params.Executables {
        exePath := filepath.Join(ctx.InstallDir, "bin", exe)
        if _, err := os.Stat(exePath); err != nil {
            return fmt.Errorf("expected executable %s not found", exe)
        }
    }

    return nil
}
```

## Security Considerations

### Supply Chain Attack Vectors

#### 1. Typosquatting and Malicious Packages

**Real-world incident (2025)**: A malicious package `github.com/boltdb-go/bolt` impersonated the legitimate BoltDB module. It remained cached in the Go Module Proxy from November 2021 until discovery in February 2025.

**Attack mechanism**:
- Attacker published malicious version to forked GitHub repo
- Go Module Proxy cached the malicious version
- Attacker later modified Git tags to point to clean code
- Manual audits of GitHub showed clean code
- But proxy continued serving cached malicious version for 3+ years

**Impact**: Backdoor granted remote access to infected systems, allowing arbitrary command execution.

**Root cause**: Go's module immutability - by design, once cached, versions never change. This is a security feature (prevents silent updates) but also allows malicious code to persist.

**Mitigations for tsuku**:
- Verify checksums during evaluation phase
- Use go.sum verification: `go mod verify`
- Consider additional scanning: Socket, deps.dev, govulncheck
- Document known vulnerabilities in plans
- Support private module proxies for organizations

#### 2. Compromised Module Proxy

**Risk**: If attacker compromises proxy.golang.org, they could serve malicious code.

**Go's defense**:
- Checksum database (sum.golang.org) is separate from proxy
- Checksums are cryptographically signed
- Append-only ledger prevents tampering
- Global consistency: all users get same checksums

**Tsuku's defense**:
- Capture checksums during evaluation (go.sum in plan)
- Verify against checksum database during execution
- Hard fail on mismatch

#### 3. Checksum Database Compromise

**Risk**: If both proxy AND sumdb are compromised, attacker could serve consistent malicious code.

**Likelihood**: Very low (separate infrastructure, cryptographic signing)

**Defense**:
- go.sum files committed to version control provide defense-in-depth
- Once a legitimate go.sum exists, tampering is detectable
- Tsuku's plan-based approach captures go.sum at eval time

#### 4. Dependency Confusion

**Risk**: Internal module names overlap with public modules, attacker publishes malicious public version.

**Go's defense**:
- GOPRIVATE environment variable
- Private module support via authentication

**Tsuku consideration**: Support GOPRIVATE configuration for enterprise users.

### Execution Isolation

**Current go_install implementation** (from `internal/actions/go_install.go`):

```go
// SECURITY: Set up isolated environment with explicit secure defaults
env := []string{
    "GOBIN=" + binDir,
    "GOMODCACHE=" + filepath.Join(homeDir, ".tsuku", ".gomodcache"),
    "CGO_ENABLED=0",
    "GOPROXY=https://proxy.golang.org,direct",
    "GOSUMDB=sum.golang.org",
}
```

**Good practices observed**:
- Isolated GOMODCACHE (not shared with user's cache)
- Explicit GOPROXY (not inherited from environment)
- CGO disabled by default
- Input validation (module path, version, executable names)

**Recommendations for go_build primitive**:
- Maintain same isolation approach
- Add go.sum verification step before build
- Consider sandboxed execution environment for untrusted modules
- Document CGO risks clearly

### Verification and Auditing

**Recommended workflow**:

1. **During evaluation**:
   ```bash
   go mod download    # Fetch and verify all dependencies
   go mod verify      # Ensure checksums match
   govulncheck ./...  # Scan for known vulnerabilities
   ```

2. **Capture in plan**:
   - Complete go.sum content
   - Dependency list from `go list -m all`
   - Go version used
   - Any vulnerability scan results

3. **During execution**:
   ```bash
   go mod verify      # Re-verify against captured go.sum
   go install [...]   # Build with locked environment
   ```

4. **Post-installation**:
   - Compute SHA256 of installed binaries
   - Store in state for future verification

### Private Module Handling

**For organizations with private modules**:

```bash
# Configure private module patterns
GOPRIVATE=github.com/mycompany/*

# Use private proxy
GOPROXY=https://internal-proxy.company.com,https://proxy.golang.org,direct

# Disable sumdb for private modules
GONOSUMDB=github.com/mycompany/*
```

**Tsuku should**:
- Support GOPRIVATE in recipe environment
- Allow custom GOPROXY configuration
- Document security implications of private modules
- Consider credential management for authenticated proxies

## Implementation Recommendations

### Evaluation Phase

1. **Create temporary module context**:
   ```bash
   mkdir /tmp/eval-module
   cd /tmp/eval-module
   go mod init temp
   go get <module>@<version>
   ```

2. **Capture lock information**:
   ```bash
   go list -m -json all > dependencies.json
   cat go.sum > lock.sum
   go version > go-version.txt
   ```

3. **Verify and scan**:
   ```bash
   go mod verify
   govulncheck ./...  # Optional but recommended
   ```

4. **Extract for plan**:
   - Parse dependencies.json
   - Include lock.sum content
   - Record Go version
   - Mark deterministic=true (or false if CGO required)

### Execution Phase

1. **Validate plan**:
   - Ensure go.sum is present
   - Verify Go version is available (via tsuku install go)

2. **Create isolated environment**:
   - Dedicated GOMODCACHE per installation
   - Filtered environment (no inherited GO* vars)
   - Explicit GOPROXY/GOSUMDB

3. **Write lock files**:
   - Reconstruct go.mod from plan
   - Write go.sum from plan

4. **Verify before build**:
   ```bash
   go mod verify
   ```

5. **Build with locked environment**:
   ```bash
   CGO_ENABLED=0 go install -trimpath -buildvcs=false <module>@<version>
   ```

6. **Verify executables**:
   - Check all expected binaries exist
   - Optionally compute checksums for state

### Optimization: Shared Module Cache

**Consideration**: Multiple installations of Go modules share dependencies.

**Options**:

A. **Isolated per-installation** (current approach)
   - Pros: Complete isolation, no cross-contamination
   - Cons: Disk space usage, repeated downloads

B. **Shared read-only cache**
   - Pros: Disk efficiency, faster installations
   - Cons: Complexity, needs locking, verification overhead

**Recommendation for tsuku**: Start with isolated approach (option A) for security and simplicity. Consider shared cache as future optimization with proper locking and verification.

### Error Handling

**Failure modes and recovery**:

1. **go.sum verification fails**:
   - Error: Dependency checksum mismatch
   - Recovery: None - this is a security failure
   - User action: Re-evaluate to get fresh checksums

2. **Module not found**:
   - Error: Proxy/VCS doesn't have module version
   - Recovery: Check GOPROXY fallback
   - User action: Verify version exists

3. **Build fails**:
   - Error: Compilation error
   - Recovery: None
   - User action: Check Go version compatibility, file issue

4. **Executable missing**:
   - Error: Expected binary not produced
   - Recovery: None
   - User action: Check recipe (wrong executable name?)

### Testing Strategy

**Unit tests**:
- go.sum parsing and storage
- Dependency list extraction
- Environment variable construction
- Input validation (module path, version)

**Integration tests**:
- Full eval → execute cycle for known modules
- Verify bit-for-bit reproducibility
- Test with CGO-enabled modules
- Test cross-platform builds

**Security tests**:
- go.sum tampering detection
- Invalid module path rejection
- Version string injection attempts
- Environment variable isolation

### Monitoring and Telemetry

**Metrics to collect**:
- Evaluation time (go mod download duration)
- Build time (go install duration)
- Cache hit rate (if shared cache implemented)
- Failure rates by failure mode
- CGO vs pure Go builds

**Security events**:
- go.sum verification failures
- Checksum database unreachable
- Known vulnerability detections

## Sources

### Go Modules and Dependency Management
- [Go Modules Reference - The Go Programming Language](https://go.dev/ref/mod)
- [Mastering Go Modules: A Practical Guide to Dependency Management | by Leapcell | Medium](https://leapcell.medium.com/mastering-go-modules-a-practical-guide-to-dependency-management-e18eed09939c)
- [Why You Must Commit go.mod and go.sum Files in Go Projects | by Anant Haral | Medium](https://medium.com/@awesomeInfinity/hy-you-must-commit-go-mod-and-go-sum-files-in-go-projects-f97c69254188)
- [Go Wiki: Go Modules - The Go Programming Language](https://go.dev/wiki/Modules)
- [Managing dependencies - The Go Programming Language](https://go.dev/doc/modules/managing-dependencies)

### Reproducible Builds
- [Perfectly Reproducible, Verified Go Toolchains - The Go Programming Language](https://go.dev/blog/rebuild)
- [Reproducible Builds - GoReleaser](https://goreleaser.com/blog/reproducible-builds/)
- [Reproducible builds - Wikipedia](https://en.wikipedia.org/wiki/Reproducible_builds)
- [Reproducible Builds — a set of software development practices](https://reproducible-builds.org/)

### Dependency Graph Tools
- [go command - cmd/go - Go Packages](https://pkg.go.dev/cmd/go)
- [GitHub - Helcaraxan/gomod: Go modules analysis tool](https://github.com/Helcaraxan/gomod)
- [GitHub - loov/goda: Go Dependency Analysis toolkit](https://github.com/loov/goda)

### Environment Variables and Configuration
- [Choosing Your GOPROXY for Go Modules | JFrog Artifactory](https://jfrog.com/blog/why-goproxy-matters-and-which-to-pick/)
- [GOSUMDB Environment](https://goproxy.io/docs/GOSUMDB-env.html)
- [Go 1.13 Release Notes - The Go Programming Language](https://go.dev/doc/go1.13)

### CGO and Reproducibility
- [Perfectly Reproducible, Verified Go Toolchains - The Go Programming Language](https://go.dev/blog/rebuild)
- [cmd/go: Build information embedded by Go 1.18 impairs build reproducibility with cgo flags · Issue #52372 · golang/go](https://github.com/golang/go/issues/52372)
- [cgo command - cmd/cgo - Go Packages](https://pkg.go.dev/cmd/cgo)
- [Go 1.25 Release Notes - The Go Programming Language](https://go.dev/doc/go1.25)

### Supply Chain Security
- [How Go Mitigates Supply Chain Attacks - The Go Programming Language](https://go.dev/blog/supply-chain)
- [Three-Year Go Module Mirror Backdoor Exposed: Supply Chain Attack - Security Boulevard](https://securityboulevard.com/2025/04/three-year-go-module-mirror-backdoor-exposed-supply-chain-attack/)
- [Researcher sniffs out three-year Go supply chain attack • The Register](https://www.theregister.com/2025/02/04/golang_supply_chain_attack/)
- [Supply Chain Attacks in the Golang Open-Source Ecosystem | CroCoder](https://www.crocoder.dev/blog/supply-chain-attacks-in-the-golang-open-source-ecosystem)

### Security Best Practices
- [Mastering go.mod: Dependency Management the Right Way in Go | by Moksh S | Medium](https://medium.com/@moksh.9/mastering-go-mod-dependency-management-the-right-way-in-go-918226a69d58)
- [How To Handle Go Security Alerts | Jakub Jarosz](https://jarosz.dev/code/how-to-handle-go-security-alerts/)
- [Go Dependency Management: 8 Best Practices for Stable, Secure Projects](https://jsschools.com/golang/go-dependency-management-8-best-practices-for-sta/)

### Minimal Version Selection
- [research!rsc: Minimal Version Selection (Go & Versioning, Part 4)](https://research.swtch.com/vgo-mvs)
- [Modules Part 03: Minimal Version Selection](https://www.ardanlabs.com/blog/2019/12/modules-03-minimal-version-selection.html)
- [mvs package - cmd/go/internal/mvs - Go Packages](https://pkg.go.dev/cmd/go/internal/mvs)
- [Minimal Version Selection Revisited](https://matklad.github.io/2024/12/24/minimal-version-selection-revisited.html)

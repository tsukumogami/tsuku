# Cargo Ecosystem: Deterministic Execution Investigation

## Executive Summary

Cargo provides strong dependency locking via `Cargo.lock` with SHA-256 checksums for all dependencies, enabling deterministic dependency resolution. However, Rust builds are **not bit-for-bit reproducible** due to compiler version differences, build timestamps, file paths embedded in binaries, and native dependencies. For tsuku's deterministic execution model, we can lock dependencies at eval time using `cargo metadata` and `cargo fetch`, then enforce locked execution with `--locked --offline` flags, while explicitly documenting residual non-determinism from compiler toolchain and build environment variations.

## Lock Mechanism

### Cargo.lock File Format

Cargo uses `Cargo.lock` to capture the complete dependency graph with exact versions and cryptographic checksums. The file uses TOML format and has evolved through multiple versions (v1-v4), with modern versions (v3+) embedding checksums directly in package entries.

**Structure:**
```toml
version = 3

[[package]]
name = "serde"
version = "1.0.197"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "3fb1c873e1b9b056a2531fc865541ca108ce6d85c2a8dfd91a9db1e1e2e3c2a1"
dependencies = [
    "serde_derive",
]

[[package]]
name = "serde_derive"
version = "1.0.197"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "50c91d4e2a7e5f8c2d8c8c0e3d2f3f7d0e6b0b5c5e6b0a5c5e5c5e5c5e5c5e"
dependencies = [
    "proc-macro2",
    "quote",
    "syn",
]
```

**Key fields:**
- `name`: Crate name
- `version`: Exact semver version
- `source`: Registry URL (typically crates.io)
- `checksum`: SHA-256 hash of the `.crate` file (tarball containing source code)
- `dependencies`: List of direct dependencies with optional version constraints

The lockfile captures the entire transitive dependency graph, ensuring that all dependencies are resolved to specific versions. When multiple packages depend on the same crate, Cargo attempts to use a single version if possible (within SemVer compatibility), reducing duplication.

**Dependency Resolution Guarantees:**

The purpose of a Cargo.lock lockfile is to describe the state of the world at the time of a successful build. Cargo uses the lockfile to provide deterministic builds at different times and on different systems, by ensuring that the exact same dependencies and versions are used as when the Cargo.lock file was originally generated. Given a set of package manifests with dependency declarations and version constraints, the resolver produces a complete dependency graph with concrete package versions that satisfies all requirements without conflicts.

**Source:** [Cargo FAQ](https://doc.rust-lang.org/cargo/faq.html), [Dependency Resolution](https://doc.rust-lang.org/cargo/reference/resolver.html)

## Eval-Time Capture

### Efficient Resolution Without Building

Cargo provides two commands for dependency resolution without compilation:

#### 1. `cargo metadata` - Extract Dependency Graph

```bash
cargo metadata --format-version 1 --locked
```

**Output:** JSON containing complete dependency graph, including:
- Workspace members and their manifests
- Resolved dependencies (all transitive deps)
- Package metadata (version, source, checksum)
- Dependency edges (which package depends on what)

**Key options:**
- `--format-version 1`: Ensures stable JSON schema (required for forward compatibility)
- `--locked`: Asserts that Cargo.lock is up-to-date and uses exact versions from it
- `--no-deps`: Skip dependency resolution (faster, only shows workspace members)
- `--filter-platform <triple>`: Restrict to dependencies for specific target

**Important for tsuku:** `cargo metadata` does **not** download crates or modify Cargo.lock. It reads the existing lockfile and registry index cache to produce the dependency graph. If Cargo.lock doesn't exist, it performs resolution and can create one, but this should be avoided at eval time (use `--locked` to fail if missing).

**Source:** [cargo metadata](https://doc.rust-lang.org/cargo/commands/cargo-metadata.html)

#### 2. `cargo fetch` - Download Dependencies

```bash
cargo fetch --locked --target x86_64-unknown-linux-musl
```

**Purpose:** Downloads all `.crate` files (source tarballs) for dependencies into `$CARGO_HOME/registry/cache/` without compiling anything.

**Use case for tsuku:** After `cargo metadata` extracts the dependency graph, `cargo fetch` can pre-download all sources to:
1. Verify checksums from Cargo.lock against downloaded `.crate` files
2. Enable fully offline builds with `--offline`
3. Compute total download size for the plan

**Key options:**
- `--locked`: Require Cargo.lock to exist and be up-to-date (fail if missing or stale)
- `--target <triple>`: Only fetch dependencies for specific target platform
- Downloads to `$CARGO_HOME/registry/` (shared global cache by default)

**Source:** [cargo fetch](https://doc.rust-lang.org/cargo/commands/cargo-fetch.html)

### Eval-Time Workflow for tsuku

```go
// At eval time (tsuku eval <crate>):
// 1. Create temporary isolated CARGO_HOME
tmpCargoHome := createTempDir()
defer cleanup(tmpCargoHome)

// 2. Copy Cargo.toml and Cargo.lock to temp workspace
// (Either from crate source or generate minimal Cargo.toml with single dependency)

// 3. Extract dependency graph
metadata := exec("cargo", "metadata", "--format-version", "1", "--locked",
    "--manifest-path", tmpManifest)

// 4. Parse JSON to extract:
//    - All package names, versions, sources
//    - Checksums for verification
//    - Build dependency flags (some deps only needed at build time)

// 5. Optionally: cargo fetch to verify checksums and compute download size
exec("cargo", "fetch", "--locked", "--manifest-path", tmpManifest)

// 6. Record in plan:
//    - Cargo.lock content (inline or hash reference)
//    - Rust toolchain version (from rust-toolchain.toml or rustc --version)
//    - Target triple
//    - Crate specification (name@version)
```

**Source:** [cargo metadata](https://doc.rust-lang.org/cargo/commands/cargo-metadata.html), [cargo fetch](https://doc.rust-lang.org/cargo/commands/cargo-fetch.html)

## Locked Execution

### Flags and Environment Variables

To ensure deterministic execution that respects the captured lock:

#### Required Flags

```bash
cargo install --root $INSTALL_DIR --locked --offline <crate>@<version>
```

**`--locked`:**
- Asserts that Cargo.lock is up-to-date with Cargo.toml
- Prevents automatic dependency resolution or lock file updates
- Exits with error if lockfile is missing or would need to be modified
- **Critical for determinism:** Without this, Cargo may resolve to newer dependency versions

**`--offline`:**
- Prevents all network access
- Forces Cargo to use only locally cached crates in `$CARGO_HOME/registry/`
- Combined with `--locked`, guarantees no external state changes during build
- **Security benefit:** Prevents MITM attacks or registry compromise during build

**`--frozen`:**
- Equivalent to `--locked --offline` combined
- Can be used as shorthand

**Source:** [cargo install](https://doc.rust-lang.org/cargo/commands/cargo-install.html), [Rust Package Guidelines](https://wiki.archlinux.org/title/Rust_package_guidelines)

#### Isolation via CARGO_HOME

```bash
export CARGO_HOME=/path/to/isolated/cargo/home
cargo install --root $INSTALL_DIR --locked --offline <crate>@<version>
```

**Why isolate CARGO_HOME:**
- Default `~/.cargo` is shared across all builds, creating non-determinism if cache is modified
- Isolated CARGO_HOME ensures builds don't interfere with each other
- Enables parallel builds of different versions without cache conflicts
- **For tsuku:** Use tool-specific CARGO_HOME: `$TSUKU_HOME/tools/<crate>-<version>/.cargo/`

**CARGO_HOME structure:**
```
$CARGO_HOME/
├── registry/
│   ├── index/          # Registry index (crates.io metadata)
│   ├── cache/          # Downloaded .crate files
│   └── src/            # Extracted crate sources
├── git/                # Git dependencies (if any)
└── bin/                # Installed binaries (when using cargo install)
```

**Source:** [Environment Variables](https://doc.rust-lang.org/cargo/reference/environment-variables.html), [Cargo Home](https://doc.rust-lang.org/cargo/guide/cargo-home.html)

#### Additional Isolation for Native Dependencies

Some crates have build scripts (`build.rs`) that invoke system compilers or link against system libraries. For maximum determinism:

```bash
# Disable native features if crate supports it
cargo install --locked --offline --no-default-features \
    --features=pure-rust <crate>@<version>

# Control compiler for build scripts
export CC=/usr/bin/gcc-11  # Pin specific compiler version
export CXX=/usr/bin/g++-11

# Or disable native compilation entirely (if crate is pure Rust)
export RUSTFLAGS="-C target-feature=-crt-static"
```

**Common environment variables affecting builds:**
- `RUSTFLAGS`: Compiler flags (affects codegen, can break determinism)
- `CARGO_TARGET_DIR`: Build artifact directory (affects incremental compilation)
- `CC`, `CXX`, `AR`: C/C++ toolchain for build scripts
- `PKG_CONFIG_PATH`: System library discovery

**Best practice for tsuku:** Document required environment variables in the `cargo_build` primitive and reset/control them during execution.

## Reproducibility Guarantees

### What Cargo Guarantees

Cargo provides **dependency-level reproducibility** but **not bit-for-bit build reproducibility**. Specifically:

**Guaranteed (with Cargo.lock):**
- Exact same dependency versions will be used
- Exact same source code (verified by checksums) will be compiled
- Dependency graph structure is deterministic

**Not guaranteed:**
- Compiled binaries will be bit-for-bit identical
- Build artifacts will have the same hash across different environments

**Source:** [cargo guide](https://doc.rust-lang.org/cargo/guide/dependencies.html)

### Official Stance on Reproducible Builds

**As of May 2024, the Rust toolchain does not support reproducible builds out-of-the-box.** The Rust compiler team tracks this as a long-standing goal (rust-lang/rust#34902), but it remains unresolved.

**Key challenges:**
1. **Compiler version sensitivity:** Different rustc versions generate different machine code
2. **Timestamps in binaries:** Build timestamps embedded in metadata sections
3. **Path embedding:** Absolute file paths appear in debug info and panic messages
4. **Non-deterministic codegen:** Some optimizations have ordering dependencies
5. **Proc macros:** Procedural macros can expand to non-deterministic code

**Ongoing work:**
- Compiler team is working on remapping `$CARGO_HOME` and `$PWD` to fixed values automatically
- Chromium team is investing in reproducibility for their distributed build system
- Community tools like `cargo-repro` aim to provide tooling for verification

**Recommendation for production:** Use Docker or Nix to pin the **entire build environment**, including:
- Rust toolchain version (via `rust-toolchain.toml` or rustup)
- Operating system and kernel version
- System libraries and compilers
- Filesystem layout

**Source:** [MultiversX Reproducible Builds](https://docs.multiversx.com/developers/reproducible-contract-builds/), [Rust Issue #34902](https://github.com/rust-lang/rust/issues/34902), [Compiler Team Issue #450](https://github.com/rust-lang/compiler-team/issues/450)

## Residual Non-Determinism

Even with perfect locking and isolation, several sources of non-determinism remain:

### 1. Rust Compiler Version

**Impact:** Different rustc versions produce different binaries, even from identical source code.

**Why:** Compiler optimizations, LLVM backend changes, MIR transformations evolve between releases.

**Mitigation:** Pin rustc version via `rust-toolchain.toml` in project root:
```toml
[toolchain]
channel = "1.76.0"
components = ["rustfmt", "clippy"]
targets = ["x86_64-unknown-linux-musl"]
```

Or: Specify in plan and verify with `rustc --version` at execution time.

### 2. Target Triple

**Impact:** Cross-compilation to different architectures or OS produces incompatible binaries.

**Format:** `<arch><sub>-<vendor>-<sys>-<abi>` (e.g., `x86_64-unknown-linux-gnu`)

**Mitigation:** Lock target triple in plan, verify at execution time.

### 3. Build Scripts and Native Dependencies

**Impact:** Crates with `build.rs` invoke arbitrary code at build time, including:
- System compiler invocations (gcc, clang)
- External build systems (make, cmake)
- Feature detection (probing for system capabilities)

**Examples:**
- `openssl-sys`: Links against system OpenSSL
- `ring`: Builds cryptographic assembly from C/asm sources
- `tokio`: Detects available platform features (io_uring, epoll)

**Mitigation:**
- Prefer `--no-default-features` to disable native dependencies
- Use `vendored` or `bundled` features to build dependencies from source
- Control `CC`/`CXX` environment variables
- Document required system libraries

### 4. Procedural Macros

**Impact:** Proc macros execute arbitrary code at compile time, potentially:
- Reading environment variables or files
- Generating code based on timestamps
- Using randomness for unique identifiers

**Examples:**
- `serde_derive`: Generally deterministic, but depends on input order
- Custom macros: May not be designed for reproducibility

**Mitigation:** Limited options; trust crate authors and audit popular macros.

### 5. Incremental Compilation State

**Impact:** Cached build artifacts in `CARGO_TARGET_DIR` can affect builds.

**Mitigation:** Always build from clean state (no incremental compilation for release builds).

### 6. File System and Path Embedding

**Impact:**
- Absolute paths embedded in panic messages and debug info
- `file!()` and `line!()` macros capture source file paths
- Build directory affects symbol names in some cases

**Ongoing work:** Cargo issue #5505 tracks automatic remapping of paths.

**Source:** [MultiversX Reproducible Builds](https://docs.multiversx.com/developers/reproducible-contract-builds/), [Rust Issue #34902](https://github.com/rust-lang/rust/issues/34902)

## Recommended Primitive Interface

```go
// CargoBuildParams defines parameters for the cargo_build primitive
type CargoBuildParams struct {
    // Core identification
    Crate        string `json:"crate"`         // Crate name (e.g., "ripgrep")
    Version      string `json:"version"`       // Exact version (e.g., "14.1.0")
    Executables  []string `json:"executables"` // Expected binary names to verify

    // Lock information (captured at eval time)
    CargoLock    string `json:"cargo_lock"`    // Full Cargo.lock content (inline)
    CargoLockSHA string `json:"cargo_lock_sha"`// SHA-256 of Cargo.lock for verification

    // Toolchain constraints
    RustVersion  string `json:"rust_version"`  // Required rustc version (e.g., "1.76.0")
    TargetTriple string `json:"target"`        // Compilation target (e.g., "x86_64-unknown-linux-musl")

    // Build configuration
    Features     []string `json:"features,omitempty"`      // Enabled features
    NoDefaultFeatures bool `json:"no_default_features,omitempty"` // Disable default features
    AllFeatures  bool   `json:"all_features,omitempty"`   // Enable all features

    // Build environment
    Env          map[string]string `json:"env,omitempty"` // Environment variables (CC, CXX, etc.)
    RustFlags    string `json:"rustflags,omitempty"`       // RUSTFLAGS for compiler

    // Isolation
    CargoHome    string `json:"cargo_home,omitempty"`     // Isolated CARGO_HOME path
    TargetDir    string `json:"target_dir,omitempty"`     // Build artifact directory
}

// CargoBuildAction implements the cargo_build primitive
type CargoBuildAction struct{}

func (a *CargoBuildAction) Execute(ctx *ExecutionContext, params CargoBuildParams) error {
    // 1. Verify Rust toolchain version
    rustcVersion := exec("rustc", "--version")
    if !matchesVersion(rustcVersion, params.RustVersion) {
        return fmt.Errorf("rustc version mismatch: have %s, need %s",
            rustcVersion, params.RustVersion)
    }

    // 2. Set up isolated CARGO_HOME
    cargoHome := params.CargoHome
    if cargoHome == "" {
        cargoHome = filepath.Join(ctx.InstallDir, ".cargo")
    }
    os.Setenv("CARGO_HOME", cargoHome)

    // 3. Create minimal workspace with Cargo.toml
    workspace := createTempWorkspace()
    defer cleanup(workspace)

    // Write Cargo.toml
    writeCargoToml(workspace, params.Crate, params.Version)

    // Write Cargo.lock from plan
    writeLockfile(workspace, params.CargoLock)

    // Verify Cargo.lock checksum
    if sha256(params.CargoLock) != params.CargoLockSHA {
        return fmt.Errorf("Cargo.lock checksum mismatch")
    }

    // 4. Pre-fetch dependencies (populates CARGO_HOME/registry)
    exec("cargo", "fetch", "--locked", "--manifest-path", workspace+"/Cargo.toml")

    // 5. Build with locked dependencies in offline mode
    args := []string{
        "install",
        "--root", ctx.InstallDir,
        "--locked",
        "--offline",
        "--manifest-path", workspace + "/Cargo.toml",
    }

    // Add target triple
    if params.TargetTriple != "" {
        args = append(args, "--target", params.TargetTriple)
    }

    // Add feature flags
    if params.NoDefaultFeatures {
        args = append(args, "--no-default-features")
    }
    if params.AllFeatures {
        args = append(args, "--all-features")
    }
    for _, feature := range params.Features {
        args = append(args, "--features", feature)
    }

    // Add crate specification
    args = append(args, fmt.Sprintf("%s@%s", params.Crate, params.Version))

    // Set environment
    env := os.Environ()
    for k, v := range params.Env {
        env = append(env, fmt.Sprintf("%s=%s", k, v))
    }
    if params.RustFlags != "" {
        env = append(env, "RUSTFLAGS="+params.RustFlags)
    }

    // Execute build
    cmd := exec.Command("cargo", args...)
    cmd.Env = env
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("cargo install failed: %w\nOutput: %s", err, output)
    }

    // 6. Verify expected executables exist
    binDir := filepath.Join(ctx.InstallDir, "bin")
    for _, exe := range params.Executables {
        exePath := filepath.Join(binDir, exe)
        if !fileExists(exePath) {
            return fmt.Errorf("expected executable %s not found", exe)
        }
    }

    return nil
}
```

### Plan Representation

```json
{
  "action": "cargo_build",
  "params": {
    "crate": "ripgrep",
    "version": "14.1.0",
    "executables": ["rg"],
    "cargo_lock": "# Cargo.lock\nversion = 3\n\n[[package]]\nname = \"ripgrep\"\n...",
    "cargo_lock_sha": "a1b2c3d4...",
    "rust_version": "1.76.0",
    "target": "x86_64-unknown-linux-musl",
    "no_default_features": false,
    "features": []
  },
  "deterministic": false,
  "non_determinism_sources": [
    "rust_compiler_version",
    "build_scripts",
    "native_dependencies"
  ]
}
```

## Security Considerations

### 1. Arbitrary Code Execution at Build Time

**Risk:** By design, Cargo allows arbitrary code execution during builds via:
- **Build scripts** (`build.rs`): Execute before crate compilation
- **Procedural macros** (`proc-macro`): Execute during compilation

**Impact:** A malicious crate can:
- Exfiltrate data (environment variables, SSH keys, source code)
- Modify the build environment
- Install backdoors in compiled binaries
- Perform supply chain attacks

**Real incidents:**
- **September 24, 2025:** Crates `faster_log` and `async_println` caught exfiltrating Ethereum and Solana private keys
- **Typosquatting:** Malicious crates using similar names to legitimate ones (e.g., `proc-macro3` vs `proc-macro2`)

**Mitigation:**
- **Audit dependencies:** Use `cargo audit` to check for known vulnerabilities
- **Review Cargo.lock:** Inspect transitive dependencies before first build
- **Sandbox builds:** Run cargo in isolated containers or VMs for untrusted crates
- **Minimal features:** Use `--no-default-features` to reduce attack surface
- **Network isolation:** Use `--offline` after fetch to prevent build-time network access

**Source:** [RustSec Advisory Database](https://rustsec.org/), [Malicious crates announcement](https://blog.rust-lang.org/2025/09/24/crates.io-malicious-crates-fasterlog-and-asyncprintln/)

### 2. Registry Compromise

**Risk:** If crates.io or an alternate registry is compromised, attackers could:
- Replace legitimate crate tarballs with malicious versions
- Serve different checksums than originally published

**Mitigation:**
- Cargo.lock checksums protect against this **if the lock was created from a trusted build**
- At eval time, tsuku downloads and computes checksums, recording them in the plan
- At exec time, verify downloaded `.crate` files match plan checksums before extraction

**Limitation:** The initial eval inherits any existing compromise (trust-on-first-use model).

### 3. Dependency Confusion

**Risk:** If a project uses both crates.io and private registries, an attacker could publish a malicious crate to crates.io with the same name as a private dependency.

**Mitigation:**
- Cargo prioritizes the registry specified in Cargo.toml's `[dependencies]` section
- Lock dependencies with Cargo.lock to prevent resolution changes
- Use `--locked --offline` to prevent registry queries during build

### 4. Zip Bomb / Resource Exhaustion (Historical)

**Risk:** CVE-2022-36114 - Malicious crates could extract far more data than their compressed size, exhausting disk space.

**Status:** Fixed in modern Cargo versions. crates.io also has server-side checks.

**Mitigation:** Use recent Cargo version (1.64+ has the fix).

**Source:** [GHSA-2hvr-h6gw-qrxp](https://github.com/rust-lang/cargo/security/advisories/GHSA-2hvr-h6gw-qrxp)

### 5. Symlink Attacks (Historical)

**Risk:** GHSA-rfj2-q3h3-hm5j - Malicious crates with symlinks in `.cargo-ok` could overwrite arbitrary files.

**Status:** Fixed in Cargo 1.64+.

**Source:** [GHSA-rfj2-q3h3-hm5j](https://github.com/rust-lang/cargo/security/advisories/GHSA-rfj2-q3h3-hm5j)

### 6. Build Script Environment Exposure

**Risk:** Build scripts can read environment variables, potentially exposing:
- `GITHUB_TOKEN`, `CARGO_REGISTRY_TOKEN`
- AWS credentials, SSH keys
- CI/CD secrets

**Mitigation:**
- Clear sensitive environment variables before invoking Cargo
- Use isolated CARGO_HOME without credentials
- Avoid running builds in environments with access to secrets

## Implementation Recommendations

### 1. Two-Phase Execution Model

**Eval Phase:**
```go
func EvaluateCargoCrate(crate, version string, platform Platform) (*Plan, error) {
    // 1. Create temp workspace with minimal Cargo.toml
    workspace := createTempWorkspace(crate, version)
    defer cleanup(workspace)

    // 2. Resolve dependencies and generate Cargo.lock
    exec("cargo", "generate-lockfile", "--manifest-path", workspace)

    // 3. Extract dependency graph
    metadata := exec("cargo", "metadata", "--format-version=1", "--locked")

    // 4. Parse and validate
    deps := parseMetadata(metadata)

    // 5. Read Cargo.lock content
    lockContent := readFile(workspace + "/Cargo.lock")
    lockSHA := sha256(lockContent)

    // 6. Detect Rust toolchain requirement
    rustVersion := detectRustVersion(workspace)  // From rust-toolchain.toml or latest

    // 7. Build plan
    return &Plan{
        Action: "cargo_build",
        Params: CargoBuildParams{
            Crate:         crate,
            Version:       version,
            Executables:   detectExecutables(crate, metadata),
            CargoLock:     lockContent,
            CargoLockSHA:  lockSHA,
            RustVersion:   rustVersion,
            TargetTriple:  platform.ToRustTarget(),
        },
        Deterministic: false,
        NonDeterminismSources: []string{
            "rust_compiler_version",
            "build_scripts",
        },
    }
}
```

**Exec Phase:**
- Verify rustc version matches plan
- Write Cargo.lock from plan to temp workspace
- Verify Cargo.lock checksum
- `cargo fetch --locked` to populate CARGO_HOME
- `cargo install --locked --offline` from local cache
- Verify expected binaries exist

### 2. Isolated CARGO_HOME Per Tool

**Why:** Prevent cache pollution and enable parallel builds.

**Structure:**
```
$TSUKU_HOME/tools/ripgrep-14.1.0/
├── bin/
│   └── rg                    # Installed binary
├── .cargo/                   # Isolated CARGO_HOME
│   ├── registry/
│   │   ├── cache/            # Downloaded .crate files
│   │   └── src/              # Extracted sources
│   └── .crates.toml          # Cargo install metadata
└── .tsuku-metadata.json      # tsuku's state
```

### 3. Checksum Verification Strategy

**At eval time:**
- Generate Cargo.lock (or use existing)
- Record lock content and SHA-256 in plan
- Optionally: `cargo fetch` and verify all `.crate` checksums match lock

**At exec time:**
- Verify Cargo.lock from plan matches expected SHA-256
- `cargo fetch --locked` downloads all crates
- Cargo automatically verifies each `.crate` checksum against Cargo.lock
- If mismatch, Cargo fails (security feature)

**Defense in depth:** tsuku doesn't need to re-verify checksums; Cargo does it. But recording the lock hash in the plan detects tampering.

### 4. Rust Toolchain Management

**Options:**

a) **Require users to install Rust:** `cargo` must be in PATH
   - Pro: Simple, delegates toolchain management
   - Con: User burden, version skew issues

b) **Bundle rustup in tsuku:** Invoke `rustup toolchain install 1.76.0` as needed
   - Pro: Automatic toolchain provisioning
   - Con: Large download, complex state management

c) **Integrate with tsuku's rust recipe:** Use `tsuku install rust` to provide toolchain
   - Pro: Consistent with tsuku philosophy
   - Con: Circular dependency if rust itself uses cargo_build

**Recommendation:** Start with option (a) - require Rust in PATH. Document required version in recipe metadata. Consider (c) for future integration.

### 5. Feature Flag Strategy

**Problem:** Crates may have optional features affecting build outputs.

**Solution:** Allow recipes to specify features:

```toml
# Recipe: ripgrep.toml
[[actions]]
action = "cargo_build"
crate = "ripgrep"
executables = ["rg"]
no_default_features = false
features = ["pcre2"]
```

At eval time, resolve with specified features:
```bash
cargo metadata --features pcre2
```

At exec time, build with same features:
```bash
cargo install --locked --offline --features pcre2
```

### 6. Cross-Compilation Considerations

**Challenge:** Building for different target than host (e.g., macOS building for Linux).

**Cargo support:**
```bash
cargo install --locked --offline --target x86_64-unknown-linux-musl
```

**Requirements:**
- Target toolchain installed via `rustup target add x86_64-unknown-linux-musl`
- Cross-linker available (e.g., `musl-gcc`)
- May need C cross-compiler for native dependencies

**Recommendation:** For MVP, only support native compilation (target = host). Document cross-compilation as future enhancement.

### 7. Error Handling and Diagnostics

**Common failure modes:**

1. **Cargo.lock missing:** Recipe should provide lock or eval should generate it
2. **Rust version mismatch:** Detect early and provide clear error message
3. **Network failure during fetch:** Pre-fetch during eval, use --offline during exec
4. **Native dependency missing:** Document required system libraries in recipe
5. **Checksum mismatch:** Security-critical - abort immediately, log for investigation

**Suggested error messages:**

```
Error: Rust compiler version mismatch
  Required: rustc 1.76.0
  Found:    rustc 1.75.0

  Install the required version:
    rustup install 1.76.0
    rustup default 1.76.0
```

```
Error: Cargo.lock checksum verification failed
  Expected: a1b2c3d4...
  Got:      e5f6g7h8...

  This may indicate:
    - Plan file was tampered with
    - Lock file was modified after plan generation

  Re-evaluate the installation:
    tsuku eval ripgrep > ripgrep.plan
```

### 8. Testing Strategy

**Unit tests:**
- Parse `cargo metadata` JSON output
- Verify Cargo.lock checksum computation
- Validate crate name and version format

**Integration tests:**
- Eval + exec roundtrip for simple crate (e.g., `ripgrep`)
- Verify --locked --offline prevents network access
- Test with features enabled/disabled
- Verify isolated CARGO_HOME

**Security tests:**
- Detect checksum mismatch (simulate corrupted lock)
- Verify build script isolation (no access to host environment)
- Test with malicious crate names (command injection)

## References

### Documentation
- [Cargo Book - Dependency Resolution](https://doc.rust-lang.org/cargo/reference/resolver.html)
- [cargo metadata command](https://doc.rust-lang.org/cargo/commands/cargo-metadata.html)
- [cargo fetch command](https://doc.rust-lang.org/cargo/commands/cargo-fetch.html)
- [cargo install command](https://doc.rust-lang.org/cargo/commands/cargo-install.html)
- [Cargo Environment Variables](https://doc.rust-lang.org/cargo/reference/environment-variables.html)
- [Cargo Home Directory](https://doc.rust-lang.org/cargo/guide/cargo-home.html)

### Reproducibility
- [Rust Issue #34902 - Bit-for-bit deterministic builds](https://github.com/rust-lang/rust/issues/34902)
- [Compiler Team Issue #450 - Reproducible command line](https://github.com/rust-lang/compiler-team/issues/450)
- [Cargo Issue #5505 - Automatically remap paths](https://github.com/rust-lang/cargo/issues/5505)
- [MultiversX Reproducible Builds Guide](https://docs.multiversx.com/developers/reproducible-contract-builds/)
- [Internet Computer Reproducible Builds Best Practices](https://internetcomputer.org/docs/building-apps/best-practices/reproducible-builds)

### Security
- [RustSec Advisory Database](https://rustsec.org/)
- [Malicious crates: faster_log and async_println](https://blog.rust-lang.org/2025/09/24/crates.io-malicious-crates-fasterlog-and-asyncprintln/)
- [CVE-2022-36114 - Zip bomb vulnerability](https://github.com/rust-lang/cargo/security/advisories/GHSA-2hvr-h6gw-qrxp)
- [GHSA-rfj2-q3h3-hm5j - Symlink file corruption](https://github.com/rust-lang/cargo/security/advisories/GHSA-rfj2-q3h3-hm5j)

### Tools
- [cargo-lock crate - Parser for Cargo.lock](https://crates.io/crates/cargo-lock)
- [cargo-audit - Security vulnerability scanner](https://crates.io/crates/cargo-audit)
- [Arch Linux Rust Package Guidelines](https://wiki.archlinux.org/title/Rust_package_guidelines)

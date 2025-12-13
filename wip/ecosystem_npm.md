# npm Ecosystem: Deterministic Execution Investigation

## Executive Summary

npm provides deterministic dependency resolution through package-lock.json (v2/v3) and npm-shrinkwrap.json lock files that capture the complete dependency tree with exact versions, resolved URLs, and integrity hashes. Deterministic execution requires using `npm ci` with `--ignore-scripts` to enforce the lockfile and prevent arbitrary code execution. However, residual non-determinism remains in native addon compilation (node-gyp), Node.js version differences, and platform-specific build environments. The recommended primitive for tsuku should capture the lockfile at eval time and enforce locked installation with security hardening flags.

## Lock Mechanism

### Lock File Formats

npm supports two lockfile formats:

1. **package-lock.json** (npm 5+)
   - Automatically generated/updated during `npm install`
   - **Not publishable** - excluded from published packages
   - Version 1 (npm 5-6): Maps packages by name
   - Version 2 (npm 7+): Maps packages by relative location (e.g., "node_modules/lodash")
   - Version 3: Similar to v2 with enhanced metadata
   - Best for: Development, libraries, general use

2. **npm-shrinkwrap.json** (npm 2+)
   - Created manually via `npm shrinkwrap` command
   - **Publishable** - included when package is published to npm registry
   - Identical format to package-lock.json
   - Takes precedence over package-lock.json when both exist
   - Best for: CLI tools, global installs, production deployments

### Lock File Contents

Both formats contain:
- Exact resolved versions for all dependencies (direct and transitive)
- Resolved URLs (registry or git)
- Integrity hashes (SHA-512 by default)
- License information
- Full dependency tree structure

Example entry:
```json
{
  "packages": {
    "node_modules/lodash": {
      "version": "4.17.21",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz",
      "integrity": "sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==",
      "license": "MIT"
    }
  }
}
```

### Key Design Insight

The v2/v3 lockfile captures the **tree shape** itself, not just versions. This enables:
- Deterministic deduplication (same tree structure regardless of install order)
- Support for features like `--prefer-dedupe` without breaking reproducibility
- Complete specification of node_modules structure

## Eval-Time Capture

### Efficient Dependency Resolution Without Installation

Several approaches exist for resolving npm dependencies at eval time without full installation:

#### 1. npm Registry API

Query package metadata directly:
```bash
# Get all versions and metadata
curl https://registry.npmjs.org/{package-name}

# Get specific version
curl https://registry.npmjs.org/{package-name}/{version}

# Get abbreviated metadata (faster)
curl -H "Accept: application/vnd.npm.install-v1+json" \
  https://registry.npmjs.org/{package-name}
```

Returns JSON with:
- All published versions
- Dependencies for each version
- Distribution tarball URLs
- Integrity hashes

#### 2. npm view Command

```bash
# Get dependencies for specific version
npm view express@4.18.2 dependencies

# Get all metadata
npm view express@4.18.2 --json
```

#### 3. npm pack --dry-run

```bash
# Preview package contents without downloading
npm pack express@4.18.2 --dry-run

# Download tarball to current directory
npm pack express@4.18.2
# Creates: express-4.18.2.tgz
```

The `--dry-run` flag shows what would be included without creating the tarball - useful for verifying contents before download.

#### 4. Programmatic Approaches

npm's internal libraries (not guaranteed stable APIs):
- **`@npmcli/arborist`**: npm's dependency resolver, can build trees without installation
- **`pacote`**: npm's package fetcher, retrieves metadata and tarballs
- **Direct registry API calls**: Use HTTP client to query and recursively resolve

### Recommended Eval-Time Workflow

For tsuku's `npm_exec` primitive:

1. **Capture package specification**
   - Package name and version (e.g., `serve@14.2.1`)
   - Target executables to extract

2. **Generate lock at eval time**
   - Create temporary directory
   - Generate package.json with single dependency
   - Run `npm install --package-lock-only` to create lockfile without installation
   - Extract package-lock.json content

3. **Capture environment constraints**
   - Node.js version (affects native addon compatibility)
   - Platform (os, arch)
   - npm version (affects lockfile format)

4. **Return lock in plan**
   - Include full package-lock.json content
   - Document Node.js version requirement
   - Mark as non-deterministic if native addons detected

## Locked Execution

### npm ci: Enforcing Lockfile Integrity

The `npm ci` (clean install) command is designed for automated/CI environments:

**Key Behaviors:**
- **Requires existing lockfile** (package-lock.json or npm-shrinkwrap.json)
- **Deletes node_modules** before installation (clean slate)
- **Fails if package.json and lockfile are out of sync** (instead of updating lockfile)
- **Never modifies package.json or lockfiles** (installs are frozen)
- **Faster than npm install** (optimized for CI)
- **Strict mode**: Treats lockfile as source of truth

Usage:
```bash
npm ci
```

### Security Hardening with --ignore-scripts

The `--ignore-scripts` flag prevents execution of lifecycle hooks:

**Prevents:**
- preinstall scripts
- postinstall scripts
- prepare scripts
- All other lifecycle hooks

**Why critical for security:**
- Install scripts run with user privileges
- Can exfiltrate environment variables (secrets, tokens)
- Can spawn processes (crypto miners, backdoors)
- Execute arbitrary code from untrusted packages

**Best practice:**
```bash
npm ci --ignore-scripts
```

**Persistent configuration:**
```bash
npm config set ignore-scripts true
```

### Environment Variable Control

npm respects environment variables prefixed with `npm_config_`:

```bash
# Example: Disable scripts via environment
export npm_config_ignore_scripts=true

# Note: npm sets lowercase versions internally, which Node.js prefers
# Use lowercase for consistency: npm_config_foo not NPM_CONFIG_FOO
```

**Special considerations:**
- `NODE_ENV=production`: Changes default omit behavior (excludes devDependencies)
- Deprecated prefix format: npm@11 deprecated `npm_config_` prefix for some variables
- Use underscores instead of dashes: `--allow-same-version` becomes `npm_config_allow_same_version=true`

### Recommended Execution Flags

For tsuku's `npm_exec` primitive at execution time:

```bash
npm ci \
  --ignore-scripts \
  --no-audit \
  --no-fund \
  --prefer-offline \
  --prefix={install_dir}
```

**Rationale:**
- `--ignore-scripts`: Security (no arbitrary code execution)
- `--no-audit`: Skip vulnerability scanning (not needed for locked install)
- `--no-fund`: Skip funding messages (cleaner output)
- `--prefer-offline`: Use cache when possible (faster, offline-capable)
- `--prefix`: Isolate installation to specific directory

## Reproducibility Guarantees

### What npm Guarantees

With `npm ci` and a valid lockfile:

1. **Exact dependency versions**: Every package resolved to locked version
2. **Tree structure**: node_modules layout matches lockfile specification
3. **Integrity verification**: All downloads verified against integrity hashes
4. **Deduplication**: Consistent tree flattening/deduplication

### What npm Does NOT Guarantee

1. **Platform-agnostic binaries**: Native addons compile per-platform
2. **Node.js version independence**: Native modules depend on Node ABI version
3. **Compiler determinism**: Native compilation varies by compiler version/config
4. **Build tool consistency**: node-gyp, Python, C++ compiler versions vary
5. **Temporal stability**: Registry contents can change (packages unpublished)

### Native Addon Problem

Native addons (packages using node-gyp) introduce significant non-determinism:

**Compilation dependencies:**
- Python (node-gyp requirement)
- C/C++ compiler (GCC, Clang, MSVC)
- Build tools (make, Visual Studio Build Tools)
- System headers and libraries

**Variability sources:**
- Compiler version differences
- Optimization flags
- ABI compatibility (Node.js version must match)
- Platform-specific code paths
- System library versions

**Detection:**
A package uses native addons if:
- Contains `binding.gyp` file
- Depends on `node-gyp` or `prebuild`
- Has install scripts that invoke compilers
- package.json has `gypfile: true`

**Mitigation strategy:**
Most popular native addon packages now ship **prebuilt binaries** for common platforms:
- Stored on GitHub releases or CDN
- Selected based on Node.js ABI version and platform
- Fallback to compilation if no prebuilt match found

## Residual Non-Determinism

Despite lockfiles and `npm ci`, these sources of non-determinism remain:

### 1. Node.js Runtime Version

**Impact:** Different Node.js versions have different:
- Built-in module behavior
- V8 JavaScript engine versions
- ABI for native addons
- Available APIs

**Example:** Native module built for Node.js 18 won't work on Node.js 20 without rebuild

### 2. Native Addon Compilation

**When prebuilt binaries unavailable:**
- Compilation happens at install time
- Output varies by compiler, platform, system libraries
- Build errors on missing system dependencies

### 3. Install Scripts (if not disabled)

**With `--ignore-scripts` NOT set:**
- postinstall scripts can have side effects
- Scripts may download additional resources
- Behavior varies by environment (PATH, available tools)

### 4. Network and Registry State

**Temporal changes:**
- Packages can be unpublished (becomes 404)
- Package metadata can change
- Registry availability varies

**Mitigation:** Lockfile includes resolved URLs and integrity hashes, but initial resolution depends on registry state

### 5. npm/Node.js Bug Variations

**Version-specific behavior:**
- npm bugs in dependency resolution (e.g., `npm ci` failures requiring `--force` or `--legacy-peer-deps`)
- Lockfile format migration issues (v1 → v2 → v3)

### 6. Platform-Specific Dependencies

**Optional platform packages:**
- `optionalDependencies` may succeed on some platforms, fail on others
- `cpu` and `os` fields in package.json restrict installation
- fsevents (macOS-only) is common example

## Recommended Primitive Interface

### Go Struct Definition

```go
// NpmExecParams defines the parameters for the npm_exec primitive.
// This primitive handles npm package installation with deterministic lockfile enforcement.
type NpmExecParams struct {
    // Package is the npm package name (required)
    // Examples: "serve", "@typescript-eslint/parser"
    Package string `json:"package"`

    // Version is the exact package version to install (required)
    // Must be semantic version, not tag (e.g., "14.2.1" not "latest")
    Version string `json:"version"`

    // Executables lists the binary names to install to $TSUKU_HOME/bin (required)
    // These will be verified to exist after installation
    Executables []string `json:"executables"`

    // PackageLock contains the full package-lock.json content captured at eval time (required)
    // This ensures identical dependency tree on execution
    PackageLock string `json:"package_lock"`

    // NodeVersion specifies the Node.js version constraint (optional)
    // Example: ">=18.0.0", "18.x"
    // Used to verify compatibility, especially for native addons
    NodeVersion string `json:"node_version,omitempty"`

    // NpmVersion documents the npm version used to generate the lockfile (optional)
    // Helps diagnose lockfile format compatibility issues
    NpmVersion string `json:"npm_version,omitempty"`

    // IgnoreScripts controls whether to skip lifecycle scripts (default: true)
    // Should be true for security unless recipe explicitly needs scripts
    IgnoreScripts bool `json:"ignore_scripts"`

    // HasNativeAddons flags whether this package includes native modules (informational)
    // Detected at eval time by checking for binding.gyp, node-gyp dependency, etc.
    // When true, reproducibility is limited to same platform + Node.js version
    HasNativeAddons bool `json:"has_native_addons"`

    // RequiredEnv specifies environment variables needed during installation (optional)
    // Example: {"NODE_ENV": "production"}
    RequiredEnv map[string]string `json:"required_env,omitempty"`
}

// NpmExec executes an npm package installation using locked dependencies.
// It writes the lockfile to a temporary directory, runs npm ci with security hardening,
// and extracts the specified executables to the install directory.
//
// Non-determinism warning: If HasNativeAddons is true, this operation is only
// reproducible on the same platform with the same Node.js version.
func (p *NpmExecParams) Execute(ctx *ExecutionContext) error {
    // Implementation would:
    // 1. Verify Node.js is available and meets version constraint
    // 2. Create temporary directory for package.json + package-lock.json
    // 3. Write package.json with single dependency: {package}@{version}
    // 4. Write package-lock.json from PackageLock field
    // 5. Run: npm ci --ignore-scripts --no-audit --no-fund --prefix={install_dir}
    // 6. Verify each executable in Executables list exists
    // 7. Return error if verification fails
    // ...
}
```

### Example Plan Entry

```json
{
  "action": "npm_exec",
  "params": {
    "package": "serve",
    "version": "14.2.1",
    "executables": ["serve"],
    "package_lock": "{\"name\":\"serve-install\",\"lockfileVersion\":3,...}",
    "node_version": ">=18.0.0",
    "npm_version": "10.2.3",
    "ignore_scripts": true,
    "has_native_addons": false
  },
  "deterministic": true
}
```

### Example with Native Addons

```json
{
  "action": "npm_exec",
  "params": {
    "package": "sharp",
    "version": "0.33.1",
    "executables": ["sharp"],
    "package_lock": "{\"name\":\"sharp-install\",\"lockfileVersion\":3,...}",
    "node_version": "18.x",
    "npm_version": "10.2.3",
    "ignore_scripts": false,
    "has_native_addons": true,
    "required_env": {
      "npm_config_sharp_binary_host": "https://github.com/lovell/sharp-libvips/releases/download",
      "npm_config_sharp_libvips_binary_host": "https://github.com/lovell/sharp-libvips/releases/download"
    }
  },
  "deterministic": false
}
```

## Security Considerations

### 1. Supply Chain Attacks

**Typosquatting:**
- Attackers publish packages with names similar to popular packages
- Examples: "crossenv" vs "cross-env", "event-strean" vs "event-stream"
- Users accidentally install malicious variants

**Mitigation for tsuku:**
- Recipes specify exact package names (reviewed by maintainers)
- Lockfile captures exact versions and integrity hashes
- Integrity verification on download prevents tampering

### 2. Malicious Install Scripts

**Attack vector:**
- postinstall/preinstall scripts execute with user privileges
- Can steal credentials from environment variables
- Can install backdoors, spawn crypto miners
- Well-known incidents: event-stream (2018), UAParser.js (2021)

**Mitigation:**
- Default to `ignore_scripts: true` in NpmExecParams
- Only allow scripts for trusted packages where essential
- Document in recipe why scripts are needed

### 3. Dependency Confusion

**Attack:**
- Attacker publishes public package with same name as private package
- npm prioritizes public registry over private
- Organization installs malicious public version

**Not applicable to tsuku:**
- tsuku installs from public registry only
- No private package namespace conflicts

### 4. Compromised Maintainer Accounts

**Risk:**
- Attacker gains access to maintainer npm account
- Publishes malicious version of popular package
- All new installs receive malicious code

**Mitigation:**
- Lockfile prevents automatic upgrades to malicious versions
- Re-evaluation detects changed integrity hashes (would fail)
- Recipes specify version ranges carefully

### 5. Package Unpublishing

**Risk:**
- Maintainer unpublishes package from registry
- Lockfile references become 404
- Installation fails

**Mitigation:**
- Cannot prevent, but lockfile makes issue explicit
- Error message would indicate missing package
- Recipes should prefer stable, well-maintained packages

### 6. Network Attacks

**Man-in-the-middle:**
- Attacker intercepts download from registry
- Serves malicious tarball

**Mitigation:**
- npm uses HTTPS for registry
- Integrity hashes in lockfile verify downloaded content
- Mismatch causes hard failure

### 7. Prebuilt Binary Manipulation

**For native addons with prebuilt binaries:**
- Binaries hosted externally (GitHub releases, CDN)
- Integrity not always verified
- Potential for serving malicious binaries

**Mitigation:**
- Prefer packages that verify prebuilt binary integrity
- Some packages include checksums in package.json
- `ignore_scripts: false` may be needed to run verification

## Implementation Recommendations

### For tsuku's npm_exec Primitive

#### 1. Eval-Time Lockfile Generation

```go
// During plan generation:
func GenerateNpmLock(pkg string, version string) (string, error) {
    tmpDir := CreateTempDir()
    defer os.RemoveAll(tmpDir)

    // Create minimal package.json
    packageJSON := fmt.Sprintf(`{
        "name": "tsuku-npm-eval",
        "version": "0.0.0",
        "dependencies": {
            "%s": "%s"
        }
    }`, pkg, version)

    os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)

    // Generate lockfile without installation
    cmd := exec.Command("npm", "install", "--package-lock-only")
    cmd.Dir = tmpDir
    if err := cmd.Run(); err != nil {
        return "", err
    }

    // Read generated lockfile
    lockBytes, err := os.ReadFile(filepath.Join(tmpDir, "package-lock.json"))
    return string(lockBytes), err
}
```

#### 2. Native Addon Detection

```go
// Check if package has native addons
func HasNativeAddons(lockfileContent string) bool {
    // Parse lockfile JSON
    var lock map[string]interface{}
    json.Unmarshal([]byte(lockfileContent), &lock)

    packages := lock["packages"].(map[string]interface{})
    for _, pkg := range packages {
        pkgMap := pkg.(map[string]interface{})

        // Check for gypfile flag
        if gypfile, ok := pkgMap["gypfile"].(bool); ok && gypfile {
            return true
        }

        // Check for node-gyp dependency
        if deps, ok := pkgMap["dependencies"].(map[string]interface{}); ok {
            if _, hasNodeGyp := deps["node-gyp"]; hasNodeGyp {
                return true
            }
        }

        // Check for hasInstallScript (often indicates native build)
        if hasInstall, ok := pkgMap["hasInstallScript"].(bool); ok && hasInstall {
            return true // Conservative: assume native
        }
    }

    return false
}
```

#### 3. Execution with Isolation

```go
func (p *NpmExecParams) Execute(ctx *ExecutionContext) error {
    tmpDir := CreateTempDir()
    defer os.RemoveAll(tmpDir)

    // Write package.json
    packageJSON := fmt.Sprintf(`{
        "name": "tsuku-install",
        "version": "0.0.0",
        "dependencies": {"%s": "%s"}
    }`, p.Package, p.Version)
    os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)

    // Write lockfile
    os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte(p.PackageLock), 0644)

    // Build npm ci command
    args := []string{"ci", "--no-audit", "--no-fund", "--prefer-offline"}
    if p.IgnoreScripts {
        args = append(args, "--ignore-scripts")
    }
    args = append(args, fmt.Sprintf("--prefix=%s", ctx.InstallDir))

    cmd := exec.Command("npm", args...)
    cmd.Dir = tmpDir

    // Set environment
    env := os.Environ()
    for k, v := range p.RequiredEnv {
        env = append(env, fmt.Sprintf("%s=%s", k, v))
    }
    cmd.Env = env

    // Execute
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("npm ci failed: %w\nOutput: %s", err, output)
    }

    // Verify executables exist
    binDir := filepath.Join(ctx.InstallDir, "bin")
    for _, exe := range p.Executables {
        if _, err := os.Stat(filepath.Join(binDir, exe)); err != nil {
            return fmt.Errorf("expected executable %s not found", exe)
        }
    }

    return nil
}
```

#### 4. Node.js Version Verification

```go
func VerifyNodeVersion(constraint string) error {
    cmd := exec.Command("node", "--version")
    output, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("node.js not found: %w", err)
    }

    version := strings.TrimPrefix(strings.TrimSpace(string(output)), "v")

    // Use semver library to check constraint
    if !semver.Satisfies(version, constraint) {
        return fmt.Errorf("node.js %s does not satisfy constraint %s", version, constraint)
    }

    return nil
}
```

#### 5. Error Handling for Common Issues

Handle known npm quirks:
- Lockfile format version mismatches
- Peer dependency conflicts requiring `--legacy-peer-deps`
- Platform-specific optional dependencies failures
- Missing prebuilt binaries triggering compilation

Provide actionable error messages guiding users to:
- Update Node.js version
- Install build tools if native compilation needed
- Report recipe bugs if lockfile is malformed

### Testing Strategy

1. **Unit tests**: Lockfile generation, native addon detection
2. **Integration tests**: Install packages with/without native addons
3. **Platform matrix**: Test on Linux, macOS, Windows
4. **Node.js version matrix**: Test across LTS versions (18.x, 20.x, 22.x)
5. **Security tests**: Verify `--ignore-scripts` prevents script execution
6. **Reproducibility tests**: Same lockfile produces same binaries (checksum comparison)

### Documentation for Recipe Authors

Provide guidance on:
- When to use `npm_exec` vs downloading prebuilt binaries
- How to determine correct Node.js version constraint
- When scripts must be enabled (and security implications)
- How to test recipes across platforms
- Handling packages with native addons (document platform requirements)

---

## Sources

- [Should You Commit package-lock.json in npm 5?](https://www.codegenes.net/blog/do-i-commit-the-package-lock-json-file-created-by-npm-5/)
- [npm – Catching Up with Package Lockfile Changes in v7](https://nitayneeman.com/blog/catching-up-with-package-lockfile-changes-in-npm-v7/)
- [npm v7 Series - Why Keep package-lock.json?](https://blog.npmjs.org/post/621733939456933888/npm-v7-series-why-keep-package-lockjson.html)
- [package-lock.json | npm Docs](https://docs.npmjs.com/cli/v9/configuring-npm/package-lock-json/)
- [The Complete Guide to package-lock.json](https://medium.com/pavesoft/package-lock-json-the-complete-guide-2ae40175ebdd)
- [NPM Ignore Scripts Best Practices](https://www.nodejs-security.com/blog/npm-ignore-scripts-best-practices-as-security-mitigation-for-malicious-packages)
- [npm-ci | npm Docs](https://docs.npmjs.com/cli/v8/commands/npm-ci/)
- [Securing npm dependencies: Best practices](https://blue.tymyrddin.dev/docs/dev/appsec/libraries/npm)
- [The Problem with Npm Install](https://bobjames.dev/articles/software-development/the-problem-with-npm-install)
- [Node-gyp Troubleshooting Guide](https://blog.openreplay.com/node-gyp-troubleshooting-guide-fix-common-installation-build-errors/)
- [GitHub - nodejs/node-gyp](https://github.com/nodejs/node-gyp)
- [Solving common issues with node-gyp](https://blog.logrocket.com/solving-common-issues-node-gyp/)
- [npm-pack | npm Docs](https://docs.npmjs.com/cli/v6/commands/npm-pack/)
- [HOWTO: Inspect, Download and Extract NPM Packages](https://blog.packagecloud.io/how-to-inspect-download-and-extract-npm-packages/)
- [Local npm Package Testing Made Simple: A Guide to npm pack](https://blog.rnsloan.com/2025/01/11/local-npm-package-testing-made-simple-a-guide-to-npm-pack/)
- [Best Practices for Creating a Modern npm Package](https://snyk.io/blog/best-practices-create-modern-npm-package/)
- [npm-shrinkwrap.json | npm Docs](https://docs.npmjs.com/cli/v9/configuring-npm/npm-shrinkwrap-json/)
- [Understanding npm-shrinkwrap.json: A Complete Guide](https://www.w3resource.com/npm/npm-shrinkwrap-json.php)
- [npm-shrinkwrap | npm Docs](https://docs.npmjs.com/cli/v8/commands/npm-shrinkwrap/)
- [Environment Variables · node-config/node-config Wiki](https://github.com/node-config/node-config/wiki/Environment-Variables)
- [config | npm Docs](https://docs.npmjs.com/cli/v8/using-npm/config/)
- [Node.js — The difference between development and production](https://nodejs.org/en/learn/getting-started/nodejs-the-difference-between-development-and-production)

# Design Document: Perl Ecosystem Support

**Status**: Implemented

## Context and Problem Statement

Tsuku provides self-contained CLI tool installations without requiring system package managers or sudo access. The current ecosystem coverage includes Rust (cargo_install), Python (pipx_install), Ruby (gem_install), Node.js (npm_install), and Go (go_install). However, the Perl/CPAN ecosystem remains unsupported despite hosting numerous DevOps and system administration tools. Perl remains prevalent in sysadmin and DevOps contexts - tools like `ack` (grep replacement), `carton` (dependency management), and various deployment utilities are written in Perl.

Users who want to install Perl-based CLI tools with tsuku currently have no path forward. Unlike other ecosystems where tsuku can leverage package managers (cargo, gem, pip, npm, go), Perl tools require:
1. A Perl runtime (not commonly pre-installed on modern systems)
2. cpanm for module installation
3. Proper isolation to avoid conflicts with system Perl

This gap limits tsuku's utility for users who need Perl tools like `ack`, `cpanm` itself, `carton`, `plenv`, or custom deployment scripts.

### Why Now

1. **Actions exist for other ecosystems**: cargo_install, gem_install, pipx_install, npm_install, and go_install demonstrate the pattern. Perl is a notable gap.
2. **Builder infrastructure is designed**: The recipe builders design (DESIGN-recipe-builders.md) establishes patterns for ecosystem-specific builders. Perl support should follow this pattern.
3. **Version provider gap**: Unlike crates.io, RubyGems, PyPI, npm, and Go proxy, there is no CPAN version provider. MetaCPAN provides a REST API that could fill this gap.
4. **Relocatable Perl exists**: The [skaji/relocatable-perl](https://github.com/skaji/relocatable-perl) project provides self-contained Perl binaries with cpanm bundled, enabling tsuku's self-contained philosophy.

### Scope

**In scope:**
- CPAN version provider (query MetaCPAN API for distribution version resolution)
- `cpan_install` action (execute cpanm with local::lib isolation)
- Perl runtime bootstrap (auto-install relocatable-perl as hidden dependency when needed)
- CPAN builder (generate recipes for CPAN distributions, following builder pattern from DESIGN-recipe-builders.md)
- Support for popular Perl CLI tools

**Out of scope:**
- XS/native extension compilation requiring system libraries (pure Perl distributions only for v1)
- Custom CPAN mirrors (requires network access to MetaCPAN)
- Private CPAN repositories
- Multiple Perl version management (single bundled version)
- Windows support (relocatable-perl only supports Linux and macOS)

## Decision Drivers

- **Self-contained philosophy**: Users should only need tsuku to install Perl tools. If Perl is not installed, tsuku should bootstrap it automatically.
- **Isolation**: Perl tools must be installed in isolation from any system Perl installation. PERL5LIB and module paths must be controlled per-tool.
- **Consistency with existing patterns**: The cpan_install action should behave like gem_install - using wrapper scripts to set up the environment before execution.
- **Registry API availability**: MetaCPAN provides a REST API for version lists and distribution metadata.
- **Executable discovery challenge**: CPAN distributions declare executables via `EXE_FILES` in Makefile.PL or `script/` directory, but this metadata isn't directly exposed via MetaCPAN API.
- **Distribution vs Module naming**: CPAN uses "distributions" (e.g., `App-Ack`) which contain "modules" (e.g., `App::Ack`). Users typically know module names, but installation targets distributions.

### Assumptions

These assumptions underpin the design and should be validated during implementation:

1. **Relocatable-perl is reliable**: The skaji/relocatable-perl project provides stable, maintained binaries for Linux (amd64, arm64) and macOS (amd64, arm64). Binaries include cpanm. If the project becomes unmaintained, tsuku would need to find an alternative source or build Perl ourselves.
2. **Pure Perl distributions suffice**: Target CLI tools and their dependency trees are pure Perl (no XS). Tools requiring native compilation are out of scope for v1.
3. **MetaCPAN API is stable**: The MetaCPAN REST API endpoints are well-established and won't change incompatibly.
4. **local::lib isolation works**: Per-tool isolation via `--local-lib` prevents version conflicts between tools.
5. **Network access**: MetaCPAN (fastapi.metacpan.org) is accessible. Restricted network environments are out of scope.
6. **Bash is available**: Wrapper scripts use bash-specific syntax (`${BASH_SOURCE[0]}`). Target systems (Linux and macOS) have bash installed.
7. **CPAN naming conventions**: Most CPAN distributions follow conventional naming where `App-Foo` creates executable `foo`. The builder relies on this pattern for executable discovery.
8. **System Perl ignored**: tsuku will always use its bundled Perl runtime, even if system Perl exists. This ensures consistent, reproducible behavior across environments.

## External Research

### MetaCPAN API

**Approach**: MetaCPAN provides a REST API for querying CPAN distribution and release metadata. The API is built on Elasticsearch and provides both convenience endpoints and full search capabilities.

**Key Endpoints**:
- `GET /release/{distribution}` - Get latest release of a distribution
- `GET /release/{author}/{release}` - Get specific release by author and name
- `POST /release/_search` - Elasticsearch query for advanced filtering
- `GET /distribution/{distribution}` - Distribution-level info (bug counts, etc.)
- `GET /download_url/{module}` - Designed for cpanm, returns download URL

**Response Example** (`/release/App-Ack`):
```json
{
  "distribution": "App-Ack",
  "version": "3.7.0",
  "author": "PETDANCE",
  "download_url": "https://cpan.metacpan.org/authors/id/P/PE/PETDANCE/ack-v3.7.0.tar.gz",
  "date": "2023-03-15T...",
  "abstract": "grep-like text finder"
}
```

**Trade-offs**:
- Pro: Official, well-maintained API with good documentation
- Pro: Provides version lists, metadata, and download URLs
- Con: No direct executable metadata (EXE_FILES not exposed in API)
- Con: Rate limiting is informal ("be polite", max 5000 items per search)

**Relevance to tsuku**: MetaCPAN provides the API needed for a CPAN version provider. The `/release/{distribution}` endpoint is the primary source for latest version, and `/_search` enables version history queries.

**Sources**: [MetaCPAN API docs](https://github.com/metacpan/metacpan-api/blob/master/docs/API-docs.md)

### local::lib for Perl Isolation

**Approach**: local::lib creates isolated Perl module directories with proper environment variable setup. It's the standard approach for per-application Perl dependency management.

**How it works**:
1. Set `PERL5LIB` to include the local lib/perl5 directory
2. Set `PERL_MM_OPT` and `PERL_MB_OPT` for ExtUtils::MakeMaker and Module::Build
3. Prepend the local bin directory to `PATH`
4. cpanm's `--local-lib` flag automatically handles this setup

**Directory structure created**:
```
<local-lib>/
├── lib/perl5/           # Perl modules
├── bin/                 # Executable scripts
└── man/                 # Man pages
```

**Trade-offs**:
- Pro: Standard, well-tested isolation mechanism
- Pro: Works with cpanm out of the box
- Pro: Supports relocatable paths via PERL_LOCAL_LIB_ROOT
- Con: Requires wrapper scripts to set environment before execution

**Relevance to tsuku**: cpan_install should use `cpanm --local-lib` for per-tool isolation, following the same wrapper script pattern as gem_install.

**Sources**: [local::lib documentation](https://metacpan.org/pod/local::lib)

### Relocatable Perl (skaji/relocatable-perl)

**Approach**: Provides self-contained, portable Perl binaries that work without system dependencies. Built with `-Duserelocatableinc` for path independence.

**How it works**:
1. Download tarball for platform (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64)
2. Extract to any directory
3. Perl binary works immediately with bundled cpanm

**Included components**:
- Perl 5.x (latest stable)
- App::cpanminus (cpanm)
- App::ChangeShebang
- libxcrypt

**Trade-offs**:
- Pro: Truly self-contained, no system dependencies
- Pro: Includes cpanm, ready for module installation
- Pro: Supports Linux and macOS on amd64 and arm64
- Con: ~50MB download size
- Con: No Windows support
- Con: Single Perl version per release (can't choose arbitrary versions)

**Relevance to tsuku**: Relocatable-perl is the ideal solution for tsuku's Perl runtime. It matches the self-contained philosophy and includes cpanm. The recipe would use `download_archive` to fetch the appropriate platform binary.

**Sources**: [skaji/relocatable-perl](https://github.com/skaji/relocatable-perl)

### mise Perl Support

**Approach**: mise manages Perl via asdf plugins or core integration. Users can install Perl versions and use cpanm for modules.

**How it works**:
1. `mise use perl@5.38` installs Perl via asdf-perl plugin
2. Perl is available in PATH
3. Users manually run `cpanm` for module installation
4. No direct CPAN tool installation support

**Trade-offs**:
- Pro: Full Perl version management
- Pro: Integrates with asdf ecosystem
- Con: Two-step process (install Perl, then install modules)
- Con: No builder for CPAN distributions
- Con: Requires shims and reshim management

**Relevance to tsuku**: mise doesn't provide direct CPAN tool installation. tsuku's cpan_install action would be unique in the tool manager space, providing atomic "install this Perl CLI tool" capability.

**Sources**: [mise Dev Tools](https://mise.jdx.dev/dev-tools/)

### CPAN Executable Discovery

**Approach**: CPAN distributions declare executables via `EXE_FILES` in Makefile.PL or files in the `script/` or `bin/` directory. These are installed to Perl's bin directory during module installation.

**How it works**:
```perl
# Makefile.PL
WriteMakefile(
    NAME      => 'App::Ack',
    EXE_FILES => ['script/ack'],
    # ...
);
```

**Discovery challenge**: MetaCPAN API doesn't directly expose EXE_FILES. Options:
1. Fetch and parse Makefile.PL from the distribution tarball
2. Fetch and parse META.json which sometimes includes `x_provides_scripts`
3. Query the `/file/_search` endpoint for files in `script/` or `bin/` directories
4. Fall back to distribution name as executable (App-Ack -> ack)

**Trade-offs**:
- Fetching Makefile.PL requires downloading/extracting the tarball
- META.json isn't guaranteed to have script information
- File search adds API complexity
- Fallback heuristics work for many common tools

**Relevance to tsuku**: The CPAN builder will need heuristics for executable discovery. Start with distribution name transformation (App-Ack -> ack), with warnings when inference is uncertain.

**Sources**: [How to upload a script to CPAN](https://www.perl.com/article/how-to-upload-a-script-to-cpan/)

### Research Summary

**Common patterns:**
1. **local::lib is standard**: Every Perl isolation approach uses local::lib or equivalent
2. **cpanm is the installer**: Modern Perl module installation uses cpanm, not the legacy cpan client
3. **Relocatable binaries exist**: skaji/relocatable-perl solves the "need Perl to install Perl tools" problem
4. **Executable metadata is fragmented**: Unlike npm's `bin` field or RubyGems' `executables`, CPAN has no standardized API for discovering executables

**Key differences from other ecosystems:**
- **vs Cargo**: No pre-built binaries; always installs from source (like pip)
- **vs RubyGems**: Similar isolation pattern (wrapper scripts), but Perl needs explicit PERL5LIB management
- **vs PyPI**: Similar in that it uses an external runtime, but pipx handles isolation automatically
- **vs npm**: npm's package.json `bin` field is directly in registry metadata; CPAN requires parsing Makefile.PL

**Implications for tsuku:**
1. **Use relocatable-perl**: Bootstrap as hidden dependency, similar to how nodejs is used for npm_install
2. **Use cpanm --local-lib**: Standard isolation mechanism, mirrors gem_install pattern
3. **Generate wrapper scripts**: Set PERL5LIB before executing, same as gem_install
4. **Executable discovery heuristics**: Transform distribution name (App-Foo -> foo, Foo-Bar -> foo-bar), warn on uncertainty

## Considered Options

### Option 1: cpan_install Action with Explicit Perl Dependency

A new `cpan_install` action that requires Perl to be installed as an explicit dependency. Recipes would declare `dependencies = ["perl"]`, and the Perl runtime recipe would download relocatable-perl.

```toml
[metadata]
name = "ack"
dependencies = ["perl"]

[[steps]]
action = "cpan_install"
distribution = "App-Ack"
executables = ["ack"]
```

**Pros:**
- Follows established pattern (npm_install depends on nodejs, go_install depends on go)
- Explicit dependency makes Perl version visible and controllable
- User can see Perl is being installed
- Simple implementation - just add cpan_install action and Perl recipe
- Debuggability: Users can inspect and use the installed Perl directly

**Cons:**
- Requires Perl recipe to exist and be maintained; someone must update it when relocatable-perl releases new versions
- Perl visible in `tsuku list`, which some users may perceive as clutter (though consistent with nodejs, go)
- User must manage Perl updates separately from tool updates

**Why wrapper scripts (like Ruby, not like npm):** Perl requires `PERL5LIB` to be set correctly before execution for modules to be found. Unlike npm where the runtime locates dependencies via node_modules, Perl's `@INC` search path must be configured via environment variable. This matches Ruby's need for `GEM_HOME`/`GEM_PATH`, hence the gem_install-style wrapper approach.

### Option 2: cpan_install Action with Hidden Bootstrap

A `cpan_install` action that automatically bootstraps a hidden Perl runtime when needed. The runtime is stored in `$TSUKU_HOME/toolchains/perl/` and not visible in `tsuku list`.

```toml
[metadata]
name = "ack"

[[steps]]
action = "cpan_install"
distribution = "App-Ack"
executables = ["ack"]
```

**Pros:**
- Cleanest user experience - just install the tool
- Perl is an implementation detail
- No toolchain clutter in installed tools list

**Cons:**
- Hidden state that user can't easily inspect or debug
- Requires new command/mechanism (e.g., `tsuku update-toolchain perl`) that doesn't exist yet
- More complex implementation
- Inconsistent with go_install and npm_install patterns
- Violates principle of least surprise - users may not expect hidden toolchains
- Security audit difficulty - hidden toolchains are harder to inspect

### Option 3: System Perl Only (No Bootstrap)

Require users to have Perl and cpanm pre-installed. The cpan_install action would use system Perl.

```toml
[metadata]
name = "ack"

[[steps]]
action = "cpan_install"
distribution = "App-Ack"
executables = ["ack"]
```

**Pros:**
- Simplest implementation
- No runtime management burden
- Smaller disk usage

**Cons:**
- Violates tsuku's self-contained philosophy (core design principle)
- Many modern systems don't have Perl pre-installed
- System Perl version varies, causing compatibility issues
- Can't guarantee cpanm is available or at a compatible version
- "Works on my machine" problems - behavior differs across environments

**Note:** This option serves as a baseline to highlight why tsuku's self-contained philosophy matters. It represents the minimal-implementation approach and is rejected for violating core principles.

### Evaluation Against Decision Drivers

| Driver | Option 1 (Explicit) | Option 2 (Hidden) | Option 3 (System) |
|--------|---------------------|-------------------|-------------------|
| Self-contained | Good | Good | Fails |
| Isolation | Good | Good | Fair |
| Consistency | Good | Fair | N/A |
| User experience | Good | Excellent | Poor |
| Maintainability | Good | Fair | Good |
| Debuggability | Good | Poor | Fair |

### Uncertainties

- **Relocatable-perl stability**: We haven't validated the reliability of relocatable-perl releases or their release cadence. Initial testing should verify the binaries work correctly.
- **XS compilation**: Some popular Perl tools may require XS modules. The scope excludes these for v1, but coverage impact is unknown.
- **Executable discovery accuracy**: The heuristic of transforming distribution names to executables may fail for non-standard naming. Need to measure accuracy across popular tools.
- **CPAN mirror availability**: Using only MetaCPAN may be limiting for users in restricted networks.
- **cpanm --notest security**: Using `--notest` skips distribution tests for faster installation, but this also skips any security-related tests. For untrusted distributions, this could allow vulnerable code paths.

## Decision Outcome

**Chosen option: Option 1 (cpan_install Action with Explicit Perl Dependency)**

This option best balances self-contained operation with consistency and implementation simplicity. The explicit dependency model follows established patterns (npm_install depends on nodejs, go_install depends on go) and provides clear visibility into what's installed.

### Rationale

This option was chosen because:

1. **Consistency with existing patterns**: go_install depends on go, npm_install depends on nodejs. Following the same pattern for Perl maintains conceptual consistency.

2. **Transparency**: Users can see that Perl is installed, inspect its version, and understand the dependency chain.

3. **Implementation simplicity**: Adding a cpan_install action and a Perl recipe is straightforward. The existing dependency resolution handles the bootstrap automatically.

4. **Debuggability**: When something goes wrong, users can verify the Perl installation, run cpanm directly, and diagnose issues.

### Alternatives Rejected

- **Option 2 (Hidden Bootstrap)**: Inconsistent with go_install and npm_install patterns. Hidden state creates maintenance burden and user confusion.

- **Option 3 (System Perl)**: Violates tsuku's core philosophy. Many users won't have Perl installed, making this a non-starter.

### Trade-offs Accepted

By choosing this option, we accept:

1. **Toolchain visibility**: Perl appears in `tsuku list`. This is consistent with other runtimes.

2. **Update responsibility**: Users must run `tsuku update perl` separately from updating Perl tools.

3. **Disk usage**: Perl runtime (~50MB) plus per-tool modules. Acceptable given the value provided.

## Solution Architecture

### Overview

Perl ecosystem support consists of four components:

1. **Perl Runtime Recipe**: Downloads relocatable-perl as an explicit dependency
2. **CPAN Version Provider**: Queries MetaCPAN API for distribution version resolution
3. **cpan_install Action**: Executes cpanm with local::lib isolation and generates wrapper scripts
4. **CPAN Builder**: Generates recipes for CPAN distributions

```
User: tsuku install ack
         |
         v
+-------------------+
| Recipe Loader     |
+-------------------+
         |
         v (ack recipe has dependencies = ["perl"])
+-------------------+
| Dependency Resolver|
+-------------------+
         |
         +-------------------+
         | (if Perl not installed)
         v
+-------------------+       +-------------------+
| Install Perl      | ----> | Relocatable Perl  |
| (download_archive)|       | $TSUKU_HOME/tools/perl-{version}/
+-------------------+       +-------------------+
         |
         v
+-------------------+       +-------------------+
| cpan_install      | ----> | local::lib        |
| App-Ack           |       | PERL5LIB isolation|
+-------------------+       +-------------------+
         |
         v
+-------------------+
| ack wrapper script|
| $TSUKU_HOME/bin/  |
+-------------------+
```

### Components

#### Perl Runtime Recipe (`perl.toml`)

Standard tsuku recipe using `download_archive` action to fetch relocatable-perl:

```toml
[metadata]
name = "perl"
description = "Perl programming language with cpanm"
homepage = "https://www.perl.org"
tier = 1

[version]
provider = "github_releases:skaji/relocatable-perl"

[[steps]]
action = "download_archive"
url = "https://github.com/skaji/relocatable-perl/releases/download/{version}/perl-{os}-{arch}.tar.xz"
archive_format = "tar.xz"
strip_dirs = 1
binaries = ["bin/perl", "bin/cpanm"]
install_mode = "directory"
os_mapping = { linux = "linux", darwin = "darwin" }
arch_mapping = { amd64 = "amd64", arm64 = "arm64" }

[verify]
command = "{install_dir}/bin/perl -v"
pattern = "perl 5"
```

#### CPAN Version Provider (`internal/version/provider_metacpan.go`)

Queries MetaCPAN API to resolve distribution versions:

```go
type MetaCPANProvider struct {
    resolver         *Resolver
    distributionName string
}

func (p *MetaCPANProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
    // GET https://fastapi.metacpan.org/v1/release/{distribution}
    // Returns JSON with version, download_url, etc.
}

func (p *MetaCPANProvider) ListVersions(ctx context.Context) ([]string, error) {
    // POST https://fastapi.metacpan.org/v1/release/_search
    // Query for all releases of distribution, extract versions
}

// Validate distribution name format
// Pattern: Letter followed by letters, numbers, hyphens
// Reject module names containing "::" - those need conversion
func isValidDistribution(name string) bool
```

**Distribution vs Module Name Handling:**
- Users may provide module names (`App::Ack`) or distribution names (`App-Ack`)
- Module names contain `::`, distribution names use `-`
- The provider should accept either and normalize to distribution format

#### cpan_install Action (`internal/actions/cpan_install.go`)

Executes cpanm with local::lib isolation:

```go
type CpanInstallAction struct{}

func (a *CpanInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    distribution := params["distribution"].(string)  // e.g., "App-Ack"
    version := params["version"].(string)            // e.g., "3.7.0" or "" for latest
    executables := params["executables"]             // e.g., ["ack"]

    // Find cpanm from perl dependency
    cpanmPath := findCpanm(ctx)
    perlPath := findPerl(ctx)

    // Validate distribution name
    if !isValidDistribution(distribution) {
        return fmt.Errorf("invalid distribution name: %s", distribution)
    }

    // Build install target
    target := distribution
    if version != "" {
        target = distribution + "@" + version
    }

    // Clear ALL Perl environment variables to prevent contamination
    // PERL5LIB, PERL_MM_OPT, PERL_MB_OPT, PERL_LOCAL_LIB_ROOT, PERL5OPT
    cleanEnv := []string{}
    for _, e := range os.Environ() {
        if !strings.HasPrefix(e, "PERL") {
            cleanEnv = append(cleanEnv, e)
        }
    }

    // Execute cpanm with local::lib
    cmd := exec.CommandContext(ctx.Context, cpanmPath,
        "--local-lib", ctx.InstallDir,
        "--notest",  // Skip tests for faster installation (see Security note)
        target,
    )
    cmd.Env = cleanEnv

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("cpanm failed: %w", err)
    }

    // Generate wrapper scripts for each executable
    for _, exe := range executables {
        if err := generateWrapper(ctx, exe, perlPath); err != nil {
            return err
        }
    }

    return nil
}

func generateWrapper(ctx *ExecutionContext, exe string, perlPath string) error {
    // Validate executable name (prevent injection)
    if !isValidExecutableName(exe) {
        return fmt.Errorf("invalid executable name: %s", exe)
    }

    binDir := filepath.Join(ctx.InstallDir, "bin")
    exePath := filepath.Join(binDir, exe)

    // Rename original cpanm-generated script (following gem_install pattern)
    cpanmWrapperPath := exePath + ".cpanm"
    if err := os.Rename(exePath, cpanmWrapperPath); err != nil {
        return fmt.Errorf("failed to rename cpanm script: %w", err)
    }

    // Wrapper script sets PERL5LIB before executing
    wrapper := fmt.Sprintf(`#!/bin/bash
SCRIPT_PATH="${BASH_SOURCE[0]}"
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
    SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
    [[ $SCRIPT_PATH != /* ]] && SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_PATH"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
INSTALL_DIR="$(dirname "$SCRIPT_DIR")"

export PERL5LIB="$INSTALL_DIR/lib/perl5"
export PATH="%s:$PATH"
exec perl "$SCRIPT_DIR/%s.cpanm" "$@"
`, filepath.Dir(perlPath), exe)

    return os.WriteFile(exePath, []byte(wrapper), 0755)
}

// isValidExecutableName validates executable names to prevent injection
// Rejects: path separators, shell metacharacters, control characters
func isValidExecutableName(name string) bool {
    if name == "" || len(name) > 256 {
        return false
    }
    for _, c := range name {
        // Reject control characters
        if c < 32 || c == 127 {
            return false
        }
        // Reject shell metacharacters and path separators
        switch c {
        case '/', '\\', ':', '.', '$', '`', '|', ';', '&', '<', '>', '(', ')', '[', ']', '{', '}', '"', '\'', '\n', '\r', '\t':
            return false
        }
    }
    return true
}
```

**Directory Structure Created:**
```
$TSUKU_HOME/tools/ack-3.7.0/
├── bin/
│   ├── ack           # Wrapper script (sets PERL5LIB, calls ack.cpanm)
│   └── ack.cpanm     # Original cpanm-installed script (renamed)
├── lib/perl5/        # Installed modules
│   └── App/
│       └── Ack.pm
└── man/              # Man pages
```

#### CPAN Builder (`internal/builders/cpan.go`)

Generates recipes for CPAN distributions:

```go
type CPANBuilder struct {
    resolver *version.Resolver
}

func (b *CPANBuilder) Name() string {
    return "cpan"
}

func (b *CPANBuilder) CanBuild(ctx context.Context, packageName string) (bool, error) {
    // Check if it looks like a CPAN distribution or module name
    // Accept: App-Ack, App::Ack, ack (will search)
    return true, nil
}

func (b *CPANBuilder) Build(ctx context.Context, packageName string, version string) (*BuildResult, error) {
    // 1. Normalize name (App::Ack -> App-Ack)
    distribution := normalizeDistribution(packageName)

    // 2. Fetch metadata from MetaCPAN
    metadata, err := fetchMetadata(distribution)
    if err != nil {
        return nil, err
    }

    // 3. Infer executable name
    executable := inferExecutable(distribution)

    // 4. Construct recipe
    recipe := &recipe.Recipe{
        Metadata: recipe.Metadata{
            Name:         executable,
            Description:  metadata.Abstract,
            Homepage:     "https://metacpan.org/dist/" + distribution,
            Dependencies: []string{"perl"},
        },
        Version: recipe.VersionConfig{
            Provider: "metacpan:" + distribution,
        },
        Steps: []recipe.Step{{
            Action: "cpan_install",
            Params: map[string]interface{}{
                "distribution": distribution,
                "executables":  []string{executable},
            },
        }},
        Verify: recipe.VerifyConfig{
            Command: executable + " --version",
        },
    }

    warnings := []string{}
    if executable != strings.ToLower(distribution) {
        warnings = append(warnings,
            fmt.Sprintf("Inferred executable name '%s' from distribution '%s'; verify this is correct",
                executable, distribution))
    }

    return &BuildResult{
        Recipe:   recipe,
        Warnings: warnings,
        Source:   "metacpan:" + distribution,
    }, nil
}

func normalizeDistribution(name string) string {
    // App::Ack -> App-Ack
    return strings.ReplaceAll(name, "::", "-")
}

func inferExecutable(distribution string) string {
    // Common patterns:
    // App-Ack -> ack
    // App-cpanminus -> cpanm (special case)
    // Perl-Critic -> perlcritic

    name := distribution

    // Remove App- prefix
    if strings.HasPrefix(name, "App-") {
        name = strings.TrimPrefix(name, "App-")
    }

    // Convert to lowercase and replace hyphens
    name = strings.ToLower(name)
    name = strings.ReplaceAll(name, "-", "")

    return name
}
```

### Data Flow

**Installing a Perl tool:**
```
1. tsuku install ack
2. Loader.Get("ack") -> Recipe with dependencies=["perl"]
3. DependencyResolver checks if "perl" is installed
4. If not: Install "perl" recipe (download relocatable-perl)
5. Executor runs cpan_install action:
   a. Find cpanm at $TSUKU_HOME/tools/perl-{version}/bin/cpanm
   b. Run: cpanm --local-lib $TSUKU_HOME/tools/ack-3.7.0 App-Ack
   c. Generate wrapper script at $TSUKU_HOME/tools/ack-3.7.0/bin/ack
6. Verify: ack --version
7. Create symlink: $TSUKU_HOME/bin/ack -> ../tools/ack-3.7.0/bin/ack
```

**Creating a recipe for a CPAN distribution:**
```
1. tsuku create App::Ack --from cpan
2. CPANBuilder.CanBuild("App::Ack") -> true
3. CPANBuilder.Build("App::Ack", ""):
   a. Normalize: App::Ack -> App-Ack
   b. Query MetaCPAN for metadata
   c. Infer executable: ack
   d. Generate recipe with cpan_install action
4. Write to $TSUKU_HOME/recipes/ack.toml
5. User runs: tsuku install ack
```

## Implementation Approach

### Phase 1: Perl Runtime Recipe

**Deliverables:**
- `perl.toml` recipe in `internal/recipe/recipes/p/`
- Uses GitHub releases provider for skaji/relocatable-perl
- Platform-specific URL templates
- Tests for Perl installation and cpanm availability

**Validation:** `tsuku install perl` downloads relocatable-perl, `perl -v` and `cpanm --version` work.

### Phase 2: CPAN Version Provider

**Deliverables:**
- `internal/version/provider_metacpan.go`
- Queries MetaCPAN API for latest release
- Elasticsearch query for version history
- Distribution name validation (reject module names with `::`)
- Factory integration

**Validation:** `tsuku versions App-Ack` lists versions from MetaCPAN.

### Phase 3: cpan_install Action

**Deliverables:**
- `internal/actions/cpan_install.go`
- local::lib isolation via cpanm
- Wrapper script generation (following gem_install pattern)
- Distribution name validation
- Find perl/cpanm from installed dependency

**Validation:** Manual recipe with `cpan_install` action successfully installs a Perl tool.

### Phase 4: CPAN Builder

**Deliverables:**
- `internal/builders/cpan.go`
- Module-to-distribution name normalization
- Executable name inference
- Recipe generation with perl dependency
- Integration with `tsuku create`

**Validation:** `tsuku create App-Ack --from cpan` generates working recipe.

### Phase 5: Popular Tool Recipes

**Deliverables:**
- Recipes for popular Perl CLI tools in `internal/recipe/recipes/`
- Tools: ack, cpanm, carton, plenv, cpm, prove, perltidy, perlcritic

**Validation:** All tools install and verify successfully.

### Phase 6: Integration Tests

**Deliverables:**
- `integration_test.go` additions for Perl tool installation
- Test: Install perl dependency, then install a Perl tool via cpan_install
- Test: Verify wrapper script correctly sets PERL5LIB
- Test: Verify tool executes correctly after installation

**Validation:** `go test -tags=integration ./...` passes for Perl tests.

## Security Considerations

### Download Verification

**Perl Runtime Downloads:**
- **Source**: GitHub releases from skaji/relocatable-perl
- **Verification**: GitHub release integrity; optionally verify against published checksums
- **Failure behavior**: If download fails or is corrupted, installation aborts

**CPAN Module Downloads (via cpanm):**
- **Source**: CPAN mirrors via MetaCPAN
- **Verification**: cpanm handles CHECKSUMS file verification automatically
- **Failure behavior**: cpanm aborts if checksums don't match

**Version Provider Queries:**
- **Source**: fastapi.metacpan.org
- **Verification**: HTTPS ensures transport security; response validation before parsing
- **Failure behavior**: Invalid responses rejected; version resolution fails with error

### Execution Isolation

**File System Access:**
- **Scope**: Write access to `$TSUKU_HOME/tools/`, `$TSUKU_HOME/bin/`
- **Controlled via**: `--local-lib` flag to cpanm
- **Outside scope**: No access to system Perl installation or global @INC

**Network Access:**
- **Required**: fastapi.metacpan.org for API queries, cpan.metacpan.org for downloads
- **No elevated privileges**: All operations run as current user

**Process Isolation:**
- **cpanm execution**: Runs as subprocess with controlled environment
- **PERL5LIB cleared**: Prevent system module contamination
- **No privilege escalation**: All operations are unprivileged

### Supply Chain Risks

**Perl Runtime Supply Chain:**
- **Trust model**: Trust skaji/relocatable-perl as source
- **Authenticity**: GitHub release integrity
- **Compromise scenario**: If relocatable-perl releases are compromised, malicious Perl could be distributed
- **Mitigation**: Monitor project health; consider checksums in recipe

**CPAN Module Supply Chain:**
- **Trust model**: Same as any use of cpanm - trust the distribution author
- **Authenticity**: CPAN CHECKSUMS files provide tamper detection
- **Compromise scenario**: A malicious distribution could execute arbitrary code during installation
- **Mitigation**: This is inherent to cpanm - same risk as using it directly

**Distribution Name Injection:**
- **Risk**: User provides malicious distribution name with shell metacharacters
- **Mitigation**: Validate distribution names against strict pattern before passing to cpanm
- **Pattern**: `^[A-Za-z][A-Za-z0-9_-]*$`

### User Data Exposure

**Data Accessed Locally:**
- Distribution names (from recipe or user input)
- Version preferences (from user input or recipe)
- No access to user source code or personal files

**Data Transmitted Externally:**
- Distribution names sent to MetaCPAN as URL path components
- User-Agent header identifying tsuku version
- IP address visible to MetaCPAN and CPAN mirrors

**Privacy Implications:**
- Package registries see which distributions users query/install (same as using cpanm directly)
- No telemetry beyond standard HTTP requests

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious Perl runtime | Use established relocatable-perl project; add checksum verification | Project compromise |
| Malicious CPAN module | cpanm's CHECKSUMS verification | Malicious code in legitimate module |
| Distribution name injection | Validate names before use | Novel injection patterns |
| Executable name injection | Validate names, reject shell metacharacters | None if implemented correctly |
| Build-time code execution | Inherent to cpanm | Cannot prevent malicious build scripts |
| Environment contamination | Clear all PERL* env vars | None if implemented correctly |
| Network eavesdropping | HTTPS for all connections | Compromised CA certificates |
| --notest skips tests | Document risk; consider making configurable | Malicious code undetected in tests |

### Security Best Practices for Implementation

1. **Input validation**: Validate distribution names match `^[A-Za-z][A-Za-z0-9_-]*$`. Convert module names containing `::` to distribution format before validation.
2. **Environment isolation**: Clear ALL PERL* environment variables (PERL5LIB, PERL_MM_OPT, PERL_MB_OPT, PERL_LOCAL_LIB_ROOT, PERL5OPT) before running cpanm.
3. **Wrapper script safety**: Validate executable names using `isValidExecutableName()` before generating wrapper scripts. Reject path separators, shell metacharacters, and control characters.
4. **HTTPS only**: All network requests use HTTPS.
5. **Error transparency**: Surface cpanm errors to users.
6. **Bash verification**: Verify `/bin/bash` exists before generating wrapper scripts.
7. **Checksum verification**: Add checksum verification to Perl runtime recipe using relocatable-perl's SHA256SUMS.

**Note on --notest**: The cpan_install action uses `--notest` by default for faster installation. This skips distribution tests, which could detect malicious behavior. For security-sensitive environments, consider making this configurable via a `skip_tests` parameter. This is a performance/security tradeoff documented here as a conscious design decision.

## Consequences

### Positive

1. **Complete Perl ecosystem coverage**: Any CPAN distribution can be installed via cpanm.
2. **Self-contained**: Users don't need Perl pre-installed; tsuku handles bootstrapping.
3. **Consistent patterns**: Follows existing dependency patterns (like go_install, npm_install).
4. **Version resolution**: MetaCPAN integration enables `tsuku versions` and `@version` syntax.
5. **Builder compatibility**: CPAN builder follows recipe-builders design.

### Negative

1. **Installation time**: cpanm installs from source, which can be slow for large modules.
2. **Disk usage**: Perl runtime (~50MB) plus per-tool modules.
3. **XS limitation**: Tools requiring XS compilation with system libraries won't work.
4. **Executable discovery**: Heuristic may fail for non-standard distribution naming.

### Mitigations

1. **Installation time**: Display progress during installation. Most CLI tools are relatively small.
2. **Disk usage**: Document in user guide. Perl is comparable to Node.js or Python.
3. **XS limitation**: Document clearly. Suggest system package manager for XS-dependent tools.
4. **Executable discovery**: Clear warnings when using heuristics. Users can edit generated recipes.

## Implementation Issues

### Milestone: [Perl Ecosystem Support](https://github.com/tsukumogami/tsuku/milestone/6)

**Completed:**
- [#129](https://github.com/tsukumogami/tsuku/issues/129): feat(version): add CPAN version provider
- [#130](https://github.com/tsukumogami/tsuku/issues/130): feat(actions): add cpan_install action
- [#131](https://github.com/tsukumogami/tsuku/issues/131): feat(builders): add CPAN builder

**Remaining:**
- Perl runtime recipe exists at `internal/recipe/recipes/p/perl.toml`
- Popular Perl tool recipes: perlcritic, perltidy, carton at `internal/recipe/recipes/`
- Additional tools (ack, cpanm, plenv, cpm, prove) may be added as needed

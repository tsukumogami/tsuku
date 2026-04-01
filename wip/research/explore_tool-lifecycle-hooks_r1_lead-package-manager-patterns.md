# Lead: What lifecycle hooks do other package managers support?

## Findings

### Homebrew

**Lifecycle Events:**
- `post_install` - Runs after package installation, can be re-executed with `brew postinstall <formula>`
- `test do` - Automated testing block executed with `brew test`
- `caveats` - User-facing warnings displayed post-installation
- `service` - launchd (macOS) or systemd (Linux) service definitions
- `head do` - Development/cutting-edge version handling
- `stable do` - Stable release-specific configurations

**Hook Declaration:** In-package, within formula file (Ruby DSL)

**Capabilities:** Setup commands, data directory creation, file manipulation, service registration, user notifications

**Security Model:** Trusted (formulas reviewed by maintainers), can be skipped with `--skip-post-install`

**Source:** [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)

---

### dpkg/apt (Debian/Ubuntu)

**Lifecycle Events:**
- `preinst` - Before package files unpacked
- `postinst` - After unpacking, configuration phase
- `prerm` - Before package removal
- `postrm` - After package removal/purging

**Hook Declaration:** External shell scripts in control directory (included in `.deb` archive)

**Capabilities:** Stop/restart services, user prompts, configuration, cleanup. Scripts receive action parameters (install, upgrade, configure, remove, purge)

**Security Model:** Trusted (maintainer scripts), must be idempotent, can fail with recovery states

**Additional Details:** Scripts can leave system in intermediate states for error recovery. Return 0 for success, non-zero for failure.

**Sources:** 
- [Debian Policy Manual - Maintainer Scripts](https://www.debian.org/doc/debian-policy/ch-maintainerscripts.html)
- [MaintainerScripts - Debian Wiki](https://wiki.debian.org/MaintainerScripts)

---

### Nix

**Lifecycle Events:**
- Activation scripts - System-level configuration during system activation/rebuild (idempotent, ordered)
- Post-build hooks - Run after each build (not package-level, but build-level)
- Home-manager activation scripts - User-level setup

**Hook Declaration:** In-package expressions (Nix language), or system-level configuration

**Capabilities:** Environment setup, /etc modifications, user/group creation, ordering dependencies between scripts

**Security Model:** Declarative and reproducible (can be inspected). Build hooks block further builds if they fail.

**Important Distinction:** Nix does NOT have traditional post-install hooks like other package managers. Activation scripts are different—they're idempotent system-level operations that run on every rebuild, not one-time package installation hooks.

**Sources:**
- [NixOS Activation Scripts](https://discourse.nixos.org/t/postinstall-or-hook/33994)
- [Post-Build Hook Documentation](https://nix.dev/manual/nix/2.18/advanced-topics/post-build-hook)
- [Nix Pills - Basic Dependencies and Hooks](https://nixos.org/guides/nix-pills/20-basic-dependencies-and-hooks.html)

---

### Cargo (Rust)

**Lifecycle Events:**
- `build.rs` - Build script executed before compilation (not after installation)
- No built-in post-install hooks for `cargo install`

**Hook Declaration:** As separate Rust script in package root

**Capabilities:** Compile-time code generation, environment variable setting, conditional compilation

**Security Model:** Trusted (package developer), same sandbox as package build

**Important Limitation:** Feature request exists for post-install hooks to allow custom actions after binaries are installed. Currently, no mechanism exists to run code after `cargo install`.

**Sources:** 
- [Build Scripts - The Cargo Book](https://doc.rust-lang.org/cargo/reference/build-scripts.html)
- [Post `cargo install` Hook - GitHub Issue](https://github.com/rust-lang/cargo/issues/11539)

---

### asdf

**Lifecycle Events:**
- `post_asdf_install_<plugin>` - After plugin installation
- `post_asdf_reshim_<plugin>` - After reshim operation
- `pre_asdf_install_<plugin>` / `pre_asdf_uninstall_<plugin>` - Before operations

**Hook Declaration:** In `.asdfrc` configuration file (not in-package)

**Capabilities:** Reshim operations (regenerate symlinks for newly available executables), post-install setup

**Security Model:** User-configured (not package-defined), hooks in system configuration

**Special Mechanism - Reshim:** When tools (like npm packages) add new executables that weren't installed via the plugin lifecycle, `asdf reshim <plugin> <version>` recalculates shims for those new executables.

**Sources:** [asdf Plugin Creation](https://asdf-vm.com/plugins/create.html)

---

### mise

**Lifecycle Events:**
- `PostInstall` hook - Tool-plugin level (receives rootPath, runtimeVersion, sdkInfo)
- `hooks.postinstall` - Configuration-level (receives MISE_INSTALLED_TOOLS JSON array)
- Tool-level `postinstall` option - Individual tool post-install scripts

**Hook Declaration:** In-package (tool plugins) or in configuration file (`.mise.toml` or `.mise.local.toml`)

**Capabilities:** Set file permissions, move files, run setup scripts after tool installation

**Context Variables:** MISE_ORIGINAL_CWD, MISE_PROJECT_ROOT, MISE_PREVIOUS_DIR, MISE_TOOL_NAME, MISE_TOOL_VERSION, MISE_TOOL_INSTALL_PATH

**Special Property:** Post-install hooks don't require `mise activate` to run

**Sources:** [mise Hooks Documentation](https://mise.jdx.dev/hooks.html)

---

### Chocolatey (Windows)

**Lifecycle Events:**
- `<pre|post>-<install|beforemodify|uninstall>-<packageID|all>.ps1`
  - pre-install-all
  - post-install-all
  - pre-install-<packageID>
  - post-install-<packageID>
  - Similar patterns for beforemodify and uninstall

**Hook Declaration:** Hook package (separate package that installs scripts to hooks directory)

**Capabilities:** Remove undesired shortcuts, apply configuration after upgrades, logging/analytics, cleanup operations

**Security Model:** Global hooks apply to all subsequent Chocolatey operations, set up as hook packages

**Execution:** Runs before/after the main chocolateyInstall.ps1, chocolateyBeforeModify.ps1, or chocolateyUninstall.ps1

**Sources:** 
- [How To Create a Hook Package](https://docs.chocolatey.org/en-us/guides/create/create-hook-package/)
- [Extend Chocolatey With PowerShell Scripts (Hooks)](https://docs.chocolatey.org/en-us/features/hook/)

---

### npm/Yarn/pnpm

**Lifecycle Events:**
- `preinstall` - Before package installation
- `postinstall` - After package installation
- `preuninstall` / `postuninstall` - Around removal
- `prepare` - Before tarball is packed/after clone

**Hook Declaration:** In-package `package.json` scripts field

**Capabilities:** Arbitrary shell commands, native module compilation, setup scripts, environment configuration

**Security Model:** Well-known security concern—malicious postinstall scripts can execute arbitrary code. Users can use `npm install --ignore-scripts` to prevent execution.

**Execution Order:** Topological—dependencies' postinstall scripts run before the package's own

**Sources:** 
- [npm Scripts Documentation](https://docs.npmjs.com/cli/v11/using-npm/scripts/)
- [Yarn Lifecycle Scripts](https://yarnpkg.com/advanced/lifecycle-scripts)
- [Understanding and Protecting Against Malicious npm Lifecycle Scripts](https://medium.com/@kyle_martin/understanding-and-protecting-against-malicious-npm-package-lifecycle-scripts-8b6129619d7c)

---

### Python/setuptools/pip

**Lifecycle Events:**
- `PostInstallCommand` class (deprecated approach) - Custom installation commands
- Entry points - Modern alternative for script installation
- PEP 660 editable install hooks - For development installs

**Hook Declaration:** In `setup.py` or `setup.cfg` (source distributions only)

**Limitation:** NO post-install hooks for wheels (standard distribution format). Hooks only work with:
- Source distributions (.tar.gz, .zip)
- Editable installs (`pip install -e .`)

**Why Limited:** Wheel installation is controlled by pip/uv, which don't expose post-install hook mechanisms

**Modern Approach:** Entry points generate executable scripts automatically at install time

**Sources:** 
- [Development Mode - setuptools Documentation](https://setuptools.pypa.io/en/latest/userguide/development_mode.html)
- [Extending or Customizing Setuptools](https://setuptools.pypa.io/en/latest/userguide/extension.html)

---

### pkgsrc (NetBSD)

**Lifecycle Events:** Limited information found, but pkgsrc does include:
- Post-installation checks (installed files, shared libraries, script interpreters)
- Different build phases (pre-build, post-build configurable via PKGCONFIG_OVERRIDE_STAGE)

**Hook Declaration:** In Portfile (pkg-specific build instructions)

**Capabilities:** Build phase management, package installation validation

**Note:** Detailed technical documentation on pkgsrc lifecycle hooks not readily available in public sources.

**Sources:** [The pkgsrc guide](https://www.netbsd.org/docs/pkgsrc/pkgsrc.html)

---

### MacPorts

**Lifecycle Events:**
- Port phases: configure, make, make install
- Post-installation: launchd .plist registration for daemon management

**Hook Declaration:** In Portfile (port-specific build instructions)

**Capabilities:** Service/daemon installation via launchd registration

**Note:** Limited detailed documentation on specific lifecycle hook patterns in search results.

---

## Cross-Cutting Patterns

### Universal Concepts
1. **Four-Phase Model** (most package managers):
   - Pre-install/Pre-upgrade
   - Post-install/Post-upgrade
   - Pre-remove/Pre-uninstall
   - Post-remove/Post-uninstall

2. **Declaration Approaches**:
   - **In-package**: Homebrew (Ruby), dpkg (shell), Nix (Nix), npm (JSON), asdf (plugin code)
   - **External configuration**: asdf (.asdfrc), Chocolatey (hook packages), mise (.mise.toml)

3. **Execution Model**:
   - **Imperative shell scripts**: dpkg, Homebrew, Chocolatey (PowerShell), npm
   - **Declarative**: Nix, mise
   - **Custom commands**: Python/setuptools (class-based)

4. **Error Recovery**:
   - dpkg: Intermediate states (Half-Installed, Failed-Config) allow recovery
   - Most others: Fail quickly, require manual recovery or re-run

5. **Idempotency Requirement**:
   - Nix: REQUIRED (scripts run on every rebuild)
   - dpkg: REQUIRED (for error recovery)
   - Others: Recommended but not enforced

### Security Variations
- **Trusted model** (Homebrew, dpkg): Reviewed by maintainers
- **User-configurable** (asdf, Chocolatey): User controls which hooks run
- **Well-known risk** (npm): Known security attack vector, can be disabled
- **Limited exposure** (Cargo): No post-install hooks yet
- **Restricted** (Python/pip): Wheels have no hooks, limiting exploit surface

### What's Unique
- **asdf/mise**: Reshim concept—regenerating symlinks after tools add new executables
- **Nix**: Activation scripts designed for idempotency and reproducibility, not one-time install
- **Chocolatey**: PowerShell-based, hook packages as separate installable units
- **npm**: Topological ordering guarantees
- **dpkg**: Intermediate state recovery model

---

## Implications for tsuku

### Tsuku's Scenario
Tsuku installs tools by:
1. Downloading binaries
2. Symlinking them into place

But tools sometimes need:
- Shell functions/completions
- Environment variable setup
- Service registration
- Cleanup on removal
- Pre-upgrade migrations

### Design Considerations

**1. Minimum Viable Hooks**
Based on patterns, tsuku should support:
- `post-install` - Setup shell integrations, completions, env vars
- `pre-remove` - Cleanup, service teardown
- `pre-upgrade` - Migrations, version-specific prep
- `post-upgrade` - Verification, updated completions

**2. Declaration Model**
- **Recommendation: In-package** (like Homebrew, asdf)
  - Each tool's manifest declares its own hooks
  - Simpler than external configuration (asdf .asdfrc)
  - Travels with the tool definition
  
- **Format**: YAML/TOML alongside binary definition
  ```yaml
  hooks:
    post-install:
      script: |
        source "$TSUKU_INSTALL_PATH/shell-setup.sh"
    pre-remove:
      script: |
        rm "$HOME/.config/tool/config"
  ```

**3. Execution Context**
- **Environment variables** (like mise):
  - `TSUKU_TOOL_NAME`
  - `TSUKU_TOOL_VERSION`
  - `TSUKU_INSTALL_PATH` (symlink destination)
  - `TSUKU_BIN_PATH` (binary location)
  - `TSUKU_ORIGINAL_CWD`

**4. Capabilities**
- Run shell scripts (bash/sh)
- Write to user's home directory (completions, configs)
- Modify shell startup files (.bashrc, .zshrc)
- Register services (if applicable)
- Output user guidance (like Homebrew's caveats)

**5. Idempotency**
- Not required on every invocation (unlike Nix)
- But should be safe to re-run (tsuku reinstall <tool>)
- Track hook completion status to avoid redundant runs

**6. Security Model**
- Similar to Homebrew: trust package manifests (curated list)
- More restrictive than npm (no arbitrary code execution from untrusted sources)
- Optional skip flag: `tsuku install --skip-hooks <tool>`
- Sandboxing (future): run hooks in restricted shell environment

**7. Shell Integration Pattern**
- Rather than modifying startup files directly, tsuku could:
  - Generate completion files to a known location
  - Provide a `tsuku init` command users source in their shell
  - Let hooks output instructions for users to apply
  - (Similar to: `eval "$(mise activate bash)"`, asdf plugin hooks)

---

## Surprises

1. **Python/pip lacks post-install hooks for wheels** - The standard format has no hook support, limiting customization
2. **Nix doesn't have traditional package post-install hooks** - Uses activation scripts instead (different paradigm)
3. **Cargo has no built-in post-install mechanism** - Considered a missing feature, but no rush to add it
4. **asdf's reshim concept** - Unique pattern where plugins can regenerate symlinks for newly available tools
5. **npm's security risk is *by design*** - Postinstall hooks are the standard way to do native compilation, making them impossible to deprecate despite security concerns

---

## Open Questions

1. **How should tsuku handle shell env setup?**
   - Modify user's dotfiles directly?
   - Generate files in `$TSUKU_CONFIG`?
   - Output instructions for users to source?

2. **Should hooks be optional in tool manifests?**
   - Always required (simpler design)?
   - Optional with fallback behavior?

3. **How to version/update hooks?**
   - Hooks change with tool version updates?
   - Should `tsuku upgrade <tool>` re-run post-install hooks?

4. **Pre-upgrade migrations - how to declare?**
   - Tool-specific migration scripts?
   - Or rely on the tool's own upgrade mechanism?

5. **Should hook execution be logged/audited?**
   - Track what hooks ran, when, with what output?
   - Important for troubleshooting and security transparency?

6. **Parallelization?**
   - Can multiple tool hooks run concurrently?
   - Are there ordering/dependency constraints?

---

## Summary

Lifecycle hooks are nearly universal across package managers, with four standard events (pre/post install, pre/post remove) appearing in most systems. The most relevant patterns for tsuku are: (1) **in-package hook declaration** like Homebrew and asdf, where each tool defines its own lifecycle scripts; (2) **shell integration via scripts** that can write completions, environment variables, and setup instructions; and (3) **environment variables for context** (tool name, version, install path) like mise provides. Unlike npm (security risk), Cargo (missing), or Python wheels (restricted), tsuku should follow Homebrew's trusted-manifest model with optional skip flags, avoiding the need for post-install hooks to verify or remediate trusted sources.

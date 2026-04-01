# Lead: What specific post-install configuration do tools need beyond binary symlinking?

## Findings

### Configuration Categories in the Tsuku Recipe Registry

Based on systematic analysis of 1,400 recipes in the registry, tools requiring post-install configuration beyond binary symlinking fall into five primary categories:

#### 1. Shell Integration & Environment Setup (High Priority)
**Critical for basic functionality**

Tools that need eval-init pattern (source shell function wrapper):
- **direnv** - Environment switcher; requires shell hook to activate on cd
- **zoxide** - Directory jumper; requires eval wrapper to intercept `cd` replacements
- **asdf** - Version manager (Ruby, Node, Erlang); requires shell initialization for runtime switching
- **mise** - Dev tools version manager; requires eval-init for shim redirection

These tools *do not work* without shell integration. Symlinking the binary alone is insufficient—users cannot use the tool's core functionality (environment switching, runtime selection) without manual shell setup.

**Current state**: Recipes exist but provide no post-install shell registration mechanism. Users must manually add to .bashrc/.zshrc.

**Estimated impact**: ~8-12 tools in registry that *require* eval-init; ~30-50 additional tools that would *benefit* from it (language managers, prompt engines, completion orchestrators).

#### 2. Completion Generation & Shell Extensions
**Nice-to-have but improves usability**

Tools that generate shell completions or need activation:
- **carapace** - Multi-shell argument completer; can generate completions for any cobra/urfave-cli tool
- **fzf** - Fuzzy finder; ships with optional shell keybindings and completions
- **oh-my-posh** - Prompt theme engine; needs shell init to activate
- **starship** (not found; likely in discovery/) - Prompt engine; needs eval-init

Most Cobra/urfave-cli tools (estimated ~300+ in registry) can generate completions via `tool completion bash/zsh/fish` but need post-install to register them in user's shell.

**Current state**: Recipes do not invoke completion generation or register output with system/shell.

**Estimated impact**: 52 npm tools, 23 cargo tools, 8 go tools, + all Cobra/urfave-cli binaries could benefit. At minimum, 200+ recipes could offer improved UX with completion registration.

#### 3. Language/Runtime Toolchain Setup
**Critical for toolchain packages; optional for standalone tools**

Tools that manage language environments or runtime versions:
- **rustup** - Rust toolchain installer; generates initialization script; should register in shell init
- **ghcup** - Haskell installer; similar pattern to rustup
- Language version managers (rbenv, nodenv, pyenv pattern from outside registry)

These install language runtimes and version management infrastructure. The tools themselves work via binary symlink, but users need environment variables and PATH setup to use managed versions effectively.

**Current state**: Installation places binary, but no automatic registration of version-manager initialization.

**Estimated impact**: ~5-10 language-specific installers in registry directly; ~94 language-dependent packages (npm, cargo, go, gem, pipx) would benefit if their parent runtimes were properly initialized.

#### 4. Configuration File Scaffolding
**Nice-to-have; depends on tool purpose**

Tools that need initial config files or setup:
- **editorconfig** - Coding style tool; can be used without config, but benefits from .editorconfig files
- **prettier**, **dprint**, formatters in general - Work without config, but need .prettierrc/.editorconfig for team setup
- Linters, language servers - Similar pattern

**Current state**: Tools can be invoked, but users must manually create config files or use defaults. No post-install scaffolding exists.

**Estimated impact**: ~100-150 development tools in registry would benefit from optional config scaffolding.

#### 5. Service/Daemon Registration & Man Pages
**Rare but important for specific tool classes**

Tools with daemon/service components:
- **dbus** - Message bus; may need service registration or socket setup
- **dropbear** - SSH server; needs service registration to run as daemon
- **gitea** - Git server; needs service setup
- **node-red** - Node.js visual editor; needs service registration

Man page registration:
- Many tools ship with man pages that would benefit from installation to system manpath
- No recipe action exists for man page placement

**Current state**: Binary installed, but service registration and man pages not handled.

**Estimated impact**: ~10-20 service/daemon tools in registry; hundreds of tools with bundled man pages.

### Recipe Distribution Summary

| Installation Method | Count | Likely Post-Install Needs |
|---|---|---|
| homebrew (pre-built binaries) | 1,186 | Varies; completions common, service registration rare |
| npm_install | 52 | Environment setup (NODE_PATH), completions |
| cargo_install | 23 | Environment setup (CARGO_HOME), PATH management |
| go_install | 8 | Environment setup (GOPATH), completions |
| github_archive | 72 | Completions, man pages, service registration |
| gem_install | 7 | Environment setup (GEM_HOME), bundler integration |
| pipx_install | 4 | Environment setup (PIPX_BIN_DIR), completions |
| **Total** | **1,400** | — |

## Implications

### Priority Ordering for Tsuku Lifecycle Hooks

**Tier 1 - Must Have (Enable core functionality)**
1. **Shell eval-init registration** - At least 8-12 critical tools cannot function without it. Direnv, zoxide, asdf, mise are clear examples. This is non-negotiable for the niwa use case and for practical tool management.
2. **Service/daemon hooks** - Tools like gitea, dbus need `post-install` hooks to set up services. Currently impossible.

**Tier 2 - Should Have (Improve user experience)**
3. **Completion generation & registration** - Estimated 200+ tools would benefit. Most Cobra/urfave-cli tools can auto-generate; need post-install to place them in /etc/bash_completion.d, ~/.fzf/completion, etc.
4. **Man page installation** - Hundreds of tools ship with man pages; automatic registration would improve discoverability.

**Tier 3 - Nice-to-Have (Developer convenience)**
5. **Configuration scaffolding** - Formatters, linters, language servers benefit from initial .prettierrc / .eslintrc templates. Users can create manually if needed.
6. **Environment variable registration** - Some tools (npm, cargo, pipx) work better with CARGO_HOME, NPM_CONFIG_USERCONFIG set in shell env. Can be done manually; post-install would be convenient.

### What the Registry Actually Contains

- **Most tools (85%) are stateless binaries** from Homebrew. They work via simple symlinking. Many of these have completion generation built-in (esp. Cobra tools) but don't currently register output.
- **Small but critical subset (5-10%) requires eval-init** for environment switching or runtime management. These are the highest-value targets for lifecycle hooks.
- **Medium subset (10-15%) would meaningfully benefit** from completion generation and service registration.
- **Low usage of run_command/set_env today** (only 1 recipe uses run_command explicitly; 0 use set_env). This suggests recipes are not yet designed with post-install in mind.

## Surprises

1. **Only one recipe uses run_command** (pipx) and none explicitly use set_env, despite both actions existing. This suggests post-install is not yet a design consideration in the registry. Most tools expect users to do manual setup.

2. **Homebrew dominates** (84.7% of recipes). This biases the registry toward "works out of the box" tools. If tsuku adds npm, cargo, gem, pipx recipes at scale (which would be useful), post-install configuration becomes much more critical.

3. **No man page handling at all** despite hundreds of binaries shipping with man pages. This is a missed UX improvement opportunity.

4. **Service/daemon tools exist but are unreachable.** Tools like gitea, dbus, node-red are in the registry but cannot be properly deployed without post-install hooks. They'd work better in systemd unit files or launchd plists, not raw binary symlinking.

5. **The completion generation gap is large:** Carapace exists (multi-shell completer), but no recipe uses it to generate completions for other tools. This suggests tools are expected to provide their own completion install steps, which very few do.

## Open Questions

1. **Should set_env-generated files be automatically sourced?** Currently set_env creates env.sh in the tool directory, but nothing loads it into the shell. Should post-install hooks also register these files in tsuku's own .bashrc hook (if one exists)? Or should users manually source them?

2. **How should lifecycle hooks interact with version management?** If a tool is installed in multiple versions (e.g., tsuku supports multiple tool versions), should shell init refer to the "latest" or "active" version? Should we track activation state separately?

3. **Should lifecycle hooks be recipe-version-specific?** If you upgrade direnv v2.30 → v2.31, should the shell init be re-run? Updated? The current upgrade flow is just "install new → symlink". Lifecycle awareness would require more.

4. **Who generates completions for tools that don't auto-generate?** Some simple binaries don't support `tool completion` subcommand. Do we manually write completion specs? Use carapace for inference?

5. **How many tools *actually* need post-install, vs. how many would be nicer with it?** We've identified 8-12 that *cannot work* without it. The remaining 200+ are UX improvements, not functionality blockers. How aggressive should tsuku be in implementing this?

## Summary

At least 8-12 critical tools in the tsuku registry (direnv, zoxide, asdf, mise, and language managers) cannot provide their core functionality without post-install shell integration; this is the highest-priority use case. Beyond that, 200+ additional tools would benefit from completion generation and registration, making this the second priority. Service/daemon registration, man page installation, and config scaffolding are less common but would unlock proper deployment of systemd services and improve discoverability.


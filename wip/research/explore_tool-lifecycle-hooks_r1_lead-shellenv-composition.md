# Lead: How should per-tool shell integration compose with tsuku shellenv?

## Findings

### Current State

**tsuku shellenv today:**
- Single static output: `export PATH="{TSUKU_HOME}/bin:{TSUKU_HOME}/tools/current:$PATH"`
- Designed for global tool version switching via symlinks
- No per-tool extension points or plugin architecture
- Eval-based: users call `eval "$(tsuku shellenv)"` in shell profiles

**tsuku hook system (command-not-found and activation):**
- Two independent marker blocks, installed via `tsuku hook install` and `tsuku hook install --activate`
- Source-a-managed-file pattern: rc files contain two-line blocks that source `$TSUKU_HOME/share/hooks/tsuku.{shell}`
- Hook files are embedded in the tsuku binary and written on install
- Activation hook calls `tsuku hook-env` on every prompt (PROMPT_COMMAND/precmd/fish_prompt)
- No per-tool hook mechanism; only tsuku's own hooks are installed

**tsuku tool installation:**
- Recipes define `[[steps]]` with action types (github_file, github_archive, etc.)
- No post-install hooks, pre-uninstall cleanup, or pre-upgrade migration phases
- Binaries are symlinked to `$TSUKU_HOME/tools/current/` after install
- No mechanism for tools to declare "I need shell functions" or "I need to eval init code"

**Per-tool integration needs (niwa, direnv, zoxide, starship):**
- **niwa:** Needs `eval "$(niwa init shell-name)"` to source shell functions and command wrappers
- **direnv:** Needs `eval "$(direnv hook shell-name)"` to hook `cd` and environment switching
- **zoxide:** Needs `eval "$(zoxide init shell-name)"` to replace `cd` with smart jumping
- **starship:** Needs `eval "$(starship init shell-name)"` to customize PROMPT_COMMAND/PS1
- All four are managed by tsuku's recipe system but need shell-level eval wrappers beyond PATH

### Composition Models in the Ecosystem

**Homebrew (post_install, caveats):**
- Post-install phase prints "caveats" text telling users to manually add lines to rc files
- No automatic composition; user responsibility to source tool init
- Pro: simple, zero startup cost, no tsuku maintenance burden
- Con: high friction, easy to miss setup, no cleanup on uninstall

**mise-en-place (eval approach):**
- `eval "$(mise activate)"` sources all tool shims and init scripts
- `mise activate` generates combined shell initialization for all installed tools
- Tools can contribute shim scripts to `~/.mise/shims/`
- Pro: single eval call, automatic on tool install, centralized
- Con: startup cost from generating init on every shell start (mitigated by caching)

**asdf (per-plugin scripts):**
- Plugins can contribute `bin/shim`, `bin/install`, `bin/uninstall`
- asdf sources plugin-provided scripts from `~/.asdf/plugins/{plugin}/`
- Plugins are opt-in; asdf doesn't automatically eval tool init
- Pro: isolated per-plugin, plugin lifecycle control
- Con: no automatic shell integration, plugins still need manual rc file edits

**direnv (early-exit hook on cd):**
- Uses shell hook on directory change to source `.envrc` files
- No single eval call; instead integration is event-driven (cd hook)
- Each project can declare its own env setup
- Pro: per-project flexibility, zero global shell cost
- Con: complex shell integration, requires bash 4.1+/zsh/fish, not compatible with cd wrapping

**zsh hook system (precmd, chpwd):**
- `precmd_functions` array lets multiple tools hook the prompt
- `chpwd_functions` array lets multiple tools hook directory changes
- Each tool registers its own function name
- Pro: orderly, composable, shell-native
- Con: zsh-only (bash/fish have no equivalent composable hook array)

### Trade-Off Analysis

**Automatic (shellenv sources everything) vs Opt-In (user adds eval lines):**

| Approach | Startup Cost | User Friction | Cleanup | Tool Lifecycle | Ordering |
|----------|--------------|---------------|---------|---------|---------:|
| Auto: shellenv plugin directory | ~10-50ms for N tool inits | Zero | Automatic | Tool controls | Must be explicit |
| Auto: tsuku hook calls tool init | ~10-50ms for N tool inits | Zero | On uninstall | Tool controls | Must be explicit |
| Opt-in: user adds eval lines | Zero (if user doesn't add) | High (manual edits) | Manual | User controls | User controls |
| Opt-in: tsuku prints caveats | Zero | Medium (discovery friction) | Manual | User controls | User controls |

**Startup time impact:**
- Each tool's init (`niwa init bash`, `direnv hook bash`, etc.) is 10-30ms on average
- 3 tools = 30-90ms added to every shell prompt (unacceptable for 50ms overall budget)
- Mitigations: pre-generate init scripts (mise approach), cache generated output, only source in interactive shells

**Ordering dependencies:**
- Some tools must initialize in a specific order (e.g., direnv before tool version switcher)
- Shell-native composable hooks (zsh precmd_functions) solve this, but only in zsh
- No universal solution across bash/zsh/fish

**Stale scripts on tool removal:**
- Automatic approach requires cleanup: when a tool is uninstalled, its init script must be removed
- User responsibility approach: stale lines in rc files don't hurt (but are ugly)
- Tsuku control approach: tool uninstall can trigger cleanup automatically

### Two Viable Compositions

**Model A: Per-tool hook scripts in a shell.d directory**
- Tsuku post-install hook writes `{TSUKU_HOME}/share/shell.d/{tool-name}.{shell}` 
- shellenv (or a new `tsuku shell-init` command) sources all scripts from `shell.d/`
- User adds single line: `eval "$(tsuku shell-init)"`
- Pros: centralized, automatic cleanup on uninstall, single user setup line
- Cons: 30-90ms startup cost, ordering must be configured in tsuku, tool-specific scripts must exist

**Model B: Tool-declared init commands in recipe metadata**
- Recipe includes optional `[shell_init]` section: `bash_init_cmd = "niwa init bash"`
- `tsuku hook-env` (on every prompt) calls all declared init commands and evals output
- Or: new `tsuku shell-inits` lists all init commands; user manually adds them or tsuku sources them from shell.d
- Pros: tool declares its own init, no post-install hooks needed, on-demand or one-time setup
- Cons: ordering dependencies must be handled by user or tsuku config, higher per-prompt cost if on-demand

### Existing Structure Enables Model A

- `tsuku hook install` already writes files to `$TSUKU_HOME/share/hooks/` and sources them from rc files
- The marker block pattern (`# tsuku hook` + source line) is proven and idempotent
- `hooks.WriteHookFiles()` is generic; could be extended to write tool-specific scripts
- Recipe system has no post-install phase, but could add one: `[[steps]] action = "shell_init" shell = "bash" script = "..."`

## Implications

**Recommended direction: Model A (shell.d directory) with tsuku-managed cleanup**

1. **Add post-install hook phase to recipe actions:** New step type `shell_init` that writes tool initialization scripts to `$TSUKU_HOME/share/shell.d/`.

2. **Extend `tsuku shellenv`:** Add optional `--with-init` flag that sources all shell.d scripts after PATH export. Or create new `tsuku shell-init` command that outputs init source calls.

3. **Update `tsuku hook install` pattern:** Instead of (or in addition to) the activation hook, install a marker block that sources shell.d scripts. This reuses the proven source-a-managed-file pattern.

4. **Tool recipes declare init scripts:** direnv.toml, starship.toml, niwa recipes include optional `[shell_init]` sections with per-shell scripts or references to external scripts.

5. **Startup cost management:** Cache generated init output (in `$TSUKU_HOME/.shell-init-cache`), keyed by shell type and installed tool list. Regenerate only when tool list changes.

6. **Ordering:** Simple TOML table in `$TSUKU_HOME/config.toml` for shell init order: `shell_init_order = ["niwa", "direnv", "zoxide", "starship"]`. Tsuku sources in that order.

This approach:
- Keeps startup cost predictable (cache hits are <1ms)
- Maintains user flexibility (can opt-in or opt-out per shell)
- Reuses proven hook installation machinery
- Allows tools to control their own init
- Cleans up automatically on uninstall
- Requires no changes to core install/activate/shellenv flow

## Surprises

1. **No composable hook array in bash/fish:** Unlike zsh's `precmd_functions` and `chpwd_functions`, bash has only `PROMPT_COMMAND` (a string, not an array) and fish has event handlers (not arrays). This means tsuku cannot adopt zsh's per-function model universally—must generate a single function or script per shell type.

2. **Hook files are already managed:** The command-not-found and activation hooks are not user-edited; they're embedded in the tsuku binary and written on install. This means tsuku already owns the hook file lifecycle—extending with tool-specific hooks is consistent with existing design.

3. **Activation hook runs on every prompt:** The `tsuku hook-env` call (PROMPT_COMMAND) already pays per-prompt cost. Adding tool init there would be 30-90ms per prompt (unacceptable). This strongly suggests either caching (Model A) or deferring tool init to an optional flag.

4. **No existing recipe post-install phase:** Recipes only define `[[steps]]` within the install sequence, then binaries are symlinked. Adding a post-install phase is a new capability, orthogonal to shell integration—several other use cases could benefit (completions, man pages, daemon registration).

## Open Questions

1. **Should tool init scripts be sourced on every prompt, or just on shell startup?** Every prompt is too slow; shell startup (`.bashrc`/`.zshrc`) is the right place. This means either a cached combined script or per-tool scripts that users source once.

2. **What if a tool's init script takes time?** Caching helps; so does lazy loading or background loading. But if a user installs many tools, 100ms+ shell startup cost may still be unacceptable. Should tsuku encourage tool authors to minimize init time?

3. **How should tsuku communicate tool init requirements to users?** Print caveats on install? Add a `tsuku setup` command that prints rc file additions? Let users discover it via `tsuku hook status` or `tsuku config show-shell-init`?

4. **Can shell.d scripts be tool-provided (in the recipe tarball) rather than generated by tsuku?** This would reduce tsuku's responsibility. Con: tools vary by shell, making per-shell tarballs complex. Pro: allows tools to optimize their init.

5. **Should the order of shell init be configurable per-shell, or global?** Some tools may need to run before others (e.g., direnv before version switcher). Should this be declarable in recipes, or a user config?

## Summary

Current tsuku shellenv is a static PATH export with no per-tool extension mechanism. The proven approach in the ecosystem (mise, asdf, direnv) is to either source tool init scripts from a shell.d directory (fast with caching, automatic cleanup) or use on-demand eval calls to tool commands. A **shell.d model with post-install recipe hooks and cached combined scripts** balances startup cost (<5ms cached, 30-90ms on tool list change), user friction (one-time setup), and tool lifecycle management (automatic cleanup). This reuses tsuku's existing hook installation machinery and requires only a new recipe action type and a shell.d sourcing mechanism in the activation hook.


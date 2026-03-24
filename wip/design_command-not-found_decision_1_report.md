# Decision 1: Hook Delivery and Lifecycle

## Context

Block 2 (command-not-found handler) needs shell hooks registered in bash, zsh, and fish so that typing an unknown command calls `tsuku suggest`. The install script (`website/install.sh`) is the primary onboarding path. The delivery choice affects first-run friction, upgrade safety, and uninstall cleanliness — and it sets a precedent for every shell integration feature tsuku adds in future blocks.

## Key Assumptions

- The installer is a curl-pipe-sh script similar to rustup, nvm, and the existing `website/install.sh`.
- Shell detection is limited to what `$SHELL` reports at install time; tsuku cannot reliably detect all shells the user runs.
- Fish's `conf.d/` mechanism is the shell's own supported extension point and does not require touching `config.fish`.
- "Clean uninstall" means the rc file is byte-for-byte identical to its pre-install state (or as close as practically achievable).
- Users who installed tsuku before shell hooks existed (i.e., upgrades from earlier versions) must not be silently left without hooks.

## Precedent Survey

| Tool | Auto-modifies rc on install? | Mechanism | Upgrade path |
|------|------------------------------|-----------|--------------|
| **nvm** | Yes | Injects two `source` lines into detected profile | Re-run installer; snippet calls `nvm.sh` from disk so upgrades are transparent |
| **rustup** | Yes (opt-out via `--no-modify-path`) | Injects `. "$HOME/.cargo/env"` — sources a managed file | Upgrading rustup rewrites `~/.cargo/env`; snippet stays stable |
| **mise** | No | Prints `eval "$(mise activate zsh)"` instructions | N/A (manual, user owns snippet) |
| **asdf** | No | Prints `export PATH=...` instructions | N/A (manual) |
| **starship** | No | Prints `eval "$(starship init bash)"` instructions | N/A (eval regenerates at shell startup) |
| **oh-my-zsh** | Yes (replaces `.zshrc`) | Replaces entire `.zshrc` with template | Backing up and replacing is extremely invasive |
| **Homebrew** | No | Prints `eval "$(brew shellenv)"` instructions | N/A (manual) |
| **pyenv** | No (recommends `eval "$(pyenv init -)"`) | Snippet evaluates dynamically generated code | Dynamic: upgrade rewrites shell logic, snippet stays stable |

**Pattern that emerges:** Tools that auto-modify rc files universally use the "source a managed file" pattern (rustup's `~/.cargo/env`, nvm's `nvm.sh`). They inject a short, stable line that delegates to a file tsuku controls. Tools that use `eval "$(tool init)"` are almost always manual-setup tools. The hybrid (source a managed file, auto-injected during install) is used by rustup — the closest analogue to tsuku in terms of audience and install experience.

**Tsuku already follows this pattern.** The existing `website/install.sh` creates `$TSUKU_HOME/env` and injects `. "$TSUKU_HOME/env"` into `.bashrc`/`.zshenv`. The hook delivery decision should extend this established approach rather than introduce a second, different pattern.

## Options Evaluated

### Option A: Auto-register during install (source a shipped file) — RECOMMENDED

The install script runs `tsuku hook install` for the detected shell as the final setup step. For bash and zsh, this injects a single stable line into the rc file:

```bash
# tsuku hook
. "$TSUKU_HOME/share/hooks/tsuku.bash"
```

The actual hook function lives in `$TSUKU_HOME/share/hooks/tsuku.<shell>`, shipped with the tsuku binary and updated on every `tsuku` upgrade. The injected line never changes after install; only the file it sources changes.

For fish, `tsuku hook install` writes `$TSUKU_HOME/share/hooks/tsuku.fish` and creates a symlink (or copies the file) to `~/.config/fish/conf.d/tsuku.fish`. No modification to `config.fish` is needed.

**Strengths:**
- Single injected line is stable across all tsuku versions. Users never need to re-register.
- Upgrade path is automatic: `tsuku` binary ships new hook logic in `share/hooks/`; the source line picks it up on next shell start.
- Uninstall is clean: `tsuku hook uninstall` removes the single source line (identifiable by a `# tsuku hook` comment marker) and deletes the hooks directory. The rc file reverts to its pre-install state.
- Consistent with the precedent set by rustup and with the pattern tsuku already uses for PATH setup.
- Fish gets its native mechanism (conf.d/) without making bash/zsh feel like second-class citizens.
- The hook file can be inspected at any time (`cat $TSUKU_HOME/share/hooks/tsuku.bash`), satisfying security-conscious users.
- `--no-modify-path` flag precedent from the existing installer can be extended to `--no-hooks` for users who prefer manual setup.

**Weaknesses:**
- Requires `$TSUKU_HOME` to be set (or defaulted) at shell startup time, before the source line runs. This is already a requirement for PATH setup, so it's not a new constraint.
- Shell must be detected at install time. Unknown shells fall back to printing manual instructions, same as today.

### Option B: Auto-register during install (inject generated snippet)

The install script injects the full hook function body directly into the rc file between `# tsuku hook begin` / `# tsuku hook end` markers.

**Strengths:**
- No dependency on a file being present at hook execution time.
- The hook is visible inline when users read their rc file.

**Weaknesses:**
- Upgrade requires `tsuku hook update` to replace the stale snippet. Users who don't run this continue using old hook behavior silently — the worst possible upgrade path for a feature like command-not-found, where tsuku's output format may evolve.
- Uninstall must precisely remove multi-line block between markers. This is fragile if users manually edit the region or if the markers appear in comments.
- The injected block grows with every new feature added to the hook (chaining, config checks, etc.), making rc files messy over time.
- Diverges from the pattern tsuku already uses for PATH setup, creating an inconsistent user experience.

### Option C: Manual only (no auto-registration)

The install script prints instructions; users run `tsuku hook install` themselves.

**Strengths:**
- Maximally explicit. Users know exactly what happened to their rc file.
- No risk of surprising users who don't want command-not-found hooks.

**Weaknesses:**
- Most users will skip this step. Command-not-found hooks are the primary value of Block 2; an opt-in install rate will be low.
- Breaks with the established tsuku installer UX, which already auto-configures PATH. A user who lets the installer run fully expects to have a working tsuku setup.
- Mise, asdf, starship, and Homebrew all require manual hook setup and this is consistently cited as friction in their communities. Tsuku's value proposition — zero friction tool management — is undermined if the feature that most directly reduces friction requires manual activation.

### Option D: Auto-register via per-shell native mechanisms

Use `~/.config/fish/conf.d/` for fish and equivalent per-user drop-in directories for bash/zsh where available (e.g., `/etc/profile.d/`-style patterns for user space, or `~/.config/bash/` on systems that support it).

**Strengths:**
- Does not touch `.bashrc`/`.zshrc` at all for shells that support conf.d equivalents.
- Fish integration is completely native.

**Weaknesses:**
- Bash and zsh have no widely-supported equivalent to fish's `conf.d/`. `/etc/profile.d/` requires root. `~/.bash.d/` conventions exist but are not built-in. This option effectively reduces to Option A for bash/zsh.
- Split strategy (conf.d for fish, source line for bash/zsh) is exactly what Option A's fish-specific path already does.
- This option adds no material benefit over Option A while adding implementation complexity.

## Chosen Option: Option A — Auto-register during install, source a shipped file

Option A best satisfies the constraints:

**Auto-registration is justified.** The install script already auto-modifies shell rc files for PATH setup. Adding hook registration follows the same consent model (users read the installer output or pass `--no-hooks` to opt out). Precedent from rustup and nvm validates this pattern for developer tool installers targeting the same audience.

**Source-a-file is the right delivery mechanism.** It decouples the upgrade path from re-registration. The hook function in `$TSUKU_HOME/share/hooks/tsuku.<shell>` can evolve with every tsuku release without requiring users to take any action. This is the decisive advantage over Option B.

**Fish divergence resolves cleanly.** Fish's `conf.d/` is a first-class supported mechanism — not a workaround — and writing to it is less invasive than modifying `config.fish`. For bash and zsh, the source line in `.bashrc`/`.zshenv` is the idiomatic approach.

**Uninstall is clean.** The injected line is identified by a `# tsuku hook` comment marker (single-line, stable). `tsuku hook uninstall` removes exactly that line. Users can verify the removal by inspecting their rc file.

## Hook file structure

`$TSUKU_HOME/share/hooks/tsuku.bash` (shipped with tsuku binary, updated on upgrade):

```bash
# tsuku command-not-found handler
if [ -x "${TSUKU_HOME:-$HOME/.tsuku}/bin/tsuku" ]; then
    command_not_found_handle() {
        "${TSUKU_HOME:-$HOME/.tsuku}/bin/tsuku" suggest -- "$1"
        return 127
    }
fi
```

The guard (`-x`) ensures the hook silently no-ops if the tsuku binary is missing (e.g., on a system where tsuku was uninstalled but the rc line wasn't cleaned up).

`$TSUKU_HOME/share/hooks/tsuku.zsh`:

```zsh
# tsuku command-not-found handler
if [[ -x "${TSUKU_HOME:-$HOME/.tsuku}/bin/tsuku" ]]; then
    command_not_found_handler() {
        "${TSUKU_HOME:-$HOME/.tsuku}/bin/tsuku" suggest -- "$1"
        return 127
    }
fi
```

`~/.config/fish/conf.d/tsuku.fish` (written by `tsuku hook install`):

```fish
# tsuku command-not-found handler
if test -x (string replace -- '$HOME' $HOME "${TSUKU_HOME:-$HOME/.tsuku}")/bin/tsuku
    function fish_command_not_found
        "{$TSUKU_HOME}/bin/tsuku" suggest -- $argv[1]
    end
end
```

## Installer integration

The install script adds a `--no-hooks` flag (analogous to the existing `--no-modify-path`). By default, after setting up PATH, the script calls `tsuku hook install` with the detected shell. The install output reads:

```
Installing shell hooks for bash...
  Configured: /home/user/.bashrc
Run 'source ~/.bashrc' or start a new shell to activate.
```

If `--no-hooks` is passed, the installer prints:

```
Skipped shell hook installation (--no-hooks).
To enable command-not-found suggestions, run: tsuku hook install
```

## Upgrade path

When tsuku upgrades (via `tsuku update tsuku` or a binary replacement), the binary ships new hook files to `$TSUKU_HOME/share/hooks/`. No user action is needed. The source line in the rc file picks up the new hook on next shell start. No "stale hook" problem exists.

## Uninstall contract

`tsuku hook uninstall` removes:
1. The `# tsuku hook` comment and `. "$TSUKU_HOME/share/hooks/tsuku.<shell>"` line from `.bashrc`/`.zshenv` (identified by exact marker match).
2. The `~/.config/fish/conf.d/tsuku.fish` file (for fish).

It does not remove `$TSUKU_HOME/share/hooks/` — that directory is cleaned up by `tsuku uninstall` (full tsuku removal), not by `tsuku hook uninstall` alone.

Users who have already run `tsuku hook uninstall` and then re-run it get an idempotent no-op with a clear message.

## Rejected Options

**Option B (inject snippet):** The upgrade path is fundamentally broken for a hook that calls `tsuku suggest`, whose output format will evolve as Block 2 matures. Every format change would require users to re-run `tsuku hook update`, and most won't. The stale-hook problem is silent and confusing.

**Option C (manual only):** Contradicts tsuku's zero-friction philosophy and breaks consistency with the existing PATH auto-configuration. The adoption gap between installed users and users-with-hooks-active would be large, making Block 2 a feature that most users never actually experience.

**Option D (native mechanisms only):** Reduces to Option A for bash/zsh because no widely-supported conf.d equivalent exists. The fish path from Option D is identical to the fish path in Option A.

## Assumptions

- `$TSUKU_HOME` resolves before shell hooks are sourced. This is guaranteed because the existing PATH setup sources `$TSUKU_HOME/env` first, which exports `TSUKU_HOME`.
- The tsuku binary is available at `$TSUKU_HOME/bin/tsuku` when hooks fire. The `-x` guard in hook files handles the case where it isn't.
- Shell detection at install time is sufficient; users who run multiple shells must run `tsuku hook install` manually for secondary shells (same limitation as the existing PATH setup).
- Fish's `conf.d/` directory exists or can be created at `~/.config/fish/conf.d/`. This is true for any fish installation that follows XDG conventions.
- The `tsuku hook install` / `tsuku hook uninstall` command pair is the canonical interface for all hook lifecycle operations, including manual use by users who skipped auto-registration.

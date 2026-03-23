# Research: Comparable Tools -- Project Auto-Install in CI

## Summary

Volta's transparent shim architecture is the closest match for "on first use without shell hooks." Devbox's `devbox run` pattern is also relevant. Most other tools (mise, asdf) require explicit install steps or shell activation. All tools with auto-install on clone/cd require explicit developer authorization for security.

---

## Tool Analysis

### mise

- **CI workflow**: Run `mise install` before any commands, or use `mise x -- <command>` as an explicit wrapper.
- **Auto-install**: `auto_install = true` in `.mise.toml` installs declared tools on shell activation (PROMPT_COMMAND/chpwd). Does NOT work in non-interactive scripts without prior `mise install`.
- **Trigger**: Shell activation (directory entry) for interactive use; explicit install for CI.
- **Developer UX on clone**: Run `mise install` or `mise x -- <command>`; tool installs automatically.
- **Shell hook required?** Yes for seamless shell use. CI needs explicit step.
- **Security**: No auto-install on clone; requires explicit invocation.

### devbox

- **CI workflow**: `devbox run <command>` triggers full Nix package install + command execution. A `devbox-install-action` exists for GitHub Actions.
- **Auto-install**: Handles in a single invocation -- `devbox run` installs everything declared in `devbox.json` then runs the command.
- **Trigger**: Explicit `devbox run <command>` wrapper.
- **Developer UX on clone**: `devbox run <tool>` works immediately; no shell setup required.
- **Shell hook required?** No. `devbox run` is a self-contained entry point.
- **Security**: Nix packages are content-addressed; supply chain risk is lower. Still requires explicit invocation.

### asdf

- **CI workflow**: Explicit `asdf install` before any commands.
- **Auto-install**: No native auto-install. Plugin-level configuration (`asdf_auto_install=true`) exists in some shells but is not standard.
- **Trigger**: Explicit. `.tool-versions` file is declarative but not a trigger.
- **Developer UX on clone**: Manual `asdf install` required.
- **Shell hook required?** Yes for version shims; directory entry does NOT trigger install.
- **Security**: N/A (no auto-install by default).

### volta (Node.js focus)

- **Architecture**: Shim-based. Volta installs itself as the `node`, `npm`, `yarn` binaries in PATH. When invoked, the shim checks `package.json` for declared version, auto-downloads if missing, then runs the real binary.
- **Auto-install**: Yes, transparent. Developer types `node` and volta ensures the declared version is installed with no explicit step.
- **Trigger**: Command invocation via shim. No shell hooks needed.
- **Developer UX on clone**: Clone repo, type `node` -- it installs and runs. Zero setup.
- **Shell hook required?** No. Shim is the mechanism.
- **Security**: Only installs from declared project config. Still auto-installs from potentially untrusted `package.json`.
- **Tradeoff**: Every `node`/`npm` invocation goes through a shim binary (small overhead, ~10ms). Shims must be installed globally and maintained as new tools are added.

### nix-shell / nix develop

- **Pattern**: `nix-shell --run <command>` or `nix develop --command <command>` provisions the full Nix environment, runs the command, exits.
- **Auto-install**: Yes, within the `--run` invocation. Content-addressed, hermetic.
- **Trigger**: Explicit `nix-shell --run` wrapper.
- **Developer UX on clone**: `nix develop --command koto run ...` works immediately.
- **Shell hook required?** No for `--run` pattern.
- **Security**: Strong -- content-addressed packages, reproducible. But requires Nix installed.

---

## Comparison Table

| Tool     | Works in CI (no hooks)? | Developer changes commands? | Trigger               | Security model          |
|----------|------------------------|-----------------------------|-----------------------|-------------------------|
| mise     | No (needs `mise install`) | Yes (`mise x -- <cmd>`)   | Shell activation / explicit | Explicit install only |
| devbox   | Yes (`devbox run`)     | Yes (`devbox run <cmd>`)    | Explicit wrapper      | Nix content-addressed   |
| asdf     | No                     | No (but no auto-install)    | Manual only           | No auto-install         |
| volta    | Yes (shim)             | No (transparent)            | Shim intercepts command | Auto from project config |
| nix-shell | Yes (`--run`)         | Yes (`nix-shell --run <cmd>`) | Explicit wrapper    | Hermetic/content-addressed |

---

## Security Note

All tools that auto-install on clone or directory entry (direnv, some mise configs) require explicit developer authorization (`direnv allow`, etc.) to prevent executing untrusted code from a cloned repository. The "on first use" pattern is safer than "on clone" but still carries supply chain risk.

---

## Synthesis

**Best fit for "developer types command, auto-install happens, no shell hooks":**

1. **Volta's shim model** (most transparent): shim binary in PATH intercepts the command, checks project config, installs if needed, executes. Developer changes nothing. But requires pre-installing the shim infrastructure.

2. **devbox's `devbox run` model** (explicit wrapper): developer types `devbox run <cmd>` instead of `<cmd>`. One-time change to invocation pattern. Works everywhere.

3. **mise `mise x` model**: similar to devbox run but scoped to a command rather than a dev environment.

For tsuku: the shim model is the most seamless UX but adds shim maintenance overhead. The `tsuku run <cmd>` explicit wrapper already exists in the design (Block 3) and matches the devbox pattern. The gap is whether koto/shirabe should call `tsuku run` internally or whether tsuku provides shims.

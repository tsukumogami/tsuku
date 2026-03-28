# Shell Integration

This guide walks through setting up tsuku for a project so that every
developer gets the right tools at the right versions, automatically.

By the end you'll have:

- A `.tsuku.toml` file checked into your repo
- Shell hooks that activate project tool versions when you `cd` into the directory
- Command-not-found handling that installs missing tools on demand
- Shims for CI pipelines and scripts

## Project Configuration

### Creating a config file

Run `tsuku init` in your project root:

```sh
tsuku init
```

This creates `.tsuku.toml` with an empty `[tools]` section:

```toml
# Project tools managed by tsuku.
# See: https://tsuku.dev/docs/project-config
[tools]
```

If the file already exists, `tsuku init` errors out. Use `--force` to overwrite it.

### Declaring tools

Add tools under `[tools]`. Each entry maps a recipe name to a version string:

```toml
[tools]
go = "1.22"
node = "20.16.0"
ripgrep = "14.1.0"
jq = "latest"
```

A few things about version strings:

- **Exact version** (`"20.16.0"`): installs that specific release
- **Prefix** (`"1.22"`): resolves to the latest matching release (e.g., 1.22.5). How prefix matching works depends on the tool's version provider.
- **`"latest"` or `""`**: resolves to the newest stable release

Pin exact versions when reproducibility matters. Tsuku warns about unpinned versions during install.

For tools that need extra options later, an inline table form is also supported:

```toml
[tools]
python = { version = "3.12" }
```

Both forms work identically today. The table form exists as an extension point for future per-tool options.

### Installing project tools

Run `tsuku install` with no arguments:

```sh
tsuku install
```

Tsuku finds the nearest `.tsuku.toml` by walking up from your current directory (stopping at `$HOME`), prints the tool list, and asks for confirmation:

```
Using: /home/dev/myproject/.tsuku.toml
Tools: go@1.22, jq, node@20.16.0, ripgrep@14.1.0
Warning: jq is unpinned (no version or "latest"). Pin versions for reproducibility.
Proceed? [Y/n]
```

After confirming, tsuku installs each tool and prints a summary. If some tools fail, the rest still install. Exit codes tell you what happened:

| Exit Code | Meaning |
|-----------|---------|
| 0 | All tools installed (or already current) |
| 6 | Every tool failed |
| 15 | Some tools installed, some failed |

Skip the confirmation prompt with `--yes` (useful in scripts):

```sh
tsuku install --yes
```

Preview what would happen without installing anything:

```sh
tsuku install --dry-run
```

### Config discovery

Tsuku searches for `.tsuku.toml` by walking up from the working directory. The first match wins. It won't look above `$HOME`.

This means a `.tsuku.toml` at your repo root applies to every subdirectory. In a monorepo, subdirectories inherit the root config unless they have their own `.tsuku.toml`.

To add extra boundaries, set `TSUKU_CEILING_PATHS` (colon-separated list of directories where traversal should stop):

```sh
export TSUKU_CEILING_PATHS="/home/dev/vendor:/tmp"
```

## Shell Activation

Shell activation makes project-declared tool versions available in your PATH automatically. Two approaches: explicit (one-shot) or automatic (prompt hooks).

### Explicit activation with `tsuku shell`

Run this in a project directory:

```sh
eval $(tsuku shell)
```

Tsuku reads `.tsuku.toml`, finds the installed versions, and prints export statements that prepend their bin directories to PATH. The `eval` applies them to your current shell.

If the project declares `go = "1.22"` and `node = "20.16.0"`, your PATH gets `$TSUKU_HOME/tools/go-1.22.5/bin` and `$TSUKU_HOME/tools/nodejs-20.16.0/bin` prepended. These shadow the global versions in `$TSUKU_HOME/tools/current/`.

The `--shell` flag overrides auto-detection if needed:

```sh
eval $(tsuku shell --shell=zsh)
```

### Automatic activation with hooks

For hands-free activation, install a prompt hook:

```sh
tsuku hook install --activate
```

This adds a small block to your shell's rc file (`~/.bashrc`, `~/.zshrc`, or fish's `conf.d/`) that runs `tsuku hook-env` on every prompt. The hook:

1. Checks if you've changed directories since the last prompt
2. If you haven't, exits immediately (under 5ms, no filesystem I/O)
3. If you have, reads `.tsuku.toml` and updates PATH

You can check what's installed:

```sh
tsuku hook status
```

To remove the activation hook:

```sh
tsuku hook uninstall --activate
```

### How it works

Activation tracks state in two shell variables:

- `_TSUKU_DIR` -- the last directory where activation ran
- `_TSUKU_PREV_PATH` -- your PATH before any project activation

When you enter a project directory, tsuku saves your current PATH and prepends the project's tool bins. When you leave (cd to a directory without `.tsuku.toml`), it restores the original PATH and unsets both variables.

Switching directly between two projects works correctly. Tsuku uses the saved original PATH as the base, not the current (project-modified) PATH.

Tools not declared in `.tsuku.toml` still resolve through your normal PATH, including `$TSUKU_HOME/tools/current/`.

### What if a version isn't installed?

Activation only works with already-installed tool versions. If `.tsuku.toml` declares `go = "1.22"` but you haven't installed Go 1.22, that tool is skipped. Run `tsuku install` to install missing versions. The auto-install feature (next section) handles this more smoothly.

## Auto-Install on Command Not Found

With the command-not-found hook and a `.tsuku.toml`, tsuku can install and run missing tools the moment you need them.

### Setup

Install the command-not-found hook if you haven't already:

```sh
tsuku hook install
```

This is separate from the activation hook. You can use both, either, or neither.

### How it works

When you type a command that doesn't exist, the hook calls `tsuku run`. If the command maps to a tool declared in `.tsuku.toml`, tsuku installs the pinned version and runs the command. No confirmation prompt.

```sh
# In a project with ripgrep = "14.1.0" in .tsuku.toml
# ripgrep isn't installed yet

$ rg "TODO" src/
# tsuku installs ripgrep 14.1.0, then runs the command
```

For tools NOT in `.tsuku.toml`, the normal consent mode applies (defaults to prompting for confirmation).

### The consent model

`.tsuku.toml` is the consent. When your team checks a config file into the repo declaring `ripgrep = "14.1.0"`, they're authorizing that tool at that version. Tsuku treats this as sufficient consent to install without prompting.

This means:

- **Tool in `.tsuku.toml`**: install the pinned version silently, then run
- **Tool not in `.tsuku.toml`**: use the normal consent mode (suggest, confirm, or auto)

### Using `tsuku run` directly

You don't need the command-not-found hook to get project-aware execution. `tsuku run` does the same thing:

```sh
tsuku run rg -- --arg foo bar
```

Use `--` to separate tsuku's flags from the target command's flags.

`tsuku run` checks `.tsuku.toml` for a version pin before falling back to the globally installed version. If the tool isn't installed at all, it installs it first (respecting the consent model).

### Consent mode configuration

For tools outside `.tsuku.toml`, the consent mode follows this priority:

1. `--mode` flag on `tsuku run`
2. `TSUKU_AUTO_INSTALL_MODE` environment variable
3. `auto_install_mode` in `$TSUKU_HOME/config.toml`
4. Default: `confirm`

The three modes:

| Mode | Behavior |
|------|----------|
| `suggest` | Print install instructions and exit |
| `confirm` | Prompt before installing (needs a TTY) |
| `auto` | Install silently |

## Shims for CI

Shell hooks don't work in CI pipelines, Makefiles, or non-interactive scripts. Shims fill this gap.

### What's a shim?

A shim is a small script in `$TSUKU_HOME/bin/` that delegates to `tsuku run`. When a CI job calls `go build`, the shim intercepts the call, and `tsuku run` handles version resolution (checking `.tsuku.toml`) and installation.

### Creating shims

From a project with `.tsuku.toml`:

```sh
tsuku shim install
```

With no arguments, this reads `.tsuku.toml` and creates shims for every declared tool. You can also shim a single tool:

```sh
tsuku shim install ripgrep
```

### Managing shims

List installed shims:

```sh
tsuku shim list
```

Remove shims for a tool:

```sh
tsuku shim uninstall ripgrep
```

Shims won't overwrite existing non-shim files in `$TSUKU_HOME/bin/`.

### Shims vs hooks

Use hooks for interactive development. Use shims for CI and scripts.

| Context | Use |
|---------|-----|
| Interactive shell | Activation hooks (`tsuku hook install --activate`) |
| Command-not-found | Command-not-found hooks (`tsuku hook install`) |
| CI pipelines | Shims (`tsuku shim install`) |
| Makefiles, scripts | Shims |
| One-off activation | `eval $(tsuku shell)` |

When both are active, shell activation takes precedence. Project tool bins appear earlier in PATH than `$TSUKU_HOME/bin/`, so real binaries win over shims. The shims only fire when activation isn't available.

### CI pipeline example

```yaml
# GitHub Actions example
steps:
  - uses: actions/checkout@v4

  - name: Install tsuku
    run: curl -fsSL https://tsuku.dev/install.sh | sh

  - name: Set up PATH
    run: echo "$HOME/.tsuku/bin" >> "$GITHUB_PATH"

  - name: Create shims
    run: tsuku shim install --yes

  - name: Build
    run: go build ./...   # shim handles version resolution from .tsuku.toml
```

The `--yes` flag on `tsuku shim install` skips any confirmation prompts. Since shims delegate to `tsuku run` at runtime, and `.tsuku.toml` provides consent, tools install automatically when first invoked.

## Quick Reference

| Command | Description |
|---------|-------------|
| `tsuku init` | Create a `.tsuku.toml` in the current directory |
| `tsuku init --force` | Overwrite an existing `.tsuku.toml` |
| `tsuku install` | Install all tools declared in `.tsuku.toml` |
| `tsuku install --yes` | Install without confirmation prompt |
| `tsuku install --dry-run` | Preview what would be installed |
| `tsuku shell` | Print shell exports to activate project tools |
| `tsuku hook install` | Install command-not-found hook |
| `tsuku hook install --activate` | Install automatic activation hook |
| `tsuku hook uninstall` | Remove command-not-found hook |
| `tsuku hook uninstall --activate` | Remove activation hook |
| `tsuku hook status` | Check hook installation status |
| `tsuku run <cmd> [args]` | Run a command, installing it if needed |
| `tsuku shim install` | Create shims for all tools in `.tsuku.toml` |
| `tsuku shim install <tool>` | Create shims for a single tool |
| `tsuku shim uninstall <tool>` | Remove shims for a tool |
| `tsuku shim list` | List installed shims |

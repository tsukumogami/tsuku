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

Version strings control which release gets installed:

- **Exact version** (`"20.16.0"`): installs that specific release. Most reproducible option.
- **Prefix** (`"1.22"`): resolves to the latest release that starts with `1.22.` (e.g., `1.22.5`). Prefix matching is dot-boundary-aware — `"1"` matches `"1.29.3"` but not `"10.0.0"`. A single major component like `"0"` resolves to the newest `0.x.y`.
- **`""` or `"latest"`**: both resolve to the newest stable release. They're equivalent. Tsuku shows a "Pin versions for reproducibility" warning for either.

All four forms work for activation as well as install: entering a project selects, from the versions you have installed, the newest one that satisfies the declaration. The example above is a working file — `go = "1.22"` puts your newest installed 1.22.x on PATH, and `jq = "latest"` puts your newest installed jq there.

To see which versions are actually available for a tool, run `tsuku versions <tool>`.

**Homebrew recipes have limited version availability.** For tools installed from Homebrew bottles, only the current stable version (and named versioned formulae like `shellcheck@0.9`) is available. Pinning to an older patch release that Homebrew no longer bottles will fail with "version not found". For these tools, either use `""` to track the current bottle, or check `tsuku versions <tool>` to confirm which versions are available before pinning.

Pin exact versions when reproducibility matters.

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

Each entry is matched against the directory being visited, exactly, and the test runs before that directory's `.tsuku.toml` is looked for. So a ceiling stops the walk at the directory it names and not at anything below it: `/home/dev/vendor` does not stop tsuku reading `/home/dev/vendor/somerepo/.tsuku.toml`, because `somerepo` is the directory tested on that iteration and it doesn't match. Name the directory whose config you want ignored.

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

Activation tracks state in three shell variables:

- `_TSUKU_DIR` -- the last directory where activation ran
- `_TSUKU_PREV_PATH` -- your PATH before any project activation
- `_TSUKU_STATE_STAMP` -- a fingerprint of what tsuku has installed, so that installing a tool takes effect at your next prompt without leaving the directory

When you enter a project directory, tsuku saves your current PATH and prepends the project's tool bins. When you leave (cd to a directory without `.tsuku.toml`), it restores the original PATH and unsets all three variables.

Switching directly between two projects works correctly. Tsuku uses the saved original PATH as the base, not the current (project-modified) PATH.

Tools not declared in `.tsuku.toml` still resolve through your normal PATH, including `$TSUKU_HOME/tools/current/`.

### Per-Tool Shell Init

Some tools need shell functions or environment setup beyond a simple binary on PATH. Recipes that include an `install_shell_init` step place init scripts in `$TSUKU_HOME/share/shell.d/`. The managed `$TSUKU_HOME/env` file sources the appropriate per-shell init cache (`.init-cache.bash` or `.init-cache.zsh`) at startup, so tools with shell functions are available in every new terminal after install — no `tsuku shellenv` call required, and nothing extra to add to your shell profile.

To skip shell init for a specific tool, pass `--no-shell-init` when installing:

```sh
tsuku install direnv --no-shell-init
```

This only affects `install_shell_init` steps. The tool's binary is still installed normally.

### What if a declaration can't be honored?

Activation works with already-installed tool versions. If nothing you have installed satisfies a declaration, that tool doesn't go on your PATH — and tsuku tells you why, on stderr, once when you enter the project:

```
tsuku: go is declared in .tsuku.toml, but nothing installed matches it. Run 'tsuku install go' to add it.
```

There are five reasons you might see:

| Message says | What happened | What to do |
|--------------|---------------|------------|
| `nothing installed matches` | No installed version satisfies the declaration | `tsuku install <tool>` |
| `recorded as installed but its files are missing` | Tsuku thinks it's installed, but the files are gone | Reinstall the version it names |
| `not a valid version string` | The version in `.tsuku.toml` is malformed, e.g. `">=26"` | Fix the version — see the version forms above |
| `is not a usable tool name` | The key is malformed, e.g. it contains `/` where an org-scoped name isn't intended | Fix the key; the version on that line is not the problem |
| `channel pin` | The declaration names a channel, e.g. `"@lts"` | Declare a version instead; activation selects among what's installed and doesn't resolve channels |
| `could not read` | Tsuku's installation state couldn't be read | Check `$TSUKU_HOME/state.json` |

The rest of the file still activates: one unhonorable declaration doesn't stop the others.

If you install the missing version without leaving the directory, it takes effect at your next prompt. You don't need to `cd` out and back in.

Pass `--quiet` to suppress these messages, or run `tsuku install` with no arguments to install everything the project declares. The auto-install feature (next section) handles this more smoothly.

## Auto-Install on Command Not Found

With the command-not-found hook and a `.tsuku.toml`, tsuku can install and run missing tools the moment you need them.

### Setup

Install the command-not-found hook if you haven't already:

```sh
tsuku hook install
```

This is separate from the activation hook. You can use both, either, or neither.

### How it works

When you type a command your shell can't resolve, the hook calls `tsuku run`. If the command maps to a tool declared in `.tsuku.toml` and you haven't configured a consent mode, tsuku installs the pinned version and runs the command without a prompt, printing a line first that says which file authorized it.

```sh
# In a project with ripgrep = "14.1.0" in .tsuku.toml
# ripgrep isn't installed yet

$ rg "TODO" src/
project-declaration: /home/dev/myproject/.tsuku.toml declares ripgrep@14.1.0 (recipe source: registry)
# tsuku installs ripgrep 14.1.0, then runs the command
```

For tools NOT in `.tsuku.toml`, the consent mode applies as configured, defaulting to a confirmation prompt.

### The consent model

`.tsuku.toml` is a consent signal, and it is bounded. When your team checks a config file into the repo declaring `ripgrep = "14.1.0"`, they're authorizing that tool at that version, and tsuku treats that as enough to skip the prompt — but only when you haven't chosen a mode yourself. A declaration raises the *default*; it does not overrule a `--mode` flag, a `TSUKU_AUTO_INSTALL_MODE` value or an `auto_install_mode` config key. Those are honored as given, `suggest` included, which is what makes `suggest` usable as protection in a repository you haven't read.

This means:

- **Tool in `.tsuku.toml`, no mode configured**: install the pinned version without a prompt, after the disclosure line, then run
- **Tool in `.tsuku.toml`, a mode configured**: your mode, unchanged
- **Tool not in `.tsuku.toml`**: your mode, or the `confirm` default

A mode of `auto` — raised by a declaration or set by you — can be lowered back to `confirm` before anything installs. Three checks do that, and each names itself on stderr when it fires:

| Identifier | Fires when |
|------------|------------|
| `config-permissions` | `$TSUKU_HOME/config.toml` grants access beyond its owner, is owned by someone else, or can't be read |
| `recipe-verification` | the recipe carries no checksum or signature to verify |
| `multiple-providers` | more than one recipe provides the command |

The last one can't fire for a declared command — the declaration already says which recipe was meant. The first two can, so a declared tool can still end up prompting.

**What a consent mode does not cover.** It governs installing, not running. A tool already installed at the version the project declares is executed straight from `$TSUKU_HOME/tools`, before any mode is consulted — so `suggest` does not stop a project from getting a tool you already have run for you. It also has nothing to say about what a tool does once it runs.

### Using `tsuku run` directly

You don't need the command-not-found hook to get project-aware execution. `tsuku run` does the same thing:

```sh
tsuku run rg -- --arg foo bar
```

Use `--` to separate tsuku's flags from the target command's flags.

`tsuku run` checks `.tsuku.toml` for a version pin before falling back to the globally installed version. If the tool isn't installed at all, it installs it first (respecting the consent model).

### Consent mode configuration

The consent mode follows this priority, for every tool — declared or not:

1. `--mode` flag on `tsuku run`
2. `TSUKU_AUTO_INSTALL_MODE` environment variable
3. `auto_install_mode` in `$TSUKU_HOME/config.toml`
4. Default: `confirm`, raised to `auto` for a tool `.tsuku.toml` declares

A declaration acts on step 4 and nowhere else, which is why it is written there and not at the top. Anything set at steps 1 through 3 reaches the install as you set it.

The one exception is `TSUKU_AUTO_INSTALL_MODE=auto`: tsuku honors it only when `config.toml` already says `auto`, so an environment variable alone can't raise the mode. Without that corroboration the run falls back to `confirm`.

The three modes:

| Mode | Behavior |
|------|----------|
| `suggest` | Print install instructions and exit |
| `confirm` | Prompt before installing (needs a TTY; exits 12 without one) |
| `auto` | Install without prompting |

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

The `--yes` flag on `tsuku shim install` skips any confirmation prompts. Since shims delegate to `tsuku run` at runtime, a tool the project declares installs on first invocation without a prompt — provided the job has configured no consent mode, which is the usual case in CI.

Two ways that stops, both of which surface as exit codes rather than hangs:

- **Exit 12** — `confirm mode requires a terminal`. A CI job has no TTY, so anything that lands in `confirm` stops here: a command the project doesn't declare, or a declared one that a mode-lowering check put back at `confirm`. Set `auto_install_mode = "auto"` in `config.toml`, or pass `--mode auto` where you can reach the command line.
- **Exit 10** — the project declares more than one recipe providing the same command. Several commands have more than one provider in the registry (`fd` comes from both `fd` and `fdclone`; `go` from both `go` and `golang`), and declaring one of them is exactly what settles the ambiguity — declaring two puts it back. Nothing installs and nothing runs; the message names each declaration and the exact `tsuku install` line that reaches it. Remove all but one.

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

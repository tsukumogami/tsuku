# Command-Not-Found Integration

When you type a command your shell can't resolve, it normally prints something
unhelpful like `command not found: fd`. With tsuku's hook installed, the shell
hands the command to `tsuku run` instead. `tsuku run` looks the command up in
the binary index and, if a recipe provides it, **installs the tool and then
executes the command you typed**. It is not a suggestion mechanism: whether it
installs is governed by the consent mode, described below.

This guide covers what the hook does, which setting controls it, how to manage
it, and how to troubleshoot it.

## How It Works

tsuku registers a handler with your shell's command-not-found mechanism. When a
command fails to resolve, the handler runs:

```sh
tsuku run <command> -- <the arguments you typed>
```

`tsuku run` looks the command up in the binary index. What happens next depends
on the consent mode and on whether the project you're standing in declares the
tool:

- **The mode permits installing.** tsuku installs the tool and replaces itself
  with the command, so you get the output you were after and its exit code.
- **The mode is `confirm`** (the default) **and a terminal is attached.** You
  get a prompt first.
- **The mode is `suggest`.** tsuku prints an install instruction and installs
  nothing.
- **No recipe provides the command.** tsuku says so and declines, and your
  shell's own handler runs afterwards.

Because the handler only fires for commands the shell could not resolve, a tool
you already have on PATH is never routed through it.

Hook scripts live in `$TSUKU_HOME/share/hooks/` and are updated when you
upgrade tsuku.

## Which Setting Governs Whether It Installs

The consent mode. Two of its three sources are reachable from a shell hook,
because the hook builds the command line itself and there is nowhere to put a
`--mode` flag:

1. `TSUKU_AUTO_INSTALL_MODE` in your environment
2. `auto_install_mode` in `$TSUKU_HOME/config.toml`
3. Otherwise the default, `confirm`

| Mode | What the hook does |
|------|--------------------|
| `suggest` | Prints `Install with: tsuku install <tool>`, installs nothing |
| `confirm` | Prompts before installing; without a terminal it stops and exits 12 |
| `auto` | Installs without asking |

To make the hook never install anything on its own:

```toml
# $TSUKU_HOME/config.toml
auto_install_mode = "suggest"
```

Setting `auto` through the environment alone doesn't take effect. tsuku honors
`TSUKU_AUTO_INSTALL_MODE=auto` only when `config.toml` already says `auto`, so
a variable exported by something you didn't write can't raise the mode by
itself; without that corroboration the run falls back to `confirm`.

**What `suggest` doesn't cover.** It governs installing, not running. A tool
that's already installed is executed straight from `$TSUKU_HOME/tools/current`
without any mode being consulted — but the hook fires only for commands the
shell could not already resolve, so this matters mainly for a project-declared
version that's already on disk. It also has nothing to say about what an
installed tool then does.

### Projects that declare their tools

If a `.tsuku.toml` in the current directory or a parent declares the tool being
run, and **you have not chosen a consent mode yourself**, the declaration is
taken as consent and the declared version installs without a prompt. A mode you
did set is honored as given — a declaration does not override
`TSUKU_AUTO_INSTALL_MODE` or `auto_install_mode`, including `suggest`, which
installs nothing.

Every install a declaration authorizes prints a line naming the recipe, the
version, the file that authorized it and where the recipe came from:

```
project-declaration: /home/dev/myproject/.tsuku.toml declares fd@10.2.0 (recipe source: registry)
```

See [Shell Integration](shell-integration.md) for the full consent model, and
`tsuku run --help` for the resolution order and exit codes.

## What It Looks Like

In a project that declares `fd = "10.2.0"`, with no consent mode configured:

```
$ fd --version
project-declaration: /home/dev/myproject/.tsuku.toml declares fd@10.2.0 (recipe source: registry)
Note: 'fd' publishes no checksums; integrity is pinned to the artifact fetched now.
    ...install progress, a success line for fd@10.2.0, and a PATH hint
       if $TSUKU_HOME/tools/current isn't on your PATH yet...
fd 10.2.0
```

The first line is the disclosure: any install a project declaration authorizes
names the recipe, the version, the file that authorized it and where the recipe
came from, before the install starts. The last line is the command's own output
— the tool ran.

With `auto_install_mode = "suggest"`, the same command in the same project:

```
$ fd --version
Install with: tsuku install fd@10.2.0
```

Nothing is installed. If your shell had its own command-not-found handler
before you installed tsuku's, that one runs next and prints its message
underneath.

For a tool the project doesn't declare, under the default mode with a terminal
attached:

```
$ hyperfine --version
Install hyperfine? [y/N]
```

When no recipe provides the command:

```
$ zzznotarealcommand
No recipe provides "zzznotarealcommand".
zzznotarealcommand: command not found
```

tsuku says it has nothing, then the shell prints its own error.

## Automatic Setup

The `install.sh` installer detects your current shell and registers the hook
automatically. Since the hook installs and runs tools rather than only naming
them, decide whether you want that before running the installer.

To skip hook installation during setup:

```bash
curl -fsSL https://tsuku.dev/install.sh | sh -s -- --no-hooks
```

You can register the hook later at any time with `tsuku hook install`.

## Managing Hooks

### Install

Register the hook for your current shell:

```bash
tsuku hook install
```

Target a specific shell with `--shell`:

```bash
tsuku hook install --shell bash
tsuku hook install --shell zsh
tsuku hook install --shell fish
```

The `--shell` flag defaults to reading `$SHELL` when omitted.

### Uninstall

Remove the hook from your shell configuration:

```bash
tsuku hook uninstall
```

For a specific shell:

```bash
tsuku hook uninstall --shell zsh
```

Uninstall is idempotent — running it multiple times is safe. With the hook
gone, an unresolvable command goes straight to your shell's own handler and
tsuku installs nothing.

### Status

Check which shells have hooks installed:

```bash
tsuku hook status
```

Example output:

```
bash: command-not-found installed
bash: activate not installed
zsh: command-not-found not installed
zsh: activate not installed
fish: command-not-found not installed
fish: activate not installed
```

Two lines per shell: the command-not-found hook this guide is about, and the
per-directory activation hook, which is separate and described in
[Shell Integration](shell-integration.md).

## Supported Shells

| Shell | Modified File |
|-------|---------------|
| bash  | `~/.bashrc` |
| zsh   | `~/.zshrc` |
| fish  | `~/.config/fish/conf.d/tsuku.fish` |

For bash and zsh, tsuku adds a marked block to your rc file. The block is
bounded by comment markers so tsuku can find and remove it cleanly. For fish,
tsuku creates a dedicated file in `conf.d/` which fish loads automatically.

## Wrapping an Existing Handler

If your shell already has a command-not-found handler (for example, from
`command-not-found` on Ubuntu or a custom function), tsuku wraps it rather than
replacing it. tsuku goes first. If `tsuku run` succeeds — which for an
installable tool means it installed and ran the command — the original handler
never runs. If tsuku declines for any reason, the original handler runs after
it, which is why you can see both messages for one command.

## Verifying the Hook Is Active

After installing, reload your shell configuration:

```bash
# bash
source ~/.bashrc

# zsh
source ~/.zshrc

# fish
source ~/.config/fish/conf.d/tsuku.fish
```

Then confirm with `tsuku hook status`. To test it without installing anything,
set `auto_install_mode = "suggest"` first and type a command you know tsuku has
a recipe for.

## Uninstalling Cleanly

`tsuku hook uninstall` removes the marker block from your rc file without
touching anything else. The file is left with the same content it had before
tsuku touched it.

If you uninstall tsuku entirely, run `tsuku hook uninstall` first to clean up
the rc files. If you've already removed tsuku, locate and remove the two-line
block manually:

```
# tsuku hook
. "${TSUKU_HOME:-$HOME/.tsuku}/share/hooks/tsuku.bash"
```

The comment line and the source line immediately after it are the only lines
tsuku adds. Delete both.

## Troubleshooting

### Nothing happens when a command is missing

1. Run `tsuku hook status` to confirm the command-not-found hook is installed.
2. Make sure you've reloaded your shell config since installing the hook.
3. Check that `$TSUKU_HOME/share/hooks/` exists and contains hook scripts.

### It stops without installing

Under the default `confirm` mode, `tsuku run` needs a terminal to ask. In a
script, a pipeline or a CI job it prints `confirm mode requires a terminal` and
exits 12. Shims are the supported route for those contexts — see
[Shell Integration](shell-integration.md).

### Hook installed but no recipe match

tsuku only acts when it finds a recipe providing the command. If you expect one
to exist, run `tsuku which <command>` or `tsuku search <name>` to check.

### Hook appears twice in rc file

This can happen if you ran `tsuku hook install` multiple times before a fix was
applied. Run `tsuku hook uninstall` once to remove all copies, then
`tsuku hook install` to add it back cleanly.

## Related Documentation

- [Shell Integration](shell-integration.md) — project configuration, the consent model, and shims
- [Actions and Primitives Guide](GUIDE-actions-and-primitives.md) — available recipe actions
- [Troubleshooting Verification](GUIDE-troubleshooting-verification.md) — diagnosing installation issues

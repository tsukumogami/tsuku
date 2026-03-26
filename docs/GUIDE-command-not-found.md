# Command-Not-Found Integration

When you type a command that isn't installed, your shell normally prints something unhelpful like `command not found: jq`. With tsuku's hook installed, you get a suggestion instead:

```
$ jq
Command 'jq' not found. Install with: tsuku install jq
```

This guide covers how the hook works, how to manage it, and how to troubleshoot it.

## How It Works

tsuku registers a handler with your shell's command-not-found mechanism. When a command fails to resolve, the handler checks whether tsuku has a recipe for it and prints an install suggestion if it does. If there's no matching recipe, tsuku stays silent and lets the shell print its default error.

Hook scripts live in `$TSUKU_HOME/share/hooks/` and are updated automatically when you upgrade tsuku.

## Automatic Setup

The `install.sh` installer detects your current shell and registers the hook automatically. You don't need to do anything.

To skip hook installation during setup:

```bash
curl -fsSL https://tsuku.dev/install.sh | sh -s -- --no-hooks
```

You can then register the hook manually at any time with `tsuku hook install`.

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

Uninstall is idempotent — running it multiple times is safe.

### Status

Check which shells have the hook installed:

```bash
tsuku hook status
```

Example output:

```
bash: installed
zsh: installed
fish: not installed
```

## Supported Shells

| Shell | Modified File |
|-------|---------------|
| bash  | `~/.bashrc` |
| zsh   | `~/.zshrc` |
| fish  | `~/.config/fish/conf.d/tsuku.fish` |

For bash and zsh, tsuku adds a marked block to your rc file. The block is bounded by comment markers so tsuku can find and remove it cleanly. For fish, tsuku creates a dedicated file in `conf.d/` which fish loads automatically.

## Wrapping an Existing Handler

If your shell already has a command-not-found handler (for example, from `command-not-found` on Ubuntu or a custom function), tsuku wraps it rather than replacing it. Both handlers run: tsuku checks for a recipe first, then calls the original handler. Your original setup is not lost.

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

Then confirm with `tsuku hook status`. You can also test by typing a command you know tsuku has a recipe for but haven't installed yet.

## Uninstalling Cleanly

`tsuku hook uninstall` removes the marker block from your rc file without touching anything else. The file is left with the same content it had before tsuku touched it.

If you uninstall tsuku entirely, run `tsuku hook uninstall` first to clean up the rc files. If you've already removed tsuku, locate and remove the two-line block manually:

```
# tsuku hook
. "$TSUKU_HOME/share/hooks/tsuku.bash"
```

The comment line and the source line immediately after it are the only lines tsuku adds. Delete both.

## Troubleshooting

### Suggestion doesn't appear

1. Run `tsuku hook status` to confirm the hook is installed.
2. Make sure you've reloaded your shell config since installing the hook.
3. Check that `$TSUKU_HOME/share/hooks/` exists and contains hook scripts.

### Hook installed but no recipe match

tsuku only prints a suggestion when it finds a matching recipe. If you expect a recipe to exist, run `tsuku search <name>` to check.

### Hook appears twice in rc file

This can happen if you ran `tsuku hook install` multiple times before a fix was applied. Run `tsuku hook uninstall` once to remove all copies, then `tsuku hook install` to add it back cleanly.

## Related Documentation

- [Actions and Primitives Guide](GUIDE-actions-and-primitives.md) — available recipe actions
- [Troubleshooting Verification](GUIDE-troubleshooting-verification.md) — diagnosing installation issues

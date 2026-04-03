# Shell Environment Customization

When tsuku sets up your shell, it writes a file called `env` to `$TSUKU_HOME`. That file is managed — tsuku rewrites it on upgrades to add new content and stay current. This guide explains where to put your own shell customizations so they survive that process.

## The two files

`$TSUKU_HOME/env` is owned by tsuku. Every time you run `tsuku install`, tsuku checks whether this file matches the expected content. If it's outdated, tsuku rewrites it. Your `.bashrc` or `.zshenv` sources this file at shell startup.

`$TSUKU_HOME/env.local` is yours. Tsuku never touches it. It's sourced automatically at the end of `env`, so anything you put there takes effect in every new shell.

The comment at the top of `env` says this directly:

```sh
# tsuku shell configuration — managed by tsuku, do not edit
# To customize, create $TSUKU_HOME/env.local (sourced automatically)
```

## Why the split exists

If you edit `env` directly, your changes will be silently overwritten the next time you install or update a tool. There's no warning, and nothing is backed up. The rewrite is intentional — it's how tsuku applies shell integration updates to existing installs.

`env.local` sidesteps this entirely. Tsuku's rewrite logic only touches `env`. Your customizations in `env.local` are never read, never compared, and never modified.

## What to put in env.local

Anything you'd normally put in `.bashrc` or `.zshenv` that's specific to your tsuku setup. Common examples:

```sh
# Opt out of telemetry
export TSUKU_NO_TELEMETRY=1

# Add a custom directory to PATH
export PATH="$HOME/bin:$PATH"

# Aliases for tools you install through tsuku
alias k="kubectl"
```

Create the file if it doesn't exist yet:

```bash
touch "$TSUKU_HOME/env.local"
```

Then open it in your editor and add what you need. It takes effect in the next shell you open (or after running `. "$TSUKU_HOME/env"` in an existing shell).

## Why the file is named env.local, not .env.local

The base file is `env`, not `.env`. `env.local` follows the same naming pattern — the `local` suffix marks it as the user-editable counterpart, the same convention used by many tools (`.gitconfig` / `.gitconfig.local`, `config` / `config.local`).

You might expect a dotfile here, but `$TSUKU_HOME` is already a hidden directory (`~/.tsuku` by default). Everything inside is already out of the way. Using a leading dot on files inside a hidden directory adds no benefit and makes tab completion slightly more awkward.

## What happens when you upgrade

When tsuku rewrites `env` (on `tsuku install` or `tsuku doctor --fix`), it runs a one-time migration for any existing `env` content that isn't part of the managed template. Specifically:

- Any `export` lines in `env` that aren't in the new template are extracted and appended to `env.local` before the rewrite.
- If `env.local` already contains a line, it's not duplicated.
- Comment lines in `env` are dropped — they're generated content, not yours.

This migration handles the common case where the official installer wrote `export TSUKU_NO_TELEMETRY=1` to `env` during setup. On the next upgrade, that line moves to `env.local` automatically.

If you added content to `env` directly (not through the installer), it will be migrated if it's an `export` statement. Other shell code you put there won't be. Put anything you want to keep in `env.local` now, before the next upgrade.

## Checking the current state

If you're not sure whether your env file is current:

```bash
tsuku doctor
```

If `env` is outdated, doctor reports it:

```
  Env file... FAIL
    Env file is outdated (run: tsuku doctor --fix)
```

To update it:

```bash
tsuku doctor --fix
```

This rewrites `env` with the current template (running the migration if needed) and rebuilds the shell integration caches. Your `env.local` is left alone.

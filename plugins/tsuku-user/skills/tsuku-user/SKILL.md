---
name: tsuku-user
description: |
  Use when helping someone manage tools with tsuku -- installing, configuring
  .tsuku.toml project files, setting up shell integration, or debugging
  PATH and update issues.
---

## .tsuku.toml Project Configuration

A `.tsuku.toml` file at your project root declares which tools the project needs. When a collaborator clones the repo and runs `tsuku install -y`, they get the same toolchain.

### Creating a Project Config

```bash
tsuku init
```

This writes a starter `.tsuku.toml` in the current directory. Use `--force` to overwrite an existing one.

### [tools] Section

```toml
[tools]
node = "20"
python = "3.12.0"
jq = "latest"
go = { version = "1.23" }
```

Each key is a tool name. The value controls how tightly the version is pinned:

| Pin Level | Syntax | Example | Auto-Update Behavior |
|-----------|--------|---------|----------------------|
| Latest | `""` or `"latest"` | `jq = "latest"` | Updates to any new version |
| Major | `"N"` | `node = "20"` | Updates within 20.x.y |
| Minor | `"N.M"` | `python = "3.12"` | Updates within 3.12.z |
| Exact | `"N.M.P"` | `go = "1.23.4"` | No auto-updates |
| Channel | `"@name"` | `rust = "@nightly"` | Provider-specific |

### Installing Project Tools

```bash
# Install all tools from .tsuku.toml (prompts for confirmation)
tsuku install

# Skip confirmation
tsuku install -y

# Preview without installing
tsuku install --dry-run
```

tsuku finds `.tsuku.toml` by walking up from the current directory, stopping at `$HOME` (or directories listed in `TSUKU_CEILING_PATHS`).

## Core CLI Commands

### Install and Manage

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `tsuku install <tool>` | Install a tool (supports `@version` suffix) | `--force`, `--sandbox`, `--dry-run`, `--reinstall`, `--fresh` |
| `tsuku install` | Install all tools from `.tsuku.toml` | `--yes`, `--dry-run`, `--fresh`, `--reinstall` |
| `tsuku remove <tool>` | Remove a tool (or specific version with `@version`) | `--force` |
| `tsuku update <tool>` | Update within pin boundaries | `--dry-run` |
| `tsuku update --all` | Update all tools (skips exact-pinned) | `--dry-run` |
| `tsuku activate <tool> <version>` | Switch to another already-installed version | |
| `tsuku rollback <tool>` | Revert to the version active before the last update | |
| `tsuku list` | List installed tools | `--json`, `--all` |
| `tsuku outdated` | Show tools with available updates | `--json` |

**Tool data outlives the tool**: a few tools keep things for you rather than just being a program — nvm holds every Node version you install, their global npm packages, and your `default` alias. Those live in `$TSUKU_HOME/data/<tool>/`, which is separate from the versioned directory tsuku installs into and recycles. Upgrading the tool leaves them alone, and so does `tsuku remove`. Nothing tsuku ships deletes that directory, so reclaim the space with `rm -rf "$TSUKU_HOME/data/<tool>"`. Data an older tsuku left somewhere else is not moved for you — see `docs/tool-data-directory.md`. Note this makes `rm -rf $TSUKU_HOME` destructive to data you cannot get back cheaply.

**Reinstalling**: plain `tsuku install <tool>` is idempotent — if the version it
resolves to is already installed, it says so and stops. `--reinstall` makes it run
the installation again and replace the files on disk, which is how a modified
install gets repaired. It reinstalls the tool you name and leaves its dependencies
alone, so reinstall a dependency by naming it. Anything the tool keeps in
`$TSUKU_HOME/data/<tool>/` is untouched. With no tool argument,
`tsuku install --reinstall` reinstalls every tool declared in `.tsuku.toml`.

`--reinstall` re-runs the plan stored when the tool was installed, so it picks up
a fix in tsuku itself but not a fix in the recipe — the stored plan still
describes the old steps. Add `--fresh` to regenerate the plan from the current
recipe: `tsuku install <tool> --fresh --reinstall`. Repairing modified files is
the case where you want the stored plan, since it is what produced the originals.

**Switching versions**: `activate` and `rollback` both re-point everything tsuku
owns for that tool — the `$TSUKU_HOME/bin` symlinks and the shell integration.
Rollback is one level deep and does not change the version pin, so auto-update
may re-apply the update on the next cycle; pin with `tsuku install <tool>@<version>`
to make it stick. Neither command downloads anything: the target version has to be
installed already.

**Install output**: In a terminal, `tsuku install` shows a single in-place status line that updates throughout the full install — including dependency resolution, action execution, and verification — then replaces with a permanent success line when done. In CI or piped output (non-TTY), it prints one line when a tool starts installing, one line per verification step, and one line on completion. A binary tool with one dependency produces 4 permanent lines; tools with no dependencies produce 3. Errors, retries, and command output also appear as permanent lines. `--quiet` suppresses all non-error output in both modes.

### Discover

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `tsuku search <query>` | Search recipes by name or description | `--json` |
| `tsuku recipes` | List all available recipes | `--local`, `--json` |
| `tsuku info <tool>` | Tool details (homepage, deps, status) | `--json` |
| `tsuku versions <tool>` | Available versions for a tool | `--refresh`, `--json` |
| `tsuku which <command>` | Which recipe provides a command | |

### Utilities

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `tsuku run <tool> [args]` | Install if missing, then execute | `--mode suggest/confirm/auto` |
| `tsuku check-deps <tool>` | Report each dependency's type and status before installing | `--json` |
| `tsuku verify <tool>` | Check binary integrity and deps | `--system-deps`, `--integrity` |
| `tsuku doctor` | Environment health check | `--fix` |
| `tsuku cache clear` | Clear download and version caches | `--downloads`, `--versions` |
| `tsuku update-registry` | Refresh all cached recipes and rebuild the binary index | `--dry-run`, `--recipe` |
| `tsuku plan show <tool>` | Show the plan an installed tool was installed from | `--json` |
| `tsuku plan export <tool>` | Write that plan to a file for `tsuku install --plan` | `-o` (`-` for stdout) |

All commands accept `--verbose` (`-v`), `--quiet` (`-q`), and `--debug` for log control.

### Reading `check-deps --json`

Don't infer "ready to install" from the exit code. `check-deps` exits 0 whenever
every system dependency is present, even if provisionable ones are missing --
tsuku installs those for you. The `all_satisfied` field is the stricter signal:
it's `true` only when every dependency, provisionable or system, already reports
`"status": "installed"`. Script against `all_satisfied`, or against each
dependency's own `status`, rather than against the exit code alone.

### Stored plans

`tsuku plan show` and `tsuku plan export` read the plan recorded at install time,
not a fresh one. If that record was written by an older tsuku, it may be missing
the tool's dependencies, its verification block, and its recipe type -- so an
exported copy would install less than the original did. Both commands warn on
stderr when they hit one. Reinstalling refreshes the record:

```bash
tsuku install <tool> --fresh
```

`tsuku install --plan` warns too, which is what covers a file exported before
that warning existed and kept for offline use. It fires when the plan declares
no recipe type, no dependencies and no verification *and* carries no marker
saying that is deliberate -- the shape of a plan exported before those fields
were stored. It installs anyway: refusing would strand anyone whose whole reason
for holding the file is that the target machine cannot reach the registry. If
you can reach it, regenerate:

```bash
tsuku eval <tool> > <tool>.plan.json
```

A plan that carries any of those fields never triggers the warning, whichever
tsuku wrote it, so `tsuku eval` output is unaffected. Seeing the warning means
the file genuinely declares nothing -- either because the tool has nothing to
declare, or because an old export dropped it, and the file cannot say which.

## Shell Integration

tsuku needs two directories on your PATH: `$TSUKU_HOME/bin` (wrapper scripts) and `$TSUKU_HOME/tools/current` (active tool symlinks). The `shellenv` command sets this up.

### Setup

Add one line to your shell profile:

**bash** (`~/.bashrc`):
```bash
eval "$(tsuku shellenv)"
```

**zsh** (`~/.zshrc`):
```zsh
eval "$(tsuku shellenv)"
```

**fish** (`~/.config/fish/config.fish`):
```fish
tsuku shellenv | source
```

`tsuku shellenv` prints the PATH exports and sources the shell init cache for your shell.

On fish this gets you PATH. Tool-specific shell init — the next section — is bash and zsh only.

### Tool-Specific Shell Init

Some tools need more than PATH — nvm is a shell function, direnv installs a hook, and a recipe can export variables the tool reads at startup. Those land in `$TSUKU_HOME/share/shell.d/`, one fragment per tool version per shell, named `<target>@<version>.<shell>`:

```
share/shell.d/
  00-env-nvm@0.40.6.bash     # variables the recipe exports
  nvm@0.40.5.bash            # nvm 0.40.5's own init script
  nvm@0.40.6.bash            # nvm 0.40.6's own init script
  .init-cache.bash           # what your shell actually sources
```

Your shell never sources those fragments directly. tsuku concatenates them in filename order into `.init-cache.{bash,zsh}`, and `$TSUKU_HOME/env` sources the cache. Only the active version's fragment goes in — install 0.40.6 alongside 0.40.5 and the older fragment stays on disk but drops out of the cache. Anything tsuku has no record of is included as-is, so a fragment you dropped in yourself keeps working.

The cache is rebuilt whenever the active version changes: install, upgrade, `activate`, `rollback`, and removing one version among several.

**bash and zsh only.** Recipes can target those two shells for init fragments, and nothing else. The cache is plain POSIX shell and it reaches you through `$TSUKU_HOME/env`, which fish can't parse — so rather than write a fish fragment nothing would load, tsuku rejects `shells = ["fish"]` when a recipe is validated. On fish you still get PATH from `tsuku shellenv | source`. What you don't get is a tool's own startup script, so something like nvm — which only exists as a shell function the fragment defines — won't be there. A recipe can still install a fish completion file, which lands in `$TSUKU_HOME/share/completions/fish/`; putting that directory somewhere your shell looks is currently up to you.

### Verifying Your Setup

```bash
tsuku doctor
```

Doctor checks that `$TSUKU_HOME` exists, both directories are on PATH, the state file is accessible, shell init caches are current, no orphaned staging directories remain, and every tool tsuku manages is the one your shell actually reaches. If something's wrong, it tells you what to fix.

Three shell.d diagnostics are worth knowing:

| Message | Meaning |
|---------|---------|
| `<shell> cache is stale` | A fragment changed since the cache was built. `--fix` rebuilds it. |
| `<file>: content hash mismatch` | The fragment's bytes no longer match what tsuku recorded when it wrote them — the file was edited or replaced after install. |
| `<file>: symlink detected` | A shell.d entry is a symlink. tsuku refuses to source it. |

```bash
tsuku doctor --fix
```

`--fix` repairs the two things it can generate from scratch: a stale `$TSUKU_HOME/env`, and stale shell caches. It won't touch a hash mismatch or a symlink, because both mean something outside tsuku wrote into `share/shell.d/` and tsuku can't reproduce the original bytes. Recover with `tsuku remove <tool>@<version> && tsuku install <tool>@<version>`, or delete the offending file if you put it there.

### When Another Copy Wins

The `Tool precedence` check is the one that catches a problem nothing else reports:

```
  Tool precedence ... WARN (1 shadowed)
    koto resolves to /home/you/.koto/bin/koto, not tsuku's $TSUKU_HOME/tools/current/koto
```

tsuku is fine here. It resolved the pin, installed the right version, and pointed `tools/current` at it. What's wrong is your PATH: something earlier on it provides a binary of the same name, so that's what runs. It happens when another installer prepends its own prefix — `~/.koto/env`, `~/.cargo/env`, a language version manager — and your shell profile sources it *after* `$TSUKU_HOME/env`. Whichever prepend runs last wins.

This is easy to miss, because every other signal looks healthy. `tsuku list` shows the version you expect, `tsuku outdated` finds nothing, and the tool keeps working — it just keeps working at whatever version the other copy is pinned to, forever.

Two ways out: move the tsuku entries ahead of the competing prefix in your profile (source `$TSUKU_HOME/env` last), or remove the other copy. If you put the other one first on purpose, ignore the warning — it's a `WARN`, it doesn't change the exit code, and `tsuku doctor || exit 1` keeps passing.

`--fix` won't touch this. Which directory wins is a property of your shell profile, and tsuku doesn't edit that.

Only tools that are visible on PATH are checked. Execution dependencies tsuku installed for its own use — the npm or Python behind a recipe — are deliberately kept off PATH, so a system copy answering for them isn't a conflict.

## Troubleshooting

### Exit Codes

When a command fails, the exit code tells you what went wrong:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments or usage |
| 3 | Recipe not found |
| 4 | Version not found |
| 5 | Network error |
| 6 | Installation failed (or all tools failed in batch install) |
| 7 | Verification failed |
| 8 | Dependency resolution failed |
| 15 | Partial failure (some tools failed in batch install) |
| 130 | Cancelled (Ctrl+C) |

### Diagnosing Issues

**Tool won't run after install?** Check that shell integration is set up:
```bash
tsuku doctor
```

**Suspect a corrupted install?** Verify the binary:
```bash
tsuku verify <tool>
```
This checks that the binary exists, its version matches the recorded state, and runs the tool's verification command if one is defined.

If verify reports files were modified after installation, put the original files back:
```bash
tsuku install <tool> --reinstall
```

**Registry out of date?** Refresh it:
```bash
tsuku update-registry
```

## Auto-Update Workflow

By default, tsuku checks for updates in the background and applies them within your pin boundaries.

### How It Works

1. On most commands, tsuku spawns a background check for newer versions.
2. If `updates.auto_apply` is enabled, updates within pin boundaries are installed automatically.
3. After the command finishes, a summary of any updates is displayed.
4. Superseded versions of the updated tool are reclaimed once they're older than
   `updates.version_retention`. The active version and the rollback target are always
   kept.

Exact-pinned tools (`go = "1.23.4"`) are never auto-updated.

### When a tool changes its shell init

Some tools ship a script your shell runs at every start (see Shell Integration
below). When an update changes what that script contains, tsuku says so:

```
warning: shell init changed for nvm (bash)
```

Every update path reports it — `tsuku update <tool>`, `tsuku update --all`, and
the background auto-apply. Auto-apply has no terminal to print to, so its warning
arrives as a notice shown by your next tsuku command instead.

It isn't an error. It means the tool rewrote code that runs in your shell, which
is worth a look if you didn't expect it. Read the new fragment under
`$TSUKU_HOME/share/shell.d/` if you want to see what changed, and
`tsuku rollback <tool>` if you'd rather go back.

Reclamation only touches versions tsuku installed and still records in its state. A
directory under `$TSUKU_HOME/tools` that tsuku has no record of -- left behind by an
interrupted install, or put there by hand -- is left alone however old it is. Remove
those yourself if you want the space back.

### Controlling Updates

| Setting | Effect |
|---------|--------|
| `TSUKU_NO_UPDATE_CHECK=1` | Disable all background checks |
| `TSUKU_AUTO_UPDATE=1` | Force auto-apply even in CI |
| `CI=true` | Suppresses auto-apply (unless overridden) |
| `TSUKU_NO_SELF_UPDATE=1` | Disable tsuku self-updates |

Or configure via `config.toml` (see below).

### Checking Manually

```bash
# See what's outdated
tsuku outdated

# Update one tool
tsuku update node

# Update everything
tsuku update --all

# Preview changes
tsuku update --all --dry-run
```

## User Configuration

User settings live in `$TSUKU_HOME/config.toml`. View and modify them with:

```bash
# Show all settings
tsuku config

# Get a specific value
tsuku config get telemetry

# Set a value
tsuku config set telemetry false
```

### Key Settings

**Telemetry**: Opt out of anonymous usage stats with `tsuku config set telemetry false`, or set environment variables: `TSUKU_NO_TELEMETRY=1` or `TSUKU_TELEMETRY=0`.

**Updates** (`[updates]` section):
- `enabled` -- toggle background update checks (default: true)
- `auto_apply` -- auto-install updates within pin boundaries (default: true)
- `check_interval` -- minimum time between checks, e.g. `"12h"` (default: `"24h"`)
- `self_update` -- check for tsuku self-updates (default: true)
- `version_retention` -- how long to keep superseded versions of a tool before reclaiming
  them, e.g. `"168h"` (default: 7 days)

**Registries**: Add third-party recipe sources:
```bash
tsuku config set registries.myorg/recipes.url https://github.com/myorg/recipes
```

**Custom home directory**: Set `TSUKU_HOME` in your shell profile to move tsuku's data out of `~/.tsuku`:
```bash
export TSUKU_HOME="$HOME/.local/share/tsuku"
```

**Secrets**: Store API keys (for LLM-powered recipe generation):
```bash
echo "sk-..." | tsuku config set secrets.anthropic_api_key
```
Secrets can also be provided via `TSUKU_SECRET_<NAME>` environment variables, which take precedence over config.toml.

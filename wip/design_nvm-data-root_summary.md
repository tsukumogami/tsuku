# Design Summary: nvm-data-root

## Input Context (Phase 0)

**Source:** /explore handoff (tsukumogami/tsuku#2464)

**Problem:** `recipes/n/nvm.toml` exports `NVM_DIR` as `{install_dir}`, a versioned
tool directory. nvm treats `NVM_DIR` as its data root, so upgrading nvm hides every
Node version and global npm package at the next shell start and tsuku's cleanup
paths later delete them. The recipe cannot express any stable path, so the fix
requires deciding where a data root lives, how `nvm exec` survives a program/data
split, what `tsuku remove nvm` does to that data, and how existing installs migrate.

**Constraints:**

- No option is recipe-only — `set_env` has six placeholders, none stable, and
  single-quotes its values.
- Nothing stable can live under `tools/` (GC prefix-matches).
- `$TSUKU_HOME/share/` is untouched by all deletion sites as written.
- Dropping the export self-locates nvm to `share/shell.d`, not `$HOME/.nvm`.
- `nvm exec` needs both `nvm.sh` and `nvm-exec` present in `$NVM_DIR`; a lone
  `nvm-exec` symlink fails silently.
- No action can mkdir, symlink, or place a file at an arbitrary path today.
- Migration must reach installs that re-run no recipe steps (short-circuit +
  plan cache).
- A recipe-only change runs no Go CI jobs.
- The fast behavioral guard belongs in `cmd/tsuku/shelld_lifecycle_test.go`.

**Primary open decision:** tsuku-owned stable data root versus nvm's own
`$HOME/.nvm`, gated on whether `tsuku remove nvm` should be able to delete the
user's Node installs.

**Prior art to carry in:** `Config.EnsureEnvFile` + `migrateEnvExports`
(`internal/config/config.go:477-556`) is the migration pattern to copy — shape
detection rather than a schema version, delivered both automatically from
`InstallWithOptions` and manually via `tsuku doctor --fix`.

## Current Status

**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-08-02

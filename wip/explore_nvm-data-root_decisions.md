# Exploration Decisions: nvm-data-root

## Round 1

- **Re-framed the problem away from garbage collection.** The user-visible failure
  is immediate, not delayed: the version-keyed `set_env` fragment plus
  `ShellDSelection`'s active-version filtering means the next shell gets an
  `NVM_DIR` pointing at a fresh empty extraction. GC only makes it permanent. Any
  fix scoped to GC alone is a fix for the slower half of the bug.

- **Eliminated the "keep the data in the versioned directory and compensate"
  family.** Inverse symlinks die on `nvm cache clear` (`rm -rf` on the symlink,
  then `mkdir -p` recreates a real directory inside the versioned dir). Per-recipe
  GC exemption and teaching GC about tool data both leave the next-shell-start bug
  intact and make disk usage unbounded. Migrating data forward on every upgrade was
  already rejected in `DESIGN-shell-d-lifecycle.md` for driving behavior off
  `VersionState.Plan`, and would copy a multi-gigabyte Node tree on every point
  release.

- **Eliminated "drop the `set_env` step" as stated in the issue.** It does not
  restore upstream behavior. `install_shell_init` *copies* nvm.sh into
  `$TSUKU_HOME/share/shell.d`, so nvm self-locates there, not to `$HOME/.nvm`.
  Dropping the export reinstates the pre-#2465 layout. "Use nvm's default root"
  remains viable only as an *explicit* export of `$HOME/.nvm`.

- **Eliminated any stable path under `tools/`.** `GarbageCollectVersions` matches a
  raw string prefix and `ReapVersion` no-ops on unrecognized versions rather than
  objecting, so `tools/nvm-data` is collected.

- **Accepted that no option is recipe-only.** `set_env` has six placeholders, none
  stable, and single-quotes its values as an injection defense. "Smallest diff" is
  therefore not a differentiator between the surviving options.

- **Accepted that `nvm exec` and `nvm run` are in scope.** They are the only
  subcommands that break under a program/data split (`${NVM_DIR}/nvm-exec`), the
  issue does not name them, and breaking them silently would be a regression traded
  for a fix. This decision is what promotes the "stable root" option from its naive
  form to the corrected one that materializes both `nvm-exec` and `nvm.sh`.

- **Deferred the central choice to design rather than settling it here.** Where the
  data root lives reduces to a product question — should `tsuku remove nvm` be able
  to delete the user's Node installs? — with real arguments on both sides and
  consequences for whether tsuku must grow a `delete_dir`-emitting cleanup. That is
  design work, and the decision framework is the right tool for it.

- **Split out an unrelated bug rather than widening scope.**
  `GarbageCollectVersions` deletes other tools' directories on recipe-name prefix
  collisions (59 pairs in the registry, verified empirically). Separate issue.

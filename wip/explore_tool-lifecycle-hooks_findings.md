# Exploration Findings: tool-lifecycle-hooks

## Core Question

Tsuku installs tools by downloading binaries and symlinking them, but some tools
need more -- shell functions, env files, completions, cleanup on removal. What
should tsuku's lifecycle hook system look like so tools can customize their
post-install setup, pre-uninstall cleanup, and pre-upgrade migrations?

## Round 1

### Key Insights

- **Lifecycle hooks are nearly universal across package managers** (lead: package-manager-patterns).
  The 4-phase model (pre/post install, pre/post remove) appears in Homebrew, dpkg, npm,
  Chocolatey, and mise. In-recipe declaration (Homebrew model) fits tsuku best since
  recipes already define everything about tool installation.

- **8-12 tools in the registry can't function without post-install shell integration**
  (lead: tool-configuration-needs). Direnv, zoxide, asdf, mise, and niwa all need
  eval-init wrappers. These tools are installed as binaries but their core features
  require shell functions that tsuku currently can't provide. 200+ more tools would
  benefit from completion registration.

- **The action system is moderately extensible** (lead: action-system-extensibility).
  The WhenClause conditional infrastructure is proven and could gain a phase qualifier.
  But remove and update flows have zero lifecycle awareness -- remove just deletes
  directories without consulting recipes, and update is a plain reinstall.

- **shell.d directory model is the right composition pattern** (lead: shellenv-composition).
  Tsuku already manages hook files in $TSUKU_HOME/share/hooks/. A shell.d/ directory
  for per-tool init scripts, sourced via an extended shellenv or shell-init command,
  reuses this machinery. Cached combined scripts keep startup under 5ms.

- **Declarative hooks should come first** (lead: security-model). Tsuku's current
  trust model (no post-install code execution) is a genuine advantage over npm-style
  arbitrary scripts. A limited vocabulary of lifecycle actions (install_shell_init,
  install_completions, cleanup_paths) preserves this while enabling the key use cases.

- **Remove/update flows need refactoring** (lead: uninstall-upgrade-hooks). Remove
  doesn't load recipes at all. Cleanup instructions should be stored in state at
  install time so removal works without registry access.

### Tensions

- **Declarative vs imperative hooks**: Declarative (limited action vocabulary) is safe
  but may not cover all use cases. Imperative (shell scripts) is flexible but changes
  tsuku's trust model. Evidence strongly favors starting declarative -- the niwa use
  case and 85% of registry tools are served by file-copy/install actions.

- **Automatic composition vs opt-in**: Automatic (shellenv sources everything) adds
  startup cost but eliminates user friction. Opt-in (user adds eval lines) has zero
  cost but high friction. Caching resolves the startup concern, making automatic the
  better default.

- **Phase qualifier on steps vs separate recipe sections**: Extending WhenClause is
  lower risk but mixes lifecycle semantics into platform conditions. A separate
  `[lifecycle]` section is cleaner but requires more schema changes. The existing
  codebase favors the WhenClause approach since it's already proven.

### Gaps

- No research into how other tools in the tsuku ecosystem (koto, niwa) would
  specifically declare their hooks in recipe format. Concrete TOML examples needed.

- The interaction between lifecycle hooks and multi-version support (multiple versions
  installed simultaneously) wasn't deeply explored. Which version's hooks "win"?

### Decisions

- Start with declarative hooks (Level 1) for security
- shell.d directory for composition, with caching
- Extend existing action system (phase qualifier) rather than schema redesign
- Focus on post-install shell integration as priority
- Store cleanup instructions in state at install time
- Hooks fail gracefully, not fatally

### User Focus

Auto-mode: The niwa shell-integration design doc triggered this exploration by
explicitly deferring tsuku generalization. The findings validate that generalization
is now warranted -- 8-12 tools need it, the action system can support it with moderate
changes, and the shell.d pattern provides a clean composition model.

## Accumulated Understanding

Tsuku needs a lifecycle hook system to move beyond "download binary and symlink."
The evidence points to a specific design:

**Recipe-level declaration** using the existing action system extended with a phase
qualifier. Recipes would mark certain steps as `phase = "post-install"` or
`phase = "pre-remove"`. New declarative actions (`install_shell_init`,
`install_completions`, `cleanup_paths`) handle the common cases without arbitrary
code execution.

**shell.d composition** for tools that need shell integration. Post-install hooks
write init scripts to `$TSUKU_HOME/share/shell.d/{tool}.{shell}`. An extended
`tsuku shellenv` (or new `tsuku shell-init`) sources these. Cached combined scripts
keep startup fast.

**State-tracked cleanup** so removal works reliably. When post-install hooks run,
the executor records what was installed (which shell.d files, which completions) in
state. The remove command reads state to clean up, without needing recipe access.

**Graceful failure model** where hook failures warn but don't block. The tool is
still installed and usable; the hook output just didn't complete.

This design preserves tsuku's current security advantage (no arbitrary post-install
code), enables the niwa use case that triggered this exploration, and serves the
broader registry (completions for 200+ tools, shell init for 8-12 critical tools).

## Decision: Crystallize

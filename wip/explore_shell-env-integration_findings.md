# Exploration Findings: shell-env-integration

## Core Question

When `tsuku install` runs a tool with `install_shell_init` (like niwa), shell functions
get written to `~/.tsuku/share/shell.d/` and the init cache is rebuilt. But `~/.tsuku/env`
— the static file sourced from `.bashrc` — only sets PATH. It doesn't source the init cache,
so tools' shell functions silently don't load in new terminals. How do we fix this so any
tool with `install_shell_init` automatically works after install?

## Round 1

### Key Insights

- **The gap is universal, not a misconfiguration** (installer lead): Every user who installs
  via the official script gets a broken setup for shell-integrated tools. The installer writes
  `. ~/.tsuku/env` to `.bashrc`; `~/.tsuku/env` never sources `.init-cache.bash`. New users
  are broken on day one when they install a tool with `install_shell_init`.

- **The env file predates shell.d by 27 days** (shellenv-vs-env-file lead): `~/.tsuku/env`
  was introduced on March 1, 2026; the shell.d system on March 28, 2026. The env file was
  designed for PATH-only and was never updated. `tsuku shellenv` correctly sources the init
  cache, but it's the "fallback" path, not the primary one.

- **`EnsureEnvFile()` is called on every `tsuku install`** (env-file-generation lead):
  It's idempotent — it checks the file content against a constant `envFileContent` and
  rewrites only if they differ. This means updating `envFileContent` in `config.go` would
  automatically migrate all active users on their next `tsuku install`.

- **Doctor has a `--rebuild-cache` flag in error messages that doesn't exist** (doctor lead):
  Doctor checks shell.d health thoroughly (stale cache, hash mismatches, syntax errors) but
  has no check for whether the env file is sourcing the init cache. And the flag it suggests
  to users (`tsuku doctor --rebuild-cache`) is not implemented.

- **niwa is the only recipe using `install_shell_init` right now** (scope lead): Out of
  ~1,400 recipes, only niwa uses this action. This is the right moment to fix the
  infrastructure before more recipes adopt it.

### Tensions

- **Static env file vs subprocess:** `~/.tsuku/env` avoids spawning a subprocess on every
  shell start (a real performance concern for shell initialization). `eval "$(tsuku shellenv)"`
  is always current but spawns a process. The middle ground is updating the static env file
  to include init cache sourcing with shell detection (`$BASH_VERSION`/`$ZSH_VERSION`).

- **Shell detection in a static file:** `~/.tsuku/env` is sourced by both bash and zsh. It
  needs to source the right cache file for the current shell. This requires shell-specific
  conditionals (`$BASH_VERSION` / `$ZSH_VERSION`) inside the file — doable but adds
  complexity to what was previously a single export line.

- **`EnsureEnvFile()` and the `TSUKU_NO_TELEMETRY` bug:** Install.sh conditionally appends
  `TSUKU_NO_TELEMETRY=1` to the env file; the Go code's `envFileContent` constant never
  includes it. If `EnsureEnvFile()` rewrites the env file (e.g., after updating the content),
  it silently drops the telemetry opt-out for users who chose it. This pre-existing bug
  means we need to be careful about the rewrite behavior.

### Gaps

- **Agent 4 (install_shell_init scope) didn't produce a full findings file** — the summary
  confirms niwa is the only recipe, but the full implementation details of
  `validateCommandBinary` weren't captured in a file.

- **Fish shell handling:** The shell.d system supports fish, but the installer only handles
  bash/zsh for env file setup. Fish shell integration is an open question.

- **What happens in CI/Docker:** The env file is designed to work in non-login shells
  (containers, CI). Adding init cache sourcing might fail silently if the cache doesn't
  exist — but the cache is only built when `install_shell_init` runs, so it won't exist
  in most CI environments. The `[ -f ... ] &&` pattern handles this safely.

### Decisions

- Update static env file (not eval): avoids subprocess overhead; migration via EnsureEnvFile already exists
- Fix scope: (1) env file content, (2) doctor --rebuild-cache, (3) TSUKU_NO_TELEMETRY bug
- Niwa recipe fix is a separate PR in niwa repo
- Fish shell deferred to follow-on

### User Focus

Ready to crystallize after round 1. Direction confirmed: update envFileContent in config.go + install.sh.

## Decision: Crystallize

## Accumulated Understanding

The shell integration gap is a design sequencing problem: the static `~/.tsuku/env` file
predates the shell.d system and was never updated. Three things need to happen:

1. **Fix the env file content** (affects all new and active users): Update `envFileContent`
   in `internal/config/config.go` to also source the appropriate `.init-cache.<shell>` file,
   using shell detection. Update `website/install.sh` to match. The idempotency check in
   `EnsureEnvFile()` ensures active users get the update on their next `tsuku install`.

2. **Fix the doctor command** (affects users who need manual repair): Implement
   `--rebuild-cache`, add a check that the env file is up to date, and consider a `--fix`
   flag that rewrites the env file.

3. **Fix the niwa recipe** (affects the triggering tool): Change `source_command` from
   bare `niwa` to `{install_dir}/bin/niwa shell-init {shell}`. This is a separate PR in
   the niwa repo.

The `TSUKU_NO_TELEMETRY` bug in `EnsureEnvFile()` is a pre-existing issue that should be
addressed alongside the env file fix to avoid dropping users' telemetry preferences.

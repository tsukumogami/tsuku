# Decision 3: Doctor Staleness Detection and Repair

## Question

How does `tsuku doctor` detect a stale env file, and what does it repair (env file, cache, or both)?

## Context

`tsuku doctor` is the health-check command users run when their environment is broken. It already detects shell.d cache staleness and tells users to run `tsuku doctor --rebuild-cache` — but that flag is not implemented. The env file check is also missing.

Two connected problems need solving together:

1. The `--rebuild-cache` flag is referenced in doctor's error output but does nothing. Whatever we implement must either honor that exact flag name or update the messages.
2. Users who installed tsuku long ago and never run `tsuku install` again won't get `EnsureEnvFile()` called automatically. Doctor with a repair mode is the right fallback path.

The decision has two coupled sub-questions: how to detect a stale env file, and what the repair scope covers.

### What `EnsureEnvFile()` already does

`internal/config/config.go` defines `envFileContent` as a constant and `EnsureEnvFile()` as a function that reads `$TSUKU_HOME/env`, compares it byte-for-byte against the constant, and rewrites if different. The function is already called by `install.Manager` during `tsuku install`. Doctor can call it directly for the repair path.

### Current env file content

The canonical content is:

```sh
# tsuku shell configuration
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
```

It does not source the init cache. The `shellenv` command outputs a `. "<path>/.init-cache.<shell>"` line separately at runtime.

## Options Evaluated

### Detection Options

#### Option A: Content comparison

Compare the on-disk `$TSUKU_HOME/env` against the `envFileContent` constant. If the bytes differ, report stale.

**Pros:**
- Simple and exact. The same logic `EnsureEnvFile()` already uses, so the detection and the fix share one code path.
- Zero ambiguity: the file is either the canonical version or it isn't.
- Works correctly for both missing files and modified content.

**Cons:**
- Flags user customizations as stale even if the changes are harmless (e.g., a user added an extra comment or export). However, the env file is explicitly managed by tsuku — the install script writes it, `EnsureEnvFile()` owns it. User customizations belong in `.bashrc`/`.zshrc`, not in `$TSUKU_HOME/env`. This is acceptable.
- Brittle to whitespace-only differences — but these would also break the intended sourcing behavior.

#### Option B: Feature detection

Check whether `$TSUKU_HOME/env` contains the PATH export line as a substring, treating any version that contains the core export as "good enough."

**Pros:**
- Tolerates user additions (comments, extra exports).
- Less likely to flag files as stale after minor wording changes in the comment.

**Cons:**
- The current `envFileContent` has no init-cache sourcing line. If we later add one (e.g., to source `.init-cache.$SHELL` from within the env file itself), feature detection would need to be updated separately, creating a second place where the "minimum required content" lives.
- Defining what patterns count as "present" is ambiguous. A user with the right PATH line but from the wrong version would pass the check.
- Harder to implement correctly — would need to check multiple substrings as the canonical content grows.

#### Option C: Hash-based

Store a hash of the managed section in a separate file; compare on each run.

**Pros:**
- Avoids reading the constant into the comparison path.

**Cons:**
- Over-engineered for a single managed file with static content. Adds a separate state artifact (`$TSUKU_HOME/env.hash` or similar) that can itself go out of sync.
- `EnsureEnvFile()` already implements content comparison; hash is redundant.

**Verdict:** Option A (content comparison). It's what `EnsureEnvFile()` already uses internally, there's no meaningful user-customization scenario for this file, and it gives a precise signal.

---

### Repair Scope Options

#### Option A: Single `--fix` flag

One flag repairs everything: rewrites the env file to canonical content, rebuilds the shell.d cache for each shell.

**Pros:**
- Simple UX. Users don't need to understand which subsystem failed.
- Matches the mental model: "something is broken, fix it."
- `--fix` subsumes both `--fix-env` and `--rebuild-cache` without needing to choose.

**Cons:**
- Replaces `--rebuild-cache` entirely, requiring an update to the existing error message.
- Less surgical for power users who want to rebuild only the cache.

#### Option B: Separate flags (`--fix-env`, `--rebuild-cache`)

Two independent flags, each targeting one subsystem.

**Pros:**
- Granular. Users can rebuild only the cache without touching the env file.

**Cons:**
- The existing error message says `--rebuild-cache` but doesn't implement it. Adding a separate `--fix-env` alongside it doubles the surface without simplifying the UX.
- Most users won't know or care which subsystem caused the failure. They want one fix command.
- `--rebuild-cache` alone doesn't fix the env file, leaving the migration problem unsolved for users who run it.

#### Option C: Implement `--rebuild-cache` as cache-only, add `--fix-env` separately

Honor the existing flag name exactly, add a new independent flag for the env file.

**Pros:**
- No changes needed to existing error message text.

**Cons:**
- The existing error message already misleads users by suggesting a flag that doesn't work. Keeping the same name while changing the message to say "run tsuku doctor --fix" is no different from Option A.
- Two flags with narrow scopes is worse UX than one fix-all flag for this use case.
- Users with a stale cache AND a stale env file need to run two commands.

**Verdict:** Option A (single `--fix` flag), replacing `--rebuild-cache` in the error messages. The migration scenario is the motivating use case and it requires both repairs. One flag that fixes everything is the right surface. The existing error message is wrong anyway (flag not implemented), so updating the message text is necessary regardless.

## Chosen Approach

**Detection:** Content comparison (Option A) — compare `$TSUKU_HOME/env` against `envFileContent` constant. If the file is missing or bytes differ, report stale.

**Repair scope:** Single `--fix` flag (Option A) — rewrites env file via `EnsureEnvFile()` and rebuilds all shell.d caches. The existing `--rebuild-cache` reference in doctor's output is updated to `--fix`.

## Key Implementation Details

### New check in doctor output

The env file check runs as a new step (after the existing PATH checks, before shell integration):

```
  Env file ($TSUKU_HOME/env)... ok
```

When stale or missing:

```
  Env file ($TSUKU_HOME/env)... FAIL
    Env file is outdated (run: tsuku doctor --fix)
```

When missing entirely:

```
  Env file ($TSUKU_HOME/env)... FAIL
    Env file not found (run: tsuku doctor --fix)
```

### Updated shell integration error message

The existing stale cache message changes from:

```
    bash cache is stale (run: tsuku doctor --rebuild-cache)
```

to:

```
    bash cache is stale (run: tsuku doctor --fix)
```

### `--fix` flag behavior

`tsuku doctor --fix` runs all the same checks, then for each detected problem:

1. **Stale or missing env file**: calls `cfg.EnsureEnvFile()`, prints `  Rewrote $TSUKU_HOME/env`.
2. **Stale shell.d cache for a shell**: rebuilds `.init-cache.<shell>` by concatenating the shell.d files in sorted order (same logic as `shellenv.RebuildCache()`), prints `  Rebuilt bash cache` (or `zsh cache`, etc.).

After repair, doctor re-runs all checks and prints the final summary. If any non-repairable issue remains (e.g., a symlink in shell.d, a hash mismatch), the summary still reports failure with actionable messages.

### What `--fix` does NOT repair

- Hash mismatches (file content changed outside tsuku — user must investigate).
- Symlinks in shell.d (security risk, user must remove manually).
- Syntax errors in shell.d scripts (tool-authored, needs recipe fix).
- Orphaned staging directories (surfaced as WARN, not FAIL; user removes manually).

### Flag definition

```go
var fixAll bool

func init() {
    doctorCmd.Flags().BoolVar(&fixAll, "fix", false, "Repair detected issues (rewrites env file, rebuilds shell caches)")
}
```

The old `--rebuild-cache` flag is not added. The error messages in the stale-cache output path are updated to reference `--fix`.

### No `--rebuild-cache` compatibility alias

Adding `--rebuild-cache` as a hidden alias that maps to `--fix` is tempting but unnecessary. The flag was never shipped in a release (it's referenced in error output but not implemented), so there are no users depending on it. A clean break is better than aliasing.

## Assumptions

- The env file (`$TSUKU_HOME/env`) is a tsuku-managed file. Users are not expected to customize it. Any diff from `envFileContent` is a migration artifact, not a user edit to preserve.
- `RebuildCache()` (or equivalent logic) is either already exported from `internal/shellenv` or can be extracted from `install.Manager` into a function doctor can call. If not yet exported, it must be added as part of this implementation.
- Doctor is run interactively; it doesn't need to be idempotent in the sense that running `--fix` when nothing is wrong should be safe (both `EnsureEnvFile()` and cache rebuilding are already idempotent).
- The `--rebuild-cache` flag name has never been released to users. No compatibility concern.

## Confidence

**High.** The detection logic directly mirrors `EnsureEnvFile()` which is already tested and correct. The single `--fix` flag is the simplest model that addresses both the migration scenario and the existing broken flag reference. The only implementation risk is whether `RebuildCache` is already exported; if not, it's a small refactor.

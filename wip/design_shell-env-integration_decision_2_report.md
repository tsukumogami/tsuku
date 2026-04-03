# Decision 2: EnsureEnvFile Preservation Strategy

## Question

How does `EnsureEnvFile()` update the env file without clobbering user customizations like `TSUKU_NO_TELEMETRY`?

## Context

`EnsureEnvFile()` in `internal/config/config.go` currently writes `$TSUKU_HOME/env` with a constant `envFileContent`. The implementation is blunt: read the file, compare it to the constant, rewrite if different. This is safe only because the constant never changes.

The upcoming shell-env-integration work will add init cache sourcing to `envFileContent`. That change breaks the invariant: any user with the old content will have their file rewritten on the next `tsuku install`. That's the intended migration path, but it silently drops anything the installer appended after the managed section — most critically, the telemetry opt-out.

The `website/install.sh` installer conditionally appends `export TSUKU_NO_TELEMETRY=1` to `$TSUKU_HOME/env` when users decline telemetry during install (lines 127-133). The Go constant `envFileContent` never includes this line. So a user who opted out during install, whose env file now contains the PATH line plus the opt-out, will have the opt-out silently dropped the first time they run `tsuku install` after the content change.

**Why this matters:**
- The telemetry system in `internal/telemetry/client.go` checks `TSUKU_NO_TELEMETRY` as a runtime environment variable via `os.Getenv()`. It also checks `userconfig` (`config.toml`). If the env file is the only place the opt-out is stored and the env is sourced by the shell, dropping it means the user starts sending telemetry again — without notice.
- A `userconfig` system already exists (`internal/userconfig/userconfig.go`) with a `Telemetry bool` field that persists in `config.toml`. The telemetry client already reads this as a fallback.

**Constraints:**
- Must not silently drop `TSUKU_NO_TELEMETRY=1` or any other user content
- Must be simple enough to implement safely in low-level config code
- Must keep `envFileContent` in sync with `website/install.sh`
- The env file is a shell script, not a structured format
- Users may have added arbitrary content beyond what the installer writes
- The telemetry system needs to reliably detect opt-out wherever it is stored

## Options Evaluated

### Option A: Marker-delimited managed section

Wrap tsuku's managed content in `# tsuku:begin` / `# tsuku:end` markers. `EnsureEnvFile()` parses the file, replaces only the section between markers, and leaves content outside the markers untouched.

Example file structure:
```sh
# tsuku:begin
# tsuku shell configuration
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:..."
# tsuku:end

# Telemetry opt-out (set during installation)
export TSUKU_NO_TELEMETRY=1
```

`EnsureEnvFile()` would:
1. Read the existing file
2. If markers are present, replace between them with current `envFileContent` (minus markers)
3. If markers are absent (legacy file), replace the whole file and re-append any non-matching lines

**Pros:**
- Preserves arbitrary user content outside the markers
- `EnsureEnvFile()` can update the managed section freely on any tsuku upgrade
- Installer can continue appending after the closing marker

**Cons:**
- Requires non-trivial string manipulation of shell scripts — a source of subtle bugs (off-by-one, trailing newline handling)
- The legacy migration path (no markers) needs its own logic, which introduces a second code path that could go wrong
- Inline parsing of shell scripts is fragile: comments, quoted strings, multi-line constructs could confuse simple line matching
- Adds meaningful complexity to the lowest-level config primitive, which is currently 8 lines

**Verdict:** Rejected — the implementation complexity is disproportionate to the problem, and the legacy migration path introduces a correctness risk at exactly the place where correctness is most critical. The benefit over Option B is modest.

### Option B: Separate file for user customizations

Move user-specific preferences out of the managed `$TSUKU_HOME/env` and into a separate `$TSUKU_HOME/env.local`. The managed `env` file sources `env.local` at the end if it exists. `EnsureEnvFile()` rewrites `env` freely; it never touches `env.local`.

The installer writes `TSUKU_NO_TELEMETRY=1` to `env.local` instead of appending to `env`. Users who want to add customizations write to `env.local`.

Example `env`:
```sh
# tsuku shell configuration — managed by tsuku, do not edit
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"

# Source user customizations if present
[ -f "${TSUKU_HOME:-$HOME/.tsuku}/env.local" ] && . "${TSUKU_HOME:-$HOME/.tsuku}/env.local"
```

Example `env.local` (written by installer when user opts out):
```sh
# Telemetry opt-out (set during installation)
export TSUKU_NO_TELEMETRY=1
```

**Pros:**
- `EnsureEnvFile()` remains trivially simple — the comparison-and-rewrite logic is unchanged
- Clear separation of concerns: `env` is owned by tsuku, `env.local` is owned by the user
- Installer change is localized to one `>>` becoming a `>` to a new file
- Pattern is familiar (mirrors `/etc/profile.d/`, `bashrc` + `bashrc.local`, etc.)
- No parsing required
- Future user customizations have a natural, documented home
- Migration path for existing users is handled implicitly: if the old `env` had the opt-out appended, the next rewrite drops it from `env`, but the installer was never run again so this affects only users who reinstall or manually had it there

**Cons:**
- Existing users who opted out during installation and who have the opt-out in `env` (not `env.local`) will lose it on first update. This is the pre-existing bug being fixed, but it requires a one-time migration.
- The migration requires either a Go-side migration in `EnsureEnvFile()` or accepting a narrow window where existing opt-out users have telemetry silently re-enabled.
- Adds a `source env.local` line to `env`, which is a new dependency on file existence checking in a startup script (minor, but worth noting).

**Verdict:** Chosen — see rationale below.

### Option C: Snapshot-and-restore (line diffing)

Read the existing file, subtract lines that appear in the current `envFileContent`, append remaining lines to the new content.

**Pros:**
- No new files or markers needed

**Cons:**
- Line-by-line diffing of shell scripts is fundamentally unreliable: comments don't diff cleanly, variable assignment syntax varies, whitespace-only lines are ambiguous
- If the user has a comment that happens to match a managed comment, it gets dropped
- The algorithm cannot distinguish "user intentionally removed a managed line" from "user added a comment that collides"
- Effectively re-implements a diff algorithm over an unstructured format
- Does not handle the `TSUKU_NO_TELEMETRY` line correctly if the user has modified it (e.g., `export TSUKU_NO_TELEMETRY=true`)

**Verdict:** Rejected — fragile by design. Shell scripts are not a diff-safe format. Any implementation will have edge cases that silently corrupt user config, which is the exact failure mode we need to prevent.

### Option D: Move telemetry opt-out to userconfig / state.json

Instead of persisting telemetry preference in the env file, write it to `config.toml` (via `userconfig`) during installation. The `env` file never carries user preferences. `EnsureEnvFile()` can rewrite freely.

The installer would run something equivalent to:
```sh
tsuku config set telemetry false
```

Or write directly to `config.toml`.

**Pros:**
- `EnsureEnvFile()` is completely decoupled from telemetry preference
- No parsing or migration in Go
- `userconfig` is already the right place for this preference — the telemetry client already reads it as a fallback (see `client.go` lines 63-68)
- Clean separation: env file for PATH, config.toml for preferences

**Cons:**
- The installer (`install.sh`) is a shell script downloading a binary — it cannot easily call `tsuku config set` before tsuku is installed
- It would need to write TOML directly, which is error-prone from shell (quoting, existing file handling)
- Does not help users who set `TSUKU_NO_TELEMETRY=1` in their env file manually (outside the installer flow)
- Does not address the general problem of user content in env — only telemetry

**Verdict:** Rejected as a standalone solution. Worthy as a complementary step (the installer could write to `config.toml` instead of `env.local` once tsuku binary is installed), but doesn't solve the general preservation problem and introduces installer complexity.

### Option E: Migrate existing opt-out during EnsureEnvFile (combined with Option B)

This is a migration sub-strategy to pair with Option B. When `EnsureEnvFile()` detects the old content (no markers, contains `TSUKU_NO_TELEMETRY`), it:
1. Extracts the opt-out line
2. Writes it to `env.local`
3. Rewrites `env` with the new managed content

**Pros:**
- Handles existing users who opted out with the old installer without breaking their preference
- One-time, self-healing migration
- After migration, the invariant holds and no further special-casing is needed

**Cons:**
- Requires detecting "old-style" content — essentially a limited form of Option C, but scoped only to the migration case
- The detection heuristic (`strings.Contains(existing, "TSUKU_NO_TELEMETRY")`) is simple enough to be safe
- Adds one migration code path that should eventually become dead code

**Verdict:** Accepted as a complement to Option B. The migration is narrow (only runs once, only extracts a known string), which keeps the risk low.

## Chosen Approach

**Option B: Separate file for user customizations, with Option E migration.**

The `$TSUKU_HOME/env` file becomes fully managed by tsuku. A new `$TSUKU_HOME/env.local` file holds user-specific content. The managed `env` sources `env.local` if it exists. `EnsureEnvFile()` rewrites `env` freely.

For existing users with the old opt-out in `env`, a one-time migration in `EnsureEnvFile()` extracts `TSUKU_NO_TELEMETRY` lines and writes them to `env.local` before rewriting `env`.

## Key Implementation Details

### New `envFileContent` constant

```go
const envFileContent = `# tsuku shell configuration — managed by tsuku, do not edit
# To customize, create $TSUKU_HOME/env.local (sourced automatically)
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"

# Source user customizations if present
[ -f "${TSUKU_HOME:-$HOME/.tsuku}/env.local" ] && . "${TSUKU_HOME:-$HOME/.tsuku}/env.local"
`
```

### Updated `EnsureEnvFile()` logic

```
1. Read existing file
2. If existing == envFileContent: return nil (already correct)
3. Migration: if existing content contains "TSUKU_NO_TELEMETRY":
   a. Extract lines containing TSUKU_NO_TELEMETRY (and their preceding comment if present)
   b. Append to env.local (create if not exists)
4. Write envFileContent to env
```

The migration step (3) uses a simple `strings.Contains` check, not full shell parsing. It only extracts the specific known line the installer writes. This is narrow enough to be reliable.

### Installer change (`website/install.sh`)

Change the telemetry opt-out block from appending to `env` to writing to `env.local`:

```sh
if [ "$NO_TELEMETRY" = true ]; then
    cat >> "$ENV_LOCAL_FILE" << 'ENVEOF'

# Telemetry opt-out (set during installation)
export TSUKU_NO_TELEMETRY=1
ENVEOF
fi
```

Where `ENV_LOCAL_FILE` is defined alongside `ENV_FILE` near the top of the script.

### Sync requirement

The comment in `config.go` noting "Keep in sync with website/install.sh" still applies. The `envFileContent` constant and the `cat > "$ENV_FILE"` block in `install.sh` must remain identical. The telemetry opt-out write in `install.sh` moves from `$ENV_FILE` to `$ENV_LOCAL_FILE`.

### Telemetry detection (no change required)

The telemetry client already works correctly with this approach:
- `DisabledByEnv()` checks `os.Getenv("TSUKU_NO_TELEMETRY")` at runtime — this works as long as the shell sources `env.local`
- `NewClient()` falls back to `userconfig.Load()` — this is a secondary backstop

No changes needed in `internal/telemetry/client.go`.

## Assumptions

- Users who added `TSUKU_NO_TELEMETRY=1` manually to `env` (not via the installer) will have it migrated to `env.local` by the one-time migration in `EnsureEnvFile()`. This is the desired outcome.
- Users who sourced `env` in their shell config will automatically pick up `env.local` once the new `env` sources it.
- The `env.local` file does not need to be created by tsuku — it is optional and user-managed. Documentation should guide users to put customizations there.
- The `[ -f ... ] && .` sourcing idiom is portable across bash, zsh, and POSIX sh.
- The migration only needs to handle the specific `TSUKU_NO_TELEMETRY` line from the installer; other hypothetical user additions to `env` are not in scope for migration and will be dropped (acceptable, as we cannot safely migrate arbitrary shell content).

## Confidence

**High.** The approach is straightforward:
- `EnsureEnvFile()` stays simple — comparison and rewrite, same as before
- The migration is narrow and low-risk (known string, one-time)
- The `env.local` pattern is idiomatic and well-understood
- The telemetry client requires no changes
- The installer change is a two-line modification (new variable + redirect target)

The only meaningful risk is the migration window for existing users — but this is bounded to the telemetry opt-out line only, and the migration runs automatically on the first `tsuku install` after upgrading.

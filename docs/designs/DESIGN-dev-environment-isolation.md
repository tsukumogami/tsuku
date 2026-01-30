---
status: Accepted
problem: Developers working on tsuku lack a zero-ceremony way to run against isolated environments without interfering with each other or the host's real installation.
decision: Use build-time ldflags to give Makefile-built binaries a different default home directory, stop exporting TSUKU_HOME from the install script, and add tsuku shellenv and tsuku doctor commands for PATH setup and environment validation.
rationale: Build-time defaults handle isolation with zero ceremony. shellenv and doctor are general-purpose commands that serve both contributors (configuring PATH for dev builds) and end users (alternative to the install script's env file, environment health checks).
---

# DESIGN: Dev Environment Isolation

## Status

Accepted

## Context and Problem Statement

When developing tsuku, you need to run your local build to test recipe changes, action modifications, and CLI behavior. Right now, that means either running against your real `$TSUKU_HOME` (risking your working installation) or manually setting `TSUKU_HOME` to a temp directory every time.

Neither approach works well. Running against your real home pollutes it with test artifacts. Manually exporting a new `TSUKU_HOME` is tedious, easy to forget, and doesn't solve the parallel execution problem.

The problem gets worse with parallel testing. Multiple checkouts may run concurrently, each testing a different feature branch. Some branches change tsuku's internal storage format, so parallel sessions can't share any state -- not even the download cache. Each checkout needs a fully independent `$TSUKU_HOME` without any manual setup.

The Build Essentials workflow already demonstrates this need: each macOS test creates a fresh `TSUKU_HOME` per tool to avoid interference. That pattern works but it's ad-hoc.

There's also a complication: the install script (`website/install.sh`) writes an env file that exports `TSUKU_HOME` in every shell session. Any mechanism that relies on "not having `TSUKU_HOME` set" won't work for developers who have tsuku installed.

### Scope

**In scope:**
- A build-time mechanism for running tsuku against an isolated home directory
- Parallel-safe execution across separate checkouts
- A fix to the install script so it doesn't block the mechanism
- State persistence across invocations within the same checkout
- Commands for shell PATH configuration and environment health checking

**Out of scope:**
- Per-directory tool version activation (future feature, separate design)
- Container-based isolation (the sandbox feature already covers that)
- Shared download cache across environments (parallel branches may change cache format)

## Decision Drivers

- **Zero-conflict isolation**: A dev build must never read or write the host's real `$TSUKU_HOME/state.json`
- **Parallel safety**: Multiple checkouts must not interfere with each other
- **Zero ceremony**: Building and running should require no extra flags, env vars, or wrapper scripts
- **Useful CLI surface only**: Any new commands must serve end users, not just contributors
- **Format independence**: Branches changing tsuku's storage format must not corrupt other checkouts' state
- **Tool reachability**: After installing a tool, it should be possible to run it directly from the shell

## Implementation Context

### Existing Patterns

**CI test isolation** (Build Essentials macOS jobs): Each test sets `TSUKU_HOME` to a fresh temp directory and symlinks the download cache. Full isolation with cache reuse, but manual shell scripting.

**Sandbox isolation** (`--sandbox` flag): Runs installation inside a container with a fresh `$TSUKU_HOME`. Full isolation, but heavyweight and doesn't persist state across runs.

### Conventions to follow

- All paths derive from `$TSUKU_HOME` via `DefaultConfig()` in `internal/config/config.go`
- `DefaultConfig()` reads `TSUKU_HOME` env var, falls back to `~/.tsuku`
- Go supports build-time variable injection via `ldflags -X`

### The install script problem

The install script creates `$TSUKU_HOME/env` which is sourced by shell init files:

```bash
export TSUKU_HOME="${TSUKU_HOME:-$HOME/.tsuku}"
export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
```

This means every shell session has `TSUKU_HOME` set, which would override any build-time default. The install script needs to stop exporting `TSUKU_HOME` and instead inline the fallback in the `PATH` setup.

## Considered Options

### Option 1: Build-time ldflags default

Use Go's `-ldflags -X` to inject a different default home directory at build time. The Makefile sets the default to `.tsuku-dev` (relative to working directory). Release builds via GoReleaser don't set the flag, so they fall back to `~/.tsuku`.

Precedence: `TSUKU_HOME` env var > ldflags default > `~/.tsuku`

Requires a one-time fix to the install script to stop exporting `TSUKU_HOME`.

Example usage:
```bash
make build
./tsuku install cmake    # uses .tsuku-dev/ in current directory
```

**Pros:**
- Zero ceremony: `make build` then use tsuku normally
- No new CLI flags or env vars
- Each checkout gets its own `.tsuku-dev` automatically
- Parallel checkouts are fully isolated
- `TSUKU_HOME` override still works for explicit control
- Release binary behavior is unchanged

**Cons:**
- Requires using `make build` instead of bare `go build` (or remembering the ldflags)
- `.tsuku-dev` is relative to the working directory, not the binary location
- Install script change is a one-time migration for existing users

### Option 2: `--env` flag + `TSUKU_ENV` variable

Add a global `--env <name>` flag and `TSUKU_ENV` env var. When active, tsuku uses `$TSUKU_HOME/envs/<name>/` as its effective home, sharing the download cache from the parent.

Example usage:
```bash
./tsuku --env dev install cmake
```

**Pros:**
- Self-documenting in command history and logs
- Named environments are discoverable via management subcommands
- Download cache shared automatically

**Cons:**
- Adds permanent CLI surface area for a contributor problem
- Shared cache assumes stable cache format across branches (breaks with format changes)
- Environments inside `$TSUKU_HOME` means parallel sessions share state by default
- Doesn't move toward per-directory version activation (orthogonal feature)
- Requires environment name validation, path traversal prevention, new subcommands

### Option 3: Wrapper script (`scripts/dev-env`)

A shell script that sets `TSUKU_HOME` to an isolated directory and execs tsuku.

Example usage:
```bash
./scripts/dev-env install cmake
```

**Pros:**
- No binary changes at all
- Simple to understand and modify

**Cons:**
- Changes the invocation syntax (`./scripts/dev-env` instead of `./tsuku`)
- Users must know to use the script instead of the binary
- Easy to forget and run `./tsuku` directly

### Evaluation Against Drivers

| Driver | Option 1 (ldflags) | Option 2 (--env) | Option 3 (script) |
|--------|-------------------|-------------------|-------------------|
| Zero-conflict | Good: separate home per checkout | Good: separate state per name | Good: separate home |
| Parallel safety | Good: different directories | Fair: same TSUKU_HOME, shared cache | Good: different directories |
| Zero ceremony | Good: make build, then use normally | Poor: extra flag every invocation | Fair: different command |
| Useful CLI surface | Good: shellenv + doctor serve all users | Poor: env subcommands serve contributors only | Good: no changes |
| Format independence | Good: nothing shared | Poor: shared download cache | Good: nothing shared |
| Tool reachability | Good: shellenv configures PATH | Good: env flag sets PATH implicitly | Poor: must manage PATH separately |

## Decision Outcome

**Chosen option: Build-time ldflags default (Option 1)**

Option 1 is the only option that scores "Good" on every driver. It requires no new CLI surface, provides full isolation (no shared state of any kind), and makes the common case (`make build && ./tsuku install cmake`) work without extra flags or wrapper scripts.

### Rationale

Option 2 (`--env`) was the original proposal but was rejected after analysis revealed three problems:
- It adds permanent CLI surface area to solve a contributor/QA problem. The production binary shouldn't carry features that don't serve end users.
- Its shared download cache assumes format stability across branches. Parallel sessions testing storage format changes would corrupt each other's cache.
- It's orthogonal to per-directory version activation (a confirmed future goal). Building `--env` now doesn't move toward that feature and could create API commitments that constrain its design.

Option 3 (wrapper script) was rejected because it changes the invocation syntax. Developers must remember to use `./scripts/dev-env` instead of `./tsuku`. That's easy to forget and adds friction.

### Trade-offs Accepted

- **Requires `make build` instead of `go build`**: Developers who run bare `go build` won't get the dev default. This is acceptable because the Makefile is the standard build entry point, and `go build` still works -- it just uses the production default.
- **`.tsuku-dev` is relative to working directory**: Running `cd /tmp && /path/to/checkout/tsuku install cmake` creates `/tmp/.tsuku-dev`. This is acceptable because the normal workflow is running from the checkout root.
- **Install script migration**: Existing users who reinstall will get the updated env file that no longer exports `TSUKU_HOME`. The `PATH` setup still works via inline fallback. Users who explicitly set `TSUKU_HOME` in their own shell config are unaffected.

## Solution Architecture

### Overview

Three changes work together:

1. **Build-time default**: A Go variable `defaultHomeOverride` is set via ldflags during `make build`. `DefaultConfig()` checks this variable when `TSUKU_HOME` isn't set in the environment.

2. **Install script fix**: The env file stops exporting `TSUKU_HOME`, using an inline fallback in the `PATH` line instead. This ensures the build-time default takes effect for developers who have tsuku installed.

3. **Shell integration commands**: `tsuku shellenv` prints PATH configuration for the current home directory. `tsuku doctor` validates the environment is set up correctly. Both commands serve end users (alternative PATH setup, diagnostics) and contributors (dev build PATH configuration).

### Precedence Chain

```
TSUKU_HOME env var  →  ldflags defaultHomeOverride  →  ~/.tsuku
(explicit override)    (dev builds: .tsuku-dev)        (release builds)
```

### Code Changes

**`cmd/tsuku/main.go`** (or appropriate entry point):

```go
// defaultHomeOverride is set via ldflags for dev builds.
// When set, it overrides the ~/.tsuku default (but not TSUKU_HOME env var).
var defaultHomeOverride string
```

**`internal/config/config.go`**:

```go
// DefaultHomeOverride can be set by the binary's main package
// to change the default home directory (e.g., for dev builds).
var DefaultHomeOverride string

func DefaultConfig() (*Config, error) {
    tsukuHome := os.Getenv(EnvTsukuHome)
    if tsukuHome == "" {
        if DefaultHomeOverride != "" {
            tsukuHome = DefaultHomeOverride
        } else {
            home, err := os.UserHomeDir()
            if err != nil {
                return nil, fmt.Errorf("failed to get user home directory: %w", err)
            }
            tsukuHome = filepath.Join(home, ".tsuku")
        }
    }

    return &Config{
        HomeDir:          tsukuHome,
        ToolsDir:         filepath.Join(tsukuHome, "tools"),
        // ... all other paths derive from tsukuHome as before
    }, nil
}
```

**`Makefile`** (new file):

```makefile
.PHONY: build test clean

build:
	go build -ldflags "-X main.defaultHomeOverride=.tsuku-dev" -o tsuku ./cmd/tsuku

test:
	go test ./...

clean:
	rm -f tsuku
	rm -rf .tsuku-dev
```

**`website/install.sh`** (env file generation, lines 115-124):

```bash
# Before:
cat > "$ENV_FILE" << 'ENVEOF'
export TSUKU_HOME="${TSUKU_HOME:-$HOME/.tsuku}"
export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
ENVEOF

# After:
cat > "$ENV_FILE" << 'ENVEOF'
# tsuku shell configuration
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
ENVEOF
```

**`cmd/tsuku/shellenv.go`** (new file):

```go
// tsuku shellenv -- prints shell commands to configure PATH
// Output: export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
// Uses the effective home directory (respects ldflags override and TSUKU_HOME env var)
```

**`cmd/tsuku/doctor.go`** (new file):

```go
// tsuku doctor -- checks environment health
// Checks:
//   1. Home directory exists and is writable
//   2. tools/current is in PATH
//   3. State file is readable
// Exits non-zero if any check fails
```

### Directory Layout

Each checkout gets its own `.tsuku-dev`:

```
~/dev/tsuku-feature-a/
├── .tsuku-dev/              # Created on first run
│   ├── state.json
│   ├── tools/
│   ├── cache/
│   │   └── downloads/       # Independent cache (format may differ)
│   └── ...
├── tsuku                    # Built binary (make build)
└── ...

~/dev/tsuku-feature-b/
├── .tsuku-dev/              # Completely independent
│   └── ...
├── tsuku
└── ...
```

### PATH and Multi-Step Recipes

The executor manages PATH internally for sub-processes. When installing a tool with dependencies (e.g., build-essentials installs ninja, then cmake needs ninja), the executor builds `ExecPaths` from each dependency's install directory and prepends them to PATH in spawned processes. This happens at the code level (`internal/actions/cmake_build.go`, `configure_make.go`, etc.), not via the shell's PATH. Multi-step recipes work correctly regardless of which directory is on the shell's PATH.

However, after installation, running the tool directly from the shell (e.g., `cmake --version`) requires `tools/current` to be on PATH. For dev builds using `.tsuku-dev`, the shell's PATH still points to the host's `~/.tsuku/tools/current`, not `.tsuku-dev/tools/current`. Anyone who installs a tool and then wants to run it directly needs a way to fix their PATH.

Two new commands solve this:

**`tsuku shellenv`** -- prints shell commands to configure PATH for the current `$TSUKU_HOME`:

```bash
$ ./tsuku shellenv
export PATH="/home/user/dev/tsuku-feature-a/.tsuku-dev/bin:/home/user/dev/tsuku-feature-a/.tsuku-dev/tools/current:$PATH"
```

Usage:
```bash
eval $(./tsuku shellenv)
cmake --version    # works
```

This follows the pattern established by Homebrew (`eval $(brew shellenv)`), rbenv (`eval $(rbenv init -)`), and mise (`eval $(mise activate bash)`). It's useful for all users, not just contributors -- anyone who installs tsuku without the install script (e.g., via `go install` or manual download) can use `tsuku shellenv` to configure their shell.

**`tsuku doctor`** -- checks that the environment is configured correctly and exits non-zero if something is wrong:

```bash
$ ./tsuku doctor
Checking tsuku environment...
  Home directory: /home/user/dev/tsuku-feature-a/.tsuku-dev ... ok
  tools/current in PATH ... FAIL
    .tsuku-dev/tools/current is not in your PATH
    Run: eval $(./tsuku shellenv)
  State file ... ok
```

This gives scripts and CI a programmatic gate (`./tsuku doctor || exit 1`) and gives end users a diagnostic tool when things aren't working. The existing `tsuku verify` command checks a specific tool's installation; `tsuku doctor` checks the overall environment.

### Typical Development Workflow

```bash
make build                    # build with dev defaults
eval $(./tsuku shellenv)      # configure PATH for .tsuku-dev
./tsuku install cmake         # install into .tsuku-dev
cmake --version               # tool is reachable
./tsuku doctor                # verify environment is healthy
```

### Data Flow

1. Developer runs `make build` in checkout directory
2. Go compiler injects `defaultHomeOverride = ".tsuku-dev"` via ldflags
3. Developer runs `./tsuku install cmake`
4. `DefaultConfig()` checks `TSUKU_HOME` env var -- not set (install script no longer exports it)
5. Checks `DefaultHomeOverride` -- set to `.tsuku-dev`
6. Uses `.tsuku-dev` as home directory (relative to working directory)
7. `EnsureDirectories()` creates `.tsuku-dev/` tree on first use
8. Installation proceeds normally against the isolated home

## Implementation Approach

### Phase 1: Install script fix

- Change the env file template to stop exporting `TSUKU_HOME`
- Use inline `${TSUKU_HOME:-$HOME/.tsuku}` fallback in the `PATH` line
- Existing installations get the updated env file on next `tsuku` reinstall

### Phase 2: Build-time default

- Add `defaultHomeOverride` variable to `cmd/tsuku/main.go`
- Add `DefaultHomeOverride` to `internal/config/config.go`
- Modify `DefaultConfig()` to check the override
- Create `Makefile` with `build`, `test`, and `clean` targets
- Add `.tsuku-dev` to `.gitignore`

### Phase 3: Shell integration commands

- Add `tsuku shellenv` command that prints `export PATH=...` for the current home directory
- Add `tsuku doctor` command that validates environment health (home dir, PATH, state file)
- `doctor` exits non-zero on failure for use as a CI/script gate

### Phase 4: Documentation and CI

- Document `make build` workflow in CONTRIBUTING.md
- Update Build Essentials CI to use `make build` where appropriate
- Add note to CLAUDE.md: "Use `make build` to build tsuku"

## Security Considerations

### Download Verification

No change. Download verification operates independently of which home directory is active.

### Execution Isolation

Dev builds don't provide execution isolation. Binaries installed in one checkout's `.tsuku-dev` run with the same user permissions as any other process. This matches the existing `$TSUKU_HOME` behavior. Container-based execution isolation is the `--sandbox` feature's domain.

### Supply Chain Risks

No change. Dev builds use the same recipe sources and verification pipeline as release builds. The home directory override doesn't affect where recipes or downloads come from.

### User Data Exposure

The `.tsuku-dev` directory is local to each checkout. It isn't transmitted externally. Adding it to `.gitignore` prevents accidental commits. No new data exposure.

## Consequences

### Positive
- Contributors get isolation by default, just by using `make build`
- Parallel checkouts are fully isolated, including download cache
- Branches changing storage format can't corrupt other checkouts' state
- The install script fix is independently correct (the binary shouldn't depend on the shell setting its home directory)
- `tsuku shellenv` gives all users a way to configure PATH without the install script
- `tsuku doctor` gives all users a diagnostic tool for environment issues

### Negative
- Developers must use `make build` instead of bare `go build` to get dev defaults
- `.tsuku-dev` is relative to working directory, which could surprise developers who run tsuku from a different directory
- Existing users need to reinstall (or re-run install script) to get the updated env file
- Two new commands (`shellenv`, `doctor`) add CLI surface area, though both serve end users

### Mitigations
- `make build` is documented as the standard build command in CONTRIBUTING.md and CLAUDE.md
- `make clean` removes `.tsuku-dev` for a fresh start
- The env file change is backward-compatible: users who explicitly set `TSUKU_HOME` in their own shell config are unaffected

---
status: Proposed
problem: Developers working on tsuku lack a low-ceremony way to run against isolated environments without interfering with their real installation or each other.
decision: Add --env flag and TSUKU_ENV environment variable that create named environments under $TSUKU_HOME/envs/ with automatic download cache sharing.
rationale: The combined flag + env var pattern eliminates manual TSUKU_HOME juggling, provides discoverability through tsuku env list, and shares cache automatically without requiring per-environment config files.
---

# DESIGN: Dev Environment Isolation

## Status

Proposed

## Context and Problem Statement

When developing tsuku, you need to run your local build to test recipe changes, action modifications, and CLI behavior. Right now, that means either running against your real `$TSUKU_HOME` (risking your working installation) or manually setting `TSUKU_HOME` to a temp directory every time.

Neither approach works well. Running against your real home pollutes it with test artifacts. Manually exporting a new `TSUKU_HOME` is tedious, easy to forget, and doesn't solve the parallel execution problem. If two terminal sessions both run `./tsuku install cmake` against the same directory, the file lock prevents corruption but one process blocks until the other finishes. For CI and automated testing, that serialization is unacceptable.

The Build Essentials workflow already demonstrates this need: each macOS test creates a fresh `TSUKU_HOME` per tool to avoid interference. That pattern works but it's ad-hoc and not available to developers outside CI.

The primary audience is tsuku contributors (developing the CLI, testing recipes) and CI workflows. End users managing project-specific toolchains may also benefit, but that's a secondary concern.

### Scope

**In scope:**
- A CLI mechanism for running tsuku against an isolated, named environment
- Cache sharing between environments (downloads)
- Parallel-safe execution across environments
- State persistence across invocations of the same environment
- Throwaway environments for quick one-off tests

**Out of scope:**
- Container-based isolation (the sandbox feature already covers that)
- Changes to the file locking mechanism
- Multi-user isolation or security boundaries

## Decision Drivers

- **Zero-conflict isolation**: A dev environment must never read or write the user's real `$TSUKU_HOME/state.json`
- **Parallel safety**: Multiple environments must be usable concurrently without blocking each other on locks
- **Cache reuse**: Downloaded bottles and tarballs shouldn't be re-downloaded per environment
- **Low ceremony**: Creating and using an environment shouldn't require more than one extra flag or env var
- **Discoverability**: Users should be able to tell which environment they're operating in, both via CLI output and via management subcommands
- **Stateful across runs**: Installing a tool in a dev environment should persist until the environment is cleaned up

## Implementation Context

### Existing Patterns

The codebase already has two isolation patterns:

**CI test isolation** (Build Essentials macOS jobs): Each test sets `TSUKU_HOME` to a fresh temp directory and symlinks the download cache from a shared location. This gives full isolation with cache reuse, but it's manual shell scripting not accessible through the CLI.

**Sandbox isolation** (`--sandbox` flag): Runs installation inside a Docker/Podman container with a fresh `TSUKU_HOME` at `/workspace/tsuku` and the download cache mounted read-only. Full isolation, but heavyweight (requires a container runtime) and doesn't persist state across runs.

### Conventions to follow

- All paths derive from `$TSUKU_HOME` via `DefaultConfig()` in `internal/config/config.go`
- Download cache uses content-addressed hashing (`sha256(url).data`) making it safe to share
- Cache directory rejects symlinks for write operations but allows read-only mounts
- State file uses advisory file locking (`flock`) for concurrent access
- `EnsureDirectories()` creates all subdirectories from `$TSUKU_HOME` on first use

### Anti-patterns to avoid

- Don't bypass `DefaultConfig()` for path resolution. All directory layout decisions flow from `$TSUKU_HOME`.
- Don't hard-share `state.json` across environments. The file lock prevents corruption, but sharing state between a dev and production environment defeats the isolation purpose.

## Considered Options

### Option 1: `--env` flag + `TSUKU_ENV` variable (combined)

Support both a `--env <name>` global CLI flag and a `TSUKU_ENV` environment variable. The flag takes precedence when both are set. When active, tsuku uses `$TSUKU_HOME/envs/<name>/` as its effective home, sharing the download cache from the parent.

This follows the `kubectl --context` / `KUBECONFIG` pattern where a flag handles single invocations and the env var handles session-wide activation.

Example usage:
```bash
# Single command
./tsuku --env dev install cmake

# Session-wide
export TSUKU_ENV=dev
./tsuku install cmake
./tsuku list

# Management
./tsuku env list
./tsuku env clean dev
```

**Pros:**
- Covers both single-command and session-wide workflows
- Named environments are discoverable via `tsuku env list`
- State persists naturally (just a directory)
- Parallel-safe: different names = different lock files, different state files
- Download cache shared automatically
- Visible in `--help`, env var works with direnv
- Flag appears in command history, making bug reports reproducible

**Cons:**
- Adds a global flag and an env var, slightly more surface area
- Cache sharing mechanism needs to handle the symlink security check
- Environment directories inside `$TSUKU_HOME` could confuse users who `ls ~/.tsuku`

### Option 2: Standalone `$TSUKU_HOME` with shared cache config

Keep the existing `TSUKU_HOME` override as the only isolation primitive. Add a `cache.shared_downloads` config key in `$TSUKU_HOME/config.toml` pointing to an external download cache.

This is essentially the status quo with one addition: a way to share cached downloads across independent `TSUKU_HOME` directories without symlinks.

Example usage:
```bash
export TSUKU_HOME=/tmp/tsuku-dev
mkdir -p /tmp/tsuku-dev
cat > /tmp/tsuku-dev/config.toml <<EOF
[cache]
shared_downloads = "/home/user/.tsuku/cache/downloads"
EOF
./tsuku install cmake
```

**Pros:**
- Uses existing `TSUKU_HOME` mechanism, no new abstraction
- Maximum flexibility: environments can be anywhere
- Cache sharing is explicit and configurable
- Clear mental model: `TSUKU_HOME` is a complete, self-contained root

**Cons:**
- High ceremony: two steps (set env var + write config) for cache sharing
- No discovery mechanism (`tsuku env list` can't exist)
- No visual indicator of which environment you're in
- Users manage directory cleanup themselves
- Reproduces the manual pattern developers already find tedious
- Env vars are invisible in command history, making bugs harder to reproduce

### Evaluation Against Drivers

| Driver | Option 1 (--env + TSUKU_ENV) | Option 2 (Standalone TSUKU_HOME) |
|--------|------------------------------|----------------------------------|
| Zero-conflict | Good: separate state per name | Good: separate TSUKU_HOME |
| Parallel safety | Good: separate lock files | Good: separate lock files |
| Cache reuse | Good: automatic sharing | Fair: manual config needed |
| Low ceremony | Good: one flag or env var | Poor: multi-step setup |
| Discoverability | Good: env list, --help | Poor: no discovery |
| Stateful | Good: named dirs persist | Good: any dir persists |

## Decision Outcome

**Chosen option: `--env` flag + `TSUKU_ENV` variable (Option 1)**

Option 1 addresses every decision driver at "Good" while Option 2 only matches on isolation and statefulness. The combined flag + env var pattern is well-established (kubectl, terraform, aws-cli) and eliminates the main usability gap: developers shouldn't need to manually set up config files to get cache sharing.

### Rationale

This option was chosen because:
- **Low ceremony** is the primary driver. The whole point is to replace manual `TSUKU_HOME` juggling with something that takes a single flag. Option 2 doesn't move the needle on ceremony.
- **Discoverability** matters for contributors who don't work on tsuku daily. `tsuku env list` and `tsuku --env dev` are self-documenting. A bare `TSUKU_HOME` override is not.
- **Cache sharing must be automatic.** Requiring a config file per environment means developers will skip it and re-download everything, or they'll symlink the cache and hit the security check.

Option 2 was rejected because it formalizes the status quo rather than solving the problem. The only addition (a config key for shared downloads) doesn't remove enough friction.

### Trade-offs Accepted

By choosing this option, we accept:
- **Slightly more CLI surface area**: A new global flag and env var. This is acceptable because the feature is opt-in and zero-impact for users who don't use it.
- **Environments live inside `$TSUKU_HOME`**: The `envs/` directory may surprise users exploring `~/.tsuku`. This is acceptable because `ls ~/.tsuku` already shows internal directories (cache, registry, tools), and `envs/` is self-explanatory.
- **Shared cache increases blast radius**: A poisoned cache entry affects all environments, not just one. This is the same trust model as the current single-`TSUKU_HOME` setup, so no regression, but worth noting.

## Solution Architecture

### Overview

When `--env <name>` or `TSUKU_ENV=<name>` is active, `DefaultConfig()` rewrites the home directory to `$TSUKU_HOME/envs/<name>/` and sets the download cache to the parent's `$TSUKU_HOME/cache/downloads/`. Every other path (tools, libs, state, registry) derives from the environment's home directory as usual.

### Environment Name Validation

Environment names must match `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`. This prevents:
- **Path traversal**: Names like `../../tools` that escape the `envs/` directory
- **Empty names**: The regex requires at least one character
- **Hidden directories**: Names can't start with `.`
- **Special characters**: No slashes, backslashes, or spaces

After validation, the implementation must verify that `filepath.Join(tsukuHome, "envs", envName)` resolves to a path under `$TSUKU_HOME/envs/` using `filepath.Rel()`. This catches edge cases the regex might miss on different platforms.

### Directory Layout

```
~/.tsuku/                          # Real TSUKU_HOME (unchanged)
├── state.json                     # User's real state
├── tools/                         # User's installed tools
├── cache/
│   └── downloads/                 # Shared download cache (content-addressed)
├── envs/                          # NEW: environment root
│   ├── dev/                       # Named environment "dev"
│   │   ├── state.json             # Environment-specific state
│   │   ├── state.json.lock        # Environment-specific lock
│   │   ├── tools/                 # Environment-specific tools
│   │   ├── libs/                  # Environment-specific libraries
│   │   ├── bin/                   # Environment-specific symlinks
│   │   ├── registry/              # Environment-specific registry cache
│   │   └── cache/
│   │       └── versions/          # Environment-specific version cache
│   └── ci-cmake/                  # Another environment
│       └── ...
```

Environments don't get their own `config.toml` or `cache/downloads/`. The config file is the parent's, and the download cache is shared via the `Config` struct (see below).

### Cache Sharing Mechanism

The download cache is shared via a config-level override, not filesystem links. When `DefaultConfig()` detects an environment, it sets `DownloadCacheDir` to the parent's `$TSUKU_HOME/cache/downloads/` directly in the `Config` struct. The `DownloadCache` code receives this path and operates normally. No symlinks, no security check changes.

The symlink approach (considered as a fallback) is explicitly rejected because:
- It conflicts with the existing `containsSymlink()` security check
- It behaves differently on Windows
- The config-level approach is simpler and doesn't touch security code

### Key Interfaces

**Config changes** (`internal/config/config.go`):

```go
const EnvTsukuEnv = "TSUKU_ENV"

func DefaultConfig() (*Config, error) {
    tsukuHome := os.Getenv(EnvTsukuHome)
    if tsukuHome == "" {
        home, _ := os.UserHomeDir()
        tsukuHome = filepath.Join(home, ".tsuku")
    }

    parentDownloadCache := filepath.Join(tsukuHome, "cache", "downloads")

    envName := os.Getenv(EnvTsukuEnv)
    if envName != "" {
        if err := ValidateEnvName(envName); err != nil {
            return nil, err
        }
        envHome := filepath.Join(tsukuHome, "envs", envName)
        // Verify resolved path is under envs/
        if !isUnder(envHome, filepath.Join(tsukuHome, "envs")) {
            return nil, fmt.Errorf("invalid environment name: path escapes envs directory")
        }
        tsukuHome = envHome
    }

    cfg := &Config{
        HomeDir:          tsukuHome,
        ToolsDir:         filepath.Join(tsukuHome, "tools"),
        // ... all other paths derive from tsukuHome as before
        DownloadCacheDir: parentDownloadCache, // Always use parent's download cache
    }
    return cfg, nil
}
```

**CLI changes** (`cmd/tsuku/root.go`):

```go
// Global persistent flag
rootCmd.PersistentFlags().StringVar(&envFlag, "env", "", "use named environment for isolation")

// In PersistentPreRun: if envFlag is set, override TSUKU_ENV
```

**New subcommand** (`cmd/tsuku/env.go`):

```go
// tsuku env list    - list environments under $TSUKU_HOME/envs/
// tsuku env clean   - remove a named environment (acquires lock first)
// tsuku env info    - show environment details (path, size, tool count)
```

### Data Flow

1. User invokes `tsuku --env dev install cmake`
2. Cobra's persistent pre-run sets `TSUKU_ENV=dev` (flag overrides env var)
3. `DefaultConfig()` validates the name, computes `$TSUKU_HOME/envs/dev/` as effective home
4. `DownloadCacheDir` stays at `$TSUKU_HOME/cache/downloads/` (parent's cache)
5. `EnsureDirectories()` creates the environment's directory tree (but not download cache)
6. Installation proceeds normally against the environment's state and tools
7. `state.json` under `envs/dev/` records the installation
8. Binaries are symlinked in `envs/dev/tools/current/`, following the existing `CurrentSymlink()` pattern

### Flag Interactions

- **`--env` + `--sandbox`**: These are compatible. `--env` determines the state and tool directories; `--sandbox` runs the installation in a container. The environment's state records the result.
- **`--env` + `TSUKU_HOME`**: `TSUKU_HOME` sets the root, then `--env` creates the environment under that root. `TSUKU_HOME=/tmp/alt --env dev` uses `/tmp/alt/envs/dev/`.
- **`--env` + `TSUKU_ENV`**: Flag takes precedence over env var.

### Environment Indicator

When operating in an environment, tsuku prints a one-line notice on commands that modify state:

```
[env: dev] Installing cmake...
```

The `tsuku config` command shows the active environment and its effective paths.

## Implementation Approach

### Phase 1: Core environment support

- Add `ValidateEnvName()` with regex and path-escape check
- Modify `DefaultConfig()` to honor `TSUKU_ENV` and compute paths
- Add `--env` persistent flag to root command, set env var in pre-run
- Pass parent download cache path through Config struct
- Ensure `EnsureDirectories()` creates environment tree (skipping download cache dir)

### Phase 2: Management subcommands and UX

- `tsuku env list` -- enumerate `$TSUKU_HOME/envs/` directories with sizes
- `tsuku env clean <name>` -- acquire state lock, then remove environment directory
- `tsuku env info <name>` -- show path, disk usage, installed tools
- Add environment indicator to state-modifying commands

### Phase 3: CI integration

- Update Build Essentials macOS tests to use `--env` instead of manual `TSUKU_HOME` export
- Remove the symlink-based cache sharing workaround
- Document environment usage in contributor guide

## Security Considerations

### Download Verification

No change to the verification pipeline. Download verification (checksums, PGP signatures) operates independently of which environment is active.

The shared download cache does increase the blast radius of a cache poisoning event: a poisoned entry would be served to all environments, not just the one that wrote it. However, this is the same trust model as today's single-`TSUKU_HOME` setup. The cache keys on `sha256(url)`, and the existing verification pipeline checks content integrity after download. No new attack surface is introduced, but the blast radius note is worth keeping in mind for future cache hardening work.

### Execution Isolation

Environments don't provide execution isolation. Binaries installed in environment A run with the same user permissions and have full filesystem access to environment B. This is intentional and matches the existing `TSUKU_HOME` behavior. The feature isolates state (what's installed, which versions), not execution privileges. Container-based execution isolation is the `--sandbox` feature's domain.

### Supply Chain Risks

No change. Environments use the same recipe sources and verification pipeline as the parent. An environment doesn't alter where recipes or bottles come from.

### User Data Exposure

Environment names are stored on the local filesystem under `$TSUKU_HOME/envs/`. They aren't transmitted externally. The telemetry system (if enabled) doesn't report environment names. No new data exposure.

### Input Validation

Environment names are validated against `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$` and the resolved path is verified to stay under `$TSUKU_HOME/envs/`. This prevents path traversal attacks like `--env ../../tools` that would otherwise escape the `envs/` directory and potentially overwrite the user's real installation.

## Consequences

### Positive
- Contributors can test recipe and CLI changes without risking their working installation
- CI workflows can replace ad-hoc `TSUKU_HOME` + symlink scripts with `--env`
- Parallel CI jobs use different environment names, eliminating lock contention
- Downloaded files are shared across all environments, saving bandwidth and disk

### Negative
- `~/.tsuku/envs/` adds directory clutter that may accumulate if environments aren't cleaned up
- The `--env` flag adds cognitive load for new contributors ("what's an env?")
- Shared cache means a poisoned download entry affects all environments (same as today, but more explicitly shared)

### Mitigations
- `tsuku env list` shows environments with disk usage, making stale ones visible
- `tsuku env clean` makes removal easy
- Documentation explains environments as a development tool, not a required concept
- Future cache hardening (content-hash verification on read) would mitigate the shared cache risk

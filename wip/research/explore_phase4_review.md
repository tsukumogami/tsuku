# Phase 4 Review: Dev Environment Isolation Design

## 1. Problem Statement Evaluation

The problem statement is well-scoped and specific. It identifies three concrete pain points:
- Risk of polluting a real `$TSUKU_HOME`
- Manual `TSUKU_HOME` export is tedious and error-prone
- Parallel execution serializes on file locks

The CI Build Essentials example grounds the problem in existing code. The scope section draws clear boundaries (container isolation is out of scope, for instance).

**One gap**: The problem doesn't quantify who the audience is beyond "developers." Is this primarily for tsuku contributors, or also for users who maintain multiple tool environments (e.g., project-specific toolchains)? The answer changes how much weight "low ceremony" and "discoverability" deserve. The current framing leans contributor-focused, which is fine, but should be stated explicitly.

## 2. Missing Alternatives

Two alternatives worth considering:

**A. Combined flag + env var (Option 1 + 2 hybrid)**: The `--env` flag and `TSUKU_ENV` variable aren't mutually exclusive. The flag could set the env for a single invocation while the env var sets it for a session. Many CLI tools do this (e.g., `kubectl --context` vs `KUBECONFIG`). This eliminates the main cons of both options without introducing new ones. The design should address why these are presented as competing rather than complementary.

**B. `tsuku dev` subcommand wrapper**: A subcommand like `tsuku dev install cmake` that implicitly creates/uses a default dev environment. This is a UX layer on top of Option 1 or 2 -- it reduces ceremony further by not requiring a name for the common single-environment case. Worth mentioning even if it's a follow-on feature.

## 3. Pros/Cons Fairness Assessment

**Option 1 (`--env` flag)**:
- The "threads through every command" con is real but overstated. It's a config resolution change in `DefaultConfig()`, not per-command logic. The flag just sets a value that config reads once.
- Missing pro: the flag appears in `--help`, making the feature self-documenting.

**Option 2 (`TSUKU_ENV` env var)**:
- The "trivial to implement" pro is accurate and significant. This is genuinely the smallest change.
- The "less discoverable" con is valid but could be mitigated by mentioning it in `--help` output or `tsuku env` subcommand docs.
- Missing con: env vars are invisible in command history. If a user reports a bug while `TSUKU_ENV` is set, the reproduction steps won't capture that context unless they remember to mention it.

**Option 3 (standalone `$TSUKU_HOME` + config)**:
- The cons are fair and this option correctly reads as the highest-ceremony approach.
- Missing pro: this is the only option that works for completely separate `$TSUKU_HOME` roots on different filesystems or partitions.
- The "exactly the pattern developers find tedious today" con is the strongest argument against it.

## 4. Unstated Assumptions

1. **Environments live under `$TSUKU_HOME`**: Options 1 and 2 both assume `$TSUKU_HOME/envs/<name>/` as the layout. This means the real `$TSUKU_HOME` must exist and be writable even when using a dev environment. If someone sets `TSUKU_HOME` to a read-only location (network mount, shared install), environments can't be created there.

2. **Named environments are sufficient**: The design assumes developers want named, persistent environments. Some use cases (quick throwaway test) might benefit from anonymous/auto-cleaned environments. This isn't discussed.

3. **Download cache is the only shared resource**: The design focuses on sharing the download cache, but the recipe registry (`$TSUKU_HOME/registry/`) is another candidate for sharing. Re-downloading or re-syncing the registry per environment adds latency.

4. **Single-machine scope**: The design assumes environments are local. No mention of whether environment definitions could be shared (e.g., checked into a project repo).

## 5. Strawman Analysis

**Option 3 is borderline strawman.** Its final con ("exactly the pattern developers find tedious today") directly contradicts the problem statement. The option exists to show that the status quo mechanism is insufficient, which is valid framing, but it's clearly not a real contender. The document would be more honest to label it as "baseline/status quo" rather than presenting it as an equal option.

Options 1 and 2 are both viable and fairly presented. Neither is designed to fail.

## 6. Recommendations

1. **Combine Options 1 and 2** rather than choosing between them. The `--env` flag for single commands and `TSUKU_ENV` for sessions are complementary, not competing. Flag takes precedence when both are set.

2. **Make the audience explicit** in the problem statement: is this for tsuku contributors only, or also end users managing project-specific toolchains?

3. **Address registry sharing** alongside download cache sharing. Both are read-heavy resources that benefit from deduplication.

4. **Relabel Option 3** as the baseline/status quo to set honest expectations about the option space.

5. **Resolve the symlink uncertainty** before proceeding. The cache-sharing mechanism is central to all options and the current uncertainty could invalidate implementation assumptions.

# Config-Based Activation: Skip --env, Build The Real Feature

## The Core Argument

The `--env` flag solves contributor isolation by creating separate state directories. But it's a temporary solution to a problem that config-based activation solves permanently **and** more powerfully. Here's why tsuku should skip `--env` entirely and jump straight to `.tsuku.toml`:

**Contributor isolation isn't about separate installations—it's about version pinning.** When a contributor works on tsuku, they need specific versions of cmake, ninja, etc. that won't interfere with their personal projects. The `--env` approach creates separate `$TSUKU_HOME/envs/test/` directories, but this is just emulating what `.tsuku.toml` does better: declaring "this directory needs cmake@3.25.0, ninja@1.11.0" and making those versions active only in that context.

With config-based activation, the tsuku repo itself gets a `.tsuku.toml` specifying test tool versions. When a contributor runs `tsuku activate` (or a shell hook loads the config), their PATH adjusts to point to those exact versions. Personal projects in other directories have their own `.tsuku.toml` files (or none, falling back to defaults). No manual environment switching, no separate state files, no "which env am I in?" confusion. The directory IS the environment.

## How This Solves Contributor Isolation Without --env

Current problem: Developers working on tsuku need cmake, ninja, etc. for tests, but can't pollute their global `~/.tsuku/bin/` with test-specific versions. The `--env test` proposal solves this by creating `$TSUKU_HOME/envs/test/state.json` and `envs/test/bin/`, but this requires manual switching (`tsuku --env test install cmake@3.25.0`).

Config-based solution: The tsuku repo ships with `.tsuku.toml`:
```toml
[tools]
cmake = "3.25.0"
ninja = "1.11.0"
gh = "2.40.0"
```

Contributors run `tsuku install` (which reads `.tsuku.toml` and installs all specified versions to the shared `$TSUKU_HOME/tools/cmake-3.25.0/`, etc.). Then `tsuku activate` or a shell hook sets up PATH to prioritize these versions when in the tsuku directory. Step outside the repo, PATH reverts to global defaults or another project's `.tsuku.toml` settings.

**Key insight:** This isn't just equivalent to `--env test`—it's strictly better. With `--env`, you still need to remember to pass the flag. With `.tsuku.toml`, activation is automatic and directory-scoped. The config file is version-controlled, so every contributor gets identical tool versions. CI uses the same file (no manual `TSUKU_HOME` juggling—just `tsuku install && tsuku activate`).

## This IS The Destination, Not A Stepping Stone

The owner said they won't add `--env` unless it's on the path to per-directory activation. The truth is, `.tsuku.toml` **is** per-directory activation. Here's the architecture:

1. **Shared installation directory:** All tools install to `$TSUKU_HOME/tools/name-version/` (just like now, no change). This is content-addressed and version-coexisting by design.

2. **Config file per project:** `.tsuku.toml` declares required versions. Example:
   ```toml
   [tools]
   cmake = "3.25.0"
   node = "20.10.0"
   ```

3. **Activation mechanism:** Two approaches (MVP uses simpler option):
   - **Shim-based (MVP):** `tsuku activate` creates shims in `$TSUKU_HOME/shims/` (a directory always on PATH). Each shim reads the nearest `.tsuku.toml` walking up from `$PWD`, then execs the right version from `tools/name-version/bin/`.
   - **Shell hook (full feature):** Shell integration (like mise's `eval "$(tsuku hook-env)"`) adjusts PATH dynamically when you `cd` into a directory with `.tsuku.toml`.

4. **State tracking:** Global `state.json` still tracks installed tools. No per-environment state needed—the config file declares intent, `state.json` tracks reality.

This model **completes** per-directory versioning. You don't need to layer it on top of `--env` later. In fact, adding `--env` first makes the migration harder—you'd have to explain why environments exist if configs replace them, or maintain both systems forever.

## MVP Scope vs Full Feature

**MVP (functional, minimal shell integration):**
- `.tsuku.toml` parsing (TOML library already exists in Go ecosystem)
- `tsuku install` reads config and installs all tools to shared `$TSUKU_HOME/tools/`
- `tsuku activate` generates shims in `$TSUKU_HOME/shims/` (user adds to PATH once)
- Shims walk up directory tree to find `.tsuku.toml`, exec correct version
- `tsuku deactivate` removes shims (or shims become no-ops outside configured projects)

**Full feature (shell hook integration):**
- Shell hooks for bash/zsh/fish that adjust PATH dynamically on `cd`
- Performance optimizations (cache config lookups, avoid repeated FS walks)
- `tsuku which cmake` shows which version is active in current directory
- `tsuku current` displays active config and versions
- Global fallbacks (if no `.tsuku.toml` found, use latest installed version or configurable default)

**Estimate:**
- MVP: ~2-3 days of focused development (TOML parsing, shim generation, directory walking logic)
- Full feature: +1-2 days (shell hook templates, edge case handling, polish)

Compared to `--env`: `--env` is ~1 day to add separate state files and `--env` flag parsing. But then you still need to build `.tsuku.toml` later (~3 days), PLUS migration complexity (how do `--env` and configs interact?). Total: 4+ days and technical debt. Jumping straight to configs: 3-5 days, no debt.

## Risks of Jumping Straight to Config-Based vs Incremental --env

**Risk 1: Shim overhead.** Shims add a process execution per tool invocation. If the shim has to walk the directory tree on every run, this could slow down tight loops (e.g., a build script calling cmake hundreds of times).

Mitigation: Cache the config lookup. Shims can read an environment variable set by the shell hook (or by `tsuku activate`) that points directly to the active `.tsuku.toml`. This makes shims nearly zero-overhead. Alternatively, use shell hooks from day one (slightly more MVP complexity, but eliminates shim overhead).

**Risk 2: User confusion about activation.** Unlike `--env` (which is explicit), config-based activation is "magic"—your PATH changes based on directory. If it's not obvious when activation is happening, users might be confused about which version they're running.

Mitigation: Make activation visible. `tsuku activate` prints "Activated .tsuku.toml from /path/to/project" and lists versions. Shell hooks update the prompt (optional, like mise's prompt indicator). `tsuku current` shows active config at any time.

**Risk 3: Shell hook compatibility.** Supporting bash, zsh, fish, and other shells requires maintaining separate hook templates and handling edge cases (non-interactive shells, restricted environments, etc.). This is complex and error-prone.

Mitigation: Start with shim-based MVP. Shims work in any shell and don't require integration. Add shell hooks as a performance/UX enhancement later. This keeps the MVP simple while proving the concept.

**Risk 4: Config file bikeshedding.** Users might want YAML, JSON, or other formats. The schema might need iteration (global defaults? environment-specific overrides?).

Mitigation: Start minimal. `.tsuku.toml` with just `[tools]` section mapping names to versions. Copy mise's proven design—they've already done the bikeshedding. Extend later if there's demand (e.g., `[env]` section for environment variables, `[tasks]` for project commands).

**Risk 5: Breaking existing workflows.** Users who rely on global `~/.tsuku/bin/` might not want per-directory activation. If activation is mandatory, this is a breaking change.

Mitigation: Activation is opt-in. If no `.tsuku.toml` exists, tsuku works exactly like it does today (global bin/ directory, latest versions). Projects that want pinning add a config file. No user is forced to change.

## Cache Sharing: Already Solved

This is the strongest argument for skipping `--env`. The current architecture already supports multiple versions coexisting in `$TSUKU_HOME/tools/`. Content-addressed downloads mean `cmake-3.25.0` and `cmake-3.26.0` share the download cache if they have identical tarballs (rare, but possible for rebuilds).

With `--env`, you'd either:
- Duplicate tools across environments (wastes disk space), OR
- Symlink `envs/test/tools/cmake-3.25.0` → `tools/cmake-3.25.0` (adds complexity, defeats the point of separate envs)

With `.tsuku.toml`, there's no duplication. All tools live in `$TSUKU_HOME/tools/name-version/`. Activation just changes which version's bin/ directory is on PATH (via shims or shell hooks). Two projects needing `cmake@3.25.0` use the same installation. Zero wasted space, zero cache coordination issues.

## Why This Is The Right Call

The `--env` flag is a workaround for the absence of config-based activation. It solves contributor isolation by brute force (separate everything), but it's a dead-end design. You can't build per-directory activation on top of it without awkward overlaps (what if a directory has both an `--env` flag and a `.tsuku.toml`?).

Config-based activation solves contributor isolation **as a side effect** of solving the real problem: making tool versions project-scoped. It's more powerful (automatic activation), more maintainable (one mechanism, not two), and more aligned with user expectations (mise, asdf, and other modern version managers all use configs, not CLI flags).

The MVP is tractable (shim-based activation is ~2-3 days of work). The risk is manageable (shims are simple, shell hooks are optional). The payoff is immediate (contributors get isolation, users get per-directory versioning, CI gets declarative tool management).

**Skip `--env`. Build `.tsuku.toml`. Ship the real feature.**

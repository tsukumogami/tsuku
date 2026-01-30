# Architecture Review: Dev Environment Isolation

## 1. Is the architecture clear enough to implement?

**Yes, with minor gaps.** The design gives a clear data flow (steps 1-8), directory layout, and pseudocode for `DefaultConfig()` changes. An implementer can start working from this document. However, several details need resolution before code:

### Gap: No `BinDir` in Config struct

The design's directory layout shows `envs/<name>/bin/` for environment-specific symlinks, and step 8 of the data flow says "Binaries are symlinked in `envs/dev/bin/`, not in `$TSUKU_HOME/bin/`." However, the current `Config` struct has no `BinDir` field. Symlinks are managed via `CurrentSymlink()` which uses `CurrentDir` (`$TSUKU_HOME/tools/current`), not a standalone bin directory.

This means either:
- The design assumes a `BinDir` field will be added (not stated)
- Or the design's `bin/` directory is actually `tools/current/` under a different name

This needs clarification. The `CurrentSymlink` approach (symlinks inside `tools/current/`) would work fine for environments since `CurrentDir` already derives from `HomeDir`. But the design explicitly shows `bin/` as a separate directory, which would be a new concept.

### Gap: `--env` flag injection into `DefaultConfig()`

The design says "CLI --env flag overrides env var (set by cobra persistent pre-run)" but `DefaultConfig()` is called from 29 different call sites. The flag's value must be set as an env var before any `DefaultConfig()` call, or `DefaultConfig()` needs a parameter. The design implies setting `TSUKU_ENV` in the persistent pre-run hook, which is clean -- but it means `DefaultConfig()` reads `TSUKU_ENV` that was set programmatically by cobra, not by the user. This works but should be documented to avoid confusion when debugging.

### Gap: Environment name validation

No constraints on environment names. Should reject names containing path separators, dots, or special characters to prevent directory traversal. The design should specify valid name patterns (e.g., `[a-zA-Z0-9_-]+`).

## 2. Are there missing components or interfaces?

### Missing: State file path derivation

The state file path is hardcoded as `filepath.Join(cfg.HomeDir, "state.json")` throughout `internal/install/`. When `HomeDir` changes to the environment path, this works automatically. Good -- but the design doesn't mention this explicitly, and it would help implementers to know this "just works."

### Missing: Lock file behavior

The design says environments get separate lock files (`state.json.lock` in the layout). The current locking code presumably uses `state.json.lock` adjacent to `state.json`. If that path is derived from `HomeDir`, it works. But this should be verified -- if the lock path is hardcoded or derived differently, parallel environments could still contend on the same lock.

### Missing: `tsuku config` integration

The design mentions "`tsuku config` command shows the active environment" but doesn't specify what fields to add. This is minor but worth noting for Phase 1 scope.

### Missing: Interaction with `--sandbox`

What happens with `tsuku --env dev --sandbox install cmake`? The design says sandbox is out of scope, but the interaction isn't addressed. Should `--env` be ignored inside sandbox mode? Should it error? This edge case will come up.

### Not missing but worth noting: Registry cache

The design's directory layout shows `registry/` per environment. The "Uncertainties" section questions whether to share registry cache. Since each environment gets its own `HomeDir` and `RegistryDir` derives from it, they'll naturally be separate. This is the right default -- a dev environment might test against a modified registry.

## 3. Are the implementation phases correctly sequenced?

**Yes.** The phasing is sound:

- **Phase 1** (core support) is correctly the foundation. Modifying `DefaultConfig()` and adding the flag must come first since everything else depends on it.
- **Phase 2** (management subcommands) depends on Phase 1's directory structure existing. Correct ordering.
- **Phase 3** (CI integration) depends on Phases 1-2 being stable. Correct ordering.

**One refinement:** Phase 1 includes "Add environment indicator to state-modifying commands." This could slip to Phase 2 without blocking anything. It's UI polish, not core plumbing. Keeping Phase 1 smaller reduces risk.

## 4. Are there simpler alternatives we overlooked?

### Alternative considered but not in the doc: Config struct parameter

Instead of env var injection, `DefaultConfig()` could accept an optional env name parameter:

```go
func DefaultConfig(opts ...Option) (*Config, error)
```

This is more explicit than setting `TSUKU_ENV` programmatically in a pre-run hook. It avoids the "who set this env var?" confusion. However, it requires changing 29 call sites. The env var approach changes zero call sites. The design's choice is pragmatically correct.

### Alternative: Simpler `--home` flag

Instead of `--env`, a `--home /path/to/dir` flag that directly overrides `TSUKU_HOME` with automatic cache sharing. This is essentially Option 2 but with a flag instead of only an env var. It's simpler (no `envs/` directory, no name management) but loses discoverability (`env list`) and the ergonomic naming. The design's choice is better for the stated audience (contributors).

### Possible simplification: Skip `env info`

`tsuku env info <name>` (showing path, disk usage, installed tools) is nice-to-have. `tsuku env list` with basic size info covers 90% of the use case. Could defer `env info` to reduce scope.

## 5. Config code compatibility assessment

The current `DefaultConfig()` in `internal/config/config.go` is well-structured for this change:

**Works cleanly:**
- All paths derive from `tsukuHome` local variable. Inserting the env rewrite between the `tsukuHome` resolution and the struct construction is a clean 5-line addition matching the design's pseudocode exactly.
- `EnsureDirectories()` iterates `Config` struct fields, so it will automatically create the environment's directory tree. No changes needed.
- `ToolDir()`, `ToolBinDir()`, `CurrentSymlink()`, `LibDir()`, `AppDir()` all derive from Config fields. They work unchanged.

**Needs attention:**
- `DownloadCacheDir` must point to the parent's download cache, not the environment's. The design handles this: compute `parentDownloadCache` before rewriting `tsukuHome`, then set `DownloadCacheDir` to the parent path. Clean.
- `EnsureDirectories()` will call `MkdirAll` on the parent's download cache dir (since it's in the struct). This is fine -- the parent dir already exists or gets created.
- `ConfigFile` will point to `envs/<name>/config.toml`. Environments shouldn't need their own config file. Consider keeping `ConfigFile` pointing to the parent's config. The design doesn't address this.

**Config struct change needed:**
The struct needs no new fields for Phase 1. The `DownloadCacheDir` override is the only field that differs from the "all paths from HomeDir" pattern, and it's already a separate field. The design's approach of computing it before rewriting `tsukuHome` is the right call.

## Summary of findings

| Area | Assessment |
|------|-----------|
| Implementability | Good. Pseudocode maps directly to current code structure. |
| Missing interfaces | Minor: name validation, sandbox interaction, ConfigFile path |
| Phase sequencing | Correct. Could defer env indicator to Phase 2. |
| Simpler alternatives | None that improve on the design without losing key features. |
| Config compatibility | Excellent. 5-line change to `DefaultConfig()` covers the core. |

### Recommended changes before implementation

1. Add environment name validation rules (alphanumeric, hyphens, underscores)
2. Clarify that `bin/` in the layout means `tools/current/` (or add a `BinDir` field)
3. Specify `ConfigFile` behavior for environments (use parent's or skip)
4. Document `--env` + `--sandbox` interaction (error or ignore)
5. Consider deferring env indicator from Phase 1 to Phase 2

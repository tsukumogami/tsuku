# Architecture Review: DESIGN-project-configuration

## Summary

The design document is well-structured and makes sound decisions. The architecture is implementable as specified with a few concrete issues that should be resolved before coding begins.

## Question 1: Is the architecture clear enough to implement?

**Verdict: Yes, with minor gaps to fill.**

The design specifies types, function signatures, data flows, and phase sequencing clearly enough for an implementer to start coding without ambiguity. The `ProjectConfig`, `ToolRequirement`, `LoadProjectConfig`, and `FindProjectDir` signatures are concrete Go code. The data flows for all three scenarios (init, install, downstream) are step-by-step.

### Gaps

1. **`UnmarshalTOML` signature mismatch.** The design says `func (t *ToolRequirement) UnmarshalTOML(data interface{}) error`. The `BurntSushi/toml` library's `Unmarshaler` interface actually requires `UnmarshalTOML(decode func(interface{}) error) error` (a decode callback, not raw data). The implementer needs to use the correct signature. This is a documentation error, not a design flaw -- the approach is sound.

2. **`ProjectConfig.Path` field is missing.** Flow 2 (step 5) prints the config file path, and Flow 2 (step 3) prints "No tools declared in <path>". The `ProjectConfig` struct has no field to carry this path. Either add a `Path string` field to `ProjectConfig` or have `LoadProjectConfig` return `(path string, *ProjectConfig, error)`. The latter is cleaner since the path is metadata about where the config was found, not part of the config content itself.

3. **Tool iteration order is unspecified.** Go maps don't guarantee iteration order. When printing "Tools: node@20.16.0, go@1.22, ..." the output will be non-deterministic. For reproducible output and testing, the install loop should sort tool names alphabetically. This is a minor implementation detail but worth calling out.

## Question 2: Are there missing components or interfaces?

### Exit code conflict (blocking)

The design proposes `ExitPartialFailure = 5`. Exit code 5 is already taken by `ExitNetwork` in `cmd/tsuku/exitcodes.go`. The design also says "ExitInstallFailed (4) if all failed" but code 4 is `ExitVersionNotFound`, and `ExitInstallFailed` is actually 6.

The document's exit code table is:
- 0 = all succeeded
- 4 = all failed (but 4 = ExitVersionNotFound in code)
- 5 = partial failure (but 5 = ExitNetwork in code)

**Resolution:** Use the next available exit code. Currently the highest non-special code is 14 (`ExitForbidden`). Assign `ExitPartialFailure = 15`. The design's references to exit code values (4 for all-failed, 5 for partial) need updating to match the actual codebase constants (`ExitInstallFailed = 6` for all-failed, `ExitPartialFailure = 15` for partial).

### Missing: `ProjectConfig.Path` or equivalent return value

As noted above, the CLI needs the file path for user-facing messages. The interface should expose it.

### Missing: version resolution for prefix/latest before install loop

The design's Flow 2 (step 5) prints `ripgrep@latest` and `jq` with their constraint forms. But step 7a says "Resolve version (exact, prefix, or latest)". The design doesn't specify whether version resolution happens eagerly (all at once before installing) or lazily (per-tool in the loop). Lazy resolution is simpler and consistent with how `runInstallWithTelemetry` already works -- version resolution happens inside the install pipeline. The design should clarify that the install loop passes the raw constraint string and lets the existing resolution handle it.

### Interface completeness for downstream consumers

The `ProjectConfig` interface is sufficient for both #1681 and #2168. `LoadProjectConfig(startDir)` gives them all they need: a map of tool names to version constraints. `FindProjectDir(startDir)` gives them the project root for environment scoping. No additional abstraction layers are needed.

## Question 3: Are the implementation phases correctly sequenced?

**Verdict: Yes, the sequencing is correct.**

- Phase 1 (core package) has zero dependencies on CLI code. Pure library + tests.
- Phase 2 (init) depends on Phase 1 for `ConfigFileName` constant and template content.
- Phase 3 (install no-args) depends on Phase 1 for `LoadProjectConfig` and Phase 2's existence (the error message says "Run 'tsuku init'").
- Phase 4 (docs) depends on all prior phases being stable.

One minor refinement: the `ExitPartialFailure` constant should be added in Phase 1 (to `exitcodes.go`) rather than Phase 3, since it's a shared constant. This avoids a Phase 3 PR that touches both the exit codes file and install logic.

## Question 4: Are there simpler alternatives we overlooked?

### Considered: skip `FindProjectDir`, derive from `LoadProjectConfig`

The design exports both `LoadProjectConfig` and `FindProjectDir`. Since `LoadProjectConfig` already walks the directory tree, `FindProjectDir` duplicates that traversal. A simpler approach: have `LoadProjectConfig` return the directory path alongside the config (or embed it in the struct). `FindProjectDir` can then be a thin wrapper that calls `LoadProjectConfig` and discards the parsed config. This avoids maintaining two independent traversal implementations.

### Considered: reuse `handleInstallError` with error collection

The design says the install loop needs error aggregation instead of fail-fast. The current `handleInstallError` calls `exitWithCode` and terminates the process. Rather than refactoring `handleInstallError`, the no-args branch can simply call `runInstallWithTelemetry` and check the returned error, collecting errors into a slice. `handleInstallError` remains untouched for the single-tool path. This is what the design implies but it's worth being explicit: no changes to `handleInstallError` are needed.

### Considered: use `cobra.MinimumNArgs(0)` instead of the current `ArbitraryArgs`

The install command already uses `cobra.ArbitraryArgs` (line 66 of install.go), which accepts zero args. The no-args detection in the Run function is just `len(args) == 0`. This works today without changing the Args validator. No simplification needed.

### Not recommended: config in user's `$TSUKU_HOME/config.toml`

Putting project tool declarations in the user config would be simpler (no new package) but defeats the purpose -- project configs must be per-project and version-controlled. The design correctly keeps these separate.

## Additional Observations

### `handleInstallError` coupling

The design correctly identifies that `handleInstallError` terminates the process. The no-args install path must NOT call `handleInstallError` for individual tool failures. It should collect errors and only call `exitWithCode` once at the end. The design's Flow 2 steps 7c-7d describe this correctly.

### TOML parsing library compatibility

The codebase uses `github.com/BurntSushi/toml`. The custom `UnmarshalTOML` approach is well-supported by this library. The `ToolRequirement` type needs to implement the `toml.Unmarshaler` interface. The decode-callback pattern works naturally with `ProjectConfig.Tools` being a `map[string]ToolRequirement`.

### Testing considerations

The design lists appropriate test cases. One addition: test behavior when `tsuku.toml` exists but is not valid TOML (e.g., binary file, permission denied). The design mentions "invalid TOML" but should also cover the permission-denied case, which is distinct from a parse error.

### `TSUKU_CEILING_PATHS` parsing

The design says colon-separated. On Windows, colons appear in paths (`C:\...`). Since tsuku targets Unix systems primarily, this is fine, but worth a comment in the code noting the Unix assumption.

## Recommendations

1. **Fix the exit code conflict.** Use `ExitPartialFailure = 15` (next available) and update the design's references to match actual codebase constants.
2. **Add a path field or return value** to `LoadProjectConfig` so the CLI can print which file governs the install.
3. **Fix the `UnmarshalTOML` signature** in the design to match the `BurntSushi/toml` `Unmarshaler` interface.
4. **Implement `FindProjectDir` as a wrapper** around the same traversal logic used by `LoadProjectConfig`, not as a separate implementation.
5. **Sort tool names** before iteration in the install loop for deterministic output.
6. **Add permission-denied test case** to Phase 1 test plan.

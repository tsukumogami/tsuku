# Security and Architecture Review: Project-Level Auto-Update

## Security Assessment

### 1. Privilege escalation / broadening updates

**No risk.** The `effectivePin()` function returns the project version only when the tool is declared in `.tsuku.toml`. The cached `LatestWithinPin` was resolved against the *global* pin during the background check, not the project pin. A broader project pin (e.g., project says `"20"`, global says `"20.16"`) can't produce a candidate outside `"20.16"` because the candidate was already computed under that narrower constraint. The filtering is purely subtractive -- it can skip entries but never manufacture new ones.

### 2. Symlink and path traversal

**Handled.** `LoadProjectConfig` calls `filepath.EvalSymlinks` on `startDir` before traversal begins. This collapses symlink chains to real paths before the ceiling check runs against resolved directories. One minor observation: if `$HOME` itself is a symlink, `os.UserHomeDir()` returns the logical path while the traversal uses the resolved path, potentially causing a ceiling miss. In practice this is unlikely (most systems resolve `$HOME`) and the filesystem-root termination provides a hard stop regardless. No action needed, but worth a comment in the code.

### 3. Data exposure

**No risk.** The design reads `.tsuku.toml` (tool names and version strings) and uses them for string-prefix comparisons. No config values reach network calls, log output, error messages to other users, or telemetry payloads. Tool names from project config are only compared against existing cache entry keys.

### 4. Ceiling detection

**Sufficient.** `$HOME` is unconditional. `TSUKU_CEILING_PATHS` can only add ceilings, not remove the `$HOME` boundary. The `MaxTools = 256` limit prevents resource exhaustion from oversized configs. The traversal terminates at filesystem root if no ceiling matches (parent == dir check).

### 5. Denial of updates

**Accepted risk per R17.** A malicious `.tsuku.toml` with exact pins on every tool suppresses all auto-updates in that directory. This is the correct behavior -- project config takes precedence. The blast radius is limited to sessions where CWD is within the project tree.

### 6. Version string validation

**Gap identified.** `effectivePin()` passes the project `Version` string directly to `VersionMatchesPin`, which does string-prefix matching. The `ValidateRequested` function exists in `pin.go` for defense-in-depth on state.json values but isn't called on `.tsuku.toml` versions. A crafted version string won't cause security issues (the match functions are safe with arbitrary strings), but calling `ValidateRequested` on project versions would be consistent. Low priority.

## Architecture Assessment

### 1. Design clarity

The design is implementable as written. The `effectivePin()` helper, the signature change, and the three-line caller change are well-specified. The pseudocode in "Components" matches the actual code structure.

### 2. Missing components

**None blocking.** Two optional additions worth noting:
- Debug logging when a project pin suppresses an update (the design acknowledges this as future work).
- Channel pin handling: `VersionMatchesPin` returns `false` for `@channel` pins, which means a project config with `tool = "@stable"` would suppress all auto-updates for that tool. This is conservative and safe but may surprise users. The design's scope explicitly excludes channel pins, so this is fine for now.

### 3. Phase sequencing

Single-phase implementation is appropriate. The change touches 3 files with no new packages, no schema migrations, and no new external dependencies. No sequencing risk.

### 4. Simpler alternatives

None overlooked. The design already rejected the simpler "exact-only suppression" and "blocklist" approaches with clear rationale tied to the R17 requirement. The chosen approach reuses existing pin functions without introducing new abstractions.

## Summary

The design is sound. The security surface is minimal -- project config can only narrow or suppress updates, never broaden them. Existing safeguards (symlink resolution, ceiling paths, MaxTools) cover the traversal and parsing risks. The one low-priority gap is missing `ValidateRequested` on project version strings for defense-in-depth consistency. Architecture is clean: three files, one new helper, one parameter addition, full reuse of existing pin semantics.

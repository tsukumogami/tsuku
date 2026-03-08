# Lead: How does tsuku currently discover and invoke dltest and llm at runtime?

## Findings

### dltest Discovery
`internal/verify/dltest.go`: State-based lookup via `stateManager.GetToolState("tsuku-dltest")`, constructs path `$TSUKU_HOME/tools/tsuku-dltest-<version>/bin/tsuku-dltest`, falls back to recipe installation if not found.

### llm Discovery
`internal/llm/addon/manager.go`: Directory scan of `$TSUKU_HOME/tools/` for `tsuku-llm-*` dirs, accepts any installed version, no version preference logic.

### dltest Version Pinning
`internal/verify/version.go`: Compile-time ldflags injection (`pinnedDltestVersion`), two modes:
- Dev mode: any version accepted
- Release mode: exact match required, detects mismatch and auto-reinstalls

### llm Version Handling
No pinning mechanism, no compatibility checking, silently accepts whatever version is installed, auto-upgrades on demand.

### Binary Invocation Patterns
- dltest: subprocess via `exec.CommandContext` with timeout
- llm: gRPC daemon over Unix socket

### Version Mismatch Behavior
- dltest: explicitly enforces version match in release mode and reinstalls if mismatched
- llm: no mismatch detection; can fail at runtime if incompatible

### No Cross-Tool Version Negotiation
CLI, dltest, and llm versions come from different sources, are pinned independently, and have no runtime checks comparing them.

## Implications

The dltest pattern (compile-time version pinning via ldflags) is the model to follow for llm. It already enforces exact version match in release mode and handles auto-reinstall. Extending this pattern to llm would give both companion binaries consistent version enforcement.

The llm addon manager's "accept any version" approach is the gap that creates backward compatibility risk. Without version checking, a stale llm binary from a previous tsuku version could silently operate with incompatible protocol changes.

## Surprises

dltest already has sophisticated version pinning with compile-time injection and auto-reinstall. The infrastructure exists -- it just needs to be extended to llm. This is simpler than building a new mechanism from scratch.

## Open Questions

- Should llm adopt the exact same ldflags pattern as dltest, or use a different approach given its gRPC daemon architecture?
- Does the gRPC protocol between tsuku and llm include any version negotiation that could serve as a fallback?
- When dltest auto-reinstalls on version mismatch, does it block the user operation or happen in the background?

## Summary
tsuku already has compile-time version pinning for dltest (`pinnedDltestVersion` via ldflags) that enforces exact match in release mode and auto-reinstalls on mismatch. llm has no equivalent -- it accepts any installed version with no compatibility checking. Extending the dltest pinning pattern to llm is the natural path forward.

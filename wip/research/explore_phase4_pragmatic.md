# Pragmatic Review: DESIGN-gpu-backend-selection.md

## Findings

### 1. Vulkan VRAM detection is a separate concern -- Blocking (scope creep)

`DESIGN-gpu-backend-selection.md:49,58,261,606-645` -- The Vulkan VRAM fix (parsing `vulkaninfo --summary` output) solves a different problem: model sizing. It has no dependency on the manifest schema change, detection logic, or fallback chain. It's a standalone Rust-side bug fix. Shipping it here inflates the PR and muddies the acceptance criteria.

**Fix:** Extract to its own issue. The design even acknowledges it: "This isn't a distribution problem per se."

### 2. Runtime fallback chain (Decision 4) should be deferred -- Blocking (speculative generality)

`DESIGN-gpu-backend-selection.md:214-259,499-559` -- The whole "download CPU variant on exit code 78" flow adds a second download path, a new error type (`BackendFailedError`), Rust-side exit code changes, and retry logic in `LocalProvider.Complete()`. All to handle the case where Go-side detection picks wrong.

But Go-side detection is filesystem `stat()` calls. If `libvulkan.so.1` exists, Vulkan is almost certainly loadable. The doc's own rationale says the detection "catches the common case." The uncommon case (library exists but wrong Vulkan version, CUDA driver mismatch) is real but rare enough to handle as a follow-up once you have telemetry on detection accuracy.

**Fix:** Ship Phase 1 (manifest) + Phase 2 (detection) first. If detection picks wrong, the binary fails and the user gets an error suggesting `tsuku config set llm.backend cpu`. That's the "Report and suggest manual override" option the doc rejected as "primary mechanism" -- but it's fine as the V1 mechanism while you validate detection accuracy. Add the automatic fallback chain in a follow-up issue.

### 3. Nested manifest schema is justified -- No finding

`DESIGN-gpu-backend-selection.md:99-161` -- The doc correctly identifies that flat composite keys (`linux-amd64-cuda`) prevent enumerating backends per platform without key scanning. The detection logic needs exactly that enumeration. The nesting adds one level of indirection but the Go types are clean. The `default` field per platform entry is necessary to avoid hardcoding fallback logic. This is the right call.

### 4. `DetectedBackend.Priority` field is dead weight -- Advisory

`DESIGN-gpu-backend-selection.md:393-396` -- The `Priority` field on `DetectedBackend` is set but never read. The `probeBackends()` function already returns backends in priority order (Vulkan first, CUDA second, CPU last). The caller takes `backends[0].Name`. The field serves no current purpose.

**Fix:** Return `[]string` from `probeBackends()`. If you later need priority as data rather than ordering, add it then.

### 5. Solution Architecture specifies too much implementation detail -- Advisory

`DESIGN-gpu-backend-selection.md:271-663` -- The Solution Architecture section is 400 lines of Go and Rust code including function signatures, struct definitions, full method bodies, and directory layouts. This is implementation, not architecture. It will diverge from reality on the first PR and become stale documentation.

The decisions (1-4) are well-articulated and sufficient. The component changes list (lines 275-291) and the schema examples are useful. The full code blocks for `detect_linux.go`, `lifecycle.go`, `local.go`, and `hardware.rs` are not -- they'll be in the PR.

**Fix:** Keep the schema example, component change list, and directory layout. Cut the Go/Rust function bodies. The implementation phases section already covers ordering.

### 6. Legacy compat functions should be cleaned up in this work -- Advisory (opportunity)

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager.go:230-262` -- `AddonPath()` and the package-level `IsInstalled()` are deprecated, but `lifecycle.go:71` still calls `AddonPath()`. Since this design rewrites the `EnsureAddon` flow and binary path resolution (adding backend segment), the deprecated functions will need updating or they'll point to wrong paths. Clean them up as part of Phase 1.

## Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | Vulkan VRAM fix is scope creep | Blocking |
| 2 | Runtime fallback chain is premature -- defer to follow-up | Blocking |
| 3 | Nested manifest schema is correct | No finding |
| 4 | `DetectedBackend.Priority` field is unused | Advisory |
| 5 | Solution Architecture section is implementation disguised as design | Advisory |
| 6 | Deprecated `AddonPath()`/`IsInstalled()` need cleanup or removal | Advisory |

The 80% solution is: manifest schema change + Go-side library probing + config override. That's Phases 1-2. It gets every user the right binary automatically. The fallback chain (Phase 3) and VRAM fix (Phase 4) are separate concerns that don't need to ship together.

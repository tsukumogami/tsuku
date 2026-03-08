# Research: llm Version Pinning via gRPC (Unified Release Versioning Round 2)

**Date**: 2026-03-08
**Exploration Phase**: 2 (Lead: llm gRPC daemon architecture)
**Status**: Complete
**Lead Question**: How should the dltest compile-time version pinning pattern be extended to llm, given llm's gRPC daemon architecture?

---

## Executive Summary

The dltest version pinning pattern can be extended to llm with two complementary strategies:
1. **Compile-time ldflags pinning** (same pattern as dltest): Inject `pinnedLlmVersion` into tsuku binary at build time; enforce exact version match in release mode via AddonManager
2. **gRPC handshake negotiation** (new pattern for daemon): Add optional `addon_version` field to StatusResponse proto; client validates on first GetStatus call to detect version mismatches early and provide diagnostic feedback

Both are necessary: ldflags ensure the binary enforces the constraint, while gRPC validation gives visibility into daemon-client version compatibility at runtime.

---

## Findings

### 1. dltest Pinning Pattern (Current)

**File**: `internal/verify/version.go` and `internal/verify/dltest.go`

**Build-time injection**:
- `.goreleaser.yaml` (line 24) injects version via ldflags:
  ```
  -X github.com/tsukumogami/tsuku/internal/verify.pinnedDltestVersion={{.Version}}
  ```
- `pinnedDltestVersion` variable defaults to `"dev"` (line 5, `version.go`)

**Runtime enforcement** (`dltest.go`, `EnsureDltest` function):
- **Dev mode** (`pinnedDltestVersion == "dev"`): Accept any installed version or install latest (lines 101-132)
- **Release mode** (specific version): Require exact match; auto-reinstall if mismatched (lines 135-153)
  - Line 135: `if installedVersion == pinnedDltestVersion`
  - Line 144: Auto-installs via subprocess if mismatch detected
- Returns path to verified binary for use in `InvokeDltest` (exec.CommandContext with 5s timeout per batch)

**Subprocess pattern**: dltest is a stateless command-line tool, invoked with `exec.CommandContext` and terminates after each batch of library tests (lines 364-407 in `dltest.go`). No long-running process or handshake needed.

---

### 2. llm Daemon Architecture (Current)

**File**: `internal/llm/addon/manager.go` and `internal/llm/lifecycle.go`

**Binary location** (`addon/manager.go`, `findInstalledBinary`):
- Scans `$TSUKU_HOME/tools/` for `tsuku-llm-*` directories (line 168)
- Accepts **any installed version** (lines 163-185)
- No version pinning or compatibility checking

**Lifecycle** (`lifecycle.go`):
- **Lock file protocol**: Uses `$TSUKU_HOME/llm.sock.lock` to reliably detect running daemon (lines 95-113, lock_ex | lock_nb pattern)
- **Server startup** (`EnsureRunning`, lines 119-195):
  - Acquires lock; if lock fails, daemon already running (returns after `waitForReady`)
  - If lock succeeds, starts `tsuku-llm serve --idle-timeout <duration>` via exec.CommandContext
  - Passes lock ownership to daemon (unlock probe lock, let daemon acquire it)
  - Monitors process in background; nulls out `s.process` on exit
- **Graceful shutdown** (`shutdownViaGRPC`, lines 267-280):
  - Connects over Unix socket and calls `Shutdown(graceful=true)` RPC
  - Falls back to SIGTERM if gRPC fails

**gRPC communication** (`internal/llm/local.go`):
- Client creates Unix socket connection: `grpc.NewClient("unix://"+socketPath, ...)`
- Three RPCs: `Complete`, `Shutdown`, `GetStatus`
- LocalProvider maintains cached `conn` and `client` fields; invalidates on gRPC error (lines 143-152)

---

### 3. Proto Definition (Current)

**File**: `proto/llm.proto`

**Current StatusResponse** (lines 142-158):
```protobuf
message StatusResponse {
  bool ready = 1;
  string model_name = 2;
  int64 model_size_bytes = 3;
  string backend = 4;
  int64 available_vram_bytes = 5;
}
```

**Key observation**: No version field. Daemon has **zero visibility** into what version of tsuku-llm binary is running.

---

### 4. Build System (Current)

**File**: `.goreleaser.yaml` and `Makefile`

**Single monorepo build**:
- Builds only `tsuku` CLI (lines 9-26 in `.goreleaser.yaml`)
- `pinnedDltestVersion` is injected during this build (line 24)
- `tsuku-llm` binary is built separately (external repo or earlier step, not shown in goreleaser)

**No artifact consolidation**:
- Release creates `tsuku-linux-amd64`, `tsuku-darwin-arm64`, etc.
- No corresponding `tsuku-llm-v<version>` artifacts tagged with same version
- Recipe system installs `tsuku-llm` from its own release tags

---

## Design Options

### Option A: Compile-time Pinning Only (Minimal)

**Approach**: Reuse dltest pattern exactly.

**Implementation**:
1. Add `pinnedLlmVersion` to `internal/verify/version.go` (same file as `pinnedDltestVersion`)
2. Inject via `.goreleaser.yaml`:
   ```
   -X github.com/tsukumogami/tsuku/internal/verify.pinnedLlmVersion={{.Version}}
   ```
3. In `addon/manager.go`, add version check in `findInstalledBinary`:
   ```go
   if pinnedLlmVersion != "dev" && !strings.HasPrefix(binaryPath, "tsuku-llm-" + pinnedLlmVersion) {
       // Version mismatch; trigger auto-reinstall
       return "", ErrVersionMismatch
   }
   ```
4. In `EnsureAddon`, catch version mismatch and auto-reinstall via recipe system

**Pros**:
- Exact parity with dltest pattern
- Single source of truth: ldflags-injected version
- Works within existing recipe + state.json infrastructure

**Cons**:
- Zero visibility at daemon startup: client doesn't know daemon version until after all gRPC calls fail with protocol incompatibility
- Daemon can't report its own version back to client
- Harder to diagnose why a specific gRPC call failed (was it a version mismatch or a real error?)

---

### Option B: gRPC Handshake + Compile-time Pinning (Recommended)

**Approach**: Add optional `addon_version` field to StatusResponse proto; validate on first GetStatus call in Complete flow.

**Proto change** (`proto/llm.proto`):
```protobuf
message StatusResponse {
  bool ready = 1;
  string model_name = 2;
  int64 model_size_bytes = 3;
  string backend = 4;
  int64 available_vram_bytes = 5;
  string addon_version = 6;  // NEW: e.g., "v1.2.0" or "dev-abc123"
}
```

**Daemon-side change** (tsuku-llm):
- Embed version string at build time (same ldflags pattern)
- Return it in GetStatus response

**Client-side change** (`internal/llm/local.go`):
1. Add `EnsureVersionCompatible()` method that calls GetStatus once:
   ```go
   func (p *LocalProvider) EnsureVersionCompatible(ctx context.Context) error {
       status, err := p.client.GetStatus(ctx, &pb.StatusRequest{})
       if err != nil {
           return fmt.Errorf("version check failed: %w", err)
       }
       
       if pinnedLlmVersion != "dev" && status.AddonVersion != pinnedLlmVersion {
           return fmt.Errorf("addon version mismatch: tsuku expects %s but daemon is %s",
               pinnedLlmVersion, status.AddonVersion)
       }
       return nil
   }
   ```

2. Call in `Complete()` before first actual inference:
   ```go
   if err := p.ensureModelReady(ctx); err != nil {
       return nil, err
   }
   if err := p.EnsureVersionCompatible(ctx); err != nil {
       return nil, err  // Version mismatch is fatal
   }
   ```

3. Also call in `EnsureAddon()` flow as fallback version check

**Pros**:
- Daemon can report its own version (self-identification)
- Early detection of version mismatches (fails fast before inference)
- Client can provide diagnostic error: "expected vX but got vY"
- Daemon unaware of pinning policy; just reports its version
- Backward compatible: StatusResponse field is optional (proto3)
- Works for both coordinated release (pinned) and dev workflows (any version)

**Cons**:
- Proto evolution required (adds field, but safe in proto3)
- Daemon and tsuku CLI must coordinate on which ldflags to inject
- Requires coordination between tsuku CLI release and tsuku-llm release

---

## Recommended Path Forward

**Phase 1: Compile-time Pinning** (immediate, unblocks unified versioning)
- Add `pinnedLlmVersion` to tsuku CLI
- Inject in `.goreleaser.yaml`
- Add version check in `addon/manager.go` to auto-reinstall on mismatch
- Similar enforcement to dltest: release mode requires exact match, dev mode accepts any

**Phase 2: gRPC Handshake** (follow-up, improves diagnostics)
- Extend proto StatusResponse with `addon_version` field
- Update tsuku-llm to inject its version at build time
- Add `EnsureVersionCompatible()` check in LocalProvider
- Provides visibility into version mismatch root cause

**Why both?**
- Pinning alone ensures only correct version is installed and running, but gives no visibility
- gRPC handshake gives visibility and enables better error messages
- Combined: "we know which version we expect, we can detect if the wrong version is running, and we can auto-fix it"

---

## Key Differences from dltest

| Aspect | dltest | llm |
|--------|--------|-----|
| **Invocation** | Subprocess (exec.CommandContext, 5s timeout) | gRPC daemon (long-running, Unix socket) |
| **Version check location** | `internal/verify/dltest.go::EnsureDltest` | `internal/llm/addon/manager.go::EnsureAddon` + optional `internal/llm/local.go::EnsureVersionCompatible` |
| **Version mismatch handling** | Auto-reinstall before use | Auto-reinstall before use + gRPC validation |
| **Proto field** | N/A (not gRPC) | StatusResponse.addon_version (new field needed) |
| **Handshake** | None (stateless) | GetStatus as version handshake |
| **Visibility** | Version error from install command | Both install error + runtime gRPC error with version mismatch context |

---

## Implementation Checklist

**Compile-time pinning**:
- [ ] Add `pinnedLlmVersion` var to `internal/verify/version.go` (or new `internal/verify/llm.go`)
- [ ] Update `.goreleaser.yaml` to inject `pinnedLlmVersion={{.Version}}`
- [ ] Modify `addon/manager.go::findInstalledBinary()` to check version
- [ ] Modify `addon/manager.go::EnsureAddon()` to auto-reinstall on mismatch
- [ ] Update `internal/buildinfo/version.go` to expose pinned version (or query from verify package)

**gRPC handshake** (follow-up PR):
- [ ] Add `addon_version` field to `proto/llm.proto::StatusResponse`
- [ ] Regenerate `internal/llm/proto/llm.pb.go` and `llm_grpc.pb.go`
- [ ] Add `EnsureVersionCompatible()` to `internal/llm/local.go`
- [ ] Call in `Complete()` or `EnsureAddon()` flow
- [ ] Update tsuku-llm to inject version in StatusResponse

---

## Conclusion

The dltest compile-time pinning pattern generalizes cleanly to llm with one key extension: gRPC adds the possibility of a runtime handshake for version negotiation. This enables early detection of incompatibilities and better error messages. Both mechanisms (ldflags pinning + gRPC validation) should be deployed together for maximum robustness in the unified release workflow.

The path forward is:
1. Apply compile-time pinning immediately (Phase 1)
2. Add gRPC handshake in follow-up (Phase 2)
3. Coordinate tsuku and tsuku-llm releases to the same tag
4. Document in release checklist that both binaries must be built and versioned together

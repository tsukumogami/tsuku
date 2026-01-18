# Release Engineering Review: dlopen Load Testing Design

**Reviewer Role:** Release Engineer (CI/CD, binary distribution, multi-platform releases)
**Design Document:** `docs/designs/DESIGN-library-verify-dlopen.md`
**Date:** 2026-01-18

---

## Executive Summary

The design proposes a helper binary (`tsuku-dltest`) built with CGO for dlopen verification. From a release engineering perspective, the approach is **feasible but introduces significant build complexity**. The main concerns are: (1) CGO cross-compilation is notoriously brittle with goreleaser, (2) the version pin coordination between helper and main binary creates release atomicity challenges, and (3) the CI matrix will need substantial expansion.

**Verdict:** Workable with modifications, but expect 2-3 weeks of CI debugging to get reliable cross-platform CGO builds.

---

## 1. Goreleaser with CGO: Practical Assessment

### Current State

The existing `.goreleaser.yaml` is clean and simple:
- Single build target (`tsuku`) with `CGO_ENABLED=0`
- Runs on `ubuntu-latest` only
- Cross-compilation handled by Go's built-in support

### The CGO Problem

Adding `CGO_ENABLED=1` builds breaks the simplicity in several ways:

**Cross-compilation requires C toolchains:**
```yaml
# This WILL NOT work out of the box
builds:
  - id: tsuku-dltest
    env:
      - CGO_ENABLED=1
    goos: [linux, darwin]
    goarch: [amd64, arm64]
```

Goreleaser on `ubuntu-latest` cannot build:
- `darwin-amd64`: No macOS SDK
- `darwin-arm64`: No macOS SDK
- `linux-arm64`: No ARM cross-compiler (without extra setup)

**Solutions (in order of complexity):**

| Approach | Complexity | Build Time | Reliability |
|----------|------------|------------|-------------|
| Matrix runners (4 native builds) | Low | 4x parallel | High |
| zig cross-compiler | Medium | 1x | Medium |
| Docker multiarch + osxcross | High | 2-3x | Medium |
| Separate release jobs per platform | Low | 4x parallel | High |

**Recommendation:** Use matrix runners with native builds. This avoids cross-compilation entirely.

### Proposed CI Structure

```yaml
jobs:
  # Build CGO binaries natively on each platform
  build-dltest:
    strategy:
      matrix:
        include:
          - os: ubuntu-latest
            goos: linux
            goarch: amd64
          - os: ubuntu-24.04-arm64  # GitHub's ARM runners
            goos: linux
            goarch: arm64
          - os: macos-latest
            goos: darwin
            goarch: arm64
          - os: macos-13  # Intel macOS
            goos: darwin
            goarch: amd64
    runs-on: ${{ matrix.os }}
    steps:
      - name: Build tsuku-dltest
        run: |
          CGO_ENABLED=1 go build -o tsuku-dltest-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/tsuku-dltest
      - name: Upload artifact
        uses: actions/upload-artifact@v4
        # ...

  release:
    needs: build-dltest
    runs-on: ubuntu-latest
    steps:
      - name: Download all artifacts
        uses: actions/download-artifact@v4
      - name: Run goreleaser (main binary only)
        # goreleaser builds tsuku, we manually add dltest binaries to release
```

### Gotchas

1. **Linux arm64 runners**: GitHub's `ubuntu-24.04-arm64` runners are relatively new. Verify availability and queue times.

2. **macOS code signing**: Unsigned binaries trigger Gatekeeper warnings. Users must `xattr -d com.apple.quarantine tsuku-dltest` or right-click > Open. Consider Apple Developer Program enrollment for notarization.

3. **glibc version lock-in**: Linux CGO binaries are tied to the build system's glibc. Building on ubuntu-latest (glibc 2.35+) may fail on older distros. Consider building on ubuntu-20.04 for broader compatibility, or use musl static linking.

4. **dlopen flag differences**: `-ldl` is needed on Linux but not macOS. Build constraints must handle this:
   ```go
   // +build linux
   // #cgo LDFLAGS: -ldl
   ```

5. **Checksum coordination**: Goreleaser generates checksums.txt. If dltest binaries are built separately, checksums must be appended before release upload.

---

## 2. Version Pin Management

### The Problem

The design specifies:
```go
var pinnedDltestVersion = "v1.0.0"
```

Updated via:
```yaml
- name: Update dltest version pin
  run: |
    sed -i "s/pinnedDltestVersion = .*/pinnedDltestVersion = \"${{ github.ref_name }}\"/" \
      internal/verify/dltest.go
```

**Issue:** This modifies source code during release. The built binary will have the correct pin, but:
- The commit used to build doesn't match the pinned version (chicken-and-egg)
- Reproducibility is compromised (building from tag won't include the pin update)
- `git diff` shows uncommitted changes during release

### Recommended Approach

**Option A: Build-time injection (preferred)**
```go
// internal/verify/dltest.go
var pinnedDltestVersion = "dev" // Placeholder

func init() {
    // Set at build time via ldflags
}
```

```yaml
ldflags:
  - -X github.com/tsukumogami/tsuku/internal/verify.pinnedDltestVersion={{.Version}}
```

**Option B: Version file in release assets**
Instead of source code pin, download a `dltest-version.txt` alongside the helper and verify versions match at runtime.

**Option C: Embed version in helper binary**
The helper already supports `--version`. Tsuku can call this and verify it matches expectations.

### Between-Release Versioning

For development:
- `pinnedDltestVersion = "dev"` bypasses version checks
- CI tests use `TSUKU_DLTEST_PATH` override
- Pre-release tags (`v1.0.0-rc.1`) publish both binaries for integration testing

---

## 3. Recipe-Based Installation Assessment

### Current Pattern (nix-portable)

```go
var nixPortableChecksums = map[string]string{
    "amd64": "b409c55904c909ac3aeda3fb1253319f86a89ddd1ba31a5dec33d4a06414c72a",
    "arm64": "af41d8defdb9fa17ee361220ee05a0c758d3e6231384a3f969a314f9133744ea",
}
```

The nix-portable approach embeds checksums directly in Go source. This is simple but requires code changes for updates.

### Design's Recipe Approach

The design proposes using tsuku's recipe system:
```toml
[[steps]]
action = "download"
checksum = "{checksums.tsuku-dltest-{os}-{arch}}"
```

**Concerns:**

1. **Circular dependency**: Tsuku uses the recipe system to install tsuku-dltest. If the recipe system has bugs, the helper can't be installed.

2. **Registry staleness**: Recipe checksums come from the registry. If a user has an old registry cache, checksums may not match new releases.

3. **First-run UX**: A fresh tsuku install running `verify --level=3` would need to:
   - Update registry (network)
   - Find tsuku-dltest recipe
   - Download helper (network)
   - Verify checksum
   - Then run verification

   This is multiple network round-trips for what should be a simple verification.

### Recommendation: Hybrid Approach

Keep embedded checksums (like nix-portable) but use a script to update them:

```go
// Generated by scripts/update-dltest-checksums.sh
// Do not edit manually
var dltestChecksums = map[string]map[string]string{
    "v1.0.0": {
        "linux-amd64":  "abc123...",
        "linux-arm64":  "def456...",
        "darwin-amd64": "789abc...",
        "darwin-arm64": "012def...",
    },
}
```

The script runs as part of release prep:
```bash
#!/bin/bash
# scripts/update-dltest-checksums.sh
VERSION=$1
for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
    sha256sum "dist/tsuku-dltest-$platform" | awk '{print $1}'
done
# Output Go map literal
```

---

## 4. Release Failure Scenarios

### Scenario: Helper builds succeed, main tsuku build fails

**Impact:** Release assets would include dltest binaries but no main tsuku.

**Mitigation:** Use a single release job that builds both, or use GitHub's draft releases:
```yaml
release:
  draft: true  # Start as draft

finalize:
  needs: [build-tsuku, build-dltest]
  if: success()
  steps:
    - name: Publish release
      run: gh release edit $TAG --draft=false
```

### Scenario: Main tsuku succeeds, helper fails on one platform

**Impact:** Release published with 3/4 helper binaries. Users on the failing platform get "helper unavailable" warnings.

**Current design handles this:** Graceful degradation skips Level 3 if helper unavailable. This is acceptable but should be documented in release notes.

**Better:** Treat any build failure as release failure. Don't publish partial releases.

### Scenario: Version pin doesn't match published helper

**Impact:** Users download helper but checksum verification fails. Verification is broken until next release.

**Mitigation:**
- CI integration test: After building both binaries, run tsuku with the helper to verify the trust chain works
- Use ldflags for version pin (avoids source modification)

### Recommended Release Workflow

```
1. [build-dltest] Build helper on 4 platform runners (parallel)
2. [build-tsuku] Build main binary with goreleaser
3. [integration-test] Test tsuku + dltest together
4. [publish] Upload all assets to draft release
5. [finalize] Mark release as published (only if all above succeed)
```

---

## 5. Pre-Release Testing Strategy

### Local Development

```bash
# Build helper locally
CGO_ENABLED=1 go build -o tsuku-dltest ./cmd/tsuku-dltest

# Test with override
TSUKU_DLTEST_PATH=./tsuku-dltest ./tsuku verify some-lib
```

### CI Integration Tests

Add to `.github/workflows/integration-tests.yml`:

```yaml
dltest-integration:
  runs-on: ${{ matrix.os }}
  strategy:
    matrix:
      os: [ubuntu-latest, macos-latest]
  steps:
    - name: Build both binaries
      run: |
        go build -o tsuku ./cmd/tsuku
        CGO_ENABLED=1 go build -o tsuku-dltest ./cmd/tsuku-dltest

    - name: Test dltest directly
      run: |
        # Verify helper works in isolation
        echo '[]' | ./tsuku-dltest /lib/x86_64-linux-gnu/libc.so.6 || true
        ./tsuku-dltest --version

    - name: Test integrated verification
      run: |
        # Install a library, then verify it
        ./tsuku install gcc-libs
        TSUKU_DLTEST_PATH=./tsuku-dltest ./tsuku verify gcc-libs --level=3
```

### Pre-Release Tags

Use `-rc.N` suffix for release candidates:
```bash
git tag v1.0.0-rc.1
git push origin v1.0.0-rc.1
```

Goreleaser marks these as pre-releases automatically (`prerelease: auto` is already configured).

### Staged Rollout

For the first few releases with dltest:
1. Publish as pre-release first
2. Have a few users test manually
3. Promote to full release after validation

---

## 6. CI Matrix Considerations

### Current Matrix

The current release workflow is single-runner:
```yaml
runs-on: ubuntu-latest
```

### Proposed Matrix for CGO

| Platform | Runner | Native? | Notes |
|----------|--------|---------|-------|
| linux-amd64 | ubuntu-latest | Yes | Standard |
| linux-arm64 | ubuntu-24.04-arm64 | Yes | Newer runner type |
| darwin-arm64 | macos-latest | Yes | M1/M2 |
| darwin-amd64 | macos-13 | Yes | Last Intel macOS |

### Cost Implications

GitHub Actions pricing:
- Linux: 1x multiplier
- macOS: 10x multiplier
- ARM: Similar to x64 Linux

A release that builds 4 native CGO binaries + goreleaser:
- 2 Linux builds: ~2 minutes each
- 2 macOS builds: ~3 minutes each
- Total: ~10 minutes actual, billed as ~60 minutes (macOS multiplier)

This is acceptable for infrequent releases.

### Build Caching

CGO builds benefit less from Go's build cache because C compilation isn't cached the same way. Consider:
- Caching Go modules: Already configured
- Caching C compiler outputs: Not practical across runners
- Keeping helper binary minimal: The simpler the C code, the faster the build

### Reliability Concerns

1. **ARM runner availability**: GitHub's ARM runners can have higher queue times during peak hours.

2. **macOS runner variations**: Different macOS versions have different SDKs. Pin to specific versions.

3. **Parallel job limits**: GitHub has per-account limits. If other workflows run concurrently, releases may queue.

**Recommendation:** Keep CGO builds in a separate workflow file for clarity, and consider self-hosted runners if reliability becomes an issue.

---

## Summary of Recommendations

### High Priority

1. **Use matrix runners instead of cross-compilation** - Native builds on each platform avoid CGO cross-compilation complexity.

2. **Use ldflags for version pin** - Avoid modifying source during release. Inject version at build time.

3. **Make helper build failures block release** - Don't publish partial releases. Use draft releases and only finalize when all builds succeed.

4. **Add integration test for dltest** - CI should verify the helper works with tsuku before release.

### Medium Priority

5. **Consider embedded checksums** - The recipe-based approach adds complexity. Embedded checksums (like nix-portable) are simpler and more reliable.

6. **Plan for macOS code signing** - Unsigned binaries cause UX friction. Budget for Apple Developer Program if this is user-facing.

7. **Build Linux binaries on ubuntu-20.04** - Broader glibc compatibility for older distributions.

### Low Priority

8. **Document staged rollout process** - For the first few releases with dltest, use pre-releases and manual validation.

9. **Consider musl static builds for Linux** - Would eliminate glibc version issues entirely, but adds build complexity.

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| CGO cross-compilation failures | High | Medium | Use native runners per platform |
| Version pin mismatch | Medium | High | Use ldflags, add CI verification |
| Partial release (some platforms fail) | Medium | Medium | Use draft releases, require all builds |
| macOS Gatekeeper warnings | High | Low | Document workaround, plan for signing |
| ARM runner unavailability | Low | Low | Queue delays only, can wait |
| Helper breaks tsuku upgrade path | Low | High | Test upgrade scenarios in CI |

---

## Conclusion

The design is sound architecturally. The release engineering implementation needs refinement around:
- Build infrastructure (matrix runners, not cross-compilation)
- Version coordination (ldflags, not source modification)
- Release atomicity (draft releases, fail-fast)

With these changes, the helper binary approach is maintainable. Expect initial investment of 2-3 weeks to stabilize the multi-platform CGO build pipeline, then low ongoing maintenance.

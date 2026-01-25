# Alpine APK Research Synthesis

**Date:** 2026-01-24
**Status:** Research complete, decision pending

This document synthesizes findings from 5 parallel research agents investigating Alpine APK as a potential hermetic library distribution mechanism for musl systems.

---

## Research Agents and Outputs

| Agent | Focus | Output File |
|-------|-------|-------------|
| APK Format Specialist | APK file structure, placeholders, relocation | `explore_apk-format.md` |
| Alpine Infrastructure Analyst | CDN, APKINDEX, versioning, checksums | `explore_apk-infrastructure.md` |
| musl Portability Researcher | Cross-distro compatibility | `explore_apk-portability.md` |
| Gap Analyst: Download & Extract | Implementation requirements | `explore_apk-download.md` |
| Gap Analyst: Relocation & Verification | RPATH, dlopen verification | `explore_apk-relocation.md` |

---

## Key Findings Summary

| Topic | Finding | Implication |
|-------|---------|-------------|
| **APK Format** | Three gzip streams (signature, control, data). No `@@HOMEBREW_*@@` placeholders. Uses hardcoded `/usr` paths. | Simpler than Homebrew in some ways, but still needs RPATH fixup if extracting to custom location |
| **Infrastructure** | Direct CDN URLs (no auth needed). APKINDEX provides metadata. SHA1 checksums on control segment. | Simpler than GHCR (no token dance), but need APKINDEX parser |
| **musl Portability** | **APK binaries only work on Alpine, not other musl distros.** Chimera uses incompatible APKv3. Void uses xbps. No universal musl format. | APK extraction is NOT a cross-distro solution |
| **Download/Extract** | ~500 LOC for full implementation. Can reuse HTTP client, checksum verification, tar extraction. | Medium complexity, comparable to Homebrew |
| **Relocation** | APK binaries don't have placeholders but would need RPATH fixup if extracted to `$TSUKU_HOME`. System packages need zero relocation. | `apk_install` is dramatically simpler |

---

## Critical Discovery: APK is NOT Cross-Distro

The portability research revealed that **APK extraction would only work on Alpine itself**:

- **Void Linux (musl):** Uses xbps package format, not APK
- **Chimera Linux:** Uses APKv3 (incompatible with Alpine's APKv2)
- **Adelie Linux:** Maintains separate repositories
- **Other musl distros:** Each has its own package ecosystem

This means APK extraction doesn't provide "Homebrew for musl" - it would be an Alpine-only feature.

---

## Detailed Findings

### 1. APK File Format

**Structure:**
- Stream 1: Signature (`.SIGN.RSA.<key>.rsa.pub`) - RSA signature of control segment SHA1
- Stream 2: Control (`.PKGINFO`, scripts) - Package metadata
- Stream 3: Data - Actual files laid out for root extraction

**Key differences from Homebrew bottles:**
- No placeholder strings (`@@HOMEBREW_PREFIX@@`, etc.)
- Hardcoded `/usr` paths instead
- Per-file SHA1 checksums in PAX headers
- Signature uses SHA1 (security concern raised by community)

**Implication:** No text relocation needed, but `.pc` files and config scripts still have hardcoded `/usr` paths that would need sed-style replacement if extracting to custom location.

### 2. Alpine Infrastructure

**CDN URL pattern:**
```
https://dl-cdn.alpinelinux.org/alpine/v{version}/{repo}/{arch}/{package}-{pkgver}.apk
```

**APKINDEX format:**
```
C:Q1...=     # SHA1 of control segment (base64, Q1 prefix)
P:zlib       # Package name
V:1.3.1-r0   # Version with revision
A:x86_64     # Architecture
S:53024      # Compressed size
I:110592     # Installed size
D:so:libc... # Dependencies
```

**Version discovery:** No REST API. Must download and parse APKINDEX.tar.gz.

**Checksum approach:** The `C:` field uses SHA1 of control segment only. For tsuku, recommend computing SHA256 of whole file (consistent with Homebrew approach).

### 3. musl Portability

**Compatibility matrix:**

| Distro | Uses musl | APK Compatible | Notes |
|--------|-----------|----------------|-------|
| Alpine Linux | Yes | Yes | Native |
| Void Linux (musl) | Yes | No | Uses xbps |
| Chimera Linux | Yes | No | Uses APKv3 (incompatible) |
| Adelie Linux | Yes | Partial | Separate repos |
| postmarketOS | Yes | Partial | Alpine-based but separate repos |

**ABI considerations:**
- musl has stable ABI with backward compatibility
- Dynamic linker must be at `/lib/ld-musl-$ARCH.so.1`
- No "universal musl" binary format exists
- Static linking is the only true cross-distro approach

**What other tools do:**
- mise: Runtime detection, separate binaries per libc, source compilation fallback
- asdf: Similar approach
- Neither provides "universal musl binaries"

### 4. Download & Extraction Implementation

**Code reuse from homebrew.go:**
- HTTP client (`newDownloadHTTPClient()`)
- Checksum verification (`VerifyChecksum()`)
- Tar extraction core (`extractTarReader()`)
- Path traversal protection
- Platform detection

**New code needed:**
- APKINDEX parser (~100 LOC)
- APK-specific extraction (~50 LOC) - skip `.` prefixed files
- `AlpineAPKAction` with `Decompose()` (~200 LOC)
- Tests (~150 LOC)

**Total estimate:** ~500 LOC (comparable to Homebrew's ~510 LOC)

**Complexity:** Medium - simpler auth (none needed), but multi-segment gzip format

### 5. Relocation & Verification

**RPATH behavior:**
- Most Alpine binaries have no RPATH set
- Rely on default search paths (`/lib:/usr/lib`)
- musl loader supports `$ORIGIN` if needed

**If extracting to `$TSUKU_HOME/libs/`:**
- Would need patchelf to set RPATH to `$ORIGIN`
- Can reuse `fixElfRpath()` from `homebrew_relocate.go`
- No text placeholder replacement needed (unlike Homebrew)

**dlopen verification:**
- tsuku-dltest works on musl if built for musl
- Existing `LD_LIBRARY_PATH` prepending is sufficient
- No Alpine-specific loader quirks

**System packages (apk_install):**
- Zero relocation needed
- Native paths work immediately
- Action already exists in `linux_pm_actions.go`

---

## Comparison: APK Extraction vs System Packages

| Aspect | APK Extraction | System Packages (apk_install) |
|--------|---------------|-------------------------------|
| Works on Alpine | Yes | Yes |
| Works on Void musl | No | Yes (via xbps_install) |
| Works on Chimera | No | Yes (via apk_install for APKv3) |
| Hermetic version control | Yes | No |
| Implementation effort | ~500 LOC | 0 (already exists) |
| RPATH fixup needed | Yes | No |
| Maintenance burden | Parse APKINDEX, track Alpine releases | None (distro handles) |
| Security updates | Manual recipe updates | Automatic via distro |

---

## Preliminary Recommendation

The research suggests **Option 1D (system packages)** is the pragmatic choice:

1. **APK extraction doesn't solve the cross-distro problem** - it only works on Alpine
2. **System packages already work** - `apk_install` action exists
3. **Zero relocation needed** - native paths, no patchelf required
4. **Each musl distro has its own ecosystem** - no universal solution without static linking

**If hermetic version control on Alpine specifically becomes critical later**, APK extraction is viable (~500 LOC, medium complexity) but provides no advantage over `apk_install` for the core problem of "make libraries work on musl systems."

---

## Open Questions for Further Investigation

1. **Static linking viability:** Could tsuku provide statically-linked library binaries that work across all musl distros?

2. **Alpine market share:** What percentage of tsuku's musl users are on Alpine specifically vs other musl distros?

3. **Version pinning importance:** How critical is hermetic version control for library dependencies in practice?

4. **Hybrid approach:** Could we use Homebrew on glibc + system packages on musl, with APK extraction as optional Alpine-only feature?

---

## Files Created

- `wip/research/explore_apk-format.md` - APK file format details
- `wip/research/explore_apk-infrastructure.md` - Alpine CDN and APKINDEX
- `wip/research/explore_apk-portability.md` - Cross-distro compatibility
- `wip/research/explore_apk-download.md` - Download/extract implementation
- `wip/research/explore_apk-relocation.md` - Relocation and verification
- `wip/research/explore_apk-synthesis.md` - This synthesis document

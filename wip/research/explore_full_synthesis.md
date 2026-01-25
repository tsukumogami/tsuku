# Platform Compatibility Verification: Full Research Synthesis

**Date:** 2026-01-24
**Issue:** #1092
**Status:** Research complete, awaiting final decision

---

## 1. The Original Problem

tsuku claims support for multiple platforms and Linux families, but testing doesn't verify actual compatibility. This was exposed when musl-based systems (Alpine Linux) couldn't load embedded libraries that link against glibc.

**Root cause:** Homebrew bottles are built for glibc. The dynamic linker on musl systems can't load them.

**Scope expanded:** Beyond just "fix musl," the exploration revealed broader questions about tsuku's library distribution philosophy.

---

## 2. What We Found About Homebrew Bottles

### The Assumed Value

The initial assumption was that Homebrew bottles provide:
- Fresher versions than distro packages
- Hermetic, reproducible builds
- "Self-contained" philosophy alignment

### The Reality

Research showed this assumption is partially incorrect:

| Claim | Finding |
|-------|---------|
| Fresher versions | **False.** Distros often lead. Debian Bookworm has zlib 1.2.13 (same as Homebrew), libyaml 0.2.5 (same), openssl 3.0.15 (Homebrew has 3.4.0 but distros backport security fixes) |
| Security-reviewed | **Homebrew is volunteer-maintained.** Distro packages have institutional security teams with formal CVE processes |
| Self-contained | **True, but misunderstood.** "Self-contained" means no build tools needed, not "duplicate system libraries" |

### The Four Embedded Libraries

Only 4 library recipes exist that use Homebrew bottles:
- zlib
- libyaml
- openssl
- gcc-libs

Dependency graph is shallow (max depth 2). This is a small surface area.

---

## 3. Philosophy Evolution

### Original Philosophy
"Self-contained = download everything, no system dependencies"

### Revised Understanding
"Self-contained = no build tools needed, but library dependencies can come from system package managers"

**Key insight from user:** The recipe IS the abstraction layer. `openssl.toml` maps the abstract name "openssl" to concrete package names (`libssl-dev`, `openssl-devel`, `openssl-dev`, etc.). Tools just say `dependencies = ["openssl"]`.

---

## 4. Existing Infrastructure

tsuku already has package manager actions with mutual exclusion:

```go
// Each action has ImplicitConstraint() that restricts execution to its family
apt_install    → only runs on Debian family
dnf_install    → only runs on RHEL family
apk_install    → only runs on Alpine family
pacman_install → only runs on Arch family
zypper_install → only runs on SUSE family
brew_install   → only runs on macOS
```

**No new action needed.** Library recipes can use existing actions:

```toml
[[steps]]
action = "apt_install"
packages = ["libssl-dev"]

[[steps]]
action = "dnf_install"
packages = ["openssl-devel"]

[[steps]]
action = "apk_install"
packages = ["openssl-dev"]

[[steps]]
action = "brew_install"
packages = ["openssl@3"]
```

---

## 5. Alternatives Researched for musl

### Option A: System Packages (apk_install)

**Approach:** Use Alpine's package manager like any other distro.

| Aspect | Assessment |
|--------|------------|
| Hermetic | No - uses whatever version Alpine has |
| Version control | No - can't pin to specific version |
| Maintenance | Low - Alpine handles updates |
| Complexity | Low - just add `apk_install` to recipes |

### Option B: Alpine APK Extraction

**Approach:** Download APK files directly from Alpine CDN and extract them (APK = tar.gz).

**Discovery:** APK files are three concatenated gzip streams:
1. Signature
2. Control data (checksums, deps)
3. Data (actual files)

Can be extracted with: `tar -xzf package.apk`

| Aspect | Assessment |
|--------|------------|
| Hermetic | Yes - download specific version |
| Version control | Yes - pin to exact version like Homebrew |
| Maintenance | Medium - need to track Alpine releases |
| Complexity | Medium - need APKINDEX parsing for checksums |

**CDN structure:**
```
https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/
├── APKINDEX.tar.gz   # Package metadata with SHA1 checksums
├── zlib-1.3.1-r0.apk
├── openssl-3.1.4-r5.apk
└── ...
```

### Option C: Static Linking via Zig

**Approach:** Use Zig to cross-compile libraries targeting musl.

```bash
zig cc -target x86_64-linux-musl
```

| Aspect | Assessment |
|--------|------------|
| Hermetic | Yes |
| Version control | Yes - compile specific version |
| Maintenance | Higher - need build infrastructure |
| Complexity | Higher - Zig adds ~50MB |

**Limitation:** Zig can compile C/C++ code but doesn't bundle higher-level libraries like openssl. Would need to actually build openssl from source.

### Option D: musl.cc Toolchains

**Approach:** Use pre-built cross-compiler toolchains from musl.cc.

Similar trade-offs to Zig - good for building, but requires source compilation of libraries.

### Option E: mise-style Fallback

**Approach:** Auto-detect glibc vs musl, select right binary, fall back to source compilation.

mise's philosophy:
- Downloads pre-built when available
- Honest about system dependencies needed
- Falls back to source compilation with clear docs
- Doesn't try to bundle everything

---

## 6. Comparison Matrix

| Approach | Hermetic? | Version Control? | Works on musl? | Maintenance |
|----------|-----------|------------------|----------------|-------------|
| Homebrew bottles (current) | Yes | Yes | **No** | Low |
| System packages (apk_install) | No | No | Yes | Low |
| Alpine APK extraction | Yes | Yes | Yes | Medium |
| Static build via Zig | Yes | Yes | Yes | Higher |
| mise-style fallback | Partial | Partial | Yes | Low |

---

## 7. The Trade-off

The core tension:

**Hermetic version control** (exact reproducible library versions)
vs.
**Universal compatibility** (works on all platforms including musl)

### Preserving Both

A hybrid approach could work:

1. **glibc systems (Debian, RHEL, Arch, SUSE, macOS):** Continue using Homebrew bottles - they work and provide version control

2. **musl systems (Alpine):** Either:
   - Use system packages (simple, loses version control)
   - Use Alpine APK extraction (preserves version control, more complex)

The user expressed concern about "giving up an important feature" (version control), which suggests Alpine APK extraction may be the preferred path if complexity is acceptable.

---

## 8. Runtime Detection

Regardless of which approach is chosen for musl, runtime detection is needed:

```go
// internal/platform/libc.go
func DetectLibc() string {
    // Check for musl interpreter
    matches, _ := filepath.Glob("/lib/ld-musl-*.so.1")
    if len(matches) > 0 {
        return "musl"
    }
    return "glibc"
}
```

This allows:
- Selecting appropriate library source (Homebrew vs APK vs system)
- Skipping dlopen verification on musl (our test binaries are glibc)
- Providing clear error messages

---

## 9. What the Design Document Currently Says

**Decision Outcome:** 1D+1C + 2C + 3A

- **1D+1C:** System packages as primary path for musl, with runtime detection
- **2C:** Hybrid testing (native runners + containers)
- **3A:** Test matrix matches release matrix

**Note:** The design may need updating based on final decision about Alpine APK extraction vs. system packages.

---

## 10. Open Questions

1. **Is Alpine APK extraction worth the complexity?** It preserves version control but adds maintenance burden.

2. **Do users actually need version-pinned libraries on musl?** Or is "it works" sufficient for Alpine users?

3. **Should Homebrew remain the primary path on glibc?** Or should all Linux families move to system packages for consistency?

---

## 11. Review Documents Created

- `wip/research/explore_phase4_review.md` - Options analysis review
- `wip/research/explore_phase8_architecture-review.md` - Architecture clarity review
- `wip/research/explore_phase8_security-review.md` - Security analysis
- `wip/explore_summary.md` - Decision summary

All reviews concluded the design is sound and ready for implementation.

---

## 12. Recommended Next Steps

1. **Decide on musl approach:**
   - Simple: System packages via `apk_install`
   - Hermetic: Alpine APK extraction (like Homebrew for musl)

2. **Update design document** with final decision

3. **Proceed to Phase 9** (implementation issues) once design is finalized

4. **Create PR** for the design document

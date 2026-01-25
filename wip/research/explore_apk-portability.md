# Alpine APK Binary Portability Across musl Distributions

**Date:** 2026-01-24
**Research Question:** Do Alpine APK binaries work on non-Alpine musl-based Linux distributions?

---

## Executive Summary

**Short answer: Theoretically yes, practically with caveats.**

Alpine APK binaries are built against musl libc, which has a stable ABI. Binaries compiled against musl are theoretically portable across all musl-based distributions (Void Linux musl, Chimera Linux, Adelie Linux, postmarketOS, etc.) as long as:

1. The dynamic linker path is at `/lib/ld-musl-$ARCH.so.1`
2. The target system has the same or newer musl version
3. All shared library dependencies are available

However, **this is not the same as "drop Alpine APK packages into Void and they work."** Each distribution maintains its own repositories, paths, and dependency graph. APK is a package format, not a universal binary distribution mechanism.

**Recommendation for tsuku:** APK extraction does NOT provide a "musl-universal" distribution mechanism comparable to Homebrew bottles on glibc. Use system package managers for each musl distro instead.

---

## 1. Compatibility Matrix

### Do Alpine Binaries Run on Other musl Distros?

| Target Distro | Uses musl | Uses APK | Alpine Binary Compatible? | Notes |
|---------------|-----------|----------|---------------------------|-------|
| Void Linux (musl) | Yes | No (uses xbps) | Likely yes* | Different package ecosystem |
| Chimera Linux | Yes | Yes (APKv3) | No | Incompatible APK version |
| Adelie Linux | Yes | Yes (APKv2) | Partial | Different repos, different packages |
| postmarketOS | Yes | Yes (APKv2) | Partial | Based on Alpine, but separate repos |
| OpenWrt (new) | Yes | Yes (APKv3) | No | Switching to APKv3 |
| Sabotage Linux | Yes | No | Likely yes* | Source-based, no binary packages |
| KISS Linux | Yes | No | Likely yes* | Source-based, no binary packages |

*"Likely yes" means: if you extract the binary and its deps, and the musl/library versions align, it should work. But you'd need to manually extract and manage deps.

### APK Version Incompatibility

**Critical finding:** Chimera Linux and OpenWrt use APKv3, while Alpine still uses APKv2. These are **not compatible**:

> "Chimera Linux is already using APKv3, at a time when this version was not yet adopted by Alpine Linux itself." - [Chimera Linux Wikipedia](https://en.wikipedia.org/wiki/Chimera_Linux)

Chimera explicitly does not re-use Alpine packages despite using the same package manager tool.

---

## 2. Technical Constraints

### 2.1 musl ABI Requirements

musl provides a stable ABI with backward compatibility:

> "A binary compiled against an earlier version of musl is guaranteed to run against a newer musl runtime." - [musl FAQ](https://www.musl-libc.org/faq.html)

**Key requirements from [musl Distribution Guidelines](https://wiki.musl-libc.org/guidelines-for-distributions.html):**

1. **Dynamic linker location must be `/lib/ld-musl-$ARCH.so.1`** - distributions must not change this to `/lib64` or anything else
2. **No multilib/multiarch** - musl doesn't support GCC-style multilib
3. **Pathname consistency** - changing paths like `/etc/resolv.conf` in static binaries breaks portability

### 2.2 musl Version Compatibility

| Distribution | musl Version (as of 2026) |
|--------------|---------------------------|
| Alpine 3.19 | 1.2.4 |
| Alpine 3.20 | 1.2.5 |
| Void Linux | 1.2.5 |
| Chimera Linux | 1.2.5 |

Minor version differences (1.2.4 vs 1.2.5) are generally compatible. A binary built against 1.2.4 should work on 1.2.5.

**Breaking changes are rare but happen:**
- musl 1.2.x changed `time_t` to 64-bit on 32-bit architectures (affects armhf, armv7, x86)
- Alpine 3.0 (musl transition from uClibc) was not ABI compatible with earlier versions

### 2.3 Kernel Header Dependencies

Kernel headers don't typically affect binary portability unless the binary uses kernel-specific features. Alpine packages are built against standard Linux kernel headers.

---

## 3. Alpine-Specific Patches and Issues

### 3.1 Does Alpine Patch musl?

Alpine generally uses upstream musl with minimal patches. Unlike some glibc distributions that carry significant downstream patches, musl's small size and clean design mean there's less need for distribution-specific modifications.

However, Alpine does make distribution-specific choices:
- Uses BusyBox instead of GNU coreutils
- Has specific filesystem layout conventions
- Packages may have Alpine-specific compile flags

### 3.2 Known Incompatibilities

1. **DNS resolution differences:** musl performs parallel DNS queries, which can cause issues in containerized environments. This is a musl behavior, not Alpine-specific.

2. **Locale support:** musl has limited locale support compared to glibc. Alpine packages are built with this limitation in mind.

3. **glibc-specific APIs:** Some software depends on glibc-specific functions or behaviors. These won't work on any musl system (not just Alpine).

---

## 4. Is There a "Universal musl" Binary Format?

### 4.1 Python's musllinux Standard (PEP 656)

The Python ecosystem has defined a [musllinux platform tag](https://peps.python.org/pep-0656/) for musl-compatible wheels:

> "The musllinux tag promises the wheel works on any mainstream Linux distribution that uses musl version ${MUSLMAJOR}.${MUSLMINOR}."

The tag format is `musllinux_X_Y_ARCH` (e.g., `musllinux_1_2_x86_64`).

**This is the closest thing to a "universal musl" binary format**, but it has requirements:
- Python interpreter must be dynamically linked against musl
- System-level dependencies must be available
- Works for Python extensions, not general binaries

### 4.2 Static Linking

The most portable option for musl binaries is static linking:

> "Binaries statically linked with musl have no external dependencies, even for features like DNS lookups or character set conversions." - [musl Getting Started](https://wiki.musl-libc.org/getting-started.html)

Static binaries work on any Linux system (even glibc ones) as long as the kernel provides compatible syscalls.

**Limitation:** Static linking doesn't work well for:
- GUI applications (need libGL, libX11, etc.)
- Software that dynamically loads plugins
- Very large applications (binary size increase)

### 4.3 Verdict

There is no universal "musl bottle" equivalent to Homebrew's glibc bottles. The options are:
1. **Static binaries** - work everywhere but have limitations
2. **musllinux wheels** - Python-specific, version-tagged
3. **Per-distro packages** - what actually works in practice

---

## 5. How Other Tools Handle musl

### 5.1 mise (formerly rtx)

mise handles musl systems with configurable behavior:

> "Setting [this] will not use precompiled binaries for all languages. This is useful if running on a Linux distribution like Alpine that does not use glibc and therefore likely won't be able to run precompiled binaries."

For Node.js, mise supports specifying flavors like `glibc-217` or `musl`:

> "For Node.js specifically, mise supports installing a specific node flavor like 'glibc-217' or 'musl' for use with the unofficial node build repo."

**Approach:** Runtime detection + separate binary downloads per libc + fallback to source builds.

### 5.2 asdf

asdf plugins handle musl inconsistently:
- Some plugins (like asdf-golang) have explicit Alpine/musl support
- Others require source compilation on musl systems

GitHub issue [asdf-nodejs#190](https://github.com/asdf-vm/asdf-nodejs/issues/190) shows community discussions about musl support.

### 5.3 gcompat

For running glibc binaries on musl systems, [gcompat](https://wiki.alpinelinux.org/wiki/Running_glibc_programs) provides compatibility shims. This is a workaround, not a solution for distributing musl binaries.

---

## 6. Why APK is Not a Universal musl Distribution Mechanism

### 6.1 The Homebrew Comparison

Homebrew bottles work across glibc distros (Debian, Fedora, Arch) because:
1. glibc has symbol versioning for backward compatibility
2. Homebrew controls the entire dependency tree
3. RPATHs are patched to point to Homebrew's lib directory
4. All bottles are built against a consistent baseline (usually Ubuntu LTS)

### 6.2 Why Alpine APK Can't Do the Same

Alpine APK packages are built for Alpine specifically:
1. **Dependency resolution assumes Alpine repos** - `apk add openssl` expects Alpine's package names
2. **No RPATH manipulation** - packages expect dependencies at system paths
3. **APK version fragmentation** - Chimera/OpenWrt use APKv3, Alpine uses APKv2
4. **No cross-distro testing** - packages are tested on Alpine, not Void/Chimera

Even if you extract binaries from APK files, you'd need to:
- Manually resolve all dependencies
- Potentially fix library paths
- Handle Alpine-specific filesystem conventions

**This is not practical for a package manager like tsuku.**

---

## 7. Recommendation for tsuku

### 7.1 What APK Extraction Could Provide

If tsuku extracted Alpine APK packages:
- **Pros:** Hermetic version control, musl-compatible binaries
- **Cons:** Only works on Alpine, dependencies are Alpine-specific

This is NOT a "musl-universal" solution. It's "Alpine support via APK extraction."

### 7.2 What System Packages Provide

Using system package managers (apk, xbps, etc.):
- **Pros:** Works on every musl distro, native integration, security updates
- **Cons:** No hermetic version control, varies by distro

### 7.3 Recommended Approach

**Stick with the design document's recommendation: system packages for library dependencies.**

The research confirms that:
1. APK binaries are not universally portable across musl distros
2. Each musl distro maintains its own package ecosystem
3. There's no "musl-universal" binary format for shared libraries
4. System packages are the only practical path to cross-distro musl support

For tsuku's 4 library dependencies (zlib, openssl, libyaml, gcc-libs), using `apk_install` on Alpine and equivalent actions on other musl distros is the correct approach.

### 7.4 If Version Pinning is Required

If hermetic version control on Alpine is a hard requirement, APK extraction could be an Alpine-only feature:
- Extract APK files from Alpine CDN
- Use APKINDEX.tar.gz for checksums
- Only enable for `os = "linux", family = "alpine"`

But this would NOT work on Void musl, Chimera, or other musl distros. It's just a more complex way to do what `apk_install` already does.

---

## 8. Summary of Findings

| Question | Answer |
|----------|--------|
| Do Alpine binaries run on Void Linux musl? | Likely yes for extracted binaries, but dependencies are an issue |
| What about Chimera Linux? | No - uses incompatible APKv3 |
| What ABI constraints exist? | musl version (backward compatible), dynamic linker path |
| Does Alpine use specific patches? | Minimal - mostly upstream musl |
| Known incompatibilities? | DNS behavior, locale support, glibc-specific APIs |
| Is there a "universal musl" binary? | No - only static linking or musllinux wheels |
| What do mise/asdf do? | Runtime detection, separate musl binaries, source fallback |

---

## Sources

### musl Documentation
- [musl Distribution Guidelines](https://wiki.musl-libc.org/guidelines-for-distributions.html)
- [musl Getting Started](https://wiki.musl-libc.org/getting-started.html)
- [musl FAQ](https://www.musl-libc.org/faq.html)
- [musl Wikipedia](https://en.wikipedia.org/wiki/Musl)

### Distribution-Specific
- [Alpine Linux Wikipedia](https://en.wikipedia.org/wiki/Alpine_Linux)
- [Chimera Linux Wikipedia](https://en.wikipedia.org/wiki/Chimera_Linux)
- [Chimera Linux Package Management](https://chimera-linux.org/docs/apk)
- [Adelie Linux FAQ](https://oldwww.adelielinux.org/about/faq.html)
- [OpenWrt APK Adoption](https://linuxiac.com/openwrt-adopts-apk-as-new-package-manager/)

### Python Ecosystem
- [PEP 656 - musllinux Platform Tag](https://peps.python.org/pep-0656/)

### Tool Comparisons
- [mise Settings - Node.js Flavors](https://mise.jdx.dev/configuration/settings.html)
- [asdf-nodejs musl Discussion](https://github.com/asdf-vm/asdf-nodejs/issues/190)

### Compatibility
- [Alpine Linux Support Issues](https://alpinelinuxsupport.com/common-alpine-linux-support-issues/)
- [Running glibc Programs on Alpine](https://wiki.alpinelinux.org/wiki/Running_glibc_programs)
- [Creating Portable Linux Binaries](https://blog.gibson.sh/2017/11/26/creating-portable-linux-binaries/)

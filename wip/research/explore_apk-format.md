# Alpine APK Package Format: Deep Research

**Date:** 2026-01-24
**Context:** Platform Compatibility Verification design (Issue #1092)
**Purpose:** Evaluate APK format for potential use as musl-compatible library source

---

## Executive Summary

Alpine APK packages are simpler than Homebrew bottles and do NOT require relocation for tsuku's use case. Unlike Homebrew bottles which contain `@@HOMEBREW_PREFIX@@` placeholders requiring complex text/binary patching, Alpine packages are built with standard `/usr` prefix and can be extracted directly if installed to `/usr`. However, since tsuku installs to `$TSUKU_HOME`, APK packages would still require RPATH fixup for binaries (similar to current Homebrew handling).

**Key finding:** The relocation complexity for APK is comparable to Homebrew bottles, but APK lacks the text placeholder mechanism. This means simpler text file handling but similar or greater challenges for ELF binaries.

---

## 1. APK File Structure: The Three Gzip Streams

An APK v2 package is three concatenated gzip streams, each containing a tar archive:

```
+-------------------+
| Stream 1: Sig     |  <- Signature tar segment (no end-of-tar records)
+-------------------+
| Stream 2: Control |  <- Control tar segment (no end-of-tar records)
+-------------------+
| Stream 3: Data    |  <- Data tarball (complete with end-of-tar records)
+-------------------+
```

### Stream 1: Signature Segment

Contains a single file: `.SIGN.RSA.<key_name>.rsa.pub`

- **Format:** DER-encoded PKCS1v15 RSA signature of the SHA1 hash of the control segment's gzip stream
- **Permissions:** 0644, uid 0, gid 0
- **Purpose:** Cryptographic verification that the control segment hasn't been tampered with

**Verification process:**
1. apk looks for `/etc/apk/keys/<key_name>.rsa.pub`
2. If found, verifies signature against control segment
3. If not found, verification fails (unless `--allow-untrusted`)

### Stream 2: Control Segment

Contains package metadata and installation scripts. All filenames are prefixed with a dot (`.`).

**Required file:** `.PKGINFO` - plain text, similar to INI format

```
# Sample .PKGINFO
pkgname = zlib
pkgver = 1.3.1-r0
pkgdesc = A compression/decompression Library
url = https://zlib.net/
arch = x86_64
origin = zlib
maintainer = Natanael Copa <ncopa@alpinelinux.org>
license = Zlib
depend = musl>=1.2.4
datahash = Q1abc123...  # SHA256 of data segment (base64, Q1 prefix)
```

**Key fields:**
| Field | Description | Format |
|-------|-------------|--------|
| `pkgname` | Package name | String |
| `pkgver` | Version with build revision | `1.3.1-r0` |
| `arch` | Target architecture | `x86_64`, `aarch64`, `armv7`, etc. |
| `depend` | Dependencies (repeatable) | `pkgname>=version` |
| `datahash` | SHA256 of data segment | Base64 with `Q1` prefix |

**Optional files:**
- `.pre-install`, `.post-install` - Installation scripts
- `.pre-upgrade`, `.post-upgrade` - Upgrade scripts
- `.pre-deinstall`, `.post-deinstall` - Removal scripts
- `.trigger` - Trigger scripts

### Stream 3: Data Tarball

Contains the actual package files, laid out for extraction at filesystem root.

**Special feature:** PAX extended headers with `APK-TOOLS.checksum.SHA1` containing per-file SHA1 hashes.

```
# Example data tarball structure
usr/
usr/lib/
usr/lib/libz.so.1.3.1
usr/lib/libz.so -> libz.so.1
usr/lib/libz.so.1 -> libz.so.1.3.1
usr/lib/pkgconfig/
usr/lib/pkgconfig/zlib.pc
```

**Note:** This is a complete tarball with end-of-tar null records, unlike the segment format used for signature and control.

---

## 2. File Permissions, Ownership, and Symlinks

### Tar Header Encoding

APK uses standard tar format with PAX extensions:

| Attribute | Encoding |
|-----------|----------|
| Permissions | Standard tar mode field (octal) |
| UID/GID | Numeric in tar header, names in PAX headers if needed |
| Symlinks | Standard tar type '2', link target in linkname field |
| Hardlinks | Standard tar type '1', link target in linkname field |
| Device files | Standard tar types '3' (char) and '4' (block) |

### PAX Extended Headers

Alpine's `abuild-tar` tool adds custom PAX headers:

```
APK-TOOLS.checksum.SHA1 = <40-char hex hash>
```

This provides per-file integrity verification beyond what tar natively offers.

### Symlink Handling

Symlinks are encoded using standard tar symlink entries:
- Type: '2' (symbolic link)
- Link target stored in `linkname` field
- Can be relative or absolute paths

**Known issue:** When using `apk add --root /some/path`, symlinks with absolute targets may cause issues with `renameat(2)` if the path crosses mount boundaries.

---

## 3. Control Stream (.PKGINFO) vs APKINDEX

| Aspect | .PKGINFO (in package) | APKINDEX (in repository) |
|--------|----------------------|--------------------------|
| Location | Inside each .apk file | `APKINDEX.tar.gz` in repo |
| Purpose | Full package metadata | Quick lookup for dependency resolution |
| Format | Key-value pairs | Abbreviated single-letter codes |

### APKINDEX Format

```
C:Q1abc123...    # Control segment checksum (SHA1, base64)
P:zlib           # Package name
V:1.3.1-r0       # Version
A:x86_64         # Architecture
S:12345          # Package size (compressed)
I:67890          # Installed size
T:A compression/decompression Library
U:https://zlib.net/
L:Zlib           # License
D:musl>=1.2.4    # Dependencies
                 # Blank line separates records
```

The `C:` field is crucial for tsuku: it's the SHA1 hash of the control segment gzip stream. This serves as the package checksum for verification.

---

## 4. Path Placeholders: APK vs Homebrew

### Homebrew Bottles

Homebrew uses text placeholders that must be replaced at install time:

```
# In text files
prefix = @@HOMEBREW_PREFIX@@
cellar_path = @@HOMEBREW_CELLAR@@/curl/8.17.0

# In binaries (RPATH)
RPATH: @@HOMEBREW_PREFIX@@/lib
```

**tsuku's current handling** (from `homebrew_relocate.go`):
1. Scan all files for placeholder strings
2. Replace placeholders in text files with install path
3. Use `patchelf`/`install_name_tool` to fix RPATH in binaries
4. Fix ELF interpreter path if it contains placeholders

### Alpine APK Packages

**No text placeholders.** Alpine packages are built with hardcoded `/usr` prefix:

```bash
# Standard Alpine build
./configure --prefix=/usr --sysconfdir=/etc

# All paths in installed files reference /usr
prefix=/usr
libdir=/usr/lib
```

**Implications for tsuku:**

| Aspect | Homebrew | Alpine APK |
|--------|----------|------------|
| Text file relocation | Required (placeholder replacement) | **Not required** (no placeholders) |
| Binary RPATH fixup | Required | **Still required** (hardcoded /usr paths) |
| ELF interpreter | May contain placeholders | Standard `/lib/ld-musl-*.so.1` |
| .pc files (pkg-config) | Placeholder paths | Hardcoded `/usr` paths |

---

## 5. Relocatable Binaries: Reality Check

### Alpine's Design Philosophy

Alpine packages are NOT designed to be relocatable:

> "Alpine uses /usr for prefix to make sure everything is installed with /usr in front of it."

> "There's too much stuff out there that has various /usr containing paths hardcoded to be worth the effort to get rid of /usr."

Packages assume installation to standard system paths (`/usr`, `/lib`, `/etc`).

### What "Not Relocatable" Means for tsuku

If tsuku extracts APK packages to `$TSUKU_HOME/libs/zlib-1.3.1/`:

1. **Library files work fine** - `.so` files can be loaded from anywhere
2. **RPATH needs fixing** - Binaries reference `/usr/lib`, need patching to `$TSUKU_HOME/libs/...`
3. **pkg-config files break** - `.pc` files have hardcoded `/usr` paths
4. **Header paths break** - Include paths reference `/usr/include`
5. **Config scripts break** - `*-config` scripts output hardcoded paths

### ELF RPATH on musl

Alpine binaries use musl's dynamic linker:

```
ELF interpreter: /lib/ld-musl-x86_64.so.1
```

For library loading, standard RPATH/RUNPATH mechanisms apply:
- `$ORIGIN` works for relative paths
- `patchelf --set-rpath` works on musl binaries
- No special musl considerations for RPATH

---

## 6. Signature and Verification Mechanisms

### Package Signature Chain

```
+----------------+     signs      +------------------+
| Alpine RSA key | ------------> | Control segment  |
+----------------+                +------------------+
                                         |
                                  SHA1 hash of gzip stream
                                         |
                                         v
                              +---------------------+
                              | .SIGN.RSA.* file    |
                              | (DER PKCS1v15 sig)  |
                              +---------------------+
```

### Data Integrity

```
Control segment (.PKGINFO)
         |
    datahash = Q1...  (SHA256 of data segment, base64)
         |
         v
+------------------+
| Data tarball     |
+------------------+
         |
    Per-file: APK-TOOLS.checksum.SHA1 PAX headers
         |
         v
+------------------+
| Individual files |
+------------------+
```

### Security Concerns

**Current limitation:** Package signatures use SHA1. While the data segment uses SHA256 for the datahash, the outer signature over the control segment still relies on SHA1.

> "The signatures on packages are still done using SHA-1. This is odd, because the signature is over control.tar.gz -- which is bound to data.tar.gz using a SHA-256 hash."

For tsuku's purposes:
- The APKINDEX `C:` field provides a checksum for package identification
- Per-file SHA1 hashes in PAX headers enable integrity verification after extraction
- Full signature verification would require Alpine's public keys

---

## 7. Comparison to Homebrew Bottles

| Aspect | Homebrew Bottles | Alpine APK |
|--------|-----------------|------------|
| **Format** | Single gzip tarball | 3 concatenated gzip streams |
| **Signature** | Checksum in bottle JSON | RSA signature in first stream |
| **Path handling** | `@@HOMEBREW_*@@` placeholders | Hardcoded `/usr` prefix |
| **Text relocation** | Replace placeholders | Fix `.pc` files, config scripts |
| **Binary relocation** | patchelf RPATH fixup | patchelf RPATH fixup |
| **Per-file integrity** | None | SHA1 in PAX headers |
| **libc** | glibc only | musl (native to Alpine) |
| **Extraction** | `tar -xzf` | `tar -xzf` (works despite 3 streams) |

---

## 8. Implications for tsuku Implementation

### If tsuku Adopts APK Extraction (Option B from synthesis)

**Advantages:**
1. Native musl compatibility (libraries built for musl)
2. Version pinning possible (specific package versions from Alpine CDN)
3. Simpler text file handling (no placeholder replacement needed)
4. Smaller packages (Alpine's minimal philosophy)

**Challenges:**
1. **RPATH fixup still required** - Same patchelf dance as Homebrew
2. **pkg-config files need patching** - Replace `/usr` with install path
3. **Config scripts need patching** - Same issue
4. **APKINDEX parsing required** - To get checksums for verification
5. **Architecture mapping** - `x86_64` vs `amd64`, `aarch64` vs `arm64`

### Relocation Complexity Comparison

```
Homebrew Bottle Relocation:
1. Scan for @@HOMEBREW_PREFIX@@ in text files
2. Replace placeholders with install path
3. patchelf --set-rpath for binaries
4. Fix ELF interpreter if placeholder

Alpine APK Relocation:
1. No text placeholder scan needed
2. Fix .pc files: sed 's|/usr|$INSTALL_PATH|g'
3. Fix *-config scripts: same pattern
4. patchelf --set-rpath for binaries
5. ELF interpreter is standard (/lib/ld-musl-*)
```

**Net assessment:** Similar complexity. APK eliminates placeholder scanning but adds `.pc` file patching. Binary handling is identical.

### Recommended Approach

Given the research findings, the design document's recommendation (Option 1D: system packages) remains sound for these reasons:

1. **No relocation needed** - System packages install to standard paths
2. **Native musl** - apk installs musl-built libraries
3. **Security maintained** - Alpine security team handles updates
4. **Simplest implementation** - Use existing `apk_install` action

If version pinning becomes critical, APK extraction is viable but requires:
- APKINDEX parser for checksums
- Relocation logic for `.pc` files and RPATH
- Testing across Alpine versions

---

## Sources

### Primary Sources
- [Alpine Wiki: Apk spec](https://wiki.alpinelinux.org/wiki/Apk_spec)
- [APK, the strangest format (hydrogen18.com)](https://www.hydrogen18.com/blog/apk-the-strangest-format.html)
- [Adelie Linux: APK internals](https://wiki.adelielinux.org/wiki/APK_internals)
- [Alpine Wiki: APKBUILD Reference](https://wiki.alpinelinux.org/wiki/APKBUILD_Reference)

### Signature and Security
- [GitLab: Package signatures should not use SHA-1](https://gitlab.alpinelinux.org/alpine/apk-tools/-/issues/10670)
- [Remote Code Execution in Alpine Linux (justi.cz)](https://justi.cz/security/2018/09/13/alpine-apk-rce.html)

### Path and Relocation
- [LWN: Alpine Linux plans /usr merge](https://lwn.net/Articles/1040410/)
- [Alpine Blog: Implementing /usr merge](https://www.alpinelinux.org/posts/2025-10-01-usr-merge.html)
- [Alpine Wiki: Creating an Alpine package](https://wiki.alpinelinux.org/wiki/Creating_an_Alpine_package)

### musl and ELF
- [Alpine: Running glibc programs](https://wiki.alpinelinux.org/wiki/Running_glibc_programs)
- [GitHub: NixOS/patchelf](https://github.com/NixOS/patchelf)
- [build-your-own.org: Alpine static linking](https://build-your-own.org/blog/20221229_alpine/)
- [Wikipedia: rpath](https://en.wikipedia.org/wiki/Rpath)

### Symlink and Extraction
- [GitLab: apk symbolic link handling](https://gitlab.alpinelinux.org/alpine/apk-tools/-/issues/9634)
- [GitLab: Extract files from .apks](https://gitlab.alpinelinux.org/alpine/apk-tools/-/issues/7103)

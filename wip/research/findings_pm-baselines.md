# Findings: Package Manager Baselines

## Summary

Each Linux distribution ships with a different baseline of pre-installed packages. This significantly affects what tsuku can assume about the environment.

## Baseline Comparison

### Package Counts

| Distribution | Package Manager | Installed Packages | /usr/bin Files | libc |
|--------------|-----------------|-------------------|----------------|------|
| debian:bookworm-slim | apt | 93 | 275 | glibc 2.36 |
| ubuntu:24.04 | apt | 97 | 297 | glibc 2.39 |
| fedora:41 | dnf | 125 | 259 | glibc 2.40 |
| alpine:3.19 | apk | 15 | 143 | musl 1.2.4 |
| archlinux:base | pacman | 123 | 927 | glibc 2.42 |

### Key Observations

1. **Alpine is minimalist**: Only 15 packages, uses BusyBox for most utilities
2. **Arch is richest**: 927 binaries in /usr/bin, includes curl by default
3. **Debian/Ubuntu similar**: Both use apt, similar baseline, Ubuntu slightly newer glibc

## Debian vs Ubuntu Comparison

### Similarities
- Both use `apt` and `dpkg`
- Both default `/bin/sh` to `dash`
- Both have `bash` available
- Both **lack** curl, wget, and CA certificates
- Same core utilities structure

### Differences

| Aspect | Debian 12 (Bookworm) | Ubuntu 24.04 |
|--------|---------------------|--------------|
| libc version | 2.36 | 2.39 |
| Package count | 93 | 97 |
| GCC base | gcc-12 | gcc-14 |
| /etc/debian_version | 12.12 | trixie/sid |
| ID_LIKE in os-release | (not set) | debian |

**Conclusion**: For tsuku's purposes, Debian and Ubuntu are functionally equivalent. Same package manager, same package names, same baseline capabilities.

## Detailed Baselines by Distribution

### Debian/Ubuntu (apt)

**Included by default:**
- Shell: dash, bash
- Coreutils: full GNU coreutils
- Text processing: grep, sed, awk, diff
- Archive: tar, gzip
- File utilities: find, xargs

**NOT included:**
- Download: curl, wget
- SSL/TLS: ca-certificates, openssl
- Compression: unzip, xz, bzip2, zstd
- Development: git, make, gcc
- JSON: jq

### Fedora (dnf)

**Included by default:**
- Shell: bash only (no dash)
- Coreutils: full GNU coreutils
- Download: **curl included**
- SSL/TLS: **ca-certificates included**
- Archive: tar, gzip, bzip2, xz, zstd

**NOT included:**
- Download: wget
- Compression: unzip
- Development: git, make, gcc
- JSON: jq
- Utility: which

**Key difference**: Fedora includes curl and CA certificates by default.

### Alpine (apk)

**Included by default:**
- Shell: busybox ash (no bash)
- Utilities: BusyBox applets (304 commands)
- Download: **wget included** (via BusyBox)
- SSL/TLS: **ca-certificates-bundle included**
- Archive: tar, gzip, unzip, bzip2 (via BusyBox)

**NOT included:**
- Download: curl (but wget works)
- Compression: xz, zstd
- Development: git, make, gcc
- JSON: jq
- Shell: bash

**Key differences**:
- Uses musl libc instead of glibc (binary compatibility implications)
- Uses BusyBox for most utilities (compatible but simpler)
- Much smaller footprint (15 packages)

### Arch Linux (pacman)

**Included by default:**
- Shell: bash only
- Coreutils: full GNU coreutils
- Download: **curl included**
- SSL/TLS: **ca-certificates included**
- Archive: tar, gzip, bzip2, xz, zstd

**NOT included:**
- Download: wget
- Compression: unzip
- Development: git (but gcc-libs present)
- JSON: jq
- Utility: which

**Key observation**: Arch base is well-equipped for downloading (curl + certs).

## Critical Capability Matrix

| Capability | debian | ubuntu | fedora | alpine | arch |
|------------|--------|--------|--------|--------|------|
| Can download HTTPS | NO | NO | YES | YES | YES |
| Has curl | NO | NO | YES | NO | YES |
| Has wget | NO | NO | NO | YES | NO |
| Has CA certs | NO | NO | YES | YES | YES |
| Has tar+gzip | YES | YES | YES | YES | YES |
| Has unzip | NO | NO | NO | YES | NO |
| Has xz | NO | NO | YES | NO | YES |
| Uses glibc | YES | YES | YES | NO | YES |

## Bootstrap Package Requirements

To ensure tsuku can function, these packages need to be installed:

### apt (Debian/Ubuntu)
```bash
apt-get update && apt-get install -y curl ca-certificates
```

### dnf (Fedora)
Already has curl and ca-certificates - no bootstrap needed.

### apk (Alpine)
```bash
apk add --no-cache curl
```
Note: Has wget and ca-certificates, but curl may be preferred for consistency.

### pacman (Arch)
Already has curl and ca-certificates - no bootstrap needed.

## Implications for tsuku

1. **apt systems need bootstrap**: Debian and Ubuntu require package installation before tsuku can download anything.

2. **Fedora and Arch are ready**: These distros can download immediately.

3. **Alpine uses musl**: Pre-compiled binaries built for glibc may not work on Alpine without static linking or musl-specific builds.

4. **Unified tar+gzip**: All distros support `.tar.gz` extraction natively.

5. **xz not universal**: Prefer `.tar.gz` over `.tar.xz` for maximum compatibility.

## Recommendations

1. **Detect and bootstrap**: On first run, detect apt systems and offer to install `curl ca-certificates`.

2. **Handle Alpine specially**: May need musl-compatible binaries or static builds.

3. **Prefer curl**: More powerful than wget, and if bootstrap is needed anyway, standardize on curl.

4. **Document requirements**: Clearly state that curl and CA certificates are required.

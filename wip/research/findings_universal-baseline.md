# Findings: Universal Linux Baseline

## Summary

Testing across 5 major Linux distributions reveals that there is **no reliable universal baseline** for download tools and SSL/TLS certificates. However, core utilities are universally present.

## Test Methodology

Tested the following base images via Docker:
- `debian:bookworm-slim`
- `ubuntu:24.04`
- `fedora:41`
- `alpine:3.19`
- `archlinux:base`

**Important Note**: The `which` command is not universally available (missing in Fedora 41 and Arch base). Use `type cmd >/dev/null 2>&1` for portable command detection.

## Results Matrix

### Shell Availability

| Image | /bin/sh | bash |
|-------|---------|------|
| debian:bookworm-slim | dash | present |
| ubuntu:24.04 | dash | present |
| fedora:41 | bash | present |
| alpine:3.19 | busybox | **MISSING** |
| archlinux:base | bash | present |

**Finding**: `/bin/sh` is always present but implementations vary (dash, bash, busybox). Bash is NOT universal (missing in Alpine).

### Download Tools

| Image | curl | wget |
|-------|------|------|
| debian:bookworm-slim | **MISSING** | **MISSING** |
| ubuntu:24.04 | **MISSING** | **MISSING** |
| fedora:41 | present | **MISSING** |
| alpine:3.19 | **MISSING** | present |
| archlinux:base | present | **MISSING** |

**Critical Finding**: NO download tool is universally available. Debian and Ubuntu ship with NEITHER curl nor wget.

### Archive Tools

| Image | tar | gzip | unzip | xz | bzip2 | zstd |
|-------|-----|------|-------|----|----|------|
| debian:bookworm-slim | Y | Y | - | - | - | - |
| ubuntu:24.04 | Y | Y | - | - | - | - |
| fedora:41 | Y | Y | - | Y | Y | Y |
| alpine:3.19 | Y | Y | Y | - | Y | - |
| archlinux:base | Y | Y | - | Y | Y | Y |

**Finding**: `tar` and `gzip` are universal. `xz` and `unzip` are not.

### SSL/TLS (CA Certificates)

| Image | /etc/ssl/certs | ca-certificates.crt |
|-------|----------------|---------------------|
| debian:bookworm-slim | **MISSING** | **MISSING** |
| ubuntu:24.04 | **MISSING** | **MISSING** |
| fedora:41 | 446 files | present |
| alpine:3.19 | 1 file | present |
| archlinux:base | 435 files | present |

**Critical Finding**: Debian and Ubuntu base images ship WITHOUT CA certificates. HTTPS requests will fail without installing `ca-certificates` package.

### Coreutils

| Command | debian | ubuntu | fedora | alpine | arch |
|---------|--------|--------|--------|--------|------|
| cat | Y | Y | Y | Y | Y |
| cp | Y | Y | Y | Y | Y |
| mv | Y | Y | Y | Y | Y |
| rm | Y | Y | Y | Y | Y |
| mkdir | Y | Y | Y | Y | Y |
| chmod | Y | Y | Y | Y | Y |
| ln | Y | Y | Y | Y | Y |
| readlink | Y | Y | Y | Y | Y |
| mktemp | Y | Y | Y | Y | Y |
| head | Y | Y | Y | Y | Y |
| tail | Y | Y | Y | Y | Y |
| sed | Y | Y | Y | Y | Y |
| awk | Y | Y | Y | Y | Y |
| grep | Y | Y | Y | Y | Y |
| find | Y | Y | Y | Y | Y |
| sort | Y | Y | Y | Y | Y |
| cut | Y | Y | Y | Y | Y |
| tr | Y | Y | Y | Y | Y |

**Finding**: All tested coreutils are universally present. Alpine uses BusyBox implementations but they are compatible.

### Environment

| Aspect | Universal? |
|--------|------------|
| $HOME defined | Yes |
| $PATH defined | Yes |
| /tmp writable | Yes |

## The Truly Universal Baseline

Based on empirical testing, tsuku can assume ONLY:

### Always Present
- `/bin/sh` (POSIX shell)
- Coreutils: `cat`, `cp`, `mv`, `rm`, `mkdir`, `chmod`, `ln`, `readlink`, `mktemp`, `head`, `tail`, `sed`, `awk`, `grep`, `find`, `sort`, `cut`, `tr`
- `tar` and `gzip` for archive operations
- `$HOME` and `$PATH` environment variables
- Writable `/tmp` directory

### NOT Universal
- `bash` (missing in Alpine)
- `curl` or `wget` (NEITHER in Debian/Ubuntu base)
- CA certificates (missing in Debian/Ubuntu base)
- `unzip`, `xz`, `bzip2`, `zstd`
- `which` command (missing in Fedora/Arch)

## Implications for tsuku

1. **Bootstrap Problem**: Tsuku needs a download tool to fetch binaries, but cannot assume one exists.

2. **HTTPS Problem**: Even if tsuku bundles a static curl, Debian/Ubuntu lack CA certificates.

3. **Detection Method**: Use `type cmd >/dev/null 2>&1` instead of `which cmd` for portable detection.

4. **Script Shebang**: Use `#!/bin/sh` not `#!/bin/bash` for portability.

5. **Archive Format**: Prefer `.tar.gz` over `.tar.xz` or `.zip` for maximum compatibility.

## Recommendations

1. **Require Prerequisites**: Document that tsuku requires curl/wget and CA certificates.

2. **Or Bundle Static Binary**: Ship tsuku as a static binary that includes TLS support (like a Go binary with embedded CA roots).

3. **Detect and Warn**: On first run, detect missing dependencies and provide package-manager-specific install commands.

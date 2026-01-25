# Alpine Linux APK Infrastructure Research

**Date:** 2026-01-24
**Purpose:** Evaluate Alpine Linux package infrastructure for tsuku's musl compatibility strategy
**Context:** Issue #1092 - Platform Compatibility Verification

---

## Executive Summary

Alpine Linux provides a well-structured package infrastructure via `dl-cdn.alpinelinux.org`. The key findings:

1. **CDN URLs are predictable**: `https://dl-cdn.alpinelinux.org/alpine/{version}/{repo}/{arch}/{package}-{version}.apk`
2. **APKINDEX provides metadata**: A line-oriented text format with package info, dependencies, and checksums
3. **Checksums use SHA1+base64**: The `C:` field contains a SHA1 hash of the control segment, base64-encoded with `Q1` prefix
4. **Version discovery requires APKINDEX parsing**: No REST API exists; parse APKINDEX.tar.gz to discover available versions
5. **Mirrors are reliable**: Tiered mirror system with health monitoring; dl-cdn.alpinelinux.org is the primary CDN

For tsuku's purposes, Alpine APK extraction is a viable hermetic alternative to system packages, though more complex than Homebrew's GHCR approach.

---

## 1. CDN URL Structure

### Base URL Pattern

```
https://dl-cdn.alpinelinux.org/alpine/{version}/{repository}/{architecture}/
```

### Components

| Component | Values | Examples |
|-----------|--------|----------|
| `version` | `v3.19`, `v3.20`, `v3.21`, `edge`, `latest-stable` | `v3.20` |
| `repository` | `main`, `community`, `testing` | `main` |
| `architecture` | `x86_64`, `aarch64`, `armhf`, `armv7`, `x86`, `ppc64le`, `s390x` | `x86_64` |

### Package Download URLs

Package filenames follow the pattern: `{name}-{version}.apk`

Where version includes the revision: `1.3.1-r0` (version 1.3.1, revision 0)

**Example URLs:**

```
# zlib for Alpine 3.20 on x86_64
https://dl-cdn.alpinelinux.org/alpine/v3.20/main/x86_64/zlib-1.3.1-r0.apk

# openssl-dev for Alpine edge on aarch64
https://dl-cdn.alpinelinux.org/alpine/edge/main/aarch64/openssl-dev-3.3.2-r0.apk

# libyaml for Alpine 3.19 on x86_64
https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/yaml-0.2.5-r2.apk
```

### APKINDEX Location

Each repository/architecture combination has an index file:

```
https://dl-cdn.alpinelinux.org/alpine/{version}/{repo}/{arch}/APKINDEX.tar.gz
```

**Example:**
```
https://dl-cdn.alpinelinux.org/alpine/v3.20/main/x86_64/APKINDEX.tar.gz
```

---

## 2. APKINDEX Format Specification

### File Structure

The `APKINDEX.tar.gz` is a gzip-compressed tar archive containing:

1. `.SIGN.RSA.<keyname>.rsa.pub` - Repository signature
2. `DESCRIPTION` - Optional description text
3. `APKINDEX` - The actual package index

### Record Format

The APKINDEX file uses a line-oriented text format:
- Each line has format: `{field}:{value}`
- Records are separated by blank lines
- Fields use single-letter prefixes

### Field Definitions

| Field | Name | Description | Example |
|-------|------|-------------|---------|
| `P:` | Package name | The package identifier | `P:zlib` |
| `V:` | Version | Full version with revision | `V:1.3.1-r0` |
| `A:` | Architecture | Target arch (optional) | `A:x86_64` |
| `S:` | Size | Compressed package size in bytes | `S:53024` |
| `I:` | Installed size | Uncompressed size in bytes | `I:110592` |
| `T:` | Description | Package description | `T:A compression/decompression library` |
| `U:` | URL | Project homepage | `U:https://zlib.net/` |
| `L:` | License | SPDX license identifier | `L:Zlib` |
| `o:` | Origin | Source package name | `o:zlib` |
| `m:` | Maintainer | Package maintainer | `m:Natanael Copa <ncopa@alpinelinux.org>` |
| `t:` | Timestamp | Build timestamp (Unix epoch) | `t:1700000000` |
| `c:` | Commit | Git commit hash | `c:abc123def456...` |
| `k:` | Priority | Provider priority (for alternatives) | `k:100` |
| `D:` | Dependencies | Space-separated dependency list | `D:so:libc.musl-x86_64.so.1` |
| `p:` | Provides | What this package provides | `p:zlib=1.3.1-r0 so:libz.so.1=1.3.1` |
| `i:` | Install-if | Conditional install triggers | `i:docs zlib` |
| `C:` | Checksum | SHA1 of control segment (base64, Q1 prefix) | `C:Q1P4IRU/u5yB4CSnUEBRD1WWwajrY=` |

### Example Record

```
C:Q1P4IRU/u5yB4CSnUEBRD1WWwajrY=
P:zlib
V:1.3.1-r0
A:x86_64
S:53024
I:110592
T:A compression/decompression library
U:https://zlib.net/
L:Zlib
o:zlib
m:Natanael Copa <ncopa@alpinelinux.org>
t:1700000000
c:f34d70b4e86dd0e938070f67db46aaa4cf68db11
D:so:libc.musl-x86_64.so.1
p:zlib=1.3.1-r0 so:libz.so.1=1.3.1

```

### Dependency Notation

Dependencies in the `D:` field use special prefixes:
- `so:libc.musl-x86_64.so.1` - Shared object dependency
- `cmd:wget` - Command dependency
- `pc:libssl` - pkg-config dependency
- Plain names are package dependencies

### Parsing APKINDEX

```bash
# Download and extract
wget https://dl-cdn.alpinelinux.org/alpine/v3.20/main/x86_64/APKINDEX.tar.gz
tar -xzf APKINDEX.tar.gz

# Search for a package
grep -A 15 '^P:zlib$' APKINDEX

# Get all versions of a package
grep '^P:zlib' APKINDEX | head -20
grep '^V:' APKINDEX | head -20
```

---

## 3. Versioning Scheme

### Release Branches

Alpine releases new stable branches in May and November each year.

| Branch | Type | Support Period | Example URL Path |
|--------|------|----------------|------------------|
| `v3.23` | Current stable | 2 years | `/alpine/v3.23/` |
| `v3.22` | Previous stable | ~1 year remaining | `/alpine/v3.22/` |
| `v3.21` | Older stable | Until EOL | `/alpine/v3.21/` |
| `edge` | Rolling development | None (unstable) | `/alpine/edge/` |
| `latest-stable` | Symlink to current | N/A | `/alpine/latest-stable/` |

### Repository Types

| Repository | Availability | Support | Purpose |
|------------|--------------|---------|---------|
| `main` | All branches | Full (2 years) | Core system packages |
| `community` | All branches | Until next release | Community-maintained packages |
| `testing` | Edge only | None | Staging for new packages |

### Package Version Format

Package versions follow the pattern: `{upstream_version}-r{revision}`

- **Upstream version**: The original software version (e.g., `1.3.1`)
- **Revision**: Alpine-specific patch level (e.g., `r0`, `r1`, `r2`)

Examples:
- `zlib-1.3.1-r0` - zlib 1.3.1, first Alpine build
- `openssl-3.1.4-r5` - openssl 3.1.4, fifth Alpine revision

### Version Availability

Alpine only keeps the latest version of each package per stable branch. Historical versions are not retained on mirrors (unlike some other distros). This means:

- For hermetic builds, tsuku would need to download and cache specific versions
- Version pinning requires tsuku to maintain its own cache
- Edge packages change frequently and are not suitable for version pinning

---

## 4. Checksum Verification

### Checksum Types

| Context | Algorithm | Format | Example |
|---------|-----------|--------|---------|
| APKINDEX `C:` field | SHA1 | Base64 with Q1 prefix | `Q1P4IRU/u5yB4CSnUEBRD1WWwajrY=` |
| Package signatures | SHA1 (PKCS1v15 RSA) | DER-encoded | `.SIGN.RSA.*.rsa.pub` |
| File checksums (PAX header) | SHA1 | Hex | `APK-TOOLS.checksum.SHA1` |
| APKBUILD sources | SHA512 | Hex | `sha512sums` in APKBUILD |

### Understanding the C: Field

The `C:` checksum in APKINDEX is NOT a checksum of the entire .apk file. It is:

1. The SHA1 hash of the **control segment** (second gzip stream in the .apk)
2. Binary digest is base64-encoded
3. Prefixed with `Q1` to distinguish from older MD5 format

**Cannot be computed with standard tools** - requires `apk index` command or parsing the APK's three gzip streams.

### APK File Structure

APK v2 packages consist of three concatenated gzip streams:

```
[signature.tar.gz][control.tar.gz][data.tar.gz]
     Stream 1         Stream 2       Stream 3
```

1. **Signature stream**: Contains `.SIGN.RSA.<keyname>.rsa.pub`
2. **Control stream**: Contains `.PKGINFO` metadata and install scripts
3. **Data stream**: Contains the actual package files

The `C:` checksum is SHA1(stream2), base64-encoded with Q1 prefix.

### Verification Approaches for tsuku

**Option A: Trust APKINDEX checksum (simpler)**
1. Download APKINDEX.tar.gz (signed by Alpine)
2. Verify APKINDEX signature using Alpine's public keys
3. Extract C: checksum for target package
4. Download package and verify control segment hash matches

**Option B: SHA256 of entire file (like Homebrew)**
1. Download package
2. Compute SHA256 of entire .apk file
3. Compare against tsuku-managed checksum database

Option B is simpler to implement and aligns with Homebrew's approach, but requires tsuku to maintain its own checksum database rather than trusting APKINDEX.

### SHA256 Support in APK

There is ongoing work to add SHA256 support to APK:
- [Use SHA-256 for file checksums](https://gitlab.alpinelinux.org/alpine/apk-tools/-/merge_requests/14)
- [Package signatures should not use SHA-1](https://gitlab.alpinelinux.org/alpine/apk-tools/-/issues/10670)

For now, SHA1+base64 with Q1 prefix remains the standard.

---

## 5. Mirror Infrastructure

### Tier System

Alpine uses a multi-tier mirror system:

```
dl-master.alpinelinux.org (Tier 0 - source)
            |
    +-------+-------+
    |       |       |
  Tier 1  Tier 1  Tier 1  (3 geographically distributed)
    |       |       |
    +-------+-------+
            |
    dl-cdn.alpinelinux.org (CDN frontend)
            |
    +-------+-------+-------+
    |       |       |       |
  Tier 2  Tier 2  Tier 2  ...  (Community mirrors)
```

### CDN Reliability

**dl-cdn.alpinelinux.org** is the primary CDN backed by Tier 1 mirrors:
- Generally reliable
- Some documented outages (see [GitHub issue](https://github.com/gliderlabs/docker-alpine/issues/527))
- Health monitoring at [mirrors.alpinelinux.org](https://mirrors.alpinelinux.org/)

**Recommendations for tsuku:**
- Use dl-cdn.alpinelinux.org as primary
- Consider fallback to alternative mirrors if cdn fails
- Cache downloaded packages locally

### Alternative Mirrors

If dl-cdn is unavailable, alternative mirrors include:
- `https://mirror.leaseweb.com/alpine/`
- `https://alpine.global.ssl.fastly.net/`
- Full list at [mirrors.alpinelinux.org](https://mirrors.alpinelinux.org/)

### Sync Frequency

- Tier 1 mirrors sync with dl-master continuously
- Mirror health is checked via HTTP Last-Modified header
- Status displayed as OK if index is within 1 hour of master

---

## 6. Version Discovery Mechanism

### No REST API

Unlike GHCR (which has a REST API for manifest queries), Alpine does not provide a REST API for package discovery. Options:

### Method 1: Parse APKINDEX (Recommended)

```go
// Pseudocode for version discovery
func GetAvailableVersions(packageName, branch, arch string) ([]string, error) {
    // 1. Download APKINDEX
    url := fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/%s/main/%s/APKINDEX.tar.gz", branch, arch)

    // 2. Extract APKINDEX from tar.gz
    index := extractAPKINDEX(url)

    // 3. Parse records and filter by package name
    versions := []string{}
    for _, record := range parseRecords(index) {
        if record.Name == packageName {
            versions = append(versions, record.Version)
        }
    }

    return versions, nil
}
```

### Method 2: Web Scraping pkgs.alpinelinux.org

The package browser at `https://pkgs.alpinelinux.org/packages` supports URL parameters:

```
https://pkgs.alpinelinux.org/packages?name=zlib&branch=v3.20&repo=main&arch=x86_64
```

However, this returns HTML and is not designed for programmatic access.

### Method 3: apk CLI (Requires Alpine System)

```bash
# List all available versions
apk policy zlib

# Search for packages
apk search zlib
```

Not suitable for tsuku since it requires an Alpine system.

### Recommended Approach for tsuku

Use Method 1 (APKINDEX parsing):

1. Cache APKINDEX.tar.gz locally (refresh periodically)
2. Parse on demand to find available versions
3. Use Go library like [go-apkutils](https://github.com/martencassel/go-apkutils) or implement parser

Example using go-apkutils:
```go
import "github.com/martencassel/go-apkutils/index"

f, _ := os.Open("APKINDEX")
idx, _ := index.ReadApkIndex(f)

for _, entry := range idx.Entries {
    if entry.Name == "zlib" {
        fmt.Printf("Version: %s, Checksum: %s\n", entry.Version, entry.Checksum)
    }
}
```

---

## 7. Comparison to GHCR/Homebrew Infrastructure

| Aspect | Alpine APK | Homebrew GHCR |
|--------|------------|---------------|
| **Index format** | Text file (APKINDEX) | JSON manifests |
| **API access** | None (parse APKINDEX) | REST API |
| **Authentication** | None required | Anonymous token |
| **Checksum algorithm** | SHA1 (control segment) | SHA256 (full blob) |
| **Checksum format** | Base64 with Q1 prefix | Hex string |
| **Package format** | .apk (3 gzip streams) | .tar.gz (standard) |
| **Version discovery** | Parse APKINDEX | Query manifest API |
| **CDN** | dl-cdn.alpinelinux.org | ghcr.io |
| **Mirrors** | Multiple community mirrors | Single source |
| **Signing** | RSA signatures | OCI signatures |

### Implementation Complexity

| Task | Alpine | Homebrew |
|------|--------|----------|
| Get package list | Parse APKINDEX (medium) | Query manifest (low) |
| Get checksum | Extract from APKINDEX (medium) | Extract from manifest (low) |
| Download package | HTTP GET (low) | HTTP GET with auth header (low) |
| Verify package | Custom SHA1 control segment (high) or SHA256 full file (low) | SHA256 full file (low) |
| Extract package | 3-stream tar.gz (medium) | Standard tar.gz (low) |

### Recommendation for tsuku

If implementing Alpine APK extraction for hermetic musl support:

1. **Use SHA256 of entire .apk file** rather than parsing the Q1 checksum
   - Simpler implementation
   - Consistent with Homebrew approach
   - tsuku maintains its own checksum database

2. **Cache APKINDEX locally** for version discovery
   - Refresh on `tsuku update-registry`
   - Parse with go-apkutils or custom parser

3. **Extract APK as simple tar.gz**
   - Skip signature stream (tsuku verifies checksum, not signature)
   - Use standard tar.gz extraction

---

## 8. Implementation Notes for tsuku

### Constructing Download URLs

```go
func GetAPKDownloadURL(branch, repo, arch, name, version string) string {
    return fmt.Sprintf(
        "https://dl-cdn.alpinelinux.org/alpine/%s/%s/%s/%s-%s.apk",
        branch,  // "v3.20" or "edge"
        repo,    // "main" or "community"
        arch,    // "x86_64" or "aarch64"
        name,    // "zlib"
        version, // "1.3.1-r0"
    )
}
```

### Parsing APKINDEX

```go
func ParseAPKINDEX(r io.Reader) ([]PackageRecord, error) {
    scanner := bufio.NewScanner(r)
    var records []PackageRecord
    var current PackageRecord

    for scanner.Scan() {
        line := scanner.Text()
        if line == "" {
            if current.Name != "" {
                records = append(records, current)
                current = PackageRecord{}
            }
            continue
        }

        if len(line) < 2 || line[1] != ':' {
            continue
        }

        field := line[0]
        value := line[2:]

        switch field {
        case 'P':
            current.Name = value
        case 'V':
            current.Version = value
        case 'C':
            current.Checksum = value
        case 'S':
            current.Size, _ = strconv.ParseInt(value, 10, 64)
        // ... other fields
        }
    }

    return records, scanner.Err()
}
```

### Package Name Mapping

Some package names differ from the library names tsuku uses:

| tsuku name | Alpine package name |
|------------|---------------------|
| libyaml | yaml |
| zlib | zlib |
| openssl | openssl-dev (for headers) or openssl (runtime) |
| gcc-libs | libstdc++ |

---

## Sources

- [Alpine Package Keeper Wiki](https://wiki.alpinelinux.org/wiki/Alpine_Package_Keeper)
- [APK Spec](https://wiki.alpinelinux.org/wiki/Apk_spec)
- [Alpine Package Format](https://wiki.alpinelinux.org/wiki/Alpine_package_format)
- [Repositories](https://wiki.alpinelinux.org/wiki/Repositories)
- [Alpine Release Branches](https://www.alpinelinux.org/releases/)
- [Mirror Health](https://mirrors.alpinelinux.org/)
- [go-apkutils](https://github.com/martencassel/go-apkutils)
- [Package signatures should not use SHA-1](https://gitlab.alpinelinux.org/alpine/apk-tools/-/issues/10670)
- [Use SHA-256 for file checksums](https://gitlab.alpinelinux.org/alpine/apk-tools/-/merge_requests/14)
- [Alpine CDN Issues](https://github.com/gliderlabs/docker-alpine/issues/527)
- [How to List All Available Package Versions](https://linuxvox.com/blog/alpine-apk-list-all-available-package-versions/)

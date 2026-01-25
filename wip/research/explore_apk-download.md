# APK Download and Extraction Analysis

**Date:** 2026-01-24
**Context:** Research for potential `alpine_apk` action to support hermetic library dependencies on musl systems

---

## Executive Summary

This document analyzes how to implement APK support in tsuku, comparing Alpine's package format with the existing Homebrew GHCR implementation. APK provides a simpler, more direct path: packages are available via plain HTTP GET with checksums in a downloadable index file, no authentication required. The extraction mechanism is slightly more complex due to APK's multi-segment gzip format, but this is well-documented and can be handled with existing Go libraries.

**Estimated Complexity: Medium**

---

## 1. Side-by-Side Comparison: GHCR vs Alpine CDN

| Aspect | Homebrew (GHCR) | Alpine (CDN) |
|--------|-----------------|--------------|
| **URL Construction** | Multi-step: token -> manifest -> blob SHA -> blob URL | Single step: base URL + package filename |
| **Authentication** | Required (anonymous Bearer token) | None required |
| **Checksum Source** | Manifest annotations (`sh.brew.bottle.digest`) | APKINDEX.tar.gz (`C:` field) |
| **Checksum Algorithm** | SHA256 (hex) | SHA1 (base64 with Q1 prefix) |
| **Package Format** | tar.gz | Multi-segment gzip (signature + control + data) |
| **Extraction** | Standard tar.gz with strip_dirs=2 | Extract third gzip segment only |
| **Platform Selection** | Manifest tags (x86_64_linux, arm64_linux) | Directory structure (/x86_64/, /aarch64/) |
| **Version Resolution** | Manifest by version tag | APKINDEX lookup by package name |

---

## 2. URL Construction

### GHCR (Homebrew) - Complex Multi-Step Process

```go
// Step 1: Get anonymous token
tokenURL := "https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/{formula}:pull"
// Response: {"token": "..."}

// Step 2: Query manifest for blob SHA
manifestURL := "https://ghcr.io/v2/homebrew/core/{formula}/manifests/{version}"
// Headers: Authorization: Bearer {token}, Accept: application/vnd.oci.image.index.v1+json
// Response contains platform-specific entries with sh.brew.bottle.digest annotation

// Step 3: Download blob using SHA
blobURL := "https://ghcr.io/v2/homebrew/core/{formula}/blobs/sha256:{blobSHA}"
// Headers: Authorization: Bearer {token}
```

### Alpine CDN - Simple Direct URL

```go
// Single step: construct URL from known components
packageURL := fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/v%s/%s/%s/%s-%s.apk",
    alpineVersion,  // e.g., "3.19"
    repo,           // "main" or "community"
    arch,           // "x86_64" or "aarch64"
    packageName,    // e.g., "zlib"
    packageVersion, // e.g., "1.3.1-r0"
)
// Example: https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/zlib-1.3.1-r0.apk
// No authentication required
```

---

## 3. Authentication

### GHCR (Homebrew)

GHCR requires an anonymous Bearer token even for public images:

```go
func (a *HomebrewAction) getGHCRToken(formula string) (string, error) {
    url := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/%s:pull", formula)
    resp, err := ghcrHTTPClient().Get(url)
    // Parse JSON response for token
    var tokenResp ghcrTokenResponse
    json.NewDecoder(resp.Body).Decode(&tokenResp)
    return tokenResp.Token, nil
}
```

This adds complexity and an extra network round-trip.

### Alpine CDN

**No authentication required.** Alpine packages are served from a public CDN with standard HTTP GET:

```bash
curl -sI "https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/zlib-1.3.1-r0.apk"
# HTTP/2 200
# content-type: application/octet-stream
# No auth headers needed
```

**Conclusion:** Alpine is simpler. We can use tsuku's existing `newDownloadHTTPClient()` with no modifications.

---

## 4. Checksum Verification

### GHCR (Homebrew)

Checksums are SHA256 (hex-encoded) and come from manifest annotations:

```go
// In getBlobSHA():
if digest, ok := entry.Annotations["sh.brew.bottle.digest"]; ok {
    if strings.HasPrefix(digest, "sha256:") {
        return strings.TrimPrefix(digest, "sha256:"), nil
    }
}
```

### Alpine CDN

Checksums are **SHA1** (base64-encoded with Q1 prefix) stored in APKINDEX.tar.gz:

```
C:Q11mV5+Yq5WHkhWIJP27rBrg7izKg=
P:zlib
V:1.3.1-r0
A:x86_64
S:53932
...
```

**Important details:**
- `C:` field contains the checksum
- `Q1` prefix indicates SHA1 algorithm with base64 encoding
- The checksum is of the **control segment** (second gzip stream), NOT the whole file
- This cannot be computed with standard tools - requires understanding APK format

**Verification approach options:**

1. **Option A: Trust package signature** - APK files are signed. Verify RSA signature instead of checksum.

2. **Option B: Compute control segment hash** - Parse APK to extract second gzip stream and hash it.

3. **Option C: Whole-file hash at download time** - Download file, compute SHA256, store in tsuku's plan (like Homebrew does).

**Recommendation:** Option C is simplest and consistent with existing patterns. Download the APK once during `Decompose()`, compute SHA256, and store it in the decomposed `download_file` step. This matches how Homebrew works.

---

## 5. Extraction

### GHCR (Homebrew)

Standard tar.gz extraction with `strip_dirs=2`:

```go
extractAction := &ExtractAction{}
extractParams := map[string]interface{}{
    "archive":    filepath.Base(bottlePath),
    "format":     "tar.gz",
    "strip_dirs": 2, // Homebrew bottles have formula/version/ prefix
}
```

### Alpine APK

APK files are **three concatenated gzip streams**:

1. **Signature segment** - RSA signature in `.SIGN.RSA.*` file
2. **Control segment** - `.PKGINFO` and install scripts
3. **Data segment** - Actual files to install

```bash
# APK contents example (zlib-1.3.1-r0.apk):
.SIGN.RSA.alpine-devel@lists.alpinelinux.org-6165ee59.rsa.pub
.PKGINFO
lib/
lib/libz.so.1
lib/libz.so.1.3.1
```

**Extraction challenge:** Go's `compress/gzip` and `archive/tar` can read concatenated gzip streams naturally, but we need to:

1. Skip the first two segments (signature and control)
2. Extract only the data segment
3. Strip the leading directory structure (files are at root like `lib/`)

**Implementation approach:**

```go
func extractAPK(apkPath, destPath string) error {
    file, err := os.Open(apkPath)
    if err != nil {
        return err
    }
    defer file.Close()

    // Go's gzip.NewReader reads concatenated streams
    gzr, err := gzip.NewReader(file)
    if err != nil {
        return err
    }
    defer gzr.Close()

    tr := tar.NewReader(gzr)

    for {
        header, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }

        // Skip signature and control files (start with .)
        if strings.HasPrefix(header.Name, ".") {
            continue
        }

        // Extract data files
        target := filepath.Join(destPath, header.Name)
        // ... standard extraction logic (already in ExtractAction)
    }

    return nil
}
```

**Alternative:** Use existing `ExtractAction` with a new format type `apk`:

```go
case "apk":
    return a.extractAPK(archivePath, destPath, stripDirs, files)
```

---

## 6. Platform Selection

### GHCR (Homebrew)

Platform is encoded in manifest entry's `org.opencontainers.image.ref.name` annotation:

```go
platformTag, err := a.getPlatformTag(ctx.OS, ctx.Arch)
// Returns: "x86_64_linux", "arm64_linux", "arm64_sonoma", "sonoma"

expectedRefName := fmt.Sprintf("%s.%s", version, platformTag)
// Example: "0.2.5.x86_64_linux"
```

### Alpine CDN

Platform is a directory in the URL:

```
https://dl-cdn.alpinelinux.org/alpine/v3.19/main/{arch}/{package}.apk
```

Architecture mapping:

| Go GOARCH | Alpine Arch |
|-----------|-------------|
| amd64 | x86_64 |
| arm64 | aarch64 |
| 386 | x86 |
| arm | armv7 |

```go
func alpineArch(goarch string) string {
    switch goarch {
    case "amd64":
        return "x86_64"
    case "arm64":
        return "aarch64"
    default:
        return goarch
    }
}
```

---

## 7. APKINDEX Parsing

To resolve package versions and checksums, we need to parse APKINDEX.tar.gz:

```go
// APKINDEX record format (separated by blank lines):
// C:Q11mV5+Yq5WHkhWIJP27rBrg7izKg=  <- Checksum
// P:zlib                              <- Package name
// V:1.3.1-r0                          <- Version
// A:x86_64                            <- Architecture
// S:53932                             <- Package size
// I:110592                            <- Installed size
// T:A compression/decompression Library <- Description
// D:so:libc.musl-x86_64.so.1          <- Dependencies
// p:so:libz.so.1=1.3.1                <- Provides

type APKIndexEntry struct {
    Checksum     string   // C: field (Q1 + base64 SHA1)
    PackageName  string   // P: field
    Version      string   // V: field
    Architecture string   // A: field
    Size         int64    // S: field
    InstalledSize int64   // I: field
    Description  string   // T: field
    Dependencies []string // D: field (space-separated)
    Provides     []string // p: field (space-separated)
}

func parseAPKIndex(indexURL string) (map[string]APKIndexEntry, error) {
    // 1. Download APKINDEX.tar.gz
    // 2. Extract APKINDEX file from tar.gz
    // 3. Parse line-by-line, building entries
    // 4. Return map of package name -> entry
}
```

---

## 8. Code Reuse Opportunities

### Can Reuse Directly

| Component | Location | Purpose |
|-----------|----------|---------|
| HTTP client | `newDownloadHTTPClient()` | Secure downloads with timeouts |
| Checksum verification | `VerifyChecksum()` in util.go | SHA256 verification |
| Tar extraction | `extractTarReader()` in extract.go | Core tar extraction logic |
| Path validation | `isPathWithinDirectory()` | Security: prevent traversal |
| Symlink validation | `validateSymlinkTarget()` | Security: prevent symlink attacks |
| Platform detection | `DetectFamily()` in platform/family.go | Detect Alpine |
| Architecture mapping | `MapArch()` in util.go | Convert GOARCH |

### Can Adapt

| Component | Source | Adaptation Needed |
|-----------|--------|-------------------|
| `Decompose()` pattern | homebrew.go | Same pattern: download, compute checksum, return steps |
| Download caching | download.go | Same interface, different URL patterns |
| Progress display | progress package | Works unchanged |

### Need New Code

| Component | Purpose | Complexity |
|-----------|---------|------------|
| APKINDEX parser | Parse package metadata | Low |
| APK extractor | Handle multi-segment gzip | Medium |
| Alpine arch mapper | Convert GOARCH to Alpine names | Low |
| Version resolver | Find latest version from APKINDEX | Low |

---

## 9. New Code Requirements

### 9.1 AlpineAPKAction (internal/actions/alpine_apk.go)

```go
type AlpineAPKAction struct{ BaseAction }

func (a *AlpineAPKAction) Name() string { return "alpine_apk" }

func (a *AlpineAPKAction) Preflight(params map[string]interface{}) *PreflightResult {
    // Validate: package (required), version (optional)
}

func (a *AlpineAPKAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // 1. Parse APKINDEX to get package info
    // 2. Download APK
    // 3. Verify checksum (computed during Decompose)
    // 4. Extract data segment to install dir
}

func (a *AlpineAPKAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
    // 1. Download APKINDEX
    // 2. Look up package version and filename
    // 3. Download APK to temp
    // 4. Compute SHA256
    // 5. Return: download_file + extract steps
}
```

### 9.2 APKINDEX Parser (internal/actions/apkindex.go)

```go
type APKIndexEntry struct {
    Checksum     string
    PackageName  string
    Version      string
    Architecture string
    Size         int64
}

func ParseAPKIndex(indexPath string) (map[string]APKIndexEntry, error)
func FetchAPKIndex(ctx context.Context, alpineVersion, repo, arch string) (map[string]APKIndexEntry, error)
```

### 9.3 APK Extraction (internal/actions/extract.go addition)

```go
// Add new format case to Execute():
case "apk":
    return a.extractAPK(archivePath, destPath, stripDirs, files)

func (a *ExtractAction) extractAPK(archivePath, destPath string, stripDirs int, files []string) error {
    // Handle multi-segment gzip, skip . prefixed files
}
```

---

## 10. Estimated Complexity

| Component | Lines of Code | Effort |
|-----------|---------------|--------|
| AlpineAPKAction | ~200 | Medium |
| APKINDEX parser | ~100 | Low |
| APK extraction | ~50 | Low |
| Tests | ~150 | Medium |
| **Total** | **~500** | **Medium** |

**Comparison to Homebrew implementation:** Homebrew action is ~510 lines including tests. Alpine should be similar or slightly less due to simpler authentication.

---

## 11. Security Considerations

### Package Integrity

- **Signature verification**: APK packages are signed with RSA. Consider verifying signatures for defense in depth.
- **SHA256 computation**: We compute SHA256 at Decompose time and verify at Execute time - same as Homebrew.
- **HTTPS only**: Alpine CDN supports HTTPS; enforce it like we do for other downloads.

### Index Integrity

- **APKINDEX.tar.gz is signed**: The index itself contains a signature file. Consider verifying.
- **Caching**: Cache APKINDEX locally to avoid repeated downloads. Include TTL to refresh periodically.

### Path Traversal

- Reuse existing `isPathWithinDirectory()` and `validateSymlinkTarget()` functions.
- APK control files (`.PKGINFO`, `.SIGN.*`) should be skipped during extraction.

---

## 12. Example Recipe Usage

```toml
name = "zlib"
description = "A compression/decompression Library"
version = "1.3.1-r0"

[metadata]
category = "libraries"
linux_families = ["alpine"]

[[steps]]
action = "alpine_apk"
package = "zlib"
alpine_version = "3.19"
repo = "main"
```

Or with version from provider:

```toml
[[steps]]
action = "alpine_apk"
package = "zlib"
alpine_version = "3.19"
repo = "main"
# version comes from recipe's version field
```

---

## Sources

- [Apk spec - Alpine Linux Wiki](https://wiki.alpinelinux.org/wiki/Apk_spec) - Official APK format specification
- [APK, the strangest format - hydrogen18.com](https://www.hydrogen18.com/blog/apk-the-strangest-format.html) - Detailed analysis of APK structure
- [Alpine Package Keeper - Alpine Linux Wiki](https://wiki.alpinelinux.org/wiki/Alpine_Package_Keeper) - APK tool documentation
- [Use SHA-256 for file checksums (MR !14)](https://gitlab.alpinelinux.org/alpine/apk-tools/-/merge_requests/14) - SHA-256 migration in apk-tools
- [One-liner to get APK checksum](https://gist.github.com/jdolitsky/3f4351d1e85ce073b0f0ad3b76e850a4) - Checksum extraction method
- Direct testing against https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/ - CDN structure verification

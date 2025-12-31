# Findings: Bootstrap Requirements

## Summary

Tsuku, as a Go binary, needs CA certificates to perform HTTPS requests. This is the **only** hard requirement, but Debian and Ubuntu base images ship without CA certificates, creating a bootstrap problem.

## The Bootstrap Problem

Tsuku needs to download things. To download via HTTPS, it needs CA certificates. But on Debian/Ubuntu, there are no CA certificates by default. To get CA certificates, tsuku would need to use the package manager. But tsuku can't use the package manager without first downloading things...

**Bootstrap Paradox**: Tsuku needs network access to install, but network access (HTTPS) requires certificates that aren't present.

## Empirical Test Results

### Go Binary HTTPS Behavior

| Distribution | CA Certs Present | HTTPS Works | Notes |
|--------------|-----------------|-------------|-------|
| debian:bookworm-slim | NO | FAIL | x509: certificate signed by unknown authority |
| ubuntu:24.04 | NO | FAIL | Same error |
| fedora:41 | YES | SUCCESS | Works out of the box |
| alpine:3.19 | YES | SUCCESS | Works (with static binary) |
| archlinux:base | YES | SUCCESS | Works out of the box |

### Key Finding: Go Uses System CA Store

Go's `crypto/tls` package looks for CA certificates in these locations:
- `/etc/ssl/certs/ca-certificates.crt` (Debian/Ubuntu after install)
- `/etc/pki/tls/certs/ca-bundle.crt` (RHEL/Fedora)
- `/etc/ssl/ca-bundle.pem` (SUSE)
- `/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem` (RHEL/Fedora)
- `/etc/ssl/cert.pem` (Various)

Also respects:
- `SSL_CERT_FILE` environment variable (single file)
- `SSL_CERT_DIR` environment variable (directory)

## Minimum Requirements for tsuku

### Hard Requirements

1. **CA Certificates**: Must exist somewhere on the system
   - Location: Standard paths or via `SSL_CERT_FILE`
   - Without this: HTTPS fails completely

2. **Writable `$HOME`**: For `$TSUKU_HOME` installation directory
   - Without this: Cannot install anything

3. **`tar` and `gzip`**: For extracting downloaded archives
   - Present universally (even on Debian slim)

### Soft Requirements (Nice to Have)

4. **`curl` or `wget`**: Not needed if tsuku handles all downloads
   - Tsuku is written in Go and handles HTTP itself

5. **`unzip`, `xz`, `bzip2`**: Only if recipes use those formats
   - Can be installed on demand

## Resolution Strategies

### Strategy A: Require Prerequisites (Recommended)

**Document** that CA certificates must be present. Provide one-liner bootstrap commands:

```bash
# Debian/Ubuntu
sudo apt-get update && sudo apt-get install -y ca-certificates

# Already present on: Fedora, Alpine, Arch, SUSE
```

**Pros**:
- Simple, no magic
- User understands system state
- Standard practice for CLI tools

**Cons**:
- Requires internet access via package manager
- Extra step for Debian/Ubuntu users

### Strategy B: Bundle CA Certificates

Embed Mozilla's CA certificate bundle in the tsuku binary itself.

**Implementation**:
```go
//go:embed ca-certificates.crt
var embeddedCACerts []byte

func init() {
    if /* system certs not found */ {
        os.WriteFile("/tmp/tsuku-ca-certs.crt", embeddedCACerts, 0644)
        os.Setenv("SSL_CERT_FILE", "/tmp/tsuku-ca-certs.crt")
    }
}
```

**Pros**:
- Tsuku works everywhere, no bootstrap
- Single binary, no dependencies
- Like Nix/Guix approach

**Cons**:
- Binary size increase (~200KB)
- Must update bundle periodically
- May conflict with system policies

### Strategy C: Detect and Bootstrap

On first run, detect missing CA certificates and offer to install them:

```
tsuku: CA certificates not found. Install required?

For Debian/Ubuntu:
  sudo apt-get update && sudo apt-get install -y ca-certificates

Then run tsuku again.
```

**Pros**:
- Clear guidance
- Doesn't require root during tsuku install
- Educates user

**Cons**:
- Poor UX (can't just run `tsuku install foo`)
- Requires two-step installation

### Strategy D: HTTP Fallback with Hash Verification

Fall back to HTTP (not HTTPS) when CA certificates are missing, but require hash verification:

```go
if /* HTTPS fails due to cert error */ {
    warn("Falling back to HTTP with hash verification")
    // Download via HTTP, verify SHA256
}
```

**Pros**:
- Works without certificates
- Still secure (hash verification)

**Cons**:
- Security theater concerns
- Recipe server must support HTTP
- GitHub raw URLs don't work over HTTP

## Recommendation: Strategy A with Graceful Error

1. **Require CA certificates** as documented prerequisite
2. **Detect at startup** and provide helpful error message
3. **Show package-manager-specific command** to fix

```go
func checkCAcertificates() error {
    // Try a simple HTTPS request
    _, err := http.Get("https://github.com")
    if err != nil && strings.Contains(err.Error(), "x509") {
        pm := detectPackageManager()
        return fmt.Errorf(`CA certificates not found. Install with:

  %s

Then run tsuku again.`, bootstrapCommand(pm))
    }
    return nil
}

func bootstrapCommand(pm string) string {
    switch pm {
    case "apt":
        return "sudo apt-get update && sudo apt-get install -y ca-certificates"
    default:
        return "# CA certificates should already be present on your system"
    }
}
```

## Archive Extraction Capability

| Format | Required Tool | Universally Available |
|--------|--------------|----------------------|
| .tar.gz | tar, gzip | YES |
| .tar.xz | tar, xz | NO (apt needs xz-utils) |
| .tar.bz2 | tar, bzip2 | NO (apt missing) |
| .tar.zst | tar, zstd | NO |
| .zip | unzip | NO |

**Recommendation**: Prefer `.tar.gz` for maximum compatibility. Install extraction tools on-demand for other formats.

## Summary Table

| Distribution | Works Out of Box | Bootstrap Needed |
|--------------|-----------------|------------------|
| Fedora | YES | None |
| Alpine | YES | None |
| Arch | YES | None |
| openSUSE | YES | None |
| Debian | NO | `apt install ca-certificates` |
| Ubuntu | NO | `apt install ca-certificates` |

## Final Answer

**Q: What does tsuku NEED to function on a minimal system?**

1. CA certificates (only missing on Debian/Ubuntu base images)
2. tar and gzip (universally present)
3. Writable $HOME

**Q: Can it download without curl/wget?**

Yes - tsuku uses Go's `net/http` package directly.

**Q: Can it extract without tar?**

No - tar is required. But tar is universally present.

**Q: Can it work without CA certs?**

No - HTTPS fails. This is the key bootstrap requirement.

**Q: Should tsuku require, attempt to install, or bundle CA certs?**

**Require** (Strategy A). Document the prerequisite and provide clear error message with fix instructions. This is the standard approach used by most CLI tools and avoids complexity.

# Findings: No Package Manager Strategy

## Summary

Systems without standard package managers (NixOS, Gentoo, distroless) can still run tsuku if they have the baseline requirements (CA certificates, tar, gzip). The strategy should be to **not assume package manager availability** for core tsuku functionality.

## Tested Systems

### 1. NixOS/Nix Container (`nixos/nix`)

| Aspect | Status | Notes |
|--------|--------|-------|
| Package Manager | `nix`, `nix-env` | Declarative, not imperative |
| Shell | bash | Via Nix store |
| curl | present | Pre-installed |
| wget | present | Pre-installed |
| CA certificates | present | Both formats available |
| tar/gzip | present | Standard tools available |

**Analysis**: NixOS works fine for tsuku's core functionality. Has all prerequisites. Tsuku should NOT try to use `nix-env` to install packages - Nix's philosophy is declarative.

### 2. Gentoo (`gentoo/stage3`)

| Aspect | Status | Notes |
|--------|--------|-------|
| Package Manager | `emerge` | Source-based, slow |
| Shell | bash | Standard |
| curl | MISSING | Not pre-installed |
| wget | present | Pre-installed |
| CA certificates | present | 295 cert files |
| tar/gzip | present | Standard tools available |

**Analysis**: Gentoo has CA certs and wget. Tsuku's Go HTTP client will work. Emerge is source-based and slow; tsuku should not attempt to use it for dependency installation.

### 3. Distroless (`gcr.io/distroless/static-debian12`)

| Aspect | Status | Notes |
|--------|--------|-------|
| Package Manager | None | By design |
| Shell | None | No /bin/sh |
| curl/wget | None | No binaries |
| CA certificates | present | SSL_CERT_FILE set |
| tar/gzip | None | No binaries |

**Analysis**: Distroless is designed for running single static binaries. Tsuku could run here as the primary binary, but cannot install other tools (no tar, no shell). **Not a target environment** for tsuku as a package manager.

## Key Findings

### 1. CA Certificates Are Usually Present

Even on "exotic" systems, CA certificates are typically available:

| System | CA Certificates |
|--------|----------------|
| NixOS | Yes |
| Gentoo | Yes |
| Distroless | Yes |
| Alpine | Yes |
| Fedora | Yes |
| Arch | Yes |
| **Debian/Ubuntu** | **No** (unique!) |

**Debian/Ubuntu are the outliers**, not the norm.

### 2. Package Managers Aren't Always Usable

| System | PM Available | Usable for Installing Deps |
|--------|--------------|---------------------------|
| Debian/Ubuntu | Yes (apt) | Yes |
| Fedora/RHEL | Yes (dnf) | Yes |
| Alpine | Yes (apk) | Yes |
| Arch | Yes (pacman) | Yes |
| SUSE | Yes (zypper) | Yes |
| NixOS | Yes (nix) | **Not for imperative installs** |
| Gentoo | Yes (emerge) | **Too slow (source-based)** |
| Distroless | No | No |

### 3. Core Tools Availability

| Tool | Debian | Ubuntu | Fedora | Alpine | Arch | NixOS | Gentoo | Distroless |
|------|--------|--------|--------|--------|------|-------|--------|------------|
| tar | Yes | Yes | Yes | Yes | Yes | Yes | Yes | No |
| gzip | Yes | Yes | Yes | Yes | Yes | Yes | Yes | No |
| curl | No | No | Yes | No | Yes | Yes | No | No |
| wget | No | No | No | Yes | No | Yes | Yes | No |

## Strategy Recommendations

### For NixOS Users

**Do not** attempt to use `nix-env` or `nix-shell` to install dependencies. Instead:

1. Document that recipes with system dependencies may not work
2. Recommend users add dependencies to their `configuration.nix`
3. Offer pure-binary recipes that need no system deps

### For Gentoo Users

**Do not** attempt to use `emerge` (too slow). Instead:

1. Same as NixOS - document limitations
2. Focus on pre-built binary recipes

### For Distroless/Minimal Containers

**Not a target platform** for tsuku. These are for running applications, not installing tools.

If someone tries to run tsuku in distroless:
1. Detect no shell/tar and warn
2. Suggest using a normal base image

### General Strategy

```go
func CanInstallSystemDeps() (bool, string) {
    pm := detectPackageManager()

    switch pm {
    case "apt", "dnf", "microdnf", "yum", "apk", "pacman", "zypper":
        return true, pm
    case "nix", "nix-env":
        return false, "NixOS uses declarative package management. Add dependencies to configuration.nix"
    case "emerge":
        return false, "Gentoo uses source-based packages. Manual dependency installation recommended"
    default:
        return false, "No supported package manager found"
    }
}
```

## Recommended Error Messages

### No Package Manager

```
tsuku: Recipe 'rust-analyzer' requires system dependencies: openssl-dev

Your system does not have a supported package manager. Please install
these dependencies manually:

  - OpenSSL development headers

Or choose a recipe variant that doesn't require system dependencies.
```

### NixOS Detected

```
tsuku: Recipe 'rust-analyzer' requires system dependencies: openssl-dev

NixOS detected. Tsuku cannot install system packages on NixOS.
Add the following to your configuration.nix:

  environment.systemPackages = with pkgs; [
    openssl.dev
  ];

Then run: sudo nixos-rebuild switch
```

### Gentoo Detected

```
tsuku: Recipe 'rust-analyzer' requires system dependencies: openssl-dev

Gentoo detected. Installing via emerge may take significant time.
Run manually:

  sudo emerge --ask dev-libs/openssl

Then run tsuku again.
```

## Summary Table: Tsuku Support Level

| System | Support Level | Notes |
|--------|--------------|-------|
| Debian/Ubuntu | Full | Requires CA cert bootstrap |
| Fedora/RHEL/CentOS | Full | Works out of box |
| Alpine | Full | Use static binary |
| Arch | Full | Works out of box |
| SUSE | Full | Works out of box |
| NixOS | Partial | Core works, no sys dep install |
| Gentoo | Partial | Core works, no sys dep install |
| Distroless | None | Not a target platform |
| scratch | None | Not a target platform |

## Final Recommendation

1. **Support Tier 1** (Full): apt, dnf, apk, pacman, zypper
   - Detect PM, install deps automatically

2. **Support Tier 2** (Partial): NixOS, Gentoo
   - Detect and provide manual instructions
   - Focus on recipes that don't need system deps

3. **Unsupported**: Distroless, scratch
   - Detect and error gracefully
   - These aren't package manager use cases

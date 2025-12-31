# Binary Distribution Patterns

Analysis of common patterns observed across 50+ developer tool releases.

## Summary Statistics

| Metric | Count | Percentage |
|--------|-------|------------|
| Total tools surveyed | 54 | 100% |
| Ship glibc variant | 45 | 83% |
| Ship musl variant | 22 | 41% |
| Statically linked | 30 | 56% |
| Support amd64 | 54 | 100% |
| Support arm64 | 48 | 89% |

## Libc Variant Patterns

### Pattern 1: Static-Only (No Libc Dependency)
**Prevalence: 56% of tools**

Go tools dominate this category. They ship a single Linux binary per architecture that works everywhere because Go statically links its runtime:

- `gh_2.83.2_linux_amd64.tar.gz`
- `helm-v4.0.4-linux-amd64.tar.gz`
- `terraform_1.15.0_linux_amd64.zip`

**Language breakdown:**
- Go: gh, helm, k9s, dive, trivy, terraform, vault, direnv, fzf, yq, task, hugo, gum, glow, age, sops, cosign, grype, syft, lazygit, lazydocker, git-lfs
- C (static): jq

### Pattern 2: Both glibc and musl Variants
**Prevalence: 37% of tools**

Most Rust tools ship both variants explicitly in the filename:

- `ripgrep-15.1.0-x86_64-unknown-linux-gnu.tar.gz`
- `ripgrep-15.1.0-x86_64-unknown-linux-musl.tar.gz`

Tools in this category:
- ripgrep, fd, bat, eza, delta, dust, sd, hyperfine, starship, nushell
- lsd, vivid, pastel, diskus, hexyl, bun, mdBook

### Pattern 3: Musl-Only
**Prevalence: 7% of tools**

Some tools exclusively ship musl builds:
- just, zoxide, btop, xsv, xh

This is interesting because musl binaries are more portable (work on both glibc and musl systems when statically linked).

### Pattern 4: glibc-Only (Dynamic)
**Prevalence: 15% of tools**

These tools ship only glibc binaries and are dynamically linked:
- deno, neovim, helix, ninja, cmake, fnm, volta, gitui

## Architecture Naming Conventions

### Common Patterns

| Pattern | Examples | Used By |
|---------|----------|---------|
| `amd64` | `linux_amd64`, `linux-amd64` | Go tools (73%) |
| `x86_64` | `x86_64-unknown-linux-gnu` | Rust tools (20%) |
| `64bit` / `x86-64` | `Linux-64bit`, `linux-x86_64` | Mixed (7%) |

### ARM64 Naming

| Pattern | Examples |
|---------|----------|
| `arm64` | Most Go tools |
| `aarch64` | Most Rust tools |
| `ARM64` | Some capitalized (trivy) |

## Asset Naming Conventions

### Major Patterns Observed

#### 1. GoReleaser Style (Most Common for Go)
```
{name}_{version}_{os}_{arch}.tar.gz
gh_2.83.2_linux_amd64.tar.gz
```

#### 2. Rust Target Triple Style
```
{name}-{version}-{arch}-unknown-linux-{libc}.tar.gz
ripgrep-15.1.0-x86_64-unknown-linux-musl.tar.gz
```

#### 3. Simple Style
```
{name}-{version}-{os}-{arch}.tar.gz
helm-v4.0.4-linux-amd64.tar.gz
```

#### 4. Raw Binary
```
{name}-{os}-{arch}
jq-linux-amd64
direnv.linux-amd64
```

## Compression Formats

| Format | Count | Percentage | Notes |
|--------|-------|------------|-------|
| `.tar.gz` | 42 | 78% | Most common |
| `.zip` | 8 | 15% | Common for Windows-centric tools |
| `.tar.xz` | 2 | 4% | helix, btop (tbz) |
| Raw binary | 5 | 9% | jq, direnv, sops, cosign |

## Package Formats (Non-Binary)

Many tools also ship distribution packages:
- `.deb` - Debian/Ubuntu
- `.rpm` - RHEL/Fedora
- `.apk` - Alpine (some Go tools like k9s, task)
- `.AppImage` - neovim

## Key Insights

### 1. Go Tools Are Universally Compatible
Go's static linking means a single binary works on any Linux. These tools don't need libc detection.

### 2. Rust Tools Require libc Detection
Rust tools typically ship both glibc and musl variants. The musl variant is safer for maximum compatibility when available.

### 3. Musl Variants Often Work Better
For tools that ship musl builds, these are typically statically linked and work on both glibc and musl systems.

### 4. C/C++ Tools Are Mixed
- Some (jq) ship static builds
- Others (cmake, ninja) ship glibc-dynamic builds
- C++ tools often need libstdc++ even with musl

### 5. arm64 Support Is Nearly Universal
89% of surveyed tools support arm64, making it a first-class platform.

### 6. Naming Is Inconsistent But Predictable
While exact naming varies, patterns cluster by build toolchain:
- GoReleaser tools follow one pattern
- Rust/cargo-dist tools follow another
- The pattern is usually consistent within a project

# Design: Deterministic Recipe Resolution and Immutable Snapshots

- **Status**: Proposed
- **Issue**: #227
- **Author**: @dangazineu
- **Created**: 2025-12-09

## Context and Problem Statement

Tsuku recipes are dynamic by design - they contain templates and version provider configuration that determine how to resolve and install tools. This dynamism is a feature: recipes like `go.toml` work across all Go versions without hardcoding URLs.

However, this creates a **reproducibility problem**:

**Current flow:**
```
tsuku install ripgrep@14.1.0
    ↓
Recipe (templates) → Version Resolution (GitHub API) → Template Expansion → Download → Install
```

Even when a user specifies an exact version like `ripgrep@14.1.0`, several factors are resolved at runtime:
1. **Platform detection**: `{{os}}` and `{{arch}}` are expanded based on the current machine
2. **URL construction**: The download URL is constructed dynamically from templates
3. **Asset selection**: For GitHub releases, the specific asset to download is matched at runtime

This means:
- Running `tsuku install ripgrep@14.1.0` today vs. tomorrow could theoretically yield different binaries
- There's no record of exactly what was downloaded - only what version was requested
- Teams cannot guarantee identical tool installations across machines
- CI/CD builds may diverge from developer environments

**Concrete non-determinism scenarios:**

1. **Upstream re-tags a release**: A maintainer force-pushes a tag to fix a security issue. `tsuku install tool@1.0.0` now downloads a different binary than last week, but the version number is unchanged.

2. **Asset naming changes**: GitHub release assets can be renamed or replaced. A recipe pattern like `tool-{version}-{os}.tar.gz` might match different files over time.

3. **Recipe updates**: The recipe registry updates `ripgrep.toml` to use a different download mirror or change the asset selection pattern. The same `tsuku install ripgrep@14.1.0` command now resolves to a different URL.

4. **Platform drift**: A developer installs on Linux x64, but CI runs on arm64. Without explicit platform tracking, reinstallation on a different machine silently uses different binaries

**Why this matters now:**

1. **Team workflows**: As tsuku gains adoption, teams need confidence that `tsuku install` produces identical results across all machines
2. **CI/CD reproducibility**: Build pipelines should install the exact same tool binaries every time
3. **Audit trail**: For security-sensitive environments, knowing exactly what was installed (URLs, checksums) matters
4. **Defense-in-depth**: This complements the verification work (#192) - verification confirms you got the right thing, determinism ensures you'll always get the same thing

### Scope

**In scope:**
- Capturing resolved state (version, URLs, checksums, platform info) after installation
- Replaying installations from captured state without re-resolving
- Lock file format for project-level reproducibility
- Integration with existing `state.json` for single-machine state

**Out of scope:**
- Changes to recipe format (recipes remain dynamic)
- Cross-platform lock files (a lock file captures one platform's resolution)
- Cryptographic signature verification (separate concern, see #208)
- Automatic lock file updates (manual workflow initially)

## Decision Drivers

1. **Reproducibility**: Same input (recipe + version + lock) must yield identical installation
2. **Transparency**: Users should be able to inspect exactly what will be downloaded
3. **Incremental adoption**: Lock files should be optional; tsuku must work without them
4. **Minimal overhead**: Lock file creation shouldn't significantly slow installation
5. **Platform awareness**: Lock files are inherently platform-specific; this should be explicit
6. **Compatibility with verification**: Must work with verification modes (#192) - lock files capture what was resolved, verification confirms what was installed

## External Research

### mise.lock (mise-en-place)

[mise](https://mise.jdx.dev/dev-tools/mise-lock.html) implements a TOML-based lock file with per-platform sections:

```toml
[[tools.ripgrep]]
version = "14.1.1"
backend = "aqua:BurntSushi/ripgrep"

[tools.ripgrep.platforms.linux-x64]
checksum = "sha256:4cf9f2741e6c465ffdb7c26f38056a59e2a2544b51f7cc128ef28337eeae4d8e"
size = 1234567
url = "https://github.com/BurntSushi/ripgrep/releases/download/14.1.1/ripgrep-14.1.1-x86_64-unknown-linux-musl.tar.gz"
```

**Key design decisions:**
- Platform keys use `{os}-{arch}` format (e.g., `linux-x64`, `macos-arm64`)
- Each platform section is independent - different team members on different platforms don't conflict
- Checksum and URL are captured together
- `mise lock` command explicitly generates/updates the lock file
- `mise settings locked=true` enables strict mode where unresolved tools fail

**Relevance to tsuku:** mise's platform-keyed approach solves the multi-platform team problem elegantly. Tsuku could adopt a similar structure.

### Cargo.lock (Rust)

[Cargo.lock](https://doc.rust-lang.org/cargo/guide/cargo-toml-vs-cargo-lock.html) captures resolved dependencies for Rust projects:

```toml
[[package]]
name = "serde"
version = "1.0.193"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "25dd9975e68d0cb5aa1120c288333fc98731bd1dd12f561e468ea4728c042b89"
```

**Key design decisions:**
- Automatically generated on first build, updated on dependency changes
- Checksums included inline with package entries
- Git-merge-friendly format (deduplicated information)
- Should be committed to version control

**Relevance to tsuku:** Cargo's approach of inline checksums per entry is cleaner than separate checksum files. The automatic generation model may not fit tsuku's explicit workflow.

### go.sum (Go)

[go.sum](https://go.dev/ref/mod) takes a different approach - it's a checksum file, not a lock file:

```
golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
```

**Key design decisions:**
- Two lines per module: one for content hash, one for go.mod hash
- Checksums verified against public transparency log (sum.golang.org)
- Format is extremely simple: `module version hash`
- Acts as a tamper detection mechanism, not version locking

**Relevance to tsuku:** The transparency log concept is interesting for future work but out of scope here. The line-per-module format is too simple for tsuku's needs (no platform info, no URLs).

### flake.lock (Nix)

[flake.lock](https://nixos.wiki/wiki/Flakes) uses JSON to capture the exact inputs for a Nix flake:

```json
{
  "nodes": {
    "nixpkgs": {
      "locked": {
        "lastModified": 1234567890,
        "narHash": "sha256-...",
        "owner": "NixOS",
        "repo": "nixpkgs",
        "rev": "abc123...",
        "type": "github"
      }
    }
  }
}
```

**Key design decisions:**
- Captures git revision, timestamp, and content hash
- Graph structure for transitive dependencies
- Fully deterministic builds when flake.lock is present
- `nix flake lock` updates the lock file

**Relevance to tsuku:** Nix's concept of content-addressable storage (narHash) provides stronger guarantees than URL+checksum. However, tsuku doesn't control upstream artifact storage, so this model doesn't directly apply.

### Research Summary

| System | Format | Platform Handling | Checksum | Auto-Update |
|--------|--------|-------------------|----------|-------------|
| mise | TOML | Per-platform sections | SHA256/Blake3 | Manual (`mise lock`) |
| Cargo | TOML | N/A (source-only) | SHA256 inline | Automatic |
| go.sum | Text | N/A | SHA256 | Automatic |
| Nix | JSON | N/A (content-addressed) | NAR hash | Manual (`nix flake lock`) |

**Key insights:**
1. **TOML is the standard** for human-readable lock files in modern tools
2. **Platform-specific sections** (mise) are the cleanest solution for multi-platform teams
3. **Explicit lock commands** (mise, Nix) work better than automatic updates for reproducibility guarantees
4. **Checksums should be inline** with the entry they protect, not in separate files

## Implementation Context

### Current Architecture

Tsuku's installation flow involves several components:

1. **Version Resolution** (`internal/version/resolver.go`): Queries external APIs (GitHub, npm, etc.) to resolve version strings
2. **Template Expansion** (`internal/actions/download.go`): Expands `{version}`, `{os}`, `{arch}` placeholders in URLs
3. **State Tracking** (`internal/install/state.go`): Records what's installed in `$TSUKU_HOME/state.json`

**Current state.json structure:**
```json
{
  "installed": {
    "ripgrep": {
      "active_version": "14.1.0",
      "versions": {
        "14.1.0": {
          "requested": "14.1.0",
          "binaries": ["rg"],
          "installed_at": "2025-01-01T00:00:00Z"
        }
      }
    }
  }
}
```

**What's missing for reproducibility:**
- The actual URL that was used for download
- The checksum of the downloaded artifact
- Platform information (OS, architecture)
- Version provider metadata (which GitHub repo, which npm package)

### Existing Patterns

**Similar implementations in the codebase:**
- `state.json` already has per-version metadata via `VersionState`
- Atomic file operations with locking already exist
- Version resolution returns `VersionInfo` with both tag and normalized version

**Conventions to follow:**
- JSON for machine state (`state.json`), TOML for user-facing config
- Atomic writes via temp file + rename
- File locking for concurrent access

### Integration Points

A lock file solution must integrate with:
1. **Executor** (`internal/executor/executor.go`): To capture resolved URLs during install
2. **Download action** (`internal/actions/download.go`): To record checksums after download
3. **State manager** (`internal/install/state.go`): To optionally extend or coexist with existing state

## Considered Options

This design involves three independent decisions that compose together:

### Decision 1: Lock File Format

**Question:** What format should tsuku.lock use?

#### Option 1A: TOML

Use TOML format, following mise and Cargo conventions:

```toml
[[tools.ripgrep]]
version = "14.1.0"

[tools.ripgrep.platforms.linux-x64]
url = "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz"
checksum = "sha256:abc123..."
```

**Pros:**
- Consistent with recipe format (also TOML)
- Human-readable and editable
- Industry standard for lock files (Cargo, mise)
- Git-merge-friendly with proper structure

**Cons:**
- Requires TOML parsing library (already used for recipes)
- Nested structures can get verbose

#### Option 1B: JSON

Use JSON format, consistent with state.json:

```json
{
  "tools": {
    "ripgrep": {
      "version": "14.1.0",
      "platforms": {
        "linux-x64": {
          "url": "https://...",
          "checksum": "sha256:abc123..."
        }
      }
    }
  }
}
```

**Pros:**
- Consistent with existing state.json format
- No additional parsing dependencies
- Native Go encoding/json support

**Cons:**
- Less human-readable than TOML
- More prone to merge conflicts
- Inconsistent with recipe format (TOML)

---

### Decision 2: Lock File Scope

**Question:** Where should lock files live and what should they capture?

#### Option 2A: Project-Level Lock File (`tsuku.lock`)

Lock file in project directory, capturing tools defined in a project manifest:

```
myproject/
├── tsuku.toml      # Project tool manifest
├── tsuku.lock      # Locked resolutions
└── src/
```

**Pros:**
- Familiar model (package.json/package-lock.json, Cargo.toml/Cargo.lock)
- Natural for teams: commit lock file to share resolutions
- Scoped to project - different projects can have different versions

**Cons:**
- Requires project manifest feature (new `tsuku.toml`)
- More complex workflow: must be in project directory
- Doesn't help with global tool installations

#### Option 2B: Extend state.json with Resolution Metadata

Add URL and checksum fields to the existing `state.json`:

```json
{
  "installed": {
    "ripgrep": {
      "active_version": "14.1.0",
      "versions": {
        "14.1.0": {
          "requested": "14.1.0",
          "binaries": ["rg"],
          "installed_at": "...",
          "resolution": {
            "url": "https://...",
            "checksum": "sha256:...",
            "platform": "linux-x64"
          }
        }
      }
    }
  }
}
```

**Pros:**
- Minimal new concepts - extends existing pattern
- Works for all installations (global and project)
- No new files to manage
- Immediate benefit: audit trail for what was installed

**Cons:**
- state.json is machine-specific, not shareable
- Doesn't directly solve team reproducibility
- Mixes "what's installed" with "how to reproduce"

#### Option 2C: Hybrid - state.json for audit + tsuku.lock for sharing

Extend state.json for audit trail, add optional tsuku.lock for team sharing:

- state.json gains resolution metadata (Option 2B)
- tsuku.lock is an optional export format for project-level reproducibility
- `tsuku lock export` generates tsuku.lock from state.json
- `tsuku install --locked` uses tsuku.lock if present

**Pros:**
- Immediate value: audit trail in state.json
- Future value: team sharing via lock file
- Clear separation: state.json = machine state, tsuku.lock = shared intent
- Incremental adoption: start with state.json, add lock file later

**Cons:**
- Two concepts to understand
- Potential for state.json and tsuku.lock to drift

#### Option 2D: Content-Addressable Storage (Nix/Bazel model)

Store downloaded artifacts by content hash rather than name/version:

```
$TSUKU_HOME/
├── cas/
│   └── sha256-abc123.../    # Artifact stored by hash
└── tools/
    └── ripgrep-14.1.0/      # Symlink to CAS entry
```

**Pros:**
- Strongest reproducibility guarantee - same hash always means same content
- Automatic deduplication of identical artifacts
- Industry standard for hermetic builds (Nix, Bazel)
- Immutable by design - content never changes once stored

**Cons:**
- Significant architectural change - not incremental
- Requires tsuku to manage artifact storage (currently delegated to filesystem)
- Doesn't help with team sharing unless CAS is shared/remote
- Higher complexity for limited benefit over URL+checksum approach

**Why not chosen as primary approach:** Tsuku downloads binaries from upstream sources; we don't control artifact storage. URL+checksum provides equivalent guarantees for our use case without requiring a storage layer. CAS would be valuable if tsuku hosted artifacts (like a proxy), but that's not the current model.

---

### Decision 3: Lock File Generation

**Question:** When and how are lock entries created?

#### Option 3A: Automatic on Install

Lock entries are automatically recorded during `tsuku install`:

```bash
tsuku install ripgrep@14.1.0
# → Installs tool AND records resolution in state.json/lock file
```

**Pros:**
- Zero friction - works automatically
- Always up-to-date with what's installed
- Familiar from npm/Cargo model

**Cons:**
- Lock file changes on every install (noisy git history)
- May capture unintended installations
- Less control over what gets locked

#### Option 3B: Explicit Lock Command

Lock entries are only created via explicit command:

```bash
tsuku install ripgrep@14.1.0  # Installs but doesn't lock
tsuku lock                     # Creates/updates lock file from current state
tsuku lock ripgrep             # Locks specific tool
```

**Pros:**
- Full control over what gets locked
- Clean separation between "install" and "lock"
- Lock file only changes intentionally
- Matches mise/Nix model

**Cons:**
- Extra step for users who want locking
- Risk of forgetting to lock
- state.json won't have resolution metadata unless we separate concerns

#### Option 3C: Automatic in state.json, Explicit for tsuku.lock

Resolution metadata is always captured in state.json during install, but tsuku.lock requires explicit generation:

```bash
tsuku install ripgrep@14.1.0  # Installs, records resolution in state.json
tsuku lock                     # Exports to tsuku.lock for sharing
```

**Pros:**
- Audit trail always available (state.json)
- Lock file for sharing is intentional
- Best of both worlds

**Cons:**
- Users may not realize resolution is being tracked
- Two places where resolution data lives

---

### Evaluation Against Decision Drivers

| Decision | Option | Reproducibility | Transparency | Incremental | Overhead | Platform-Aware | Verification |
|----------|--------|-----------------|--------------|-------------|----------|----------------|--------------|
| Format | 1A: TOML | Good | Good | Good | Low | Good | Good |
| Format | 1B: JSON | Good | Fair | Good | Low | Good | Good |
| Scope | 2A: Project-only | Good | Good | Poor | Medium | Good | Good |
| Scope | 2B: state.json | Fair | Good | Good | Low | Fair | Good |
| Scope | 2C: Hybrid | Good | Good | Good | Medium | Good | Good |
| Scope | 2D: CAS | Excellent | Fair | Poor | High | Good | Good |
| Generation | 3A: Automatic | Good | Fair | Good | Low | Good | Good |
| Generation | 3B: Explicit | Good | Good | Fair | Medium | Good | Good |
| Generation | 3C: Hybrid | Good | Good | Good | Low | Good | Good |
| Platform | 4A: Single-platform | Fair | Good | Good | Low | Poor | Good |
| Platform | 4B: Multi-platform | Good | Good | Fair | Medium | Good | Good |
| Platform | 4C: On-demand | Fair | Good | Good | Low | Fair | Good |

---

### Decision 4: Platform Handling

**Question:** How should lock files handle multi-platform teams?

#### Option 4A: Single-Platform Lock Files

Each lock file captures resolutions for one platform only:

```toml
# tsuku.lock (generated on Linux x64)
platform = "linux-x64"

[[tools.ripgrep]]
version = "14.1.0"
url = "https://..."
checksum = "sha256:..."
```

**Pros:**
- Simple implementation
- No need to fetch metadata for platforms you're not on
- Mirrors how state.json works today

**Cons:**
- Teams need separate lock files per platform (tsuku-linux-x64.lock, tsuku-darwin-arm64.lock)
- More files to manage and keep in sync
- CI/CD must generate lock files for all target platforms

#### Option 4B: Multi-Platform Lock Files (mise model)

Lock file contains sections for each platform:

```toml
[[tools.ripgrep]]
version = "14.1.0"

[tools.ripgrep.platforms.linux-x64]
url = "https://github.com/.../ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz"
checksum = "sha256:abc123..."

[tools.ripgrep.platforms.darwin-arm64]
url = "https://github.com/.../ripgrep-14.1.0-aarch64-apple-darwin.tar.gz"
checksum = "sha256:def456..."
```

**Pros:**
- Single lock file for entire team
- Platforms don't conflict - entries are additive
- Standard approach (mise uses this)

**Cons:**
- Requires mechanism to populate other platforms' entries
- Lock file grows with platform count
- Some tools may not be available on all platforms

#### Option 4C: On-Demand Platform Resolution

Lock file captures current platform; other platforms resolve dynamically:

```toml
[[tools.ripgrep]]
version = "14.1.0"
# Only linux-x64 is locked; darwin users will resolve dynamically

[tools.ripgrep.platforms.linux-x64]
url = "https://..."
checksum = "sha256:..."
```

**Pros:**
- Minimal overhead - only lock what you've installed
- Natural workflow - run `tsuku lock` on each platform as needed
- Graceful fallback for unlocked platforms

**Cons:**
- Partial reproducibility - not all platforms are deterministic
- May surprise users who expect full locking

---

### Resolution Precedence

When installing a tool, multiple sources of information may be available. This section defines the precedence order.

**Precedence (highest to lowest):**

1. **Lock file entry for current platform** (`tsuku.lock` with matching platform key)
   - If `--locked` flag is set and no entry exists, fail
   - URL and checksum from lock file are used directly; no version resolution occurs

2. **Lock file version without platform entry**
   - Version is pinned, but platform-specific resolution happens dynamically
   - Checksum is computed post-download and compared if present

3. **Explicit version from command line** (`tsuku install tool@1.0.0`)
   - Version resolution is skipped; URL is constructed from recipe template
   - Checksum is computed post-download

4. **Dynamic resolution** (no lock file, no explicit version)
   - Full resolution: query version provider, expand templates, download
   - Current behavior unchanged

**Conflict handling:**

- If lock file specifies version `1.0.0` but user runs `tsuku install tool@2.0.0`, the explicit version wins (with warning)
- If `--locked` flag is used and versions don't match, fail with error
- If lock file checksum doesn't match downloaded content, fail with error

### Uncertainties

- **Cross-platform population:** How should `tsuku lock` populate entries for platforms the user isn't running on? Options:
  - Manual: Run `tsuku lock` on each platform (most reliable)
  - API-based: Query GitHub releases API for all platform assets (requires parsing asset names)
  - Hybrid: Lock current platform automatically, allow `tsuku lock --platforms linux-x64,darwin-arm64` to fetch others

- **Checksum computation:** Some downloads don't have published checksums. We will compute SHA256 after download, which adds latency but ensures we always have a checksum.

- **Recipe version pinning:** Should the lock file also pin the recipe version (commit hash)? This provides stronger reproducibility but adds complexity. Initial implementation will not pin recipe versions.

- **Multi-step installs:** Some recipes have multiple download steps. All downloads will be captured in an array:
  ```toml
  [[tools.complex-tool.platforms.linux-x64.downloads]]
  url = "https://..."
  checksum = "sha256:..."

  [[tools.complex-tool.platforms.linux-x64.downloads]]
  url = "https://..."
  checksum = "sha256:..."
  ```

## Decision Outcome

**Chosen: 1A (TOML) + 2C (Hybrid) + 3C (Automatic state.json, Explicit lock) + 4B (Multi-platform)**

### Summary

We will use TOML format for a multi-platform lock file (`tsuku.lock`) that is explicitly generated via `tsuku lock`, while automatically capturing resolution metadata in `state.json` during every install. This provides immediate audit trail value, supports team sharing via a familiar lock file model, and handles multi-platform teams cleanly.

### Rationale

**Format (1A: TOML):**
- Consistent with recipe format - users already understand TOML from `ripgrep.toml`
- Industry standard for lock files (Cargo, mise) - familiar to developers
- Human-readable and git-merge-friendly - essential for team workflows

**Scope (2C: Hybrid):**
- Immediate value via state.json - every install captures URL and checksum (audit trail)
- Optional sharing via tsuku.lock - teams can opt-in to reproducibility
- Clean separation - state.json is machine state, tsuku.lock is shared intent
- Incremental adoption - works without lock file, gains value with it

**Generation (3C: Automatic state.json + Explicit lock):**
- Zero friction for audit trail - state.json captures resolution automatically
- Intentional sharing - `tsuku lock` is an explicit action, not a side effect
- Matches mise/Nix model - proven UX for reproducibility workflows
- Avoids noisy git history - lock file only changes when user runs `tsuku lock`

**Platform (4B: Multi-platform):**
- Single lock file for teams - no platform-specific files to manage
- Additive entries - different developers add their platform sections
- Industry standard - mise uses exactly this model
- Scales gracefully - only populated platforms are present

### Alternatives Rejected

- **1B (JSON):** Less readable, more merge conflicts, inconsistent with recipe format
- **2A (Project-only):** Requires new project manifest feature, doesn't help global installs
- **2B (state.json only):** Doesn't solve team reproducibility - state.json is machine-specific
- **2D (CAS):** Significant architectural change with limited incremental benefit
- **3A (Automatic lock):** Too noisy for version control, less control over what gets locked
- **3B (Explicit only):** No audit trail unless user explicitly locks
- **4A (Single-platform):** Multiple files to manage, poor team experience
- **4C (On-demand):** Partial reproducibility defeats the purpose

### Trade-offs Accepted

1. **Two concepts to learn:** Users need to understand both state.json (audit) and tsuku.lock (sharing). This is acceptable because:
   - state.json already exists; we're just adding fields
   - tsuku.lock is opt-in; users who don't need team reproducibility can ignore it
   - The concepts mirror npm (node_modules vs package-lock.json) which developers understand

2. **Potential for drift:** state.json and tsuku.lock can get out of sync. This is acceptable because:
   - They serve different purposes - drift is expected (state.json reflects what's installed now, tsuku.lock reflects shared intent)
   - `tsuku lock` always regenerates from state.json, so syncing is one command

3. **Multi-platform population requires coordination:** Each developer must run `tsuku lock` on their platform. This is acceptable because:
   - This is the most reliable approach (no need to parse upstream asset names)
   - CI can automate this for common platforms
   - Future enhancement can add `tsuku lock --platforms` for API-based resolution

## Solution Architecture

### Overview

The solution extends tsuku's installation flow with resolution capture and lock file support:

```
                               ┌─────────────────────────────────────────────────────────────┐
                               │                    tsuku install                            │
                               └─────────────────────────────────────────────────────────────┘
                                                          │
                               ┌──────────────────────────┼──────────────────────────┐
                               │                          │                          │
                               ▼                          ▼                          ▼
                        ┌─────────────┐           ┌─────────────┐           ┌─────────────┐
                        │ --locked    │           │ tsuku.lock  │           │ No lock     │
                        │ flag set    │           │ exists      │           │ (default)   │
                        └─────────────┘           └─────────────┘           └─────────────┘
                               │                          │                          │
                               ▼                          ▼                          ▼
                        Require lock entry         Use lock if present        Dynamic resolution
                        for current platform       else resolve dynamically   (current behavior)
                               │                          │                          │
                               └──────────────────────────┴──────────────────────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │    Download     │
                                                 │  (capture URL)  │
                                                 └─────────────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │ Compute SHA256  │
                                                 │  (verify if     │
                                                 │   locked)       │
                                                 └─────────────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │ Update state.json│
                                                 │ (resolution     │
                                                 │  metadata)      │
                                                 └─────────────────┘
```

### Components

#### 1. Resolution Metadata (`internal/install/resolution.go`)

New struct to capture resolved installation details:

```go
// ResolutionMetadata captures the exact resolution used for an installation.
// This data enables reproducible installs and provides an audit trail.
type ResolutionMetadata struct {
    Platform  string   `json:"platform"`            // e.g., "linux-x64"
    Downloads []Download `json:"downloads"`         // All downloaded artifacts
    ResolvedAt time.Time `json:"resolved_at"`       // When resolution occurred
}

type Download struct {
    URL       string `json:"url"`                   // Actual URL used
    Checksum  string `json:"checksum"`              // sha256:... computed post-download
    Size      int64  `json:"size,omitempty"`        // File size in bytes
}
```

#### 2. Extended VersionState (`internal/install/state.go`)

Extend existing struct to include resolution metadata:

```go
type VersionState struct {
    Requested   string             `json:"requested"`
    Binaries    []string           `json:"binaries,omitempty"`
    InstalledAt time.Time          `json:"installed_at"`
    Resolution  *ResolutionMetadata `json:"resolution,omitempty"`  // NEW
}
```

#### 3. Lock File Types (`internal/lock/types.go`)

New package for lock file handling:

```go
// LockFile represents the tsuku.lock structure
type LockFile struct {
    Version int                     `toml:"version"`  // Lock file format version
    Tools   map[string]LockedTool   `toml:"tools"`
}

// LockedTool represents a locked tool with platform-specific entries
type LockedTool struct {
    Version   string                      `toml:"version"`
    Platforms map[string]PlatformEntry    `toml:"platforms"`
}

// PlatformEntry contains resolution data for a specific platform.
// For single-download recipes, use URL/Checksum/Size directly.
// For multi-download recipes, use Downloads array instead (URL/Checksum/Size should be empty).
type PlatformEntry struct {
    // Single-download fields (use when Downloads is empty)
    URL       string   `toml:"url,omitempty"`
    Checksum  string   `toml:"checksum,omitempty"`
    Size      int64    `toml:"size,omitempty"`
    // Multi-download field (use when recipe has multiple download steps)
    Downloads []DownloadEntry `toml:"downloads,omitempty"`
}

type DownloadEntry struct {
    URL      string `toml:"url"`
    Checksum string `toml:"checksum"`
    Size     int64  `toml:"size,omitempty"`
}
```

#### 4. Lock File Manager (`internal/lock/manager.go`)

Operations for reading and writing lock files:

```go
type Manager struct {
    path string  // Path to tsuku.lock
}

func (m *Manager) Load() (*LockFile, error)
func (m *Manager) Save(lock *LockFile) error
func (m *Manager) GetEntry(tool, platform string) (*PlatformEntry, error)
func (m *Manager) UpdateFromState(state *install.State, tools []string) error
```

### Key Interfaces

#### Platform Detection

```go
// internal/platform/platform.go
func CurrentPlatform() string  // Returns "linux-x64", "darwin-arm64", etc.
func NormalizePlatform(os, arch string) string
```

Platform key format: `{os}-{arch}` where:
- `os`: `linux`, `darwin`, `windows`
- `arch`: `x64`, `arm64`, `x86`

Platform normalization uses `runtime.GOOS` and `runtime.GOARCH`:
- `amd64` → `x64`
- `386` → `x86`

**Lock file location:** `tsuku.lock` is always in the current working directory (project-local), following the mise/npm model. This enables project-specific tool versions.

#### Checksum Computation

```go
// internal/checksum/sha256.go
func ComputeSHA256(reader io.Reader) (string, error)
func FormatChecksum(hash []byte) string  // Returns "sha256:..."
func VerifyChecksum(reader io.Reader, expected string) error
```

### Data Flow

#### Install with Lock File

```
1. User runs: tsuku install ripgrep --locked
2. Load tsuku.lock from current directory
3. Look up entry for ripgrep + current platform
4. If found:
   a. Use URL from lock entry directly (skip version resolution)
   b. Download artifact
   c. Compute SHA256 of downloaded content
   d. Compare with locked checksum → fail if mismatch
   e. Continue with normal installation
5. If not found and --locked: fail with error
6. Update state.json with resolution metadata
```

#### Lock File Generation

```
1. User runs: tsuku lock [tool...]
2. Load current state.json
3. For each installed tool (or specified tools):
   a. Get resolution metadata from state
   b. Create/update lock entry for current platform
4. Write tsuku.lock (merge with existing entries for other platforms)
```

## Implementation Approach

### Phase 1: Resolution Capture

**Goal:** Capture resolution metadata during installation without changing behavior.

**Changes:**
- Add `ResolutionMetadata` struct to `internal/install/`
- Extend `VersionState` with optional `Resolution` field
- Update download action to compute SHA256 and record URL
- Modify executor to populate resolution metadata before state save

**Deliverables:**
- `state.json` captures URL, checksum, platform for every install
- Existing tests pass unchanged
- New tests verify resolution capture

**Backward compatibility:** Existing `state.json` files without `resolution` field continue to work. The field is optional (`omitempty`) and only populated for new installs.

### Phase 2: Lock File Format

**Goal:** Implement lock file reading and writing.

**Changes:**
- Create `internal/lock/` package with types and manager
- Add `tsuku lock` command to generate lock file from state
- Support tool filtering: `tsuku lock ripgrep go` locks specific tools

**Deliverables:**
- `tsuku lock` generates `tsuku.lock` from state.json
- Lock file uses TOML format with platform sections
- Lock entries merge with existing entries for other platforms

### Phase 3: Locked Installation

**Goal:** Enable installation from lock file.

**Changes:**
- Add `--locked` flag to `tsuku install`
- Modify executor to check lock file before version resolution
- Implement checksum verification during download
- Add clear error messages for missing/mismatched entries

**Deliverables:**
- `tsuku install --locked` uses lock file when present
- Checksum mismatch fails installation with clear error
- Missing lock entry with `--locked` flag fails

### Phase 4: Documentation and Polish

**Goal:** Complete the feature for release.

**Changes:**
- Add user documentation for lock file workflow
- Add `tsuku lock --help` with examples
- Consider `TSUKU_LOCKED=1` environment variable for CI
- Add warnings when lock file is stale (installed versions differ)

**Deliverables:**
- User documentation in README or docs/
- Help text for new commands and flags
- CI-friendly environment variable support

## Consequences

### Positive

1. **Reproducible installations**: Teams can guarantee identical tool versions across machines
2. **Audit trail**: `state.json` now records exactly what was downloaded
3. **CI/CD confidence**: Locked installations ensure build consistency
4. **Incremental adoption**: Works without lock file; lock file is purely additive
5. **Debugging support**: When something goes wrong, the URL and checksum are recorded

### Negative

1. **Increased complexity**: Two concepts (state.json resolution, tsuku.lock) to understand
2. **Checksum computation overhead**: SHA256 adds latency to every download
3. **Lock file maintenance**: Teams must run `tsuku lock` on each platform
4. **Storage increase**: state.json grows with resolution metadata (~100-200 bytes per tool)

### Mitigations

| Consequence | Mitigation |
|-------------|------------|
| Complexity | Clear documentation; tsuku.lock is opt-in |
| SHA256 overhead | Computed during download (streaming); negligible for typical binary sizes |
| Lock file maintenance | CI can automate; future `--platforms` flag for API-based population |
| Storage increase | Resolution metadata is compact; state.json remains small |

## Security Considerations

### Download Verification

**How are downloaded artifacts validated?**

This design adds SHA256 checksum verification as a mandatory part of locked installations:

1. **During normal install**: SHA256 is computed post-download and stored in `state.json`. This provides an audit trail but does not verify against a known-good value.

2. **During locked install** (`--locked` flag): SHA256 is computed post-download and compared against the checksum in `tsuku.lock`. Mismatch causes installation to fail.

3. **Checksum format**: Uses `sha256:` prefix (e.g., `sha256:a1b2c3...`) for future extensibility to other algorithms.

**Failure behavior:**
- Checksum mismatch with `--locked`: Installation fails immediately with clear error message showing expected vs actual checksum
- Checksum computation failure: Installation fails (do not proceed with unverified binary)

**Limitations:**
- Without `--locked`, checksum is recorded but not verified - this is intentional (first install creates the baseline)
- Checksums are computed post-download; a compromised binary is already on disk (but not installed)

**TOCTOU mitigation:**
To prevent time-of-check/time-of-use attacks where a binary is swapped between verification and installation:
1. Download to a temporary directory with restricted permissions (0700)
2. Compute checksum on the downloaded file
3. Verify checksum before any extraction or installation
4. Use atomic rename to move verified content to final location
5. For extracted archives, verify checksum of the archive file, not extracted contents

This ensures the verified content is the same content that gets installed.

### Execution Isolation

**What permissions does this feature require?**

This feature does not change the execution model:

- **File system access**: Same as current tsuku - write to `$TSUKU_HOME/` directory and `tsuku.lock` in current directory
- **Network access**: Same as current tsuku - HTTPS to upstream artifact sources
- **No privilege escalation**: Lock file operations run as current user

**New file locations:**
- `tsuku.lock` in current working directory (user-controlled, committed to version control)
- Resolution metadata in `$TSUKU_HOME/state.json` (existing file, new fields)

**Lock file as trusted input:**
When using `--locked`, the `tsuku.lock` file is treated as trusted input:
- URLs from the lock file are used directly for downloads
- A malicious lock file could point to malicious URLs
- **Mitigation**: Lock files are committed to version control and should be reviewed like code

### Supply Chain Risks

**Where do artifacts come from?**

This feature does not change artifact sources - it records and verifies them:

1. **Source trust model**: Artifacts still come from upstream sources (GitHub releases, npm, etc.) - no change
2. **Lock file trust**: The lock file becomes part of the supply chain:
   - Lock files should be committed to version control
   - Lock file changes should be reviewed (new URLs, changed checksums)
   - CI should use `--locked` to ensure lock file is authoritative

**New supply chain considerations:**

| Risk | Impact | Mitigation |
|------|--------|------------|
| Malicious lock file | Attacker could redirect downloads to compromised URLs | Lock files committed to VCS, reviewed like code |
| Lock file tampering | Attacker modifies lock file to bypass verification | VCS history provides audit trail; signed commits |
| Stale lock file | Old checksums may mask upstream compromise | `tsuku outdated` warns of version drift; regular updates |

**What if upstream is compromised?**

Lock files provide defense-in-depth against upstream compromise:
- If upstream re-tags a release with different content, checksum mismatch is detected
- Historical lock files can be compared to identify when compromise occurred
- However, if lock file is generated after compromise, it will contain the malicious checksum

**Residual risk:** Lock files cannot protect against compromise that occurs before the lock file is created.

### User Data Exposure

**What user data does this feature access or transmit?**

This feature has minimal data exposure:

1. **Local data accessed:**
   - `$TSUKU_HOME/state.json` - reads and writes installation state
   - `tsuku.lock` - reads and writes lock file
   - Downloaded artifacts (existing behavior)

2. **Data transmitted:**
   - No new data transmitted
   - Same HTTP requests as current installation flow
   - URLs from lock file may reveal tool preferences (same as current behavior)

3. **Privacy implications:**
   - Lock file may contain internal tool versions if committed publicly
   - This is intentional - lock files are designed to be shared
   - Sensitive tools should not be in public lock files

**No telemetry changes:** This feature does not add any new data collection or transmission.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious lock file | VCS review, treat as code | Trusted committers could inject malicious entries |
| Checksum bypass | `--locked` flag makes verification mandatory | Users may forget to use flag |
| Stale checksums | `tsuku outdated` warnings | Users may ignore warnings |
| URL manipulation | Lock file URLs are exact; no template expansion | Lock file reviewer must verify URLs |
| Post-download tampering | Checksum computed before extraction | Narrow window between download and checksum |
| Compromised upstream | Checksum mismatch detected on reinstall | Initial lock creation inherits compromise |
| Archive path traversal | Existing extraction code validates paths | Depends on extraction implementation correctness |

### Defense-in-Depth Position

This feature is Layer 3 in tsuku's defense-in-depth strategy:

```
Layer 4: Functional Testing (future)
Layer 3: Deterministic Resolution (this design) ← NEW
Layer 2: Version Verification (#192)
Layer 1: Download Checksums (existing)
```

Each layer provides independent protection:
- **Layer 1** verifies artifact integrity (what was downloaded)
- **Layer 2** verifies version correctness (what version is installed)
- **Layer 3** ensures reproducibility (same artifact every time)
- **Layer 4** verifies functionality (does it work correctly)


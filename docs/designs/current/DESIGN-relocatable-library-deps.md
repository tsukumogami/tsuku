---
status: Current
problem: Native libraries required by tools like Ruby are embedded as 30+ lines of shell script in tool recipes, causing maintenance burden from hardcoded SHA256s, code duplication across tools, tight coupling to Homebrew internals, and inability to resolve library versions at runtime.
decision: Libraries become first-class recipes with type = "library", installed via a suite of actions (homebrew, install_libraries, link_dependencies, set_rpath) that handle downloading, extraction, linking, and RPATH modification.
rationale: This approach aligns library handling with tsuku's existing dependency model (similar to how tools depend on language runtimes), eliminates hardcoded values, enables reuse across multiple tools, and provides security benefits through per-binary RPATH control instead of environment variables. It also establishes a foundation for expanding to other library-dependent tools like Python and Perl.
---

# Design Document: Relocatable Library Dependency System

## Status

Current

## Context and Problem Statement

Tsuku provides self-contained CLI tool installations without requiring system package managers or sudo access. Some tools, however, depend on shared libraries (e.g., libyaml for Ruby's YAML parsing). Currently, the Ruby recipe embeds 30+ lines of shell script to:

1. Determine platform and architecture
2. Look up the correct Homebrew bottle SHA256 from hardcoded values
3. Obtain an anonymous GHCR token
4. Download the libyaml bottle from GHCR
5. Extract and copy the library files to Ruby's lib directory

This inline approach creates several problems:

**Maintenance burden**: Every library update requires editing the recipe with new hardcoded SHA256 values for each platform. The Ruby recipe currently has four hardcoded SHA256s that must be updated whenever libyaml releases a new version.

**Code duplication**: If another tool needs libyaml (e.g., a hypothetical Python tool with YAML dependencies), the entire shell script would need to be duplicated. More broadly, any tool needing a Homebrew bottle dependency must reinvent the same GHCR authentication and extraction logic.

**Inconsistent abstraction**: Tsuku has first-class actions for downloading archives, extracting files, and installing binaries. Library dependencies bypass this action system entirely, embedding raw shell scripts that are harder to test, debug, and maintain.

**Tight coupling to Homebrew internals**: The recipe contains Homebrew-specific GHCR authentication logic, SHA256 blob addressing, and platform naming conventions. Changes to Homebrew's bottle distribution would require updating every recipe that uses this pattern.

**The better model - dependencies**: Tsuku already has a dependency model for runtimes:
- `tsuku install ruff` automatically provisions Python
- `tsuku install cargo-audit` automatically provisions Rust
- The tool recipe doesn't contain steps to install the runtime

The same pattern should apply to native library dependencies. Ruby should declare `dependencies = ["libyaml"]`, and libyaml should be its own recipe.

### Why Now

1. **Ruby recipe exists as proof of concept**: The current Ruby recipe demonstrates that relocatable library extraction from Homebrew bottles works. The pattern is validated; it now needs proper abstraction.

2. **Architecture decisions made**: Key decisions on library location, RPATH approach, and version handling have been analyzed and validated.

3. **Additional library-dependent tools**: Tools like Python (with native extensions) and Perl (with XS modules) may need similar library bundling. Without proper abstraction, each would repeat the Ruby recipe's complexity.

4. **RPATH expertise accumulated**: Through the ruby.toml implementation and dependency resolution research, we understand how RPATH-based relocation works for Homebrew bottles.

### Scope

**In scope:**
- Library recipes: Define libraries as recipes with `type = "library"` metadata
- `homebrew` action: Download, extract, and relocate Homebrew bottles with runtime version resolution
- `install_libraries` action: Copy libraries and preserve symlink structure
- `link_dependencies` action: Create symlinks from tool lib/ to shared libs/
- `set_rpath` action: Cross-platform RPATH modification (patchelf/install_name_tool)
- Homebrew version provider: Query Homebrew API for available library versions
- Library storage: `$TSUKU_HOME/libs/` with tool-specific symlinks and `used_by` tracking
- Migration: Update ruby.toml to use the new system

**Out of scope:**
- deb/rpm extraction (to be addressed in the future, see [#212](https://github.com/tsukumogami/tsuku/issues/212))
- User-installable libraries (`tsuku install libyaml` should fail)
- Version constraints on library dependencies
- Libraries for compilation use (PKG_CONFIG_PATH, C_INCLUDE_PATH, etc.)
- Automatic library discovery (tools must declare dependencies)
- nix-portable library handling (different mechanism)

## Decision Drivers

- **Maintainability**: Library updates should require changing only the library recipe, not every dependent tool recipe
- **Reusability**: The same library recipe should serve multiple tools
- **Consistency**: Library handling should use tsuku's action system and dependency model, not raw shell scripts
- **Isolation**: Libraries must be relocatable; no assumptions about system paths
- **Security**: RPATH is preferred over LD_LIBRARY_PATH for per-binary control
- **Runtime version resolution**: Library versions should be resolved at install time, not hardcoded in recipes

### Assumptions Requiring Validation

1. **Homebrew bottle stability**: Homebrew's GHCR distribution, bottle format, and platform naming conventions are stable enough to depend on. Based on Ruby recipe experience, this appears true.

2. **RPATH sufficiency**: RPATH with `$ORIGIN` is sufficient for library resolution on Linux and macOS. Wrapper scripts serve as fallback for signed binaries or when RPATH modification fails.

3. **Few library dependencies**: Most tsuku tools are statically linked or use ecosystem package managers. Library dependencies are the exception, not the rule.

4. **Full version isolation**: Different library versions can coexist (e.g., `libyaml-0.2.5/` and `libyaml-0.1.8/`) with each tool's RPATH pointing to its required version.

5. **Exact version matching**: For Phase 1, library dependency resolution uses exact version matching only. Semver-compatible version constraints are deferred to future work.

6. **Path length constraint**: `$TSUKU_HOME` path must be 19 characters or less due to Homebrew's `@@HOMEBREW_PREFIX@@` placeholder size. Longer paths require symlink workarounds.

## External Research

### Homebrew Bottle Distribution

**How it works:**
- Bottles are pre-built binaries published as tar.gz archives to GitHub Container Registry (GHCR)
- Access requires anonymous token authentication via `ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/{formula}:pull`
- GHCR manifest contains platform-specific blob SHAs in annotations (`sh.brew.bottle.digest`)
- Bottles contain `@@HOMEBREW_PREFIX@@` placeholders (19 chars, null-padded in binaries) that must be replaced with actual paths

**Relevance to tsuku:**
The Ruby recipe already implements this pattern with inline shell. Abstracting it to an action makes the pattern reusable and enables runtime version resolution.

**Source:** Homebrew documentation and existing Ruby recipe implementation

### RPATH and Library Resolution

**How it works:**
- ELF binaries on Linux use RPATH/RUNPATH to specify library search paths
- `$ORIGIN` is a special token resolved at runtime to the binary's directory
- patchelf can rewrite RPATH: `patchelf --set-rpath '$ORIGIN/../lib' binary`
- macOS uses `@executable_path`, `@loader_path`, and requires re-signing after modification

**Why RPATH over LD_LIBRARY_PATH:**

| Aspect | RPATH | LD_LIBRARY_PATH |
|--------|-------|-----------------|
| Security | Per-binary, can't be overridden | Global, affects all children |
| Reliability | Embedded in binary | Can be clobbered by user env |
| Performance | Direct lookup | Searches multiple paths |
| Debugging | `readelf -d` / `otool -L` | Invisible until runtime |

**Relevance to tsuku:**
RPATH is the primary mechanism. Wrapper scripts serve as fallback for signed binaries or when RPATH modification fails.

## Considered Options

### Option 1: Library as Recipe with Runtime Actions

Libraries become first-class recipes with `type = "library"`. A suite of actions handles the full workflow: `homebrew` for download/extraction, `install_libraries` for copying, `link_dependencies` for symlinks, and `set_rpath` for binary patching. Version resolution happens at runtime via Homebrew API.

**Library recipe (libyaml.toml):**
```toml
[metadata]
name = "libyaml"
description = "YAML 1.1 parser and emitter library"
type = "library"

[version]
source = "homebrew"
formula = "libyaml"

[[steps]]
action = "homebrew"
formula = "libyaml"

[[steps]]
action = "install_libraries"
patterns = ["lib/*.so*", "lib/*.dylib"]
```

**Tool recipe (ruby.toml):**
```toml
[metadata]
name = "ruby"
dependencies = ["libyaml"]
# ... rest of recipe unchanged, shell script removed
```

**Pros:**
- Clean separation: Library logic in library recipe, tool logic in tool recipe
- Reusable: Multiple tools can depend on the same library recipe
- Testable: Each action can be unit tested independently
- Consistent: Follows existing dependency pattern (like go depends on go toolchain)
- Version flexibility: `tsuku install libyaml@0.2.4` works via runtime resolution
- State tracking: `used_by` in state.json enables garbage collection

**Cons:**
- Four new actions: More code to implement and maintain
- New recipe type: Adds `type = "library"` concept to recipe schema
- Overhead: Simple cases (one tool, one library) require two recipes

### Option 2: Inline Library Extraction (Current Approach, Formalized)

Formalize the current shell script approach as a single composite action within the tool recipe.

**Tool recipe (ruby.toml):**
```toml
[[steps]]
action = "extract_homebrew_library"
formula = "libyaml"
dest = "lib"
files = ["libyaml*.so*", "libyaml*.dylib"]
```

**Pros:**
- No new recipe type: Libraries aren't recipes, just action parameters
- Single action: Less code to implement
- No dependency resolution changes: Everything in one recipe

**Cons:**
- Duplication: If two tools need libyaml, each copies the action config
- Version coupling: Library version is implicit, can't be pinned separately
- No sharing: Each tool downloads and stores its own copy
- Maintenance: Updating library patterns requires editing each dependent recipe
- No state tracking: Can't track which tools use which libraries

### Option 3: Magic Library Resolution

Libraries are declared as simple strings, resolved automatically without explicit recipes.

**Tool recipe:**
```toml
[metadata]
name = "ruby"
library_dependencies = ["libyaml"]
```

**Pros:**
- Minimal recipe changes: Just add library names to metadata
- Clean tool recipes: No library-specific steps

**Cons:**
- Magic behavior: How tsuku gets libraries is opaque
- No version control: Can't pin library versions
- No extensibility: Adding library sources requires core changes
- Debugging: When something fails, users can't inspect the process

### Evaluation Against Decision Drivers

| Driver | Option 1 (Library Recipe) | Option 2 (Inline Action) | Option 3 (Magic) |
|--------|--------------------------|--------------------------|------------------|
| Maintainability | Excellent | Poor | Good |
| Reusability | Excellent | Poor | Good |
| Consistency | Excellent | Good | Poor |
| Isolation | Excellent | Good | Good |
| Security (RPATH) | Excellent | Fair (no RPATH) | Fair |
| Runtime versioning | Excellent | Poor (hardcoded) | Poor |

## Decision Outcome

**Chosen option: Option 1 (Library as Recipe with Runtime Actions)**

Libraries become explicit recipes with `type = "library"`, installed via a suite of actions that handle downloading, extraction, linking, and RPATH modification. This aligns with the approved strategic design.

### Rationale

This option was chosen because:

1. **Best practices**: This architecture follows established patterns for relocatable binaries (RPATH, library deduplication, version isolation).

2. **Maintainability**: When libyaml updates, change one recipe. Library version resolution happens at runtime, not via hardcoded SHAs.

3. **Consistency with existing patterns**: Go tools depend on the go toolchain. Ruby can depend on libyaml the same way.

4. **Security**: RPATH modification provides per-binary library resolution that can't be overridden by environment variables.

5. **State tracking**: The `used_by` tracking in state.json enables future garbage collection of unused library versions.

### Alternatives Rejected

- **Option 2 (Inline Action)**: Rejected because it perpetuates the duplication and hardcoding problems. No path to version flexibility.

- **Option 3 (Magic)**: Rejected because it hides too much. Tsuku emphasizes transparency and debuggability.

### Trade-offs Accepted

1. **Four new actions**: More implementation work, but each action is focused and testable.

2. **Two recipes for simple cases**: This is overhead that pays off with reuse and maintainability.

3. **RPATH complexity**: Cross-platform RPATH handling (patchelf vs install_name_tool + codesign) adds complexity, but the security benefits justify it.

## Solution Architecture

### Overview

The library dependency system consists of these components:

1. **Library Recipes**: TOML files with `type = "library"` that define how to obtain and install libraries
2. **homebrew Action**: Downloads and extracts Homebrew bottles with runtime version resolution
3. **install_libraries Action**: Copies libraries and preserves symlink structure
4. **link_dependencies Action**: Creates symlinks from tool lib/ to shared libs/
5. **set_rpath Action**: Cross-platform RPATH modification
6. **Homebrew Version Provider**: Queries Homebrew API for available versions
7. **State Tracking**: `used_by` tracking in state.json

```
tsuku install ruby
  |
  +-- Parse ruby.toml
  |     \-- dependencies = ["libyaml"]
  |
  +-- Resolve libyaml
  |     +-- Already installed at compatible version? -> reuse
  |     \-- Not installed? -> install libyaml.toml first
  |
  +-- Install libyaml
  |     \-- -> $TSUKU_HOME/libs/libyaml-0.2.5/
  |
  +-- Install ruby
  |     \-- -> $TSUKU_HOME/tools/ruby-3.4.0/
  |
  +-- Link dependencies
  |     \-- symlink: ruby-3.4.0/lib/libyaml.so.2 -> libs/libyaml-0.2.5/lib/libyaml.so.2
  |
  +-- Set RPATH (or create wrapper)
  |     \-- patchelf --set-rpath '$ORIGIN/../lib' ruby-3.4.0/bin/ruby
  |
  \-- Track state
        \-- state.json: libyaml-0.2.5 used_by: [ruby-3.4.0]
```

### Directory Structure

```
$TSUKU_HOME/
+-- bin/                               # User's PATH
|   \-- ruby -> ../tools/ruby-3.4.0/bin/ruby
+-- libs/                              # Shared libraries (versioned)
|   +-- libyaml-0.2.5/
|   |   \-- lib/
|   |       +-- libyaml.so.2 -> libyaml.so.2.0.9
|   |       \-- libyaml.so.2.0.9
|   \-- openssl-3.0.0/
|       \-- lib/...
+-- tools/                             # Installed tools
|   \-- ruby-3.4.0/
|       +-- bin/ruby                   # RPATH: $ORIGIN/../lib
|       \-- lib/
|           \-- libyaml.so.2 -> ../../../libs/libyaml-0.2.5/lib/libyaml.so.2
\-- state.json
```

### Key Design Constraints

#### Library Recipe Format

Library recipes use `type = "library"` in metadata and reference a Homebrew formula for version resolution:

```toml
# libyaml.toml
[metadata]
name = "libyaml"
description = "YAML 1.1 parser and emitter library"
type = "library"

[version]
source = "homebrew"
formula = "libyaml"

[[steps]]
action = "homebrew"
formula = "libyaml"

[[steps]]
action = "install_libraries"
patterns = ["lib/*.so*", "lib/*.dylib"]
```

#### Action Responsibilities

| Action | Purpose | Key Constraints |
|--------|---------|-----------------|
| `homebrew` | Download and extract Homebrew bottles | Must relocate `@@HOMEBREW_PREFIX@@` placeholders; verify SHA256 from GHCR manifest |
| `install_libraries` | Copy libraries to shared location | Must preserve symlink structure (don't resolve symlinks) |
| `link_dependencies` | Create symlinks from tool to shared libs | Must check for file collisions before overwriting |
| `set_rpath` | Modify binary RPATH | Must strip existing RPATH before setting new value; use `$ORIGIN/../lib` not bare `$ORIGIN` |

#### State Tracking

The `libs` section in state.json tracks library versions and which tools use them:

```json
{
  "libs": {
    "libyaml": {
      "0.2.5": { "used_by": ["ruby-3.4.0", "python-3.12"] }
    }
  }
}
```

This enables garbage collection when tools are removed.

### Security Considerations

#### RPATH Security

Security best practices for RPATH:
- Never use bare `$ORIGIN` (allows library injection if binary is copied)
- Always use subdirectory: `$ORIGIN/../lib`
- tsuku controls `$TSUKU_HOME/` (user-private, mode 700)
- Always inspect and strip existing RPATH values before setting new ones (prevents malicious bottles from injecting attacker-controlled paths)

#### Download Verification

**Homebrew Bottle Downloads:**
- **Source:** GHCR (ghcr.io), operated by GitHub
- **Verification:** SHA256 from GHCR manifest annotations verified against downloaded content
- **Trust model:** GHCR manifest annotations are not cryptographically signed by Homebrew; integrity relies on HTTPS transport security. This is the same trust model as using Homebrew directly.
- **Failure behavior:** Installation aborts if checksum doesn't match

**Homebrew API Queries:**
- **Source:** formulae.brew.sh (official Homebrew API)
- **Transport:** HTTPS only
- **Validation:** JSON response structure validated before parsing

#### Placeholder Relocation

**Path length validation:**
- `@@HOMEBREW_PREFIX@@` placeholder is exactly 19 characters
- Replacement path must be <= 19 characters or use symlink workaround
- Binary relocation uses null-padding to preserve file structure

**Path sanitization:**
- Validate `$TSUKU_HOME` is an absolute path without `..` components
- Reject paths containing shell metacharacters (`;`, `|`, `&`, `$`, backticks)
- Use atomic file operations (write to temp, verify, rename) for binary patching

#### Execution Isolation

- **No system paths:** Libraries written only to `$TSUKU_HOME/libs/`
- **No elevated privileges:** All operations run as current user
- **Limited code execution:** Don't run Homebrew postinst scripts. However, libraries execute when loaded by tools. Relocation process performs binary patching but does not execute downloaded code.

#### Supply Chain Risks

- **Trust model:** Trust Homebrew maintainers and GitHub's GHCR infrastructure
- **Compromise scenario:** If Homebrew or GHCR compromised, malicious libraries could be distributed
- **Detection:** No built-in detection mechanism; relies on community detection of Homebrew compromises
- **Future mitigation:** Checksum pinning (record first-install checksums, alert on changes) could be added

#### User Data Exposure

- Formula names sent to formulae.brew.sh and ghcr.io
- Platform and architecture information implicit in bottle selection
- Same privacy model as using Homebrew directly
- Installation paths are not transmitted externally

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious bottle | SHA256 verification from GHCR manifest | Compromise of Homebrew/GHCR |
| Library injection via RPATH | Strip existing RPATH; use `$ORIGIN/../lib` | Local privilege escalation |
| Man-in-the-middle | HTTPS for all connections | Compromised CA certificates |
| Path traversal | Validate extraction paths | Novel traversal patterns |
| Placeholder overflow | Validate path length <= 19 chars | Symlink workaround complexity |
| Library collisions | Check for existing files before symlinking | Edge cases in collision detection |

## Implementation Approach

### Phase 1: Dependency Model Foundation

- Add `type` field to recipe metadata schema
- Add `$TSUKU_HOME/libs/` directory to config
- Implement library dependency resolution in installer
- Add `used_by` tracking in state.json

### Phase 2: RPATH Utilities

- Cross-platform RPATH inspection and modification
- Binary format detection (ELF/Mach-O)
- `set_rpath` action implementation
- Platform-specific: patchelf for Linux, install_name_tool for macOS

**Rationale:** RPATH utilities are foundational for both library installation and homebrew. Moving them earlier reduces rework.

### Phase 3: Library Installation Actions

- `install_libraries` action: copy libraries preserving symlinks
- `link_dependencies` action: create tool-to-lib symlinks with collision detection

### Phase 4: Homebrew Integration

- Homebrew version provider for `source = "homebrew"`
- `homebrew` action: GHCR auth, download, extract, placeholder relocation

### Phase 5: Ruby Recipe Migration

- Create `libyaml.toml` library recipe
- Update `ruby.toml` to use `dependencies = ["libyaml"]`
- Remove inline shell script from ruby.toml

### Phase 6: User Experience Polish

- Prevent direct library installation with helpful error message
- Hide libraries from `tsuku list` (or show with marker)

## Consequences

### Positive

1. **Maintainability:** Library updates isolated to library recipes. Runtime version resolution eliminates hardcoded SHAs.

2. **Reusability:** Multiple tools share the same library recipe and installation.

3. **Security:** RPATH provides per-binary library resolution immune to environment manipulation.

4. **State tracking:** `used_by` enables garbage collection and version coexistence.

5. **Consistency:** Library handling uses tsuku's action system and dependency model.

6. **Foundation for expansion:** Pattern extends to other libraries (openssl, readline, libffi).

### Negative

1. **Implementation scope:** Four new actions plus version provider is significant work.

2. **RPATH complexity:** Cross-platform handling (patchelf vs install_name_tool + codesign).

3. **Homebrew dependency:** Relies on Homebrew infrastructure availability.

### Mitigations

1. **Implementation scope:** Each action is focused and independently testable.

2. **RPATH complexity:** Wrapper scripts serve as fallback for edge cases.

3. **Homebrew dependency:** Document the dependency; Homebrew is stable and widely used.

## Implementation Issues

### Milestone: [Relocatable Library Dependencies](https://github.com/tsukumogami/tsuku/milestone/10)

**Foundation (no dependencies):**
- [#214](https://github.com/tsukumogami/tsuku/issues/214): feat(recipe): add type field to recipe metadata schema
- [#215](https://github.com/tsukumogami/tsuku/issues/215): feat(config): add libs directory to tsuku home
- [#216](https://github.com/tsukumogami/tsuku/issues/216): feat(state): add libs section with used_by tracking
- [#217](https://github.com/tsukumogami/tsuku/issues/217): feat(action): implement set_rpath action
- [#218](https://github.com/tsukumogami/tsuku/issues/218): feat(version): implement homebrew version provider

**Actions (blocked by foundation):**
- [#219](https://github.com/tsukumogami/tsuku/issues/219): feat(action): implement install_libraries action
- [#220](https://github.com/tsukumogami/tsuku/issues/220): feat(action): implement link_dependencies action
- [#221](https://github.com/tsukumogami/tsuku/issues/221): feat(install): resolve library dependencies during tool installation
- [#222](https://github.com/tsukumogami/tsuku/issues/222): feat(action): implement homebrew action

**Migration (blocked by actions):**
- [#223](https://github.com/tsukumogami/tsuku/issues/223): feat(recipe): add libyaml library recipe
- [#224](https://github.com/tsukumogami/tsuku/issues/224): feat(recipe): migrate recipes to use library dependencies

**UX Polish (blocked by foundation):**
- [#225](https://github.com/tsukumogami/tsuku/issues/225): feat(cli): prevent direct library installation
- [#226](https://github.com/tsukumogami/tsuku/issues/226): feat(cli): filter libraries from tsuku list output

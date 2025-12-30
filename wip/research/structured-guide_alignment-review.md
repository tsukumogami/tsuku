# Alignment Review: Structured Install Guide vs System Dependency Actions

## Summary

This assessment compares `DESIGN-structured-install-guide.md` (the "structured guide") against `DESIGN-system-dependency-actions.md` (the "action vocabulary") to identify divergences and recommend alignment.

**Key finding**: The two designs evolved separately and now contradict each other in recipe syntax, scope, and content organization. The action vocabulary design should be authoritative as it represents the more recent, refined thinking.

---

## 1. Recipe Syntax Divergence

### Current State

The two designs propose fundamentally different recipe syntax:

**Structured Guide approach** - primitives nested inside `require_system`:

```toml
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }

# Or complex form with primitives array
[[steps]]
action = "require_system"
command = "docker"
primitives = [
  { apt_repo = { ... } },
  { apt = ["docker-ce"] },
  { group_add = { group = "docker" } },
]
when = { os = ["linux"] }
```

**Action Vocabulary approach** - separate action steps:

```toml
[[steps]]
action = "apt_repo"
url = "https://download.docker.com/linux/ubuntu"
key_url = "..."
key_sha256 = "..."
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "apt_install"
packages = ["docker-ce", "docker-ce-cli", "containerd.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "group_add"
group = "docker"
when = { os = ["linux"] }

[[steps]]
action = "require_command"
command = "docker"
```

### Analysis

| Aspect | Structured Guide | Action Vocabulary |
|--------|-----------------|-------------------|
| Granularity | Monolithic (`require_system` with embedded content) | Composable (one action per operation) |
| Schema complexity | Polymorphic (`packages` OR `primitives` nested inside) | Typed (each action has own schema) |
| Platform filtering | `when` at step level (good) | `when` at step level + new `distro` field (better) |
| Consistency with other actions | Low (embeds platform-specific content in generic wrapper) | High (follows same pattern as `download`, `extract`) |

### Recommendation

**The action vocabulary syntax should be canonical.**

Rationale from action vocabulary design (D1):

> "Each action has exactly one schema, making validation straightforward"
> "Creates polymorphic schemas where valid fields depend on which operation is intended" [rejected]

The structured guide's `packages = { apt = [...] }` recreates the original problem of "generic containers with polymorphic content" that the action vocabulary explicitly rejects.

### Specific Changes Needed in Structured Guide

1. **Remove `packages` parameter** from `require_system` - it embeds platform-specific content

2. **Remove `primitives` array** from `require_system` - replace with separate action steps

3. **Replace all recipe examples** using the action vocabulary syntax:

   **Before** (structured guide):
   ```toml
   [[steps]]
   action = "require_system"
   command = "docker"
   packages = { apt = ["docker.io"] }
   when = { os = ["linux"] }
   ```

   **After** (action vocabulary):
   ```toml
   [[steps]]
   action = "apt_install"
   packages = ["docker.io"]
   when = { distro = ["ubuntu", "debian"] }

   [[steps]]
   action = "require_command"
   command = "docker"
   ```

4. **Update "Parameter Schema" section** (lines 408-436) to reflect that `require_system` is replaced by:
   - Package installation actions (`apt_install`, `brew_cask`, etc.)
   - `require_command` for verification

---

## 2. Scope Misalignment

### Current State

**Action vocabulary design** clearly separates scope:

| Use Case | Scope |
|----------|-------|
| Documentation Generation | This design |
| Sandbox Container Building | This design |
| Host Execution | **Future design** |

**Structured guide design** includes host execution in current scope:

> "Phase 4: User Consent and Host Execution"
> 1. Implement user consent flow (display primitives, confirm)
> 2. Add `--system-deps` flag to `tsuku install` to enable host execution
> 3. Implement privilege escalation (sudo) for host execution

### Analysis

The action vocabulary explicitly states:

> "Current scope: This design focuses on documentation generation and sandbox container building. These features require machine-readable recipes but do NOT execute privileged operations on the user's host."
>
> "Future scope: Host execution (where tsuku actually runs `apt-get install`, etc. on the user's machine) requires additional design work covering UX, consent flows, and security constraints."

Phase 4 of the structured guide directly contradicts this by treating host execution as current scope.

### Sections Needing Scope Adjustment

1. **Phase 4: User Consent and Host Execution** (lines 771-777)
   - Should be moved to "Future Work" section
   - Should reference that a separate design is needed

2. **User Consent Flow section** (lines 519-539)
   - Shows `tsuku install docker` executing privileged operations
   - Should be rewritten to show documentation generation instead

3. **Primitive Execution section** (lines 485-517)
   - Shows `Execute(ctx *ExecutionContext)` method
   - Should clarify this is for sandbox context only (current scope)
   - Host execution interface deferred to future design

4. **Security Considerations - Host context** (lines 825-830)
   - Describes primitives executing via sudo
   - Should be moved to "Future Work" or prefaced with "When host execution is implemented..."

5. **Execution Isolation - Host context** (lines 825-830)
   - Same issue - describes future functionality as current

### Recommendation

**Phase 4 should be moved to Future Work with explicit reference to the action vocabulary design's future scope.**

The structured guide should focus on:
1. Refactoring `require_system` to use action vocabulary
2. Documentation generation from structured actions
3. Sandbox container building from extracted packages

---

## 3. Duplicate Content

### Current State

Both designs define primitive/action vocabularies:

**Structured Guide - "Primitive Vocabulary" section** (lines 438-465):

| Primitive | Parameters | Privilege | Description |
|-----------|------------|-----------|-------------|
| `apt` | packages: []string | sudo | Install Debian/Ubuntu packages |
| `apt_repo` | url, key_url, key_sha256 | sudo | Add APT repository with GPG key |
| `brew` | packages: []string | user | Install Homebrew formulae |
| `brew_cask` | packages: []string | user | Install Homebrew casks |
| `group_add` | group: string | sudo | Add current user to group |
| `service_enable` | service: string | sudo | Enable systemd service |
| `manual` | text: string | none | Display instructions |

**Action Vocabulary - "Action Vocabulary" section** (lines 242-274):

| Action | Fields | Description |
|--------|--------|-------------|
| `apt_install` | packages, fallback? | Install Debian/Ubuntu packages |
| `apt_repo` | url, key_url, key_sha256 | Add APT repository with GPG key |
| `apt_ppa` | ppa | Add Ubuntu PPA |
| `brew_install` | packages, tap?, fallback? | Install Homebrew formulae |
| `brew_cask` | packages, tap?, fallback? | Install Homebrew casks |
| `group_add` | group | Add current user to group |
| `service_enable` | service | Enable systemd service |
| `require_command` | command, version_flag?, etc. | Verify command exists |
| `manual` | text | Display instructions |

### Analysis

The action vocabulary design is more complete:
- Adds `apt_ppa` (missing from structured guide)
- Adds `pacman_install` (missing from structured guide)
- Adds `require_command` (separated from `require_system`)
- Includes `fallback` and `tap` optional fields
- Uses consistent naming (`apt_install` vs `apt`, `brew_install` vs `brew`)

### Recommendation

**The action vocabulary design should be authoritative for the action/primitive vocabulary.**

The structured guide should:
1. Remove its "Primitive Vocabulary" section entirely
2. Reference the action vocabulary design for the canonical vocabulary
3. Focus on container building mechanics, which is its unique contribution

---

## 4. Missing Alignment

### Sections in Action Vocabulary Missing from Structured Guide

1. **Scope section** - Structured guide lacks the clear scope table:
   ```
   | Use Case | Description | Scope |
   |----------|-------------|-------|
   | Documentation Generation | ... | This design |
   | Sandbox Container Building | ... | This design |
   | Host Execution | ... | Future design |
   ```

2. **Distro detection** - Action vocabulary adds `distro` to `when` clause:
   ```toml
   when = { distro = ["ubuntu", "debian"] }
   ```
   Structured guide uses only `os`:
   ```toml
   when = { os = ["linux"] }
   ```
   This is less precise (Linux has many distros with different package managers).

3. **Documentation Generation section** - Action vocabulary has explicit section (lines 276-328) showing:
   - `Describe()` method signature
   - Example output format
   - CLI behavior when deps are missing

   Structured guide mentions "Human-Readable Text Generation" (lines 543-562) but less complete.

4. **Require semantics** - Action vocabulary decision D3 clarifies:
   - Package managers are idempotent
   - Use `require_command` as final verification
   - Optional `unless_command` escape hatch

5. **Fallback field** - Action vocabulary decision D5 adds `fallback` field to install actions:
   ```toml
   [[steps]]
   action = "apt_install"
   packages = ["nvidia-cuda-toolkit"]
   fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
   ```

6. **WhenClause Extension** - Action vocabulary defines the `distro` field extension (lines 354-390)

### Recommendation

The structured guide should add or reference:
1. A "Scope" section matching the action vocabulary's scope table
2. Reference to `distro` field for Linux package manager filtering
3. Reference to action vocabulary's `Describe()` pattern for documentation generation
4. The `require_command` / `require_system` separation pattern

---

## 5. Specific Change Recommendations

### Section-by-Section Changes

#### Title and Scope
**Change**: Update title to reflect narrower focus.

**Before**: "DESIGN: Structured Primitives for System Dependencies"

**After**: "DESIGN: Sandbox Container Building for System Dependencies"

**Rationale**: The action vocabulary now owns the "structured primitives" concept. This design's unique value is container building.

#### Add Scope Section (after Status)

**Add new section**:
```markdown
## Scope

This design implements sandbox container building for recipes with system dependencies.

| Use Case | Scope |
|----------|-------|
| Action vocabulary (what actions exist) | [DESIGN-system-dependency-actions.md](DESIGN-system-dependency-actions.md) |
| Container building (how to build sandbox containers) | This design |
| Host execution (running actions on user's machine) | Future design |

This design depends on the action vocabulary for the machine-readable format.
```

#### Update "Step Structure" Section (lines 351-404)

**Before**:
```toml
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }
```

**After**:
```toml
# Uses action vocabulary from DESIGN-system-dependency-actions.md
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "require_command"
command = "docker"
```

#### Remove "Parameter Schema" Section (lines 408-436)

This section defines `require_system` parameters which are superseded by the action vocabulary.

**Replace with reference**:
```markdown
### Action Schema

See [DESIGN-system-dependency-actions.md](DESIGN-system-dependency-actions.md) for the canonical action vocabulary including:
- Package installation actions (`apt_install`, `brew_cask`, `dnf_install`, etc.)
- System configuration actions (`group_add`, `service_enable`)
- Verification (`require_command`)
- Fallback (`manual`)
```

#### Remove "Primitive Vocabulary" Section (lines 438-466)

**Rationale**: Duplicates action vocabulary design and uses different naming (`apt` vs `apt_install`).

#### Update "Sandbox Executor Changes" Section (lines 600-668)

**Before**:
```go
// Check for packages (simple form)
if packages, ok := step.Params["packages"]; ok {
    for manager, pkgs := range parsePackages(packages) {
        spec.Packages[manager] = append(spec.Packages[manager], pkgs...)
    }
}
```

**After**:
```go
// Extract packages from typed action steps
switch step.Action {
case "apt_install":
    spec.Packages["apt"] = append(spec.Packages["apt"], step.Packages...)
case "brew_install", "brew_cask":
    spec.Packages["brew"] = append(spec.Packages["brew"], step.Packages...)
case "dnf_install":
    spec.Packages["dnf"] = append(spec.Packages["dnf"], step.Packages...)
}
```

This aligns with the `ExtractPackages()` example in the action vocabulary design (lines 334-350).

#### Move Phase 4 to Future Work

**Before** (in Implementation Approach):
```markdown
### Phase 4: User Consent and Host Execution

1. Implement user consent flow (display primitives, confirm)
2. Add `--system-deps` flag to `tsuku install` to enable host execution
...
```

**After** (in Future Work):
```markdown
### Host Execution

Host execution is out of scope for this design. See [DESIGN-system-dependency-actions.md](DESIGN-system-dependency-actions.md) "Future Work" section for:
- UX considerations (consent flow, progress display, error recovery)
- Security constraints (group allowlisting, repository allowlisting, tiered consent)
- Required design work before implementation
```

#### Update "User Consent Flow" Section (lines 519-539)

**Before**: Shows `tsuku install docker` executing privileged operations.

**After**: Show documentation generation output:
```markdown
### Documentation Generation Output

When `tsuku install docker` runs and Docker is not installed:

```
$ tsuku install docker

Docker requires system dependencies that tsuku cannot install directly.

For Ubuntu/Debian:

  1. Install packages:
     sudo apt-get install docker.io

  2. Add yourself to docker group:
     sudo usermod -aG docker $USER

After completing these steps, run: tsuku install docker --verify
```

This output is generated from the structured actions using each action's `Describe()` method.
```

#### Rename Implementation Phases

**Before**:
- Phase 1: Refactor require_system Action
- Phase 2: Primitive Framework
- Phase 3: Sandbox Execution
- Phase 4: User Consent and Host Execution
- Phase 5: Extension

**After**:
- Phase 1: Adopt Action Vocabulary (migrate from require_system to action steps)
- Phase 2: Documentation Generation (implement Describe() methods)
- Phase 3: Sandbox Container Building (minimal base image, container caching)
- Phase 4: Extension (additional package managers as needed)

Host execution becomes a separate milestone dependent on a dedicated design.

---

## 6. Relationship Section Update

The structured guide has a brief mention in Future Work about the relationship. The action vocabulary design has a clearer "Relationship to Original Design" section (lines 546-559).

**Recommendation**: Add or update a "Relationship" section in the structured guide:

```markdown
## Relationship to Action Vocabulary Design

This design and [DESIGN-system-dependency-actions.md](DESIGN-system-dependency-actions.md) are complementary:

| Design | Focus |
|--------|-------|
| Action Vocabulary | What actions exist, how they compose, platform filtering |
| This Design | Container building, sandbox execution, caching |

**Dependencies:**
- This design consumes the action vocabulary for recipe parsing
- Action vocabulary defines `Describe()` interface; this design uses it for documentation
- Action vocabulary defines package actions; this design extracts packages for container building

**Implementation order:**
1. Action vocabulary: Define actions with schemas and `Describe()` methods
2. This design: Implement container building using parsed action data
```

---

## Summary of Required Changes

| Section | Action | Reason |
|---------|--------|--------|
| Title | Rename to focus on container building | Action vocabulary owns primitives concept |
| Scope | Add scope table matching action vocabulary | Clarify boundaries |
| Step Structure | Replace with action vocabulary syntax | Syntax divergence |
| Parameter Schema | Remove or replace with reference | Duplicate content |
| Primitive Vocabulary | Remove entirely | Duplicate content |
| Sandbox Executor Changes | Update to use typed actions | Syntax alignment |
| User Consent Flow | Rewrite as documentation output | Scope misalignment |
| Phase 4 (Implementation) | Move to Future Work | Scope misalignment |
| Security (Host context) | Move to Future Work or prefix with "Future:" | Scope misalignment |
| Relationship | Add explicit relationship section | Clarify design dependencies |

---

## Conclusion

The two designs have significant divergences that should be reconciled. The action vocabulary design represents the more refined thinking and should be authoritative for:
- Recipe syntax (separate action steps, not nested primitives)
- Action vocabulary (naming, parameters, optional fields)
- Scope boundaries (host execution is future scope)

The structured guide should be refocused on its unique contribution: container building mechanics, caching strategy, and sandbox execution. It should consume and reference the action vocabulary rather than duplicate or contradict it.

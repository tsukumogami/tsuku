# Final Review: System Dependency Design Documents Consistency

## Documents Reviewed

- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/DESIGN-system-dependency-actions.md` (Action Vocabulary)
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/DESIGN-structured-install-guide.md` (Sandbox Container Building)

---

## 1. Cross-Reference Accuracy

### 1.1 Links Between Documents

| Source Document | Line | Link | Target | Status |
|-----------------|------|------|--------|--------|
| system-dependency-actions.md | 352 | `[DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md)` | Container building details | Valid |
| system-dependency-actions.md | 476 | `[DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md)` | Container building details | Valid |
| system-dependency-actions.md | 547 | `[DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md)` | Relationship section | Valid |
| structured-install-guide.md | 9 | `[DESIGN-system-dependency-actions.md](DESIGN-system-dependency-actions.md)` | Action vocabulary | Valid |
| structured-install-guide.md | 12-15 | Multiple references | Action vocabulary sections | Valid |
| structured-install-guide.md | 24 | `[DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md)` | Upstream design | **Unverified** - file not read |

### 1.2 Section Anchor Validity

| Source | Line | Anchor Reference | Status |
|--------|------|------------------|--------|
| structured-install-guide.md | 20 | `DESIGN-system-dependency-actions.md#host-execution` | **Invalid** - Section is "Future Work" with "Host Execution" as subsection, anchor would be `#host-execution` which is not a direct header |
| structured-install-guide.md | 516 | `DESIGN-system-dependency-actions.md#documentation-generation` | **Valid** - Direct H2 header "## Documentation Generation" at line 276 |
| structured-install-guide.md | 545 | `DESIGN-system-dependency-actions.md#documentation-generation` | **Valid** - Same as above |
| structured-install-guide.md | 549 | `DESIGN-system-dependency-actions.md#future-work` | **Valid** - Direct H2 header "## Future Work" at line 479 |
| structured-install-guide.md | 761 | `DESIGN-system-dependency-actions.md` (no anchor) | Valid |
| structured-install-guide.md | 790 | `DESIGN-system-dependency-actions.md#host-execution` | **Uncertain** - "Host Execution" is H3 under "Future Work" - anchor may or may not work depending on renderer |
| structured-install-guide.md | 824 | `DESIGN-system-dependency-actions.md#host-execution` | Same issue |
| structured-install-guide.md | 837 | `DESIGN-system-dependency-actions.md#host-execution` | Same issue |
| structured-install-guide.md | 847 | `DESIGN-system-dependency-actions.md#composite-shorthand-syntax` | **Invalid** - Section is "### Composite Shorthand Syntax" at line 512, anchor would be `#composite-shorthand-syntax` but it's under "Future Work" |
| structured-install-guide.md | 862 | `DESIGN-system-dependency-actions.md#d2-distro-detection` | **Valid** - Direct H3 header "### D2: Distro Detection" at line 94 |
| structured-install-guide.md | 888 | `DESIGN-system-dependency-actions.md#host-execution` | Same issue as above |

**Recommendation**: The anchor `#host-execution` should work since GitHub generates anchors from headings regardless of nesting level. However, the inconsistency should be verified by testing the actual rendered links.

### 1.3 Decision References (D1, D2, etc.)

| Reference | Context | Accuracy |
|-----------|---------|----------|
| D1 | Action Granularity | Correctly defined at line 74-91 |
| D2 | Distro Detection | Correctly defined at line 94-139 |
| D3 | Require Semantics | Correctly defined at line 143-178 |
| D4 | Post-Install Configuration | Correctly defined at line 182-211 |
| D5 | Manual/Fallback Instructions | Correctly defined at line 213-240 |

All decision references in the action vocabulary design are internally consistent. The structured-install-guide.md does not reference specific decision IDs directly, instead using descriptive references like "per the action vocabulary design" which is appropriate.

---

## 2. Terminology Consistency

### 2.1 "Action" vs "Primitive"

**Finding: Inconsistent terminology**

| Document | Lines | Term Used | Context |
|----------|-------|-----------|---------|
| system-dependency-actions.md | Throughout | "action" | Consistent use of "action" |
| structured-install-guide.md | 194-217 | "primitives" | Option 2B description uses `primitives = [...]` |
| structured-install-guide.md | 217-229 | "primitives" | "No shell primitive" |
| structured-install-guide.md | 328 | "typed actions" | Decision outcome |

**Issue**: In structured-install-guide.md lines 194-217, the term "primitives" is used in code examples:

```toml
primitives = [
  { apt_repo = { url = "...", key_url = "...", key_sha256 = "..." } },
  { apt = ["docker-ce", "docker-ce-cli", "containerd.io"] },
  ...
]
```

This uses `apt` instead of `apt_install` and embeds actions inside a `primitives` array rather than as separate steps. However, the "Decision Outcome" section at line 328 states:

> "We replace `install_guide` and the polymorphic `require_system` with typed actions (`apt_install`, `brew_cask`, `require_command`, etc.)"

The "Considered Options" section (lines 173-231) presents Option 2B using `primitives` syntax, but the rest of the document uses the separate-step pattern from action-vocabulary design. This is a **documentation artifact** - the old option description should be updated to reflect the chosen design.

**Recommendation**: Update lines 194-217 in structured-install-guide.md to use the term "actions" consistently, and update the syntax to match the chosen pattern (separate steps with `action = "apt_install"` etc.).

### 2.2 "Distro" vs "OS"

| Document | Usage | Status |
|----------|-------|--------|
| Both | `when = { distro = ["ubuntu", "debian"] }` | Consistent |
| Both | `when = { os = ["linux"] }` | Consistent |
| Both | `when = { os = ["darwin"] }` | Consistent |

**Finding**: Terminology is consistent. Both documents correctly use:
- `distro` for Linux distribution filtering (ubuntu, debian, fedora)
- `os` for operating system level filtering (linux, darwin)

The distinction is well-documented in system-dependency-actions.md lines 136-139:
> - `Distro` and `OS` are mutually exclusive
> - `Distro` and `Platform` are mutually exclusive
> - If `Distro` is set, step only runs on Linux

### 2.3 Action Names

| Canonical Name (actions doc) | Used in container doc | Status |
|------------------------------|----------------------|--------|
| `apt_install` | `apt_install` | Consistent |
| `apt_repo` | `apt_repo` | Consistent |
| `apt_ppa` | Not mentioned | - |
| `dnf_install` | `dnf_install` | Consistent |
| `dnf_repo` | `dnf_repo` | Consistent |
| `brew_install` | `brew_install` | Consistent |
| `brew_cask` | `brew_cask` | Consistent |
| `pacman_install` | `pacman_install` | Consistent |
| `group_add` | `group_add` | Consistent |
| `service_enable` | `service_enable` | Consistent |
| `service_start` | Not used in examples | - |
| `require_command` | `require_command` | Consistent |
| `manual` | `manual` | Consistent |

**Issue Found**: In structured-install-guide.md line 210, Option 2B shows:
```toml
{ apt = ["docker-ce", ...] }
```

This uses `apt` instead of `apt_install`. This is inconsistent with the action vocabulary which defines `apt_install` (line 247).

**Recommendation**: The Option 2B example at line 210 should use `apt_install` to match the canonical vocabulary, or add a note that this is the rejected shorthand syntax.

---

## 3. Scope Boundary Clarity

### 3.1 Document Ownership

| Concern | Owner | Clear? |
|---------|-------|--------|
| Action vocabulary (what actions exist) | system-dependency-actions.md | Yes (lines 241-275) |
| Action schemas and validation | system-dependency-actions.md | Yes (implied by D1) |
| Platform filtering semantics (`when` clause) | system-dependency-actions.md | Yes (D2, lines 94-139) |
| Documentation generation (`Describe()`) | system-dependency-actions.md | Yes (lines 278-328) |
| Container building | structured-install-guide.md | Yes (lines 559-661) |
| Container caching | structured-install-guide.md | Yes (lines 645-661) |
| Package extraction logic | structured-install-guide.md | Yes (lines 596-637) |

The scope table at lines 10-17 of structured-install-guide.md is clear:

| Concern | Design |
|---------|--------|
| Action vocabulary | DESIGN-system-dependency-actions.md |
| Platform filtering | DESIGN-system-dependency-actions.md |
| Documentation generation | DESIGN-system-dependency-actions.md |
| **Container building, caching, sandbox execution** | **This design** |

### 3.2 Overlapping Responsibilities

**Finding: Minor overlap in implementation approach**

Both documents describe implementation phases, but with different granularity:

**system-dependency-actions.md** (lines 442-476):
- Phase 1: Infrastructure (distro detection)
- Phase 2: Action Vocabulary
- Phase 3: Documentation Generation
- Phase 4: Sandbox Integration

**structured-install-guide.md** (lines 718-748):
- Phase 1: Adopt Action Vocabulary
- Phase 2: Documentation Generation
- Phase 3: Sandbox Container Building
- Phase 4: Extension

**Issue**: Phase numbering differs between documents. structured-install-guide.md Phase 1 corresponds to actions.md Phases 1-2. This could cause confusion during implementation.

**Recommendation**: Add a mapping note in structured-install-guide.md stating: "This design depends on phases 1-3 of DESIGN-system-dependency-actions.md being complete." This is partially stated at line 720 but could be more explicit.

### 3.3 Gaps

**Finding: One potential gap identified**

Neither document clearly addresses:
1. **Error handling for missing distro**: What happens when a recipe requires `distro = ["ubuntu"]` but runs on Arch Linux?

system-dependency-actions.md line 133-134 mentions:
> If `/etc/os-release` is missing or distro is unknown, steps with `distro` conditions are skipped. Fallback `manual` actions can guide users.

But structured-install-guide.md doesn't mention this scenario in the sandbox context. In sandbox mode with a minimal container, what base OS is used? The Dockerfile example at line 562-568 shows `FROM scratch` but doesn't specify how apt-based actions would work.

**Recommendation**: Add clarification in structured-install-guide.md that sandbox containers for apt-based recipes use a Debian/Ubuntu base layer, while the "minimal" aspect refers to removing pre-installed packages rather than the OS itself.

---

## 4. Example Consistency

### 4.1 Docker Examples Syntax

| Document | Example Location | Syntax Pattern | Consistent? |
|----------|------------------|----------------|-------------|
| actions.md | lines 396-438 | Separate `[[steps]]` with `action = "..."` | Yes |
| container.md | lines 377-428 | Separate `[[steps]]` with `action = "..."` | Yes |
| container.md | lines 194-217 | **`primitives = [...]` array** | **No** |

**Issue**: The Option 2B example in structured-install-guide.md uses a different syntax that was rejected:

```toml
# From container doc, lines 206-217 (rejected option)
primitives = [
  { apt_repo = { url = "...", key_url = "...", key_sha256 = "..." } },
  { apt = ["docker-ce", "docker-ce-cli", "containerd.io"] },
  { group_add = { group = "docker" } },
  { service_enable = "docker" },
]
```

vs the chosen pattern:

```toml
# From actions doc, lines 396-438 (chosen pattern)
[[steps]]
action = "apt_repo"
url = "https://download.docker.com/linux/ubuntu"
key_url = "https://download.docker.com/linux/ubuntu/gpg"
key_sha256 = "1500c1f..."
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "apt_install"
packages = ["docker-ce", "docker-ce-cli", "containerd.io"]
when = { distro = ["ubuntu", "debian"] }
```

This is because the "Considered Options" section preserved the rejected syntax for historical context. However, it may confuse readers.

**Recommendation**: Either remove the detailed code examples from rejected options, or add a clear "REJECTED" label and strike-through formatting.

### 4.2 When Clauses

Both documents use consistent `when` clause syntax:

```toml
when = { distro = ["ubuntu", "debian"] }
when = { os = ["darwin"] }
when = { os = ["linux"] }
```

**Finding**: Consistent across both documents.

### 4.3 Code Interface Consistency

**Action Interface Definition**

| Document | Interface | Methods |
|----------|-----------|---------|
| actions.md | lines 281-285 | `Describe() string` |
| container.md | lines 489-498 | `Preflight()`, `ExecuteInSandbox()`, `Describe()` |

**Issue**: The Action interface is more complete in structured-install-guide.md:

```go
// From container doc, lines 489-498
type Action interface {
    Preflight(params map[string]interface{}) *PreflightResult
    ExecuteInSandbox(ctx *SandboxContext) error
    Describe() string
}
```

vs actions doc (lines 281-285):
```go
type Action interface {
    Describe() string
}
```

This is actually appropriate - the actions doc focuses on the `Describe()` method for documentation generation (its scope), while the container doc adds `ExecuteInSandbox()` for sandbox execution (its scope). However, it may confuse readers seeing different interface definitions.

**Recommendation**: Add a note in actions.md line 281 stating that the interface shown is partial and focused on documentation generation, with the full interface defined in the container building design.

### 4.4 ExtractPackages Function

Both documents include `ExtractPackages()`:

**actions.md lines 336-350**:
```go
func ExtractPackages(plan *InstallationPlan) map[string][]string {
    packages := make(map[string][]string)
    for _, step := range plan.Steps {
        switch step.Action {
        case "apt_install":
            packages["apt"] = append(packages["apt"], step.Packages...)
        // ...
        }
    }
    return packages
}
```

**container.md lines 596-625**:
```go
func ExtractPackages(plan *executor.InstallationPlan) (map[string][]string, error) {
    packages := make(map[string][]string)
    hasSystemDeps := false
    // ... more complete implementation with error handling
    if !hasSystemDeps {
        return nil, nil
    }
    return packages, nil
}
```

**Issue**: The function signature differs:
- actions.md: `map[string][]string` (no error return)
- container.md: `(map[string][]string, error)` (with error return)

Also, parameter type differs:
- actions.md: `*InstallationPlan`
- container.md: `*executor.InstallationPlan`

**Recommendation**: The actions.md version is intentionally simplified (line 335 says "extracts package requirements from actions"). The container doc should be considered canonical for the actual implementation. Add a note in actions.md that the simplified version is illustrative only.

---

## 5. Specific Recommendations

### 5.1 High Priority (Inconsistencies)

| File | Line(s) | Issue | Fix |
|------|---------|-------|-----|
| structured-install-guide.md | 194-217 | Uses `primitives` and `apt` instead of `action` and `apt_install` | Update rejected option example to use consistent terminology, or add "REJECTED" header |
| structured-install-guide.md | 210 | `{ apt = [...] }` instead of `{ apt_install = [...] }` | Change to `apt_install` for consistency |
| system-dependency-actions.md | 281-285 | Partial Action interface | Add note that full interface is in container doc |

### 5.2 Medium Priority (Clarifications)

| File | Line(s) | Issue | Fix |
|------|---------|-------|-----|
| structured-install-guide.md | 562-568 | Minimal container uses `FROM scratch` but apt requires Debian/Ubuntu | Clarify that "minimal" means stripped of extra packages, not bare OS |
| structured-install-guide.md | 720 | Dependency on actions phases could be clearer | Add explicit mapping of which phases from actions.md must complete first |
| system-dependency-actions.md | 335-350 | `ExtractPackages` signature differs from container doc | Add note that this is illustrative; canonical implementation in container doc |

### 5.3 Low Priority (Polish)

| File | Line(s) | Issue | Fix |
|------|---------|-------|-----|
| structured-install-guide.md | 20, 790, 824, 837, 888 | Multiple references to `#host-execution` anchor | Verify anchor works with GitHub's renderer |
| structured-install-guide.md | 24 | Reference to DESIGN-golden-plan-testing.md | Verify file exists |
| Both | Various | Phase numbering differs | Consider aligning phase numbers or adding explicit mapping |

---

## Summary

The two design documents are well-structured and mostly consistent. The main issues are:

1. **Terminology inconsistency**: The "primitives" syntax in rejected Option 2B uses different terminology than the chosen design
2. **Interface fragmentation**: The Action interface is partially defined in one doc and fully in another
3. **Function signature differences**: `ExtractPackages` has different signatures between docs

These issues are minor and don't affect the correctness of the designs. The scope boundaries are clear, and both documents properly cross-reference each other for topics outside their scope.

**Overall Assessment**: The documents are ready for implementation with the minor fixes noted above.

# Design Document Clarity Review

## Documents Reviewed

- `docs/DESIGN-system-dependency-actions.md` - Action vocabulary for system dependencies
- `docs/DESIGN-structured-install-guide.md` - Sandbox container building

## Executive Summary

Both documents are well-structured and technically sound. The action vocabulary design clearly explains the "what" and "why" of the typed action approach. The sandbox container building design effectively explains how these actions enable container-based testing. However, there are opportunities to improve the recipe author experience through clearer migration guidance, a consolidated quick reference, and reduced cross-document navigation.

---

## 1. Recipe Author Experience

### 1.1 Can a recipe author understand how to write system dependency steps?

**Verdict: Yes, with some effort**

The action vocabulary design provides clear examples (Docker installation) that demonstrate the core pattern. A recipe author reading the design can understand:

- Actions are typed (e.g., `apt_install`, `brew_cask`)
- Platform filtering uses `when` clause with `os` or `distro`
- Post-install configuration uses separate actions (`group_add`, `service_enable`)
- Verification uses `require_command`

**Strengths:**

1. The Docker example (lines 396-438 in action vocabulary doc) is comprehensive
2. Decision D3 (Require Semantics) clearly explains idempotent behavior
3. The action vocabulary table (lines 245-274) provides a quick reference

**Gaps:**

1. **No minimal "hello world" example.** The Docker example is complex. Recipe authors would benefit from a simpler example showing just a single package install:
   ```toml
   # Simple system dependency: install curl
   [[steps]]
   action = "apt_install"
   packages = ["curl"]
   when = { distro = ["ubuntu", "debian"] }

   [[steps]]
   action = "brew_install"
   packages = ["curl"]
   when = { os = ["darwin"] }
   ```

2. **Field documentation is sparse.** The action vocabulary table lists fields but doesn't document their types or constraints. For example, `apt_repo` has `url, key_url, key_sha256` but no indication that all three are required.

3. **No error message examples.** Recipe authors don't know what happens when validation fails.

### 1.2 Is the typed action syntax clear?

**Verdict: Yes**

The `<manager>_<operation>` naming convention is intuitive:
- `apt_install`, `apt_repo`, `apt_ppa`
- `brew_install`, `brew_cask`
- `dnf_install`, `dnf_repo`

The decision to use separate actions per operation (D1: Action Granularity) results in a learnable pattern. Each action does one thing.

**One potential confusion:** The difference between `when = { os = ["linux"] }` and `when = { distro = ["ubuntu", "debian"] }` needs clearer explanation:

- `os` filters by operating system (darwin, linux, windows)
- `distro` filters by Linux distribution (ubuntu, debian, fedora)

The design states these are mutually exclusive but doesn't explain when to use which. Recommendation: Add a decision tree or guidelines:

```
Use `os = ["linux"]` when the action works on ALL Linux distributions
Use `distro = ["ubuntu", "debian"]` when the action is distribution-specific
```

### 1.3 Are the examples sufficient and correct?

**Verdict: Sufficient but could be expanded**

**Examples provided:**

1. Docker installation (comprehensive, multi-platform, multi-step)
2. Simple apt_install with unless_command escape hatch
3. Manual fallback for CUDA

**Missing examples:**

1. **Mixed recipe** - The sandbox container doc (lines 431-452) has a great "mixed recipe" example showing tsuku download on Linux and brew_install on macOS. This should be in the action vocabulary doc where recipe authors will look first.

2. **Tap usage for Homebrew** - The `brew_install` and `brew_cask` actions have a `tap?` field but no example showing how to use it.

3. **APT PPA example** - `apt_ppa` is listed but never demonstrated.

4. **Fallback field example** - The `fallback` field on install actions is mentioned but only with a fragment example.

---

## 2. Documentation Flow

### 2.1 Do the documents read well in sequence?

**Verdict: Mostly yes, but the split creates friction**

The logical reading order is:

1. Action vocabulary (defines what actions exist)
2. Sandbox container building (explains how actions enable testing)

**Issue: Scope confusion at the start**

Both documents open with a "Scope" section explaining what is and isn't covered. Reading them back-to-back, a reader encounters:

- Action vocabulary: "This design focuses on documentation generation and sandbox container building"
- Sandbox container: "This design addresses sandbox container building for recipes with system dependencies"

The overlap in scope statements is confusing. Recommendation: The action vocabulary doc should state more clearly upfront that it defines the vocabulary, while the sandbox doc defines how that vocabulary enables container building.

### 2.2 Is there unnecessary duplication?

**Verdict: Yes, significant duplication**

The following content appears in both documents:

1. **Docker example** - Nearly identical in both (lines 396-438 in action vocab, lines 375-428 in sandbox)

2. **ExtractPackages function** - Appears in both (lines 336-350 in action vocab, lines 596-625 in sandbox)

3. **Documentation generation output** - Same terminal output example in both (lines 303-326 in action vocab, lines 521-543 in sandbox)

4. **Describe() method examples** - Both documents show the same Go interface

5. **Future Work: Host Execution** - Both documents discuss this with similar content

**Recommendation:** Keep examples and code in one canonical location with cross-references. Suggested approach:

- Keep Docker recipe example ONLY in action vocabulary doc
- Keep ExtractPackages and container building code ONLY in sandbox doc
- Use explicit cross-references: "See [action vocabulary doc, lines X-Y] for the complete example"

### 2.3 Are forward/backward references helpful or confusing?

**Verdict: Helpful but inconsistent**

**Good references:**

- The sandbox doc consistently references the action vocabulary doc for definitions
- Both docs link to each other at the end (Relationship section)
- The anchor-based references (e.g., `#host-execution`) are precise

**Confusing references:**

1. The action vocabulary doc references `wip/research/` files (lines 566-573) that readers cannot access. These should either be removed or summarized inline.

2. The sandbox doc references `DESIGN-golden-plan-testing.md` (line 24) but doesn't explain what golden plan testing is. A recipe author would need to read a third document to understand the context.

3. Line references like "(lines 1465-1515)" are brittle - they'll break when documents are edited.

---

## 3. Decision Rationale

### 3.1 Are the "why" explanations clear?

**Verdict: Yes, the action vocabulary doc excels here**

The action vocabulary doc uses a clear decision format:

- **Decision**: One-line statement of the choice
- **Rationale**: Bullet points explaining why
- **Rejected alternatives**: What was considered and why it was rejected

Example (D1: Action Granularity):
- Decision: One action per operation
- Rationale: Consistency, Learnability, Extensibility, Error messages
- Rejected: Option B (one action per manager) and Option C (unified action)

### 3.2 Can readers understand the trade-offs?

**Verdict: Yes**

Both documents explicitly acknowledge trade-offs:

- Action vocabulary: "The verbosity trade-off (separate steps per platform) is acceptable because..."
- Sandbox: Consequences section lists both Positive and Negative outcomes with Mitigations

**Particularly clear:** The explanation of why `install_guide` with platform keys inside the parameter is wrong (sandbox doc, lines 56-74). The before/after comparison makes the inconsistency obvious.

### 3.3 Are rejected alternatives explained sufficiently?

**Verdict: Yes for action vocabulary, lacking for sandbox**

**Action vocabulary doc:** Each decision includes rejected alternatives with clear explanations.

**Sandbox doc:** The "Considered Options" section (lines 116-320) thoroughly explores options but:

- Options are numbered inconsistently (1A/1B, 2A/2B, 3A/3B/3C, 4A/4B/4C)
- Option 2B (Structured Primitives) uses different syntax than the final solution (uses `primitives = [...]` array instead of separate steps with typed actions)

**Issue:** Option 2B syntax doesn't match the actual solution. The `primitives` array syntax was apparently rejected in favor of separate steps, but this isn't explained.

---

## 4. Migration Guidance

### 4.1 Is it clear how to migrate existing recipes?

**Verdict: No - migration guidance is inadequate**

The sandbox doc mentions migration (lines 709-716) but only at a high level:

1. Remove `install_guide`
2. Implement typed actions
3. Migrate recipes
4. Validate

**Missing:**

1. **Before/after comparison for a real recipe.** Neither doc shows what an existing recipe looks like and what it should become.

2. **Step-by-step migration checklist.** Recipe authors need:
   - How to identify which actions to use
   - How to determine the correct `distro` values
   - How to find SHA256 hashes for GPG keys
   - How to test the migrated recipe

3. **Tooling support.** The "Future Work: Automatic Action Analysis" (sandbox doc, lines 849-858) mentions a `tsuku analyze` command but provides no interim guidance.

### 4.2 What does a recipe author need to do differently?

**Partially clear from examples, not explicitly stated:**

| Before | After |
|--------|-------|
| Single `require_system` step with `install_guide` | Multiple typed action steps with `when` clauses |
| Platform keys in parameter (`darwin`, `linux`) | Platform filtering via `when` clause |
| Free-form text instructions | Machine-readable action parameters |
| No content-addressing | SHA256 required for external resources |

**Recommendation:** Add an explicit "What Changes for Recipe Authors" section covering:

1. The shift from polymorphic `require_system` to typed actions
2. Moving platform filtering from parameter keys to `when` clauses
3. New requirement to provide SHA256 hashes for external resources
4. New distro detection capability (`when = { distro = [...] }`)

### 4.3 Are before/after examples provided?

**Verdict: No explicit migration examples**

The "Current state" examples at the start of both documents show what exists today:

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
```

The "Solution" sections show what the new format looks like. But there is no side-by-side "transform this into that" example.

---

## 5. Specific Recommendations

### 5.1 Confusing Sections

1. **Sandbox doc, lines 193-228 (Option 2B):** The `primitives = [...]` syntax shown here is NOT used in the final solution. This will confuse readers who think they should use this syntax. Either remove it or clarify it was an intermediate design.

2. **Action vocabulary doc, lines 171-178 (`unless_command`):** This escape hatch is introduced but never shown in the full examples. Is it implemented? When should it be used?

3. **Sandbox doc, lines 471-479 (Content-addressing in old syntax):** Shows `apt_repo` using object syntax `{ apt_repo = { ... } }` which is inconsistent with the step-based syntax elsewhere.

4. **Both docs:** The relationship between `require_system` and `require_command` is unclear. Is `require_system` being removed entirely? Is `require_command` a replacement or an evolution?

### 5.2 Suggested Clarifications

1. **Add a quick reference card** at the end of the action vocabulary doc:

   ```
   ## Quick Reference for Recipe Authors

   ### Simple package installation
   action = "apt_install"
   packages = ["package-name"]
   when = { distro = ["ubuntu", "debian"] }

   ### With custom repository
   action = "apt_repo"
   url = "https://..."
   key_url = "https://..."
   key_sha256 = "..."
   when = { distro = ["ubuntu"] }

   ### macOS cask
   action = "brew_cask"
   packages = ["app-name"]
   when = { os = ["darwin"] }

   ### Verify installation
   action = "require_command"
   command = "binary-name"
   ```

2. **Add a decision tree for when to use `os` vs `distro`:**

   ```
   Does your action work on ALL Linux distributions?
   ├── Yes → use when = { os = ["linux"] }
   └── No → use when = { distro = ["ubuntu", "debian", ...] }
   ```

3. **Add explicit migration instructions:**

   ```
   ## Migrating from require_system

   ### Step 1: Identify platforms
   Look at your current `install_guide` keys (darwin, linux, etc.)

   ### Step 2: Convert to typed actions
   For each platform, create a step with the appropriate action:
   - brew commands → brew_install or brew_cask
   - apt commands → apt_install
   - dnf commands → dnf_install

   ### Step 3: Add platform filtering
   Add `when` clause to each step

   ### Step 4: Add require_command
   Add final verification step
   ```

4. **Clarify the fate of `require_system`:**

   Add a clear statement: "The legacy `require_system` action is being deprecated. Use typed actions (`apt_install`, `brew_cask`, etc.) combined with `require_command` for verification."

### 5.3 Jargon Needing Definition

| Term | First Appearance | Needs Definition |
|------|-----------------|------------------|
| Golden plan/golden files | Sandbox doc, line 33 | Yes - no definition provided |
| TOCTOU | Sandbox doc, line 349 | Yes - Time-of-Check Time-of-Use should be spelled out |
| Preflight | Both docs, multiple locations | Partially - context makes it clear but explicit definition helps |
| Content-addressing | Both docs, multiple locations | Yes - explain what it means and why it matters |
| Idempotent | Action vocab, line 143 | Partially - technical readers know, others may not |

### 5.4 Additional Examples Needed

1. **Homebrew tap usage:**
   ```toml
   [[steps]]
   action = "brew_install"
   packages = ["some-tool"]
   tap = "owner/repo"
   when = { os = ["darwin"] }
   ```

2. **APT PPA usage:**
   ```toml
   [[steps]]
   action = "apt_ppa"
   ppa = "deadsnakes/ppa"
   when = { distro = ["ubuntu"] }

   [[steps]]
   action = "apt_install"
   packages = ["python3.11"]
   when = { distro = ["ubuntu"] }
   ```

3. **Fedora/RHEL example:**
   ```toml
   [[steps]]
   action = "dnf_install"
   packages = ["docker"]
   when = { distro = ["fedora", "rhel"] }
   ```

4. **Fallback field usage:**
   ```toml
   [[steps]]
   action = "apt_install"
   packages = ["nvidia-cuda-toolkit"]
   fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
   when = { distro = ["ubuntu"] }
   ```

---

## 6. Summary of Findings

### What Works Well

1. **Clear decision documentation** - The action vocabulary doc's decision format (Decision/Rationale/Rejected Alternatives) is excellent
2. **Comprehensive Docker example** - Shows real-world complexity
3. **Security reasoning** - The "no shell" constraint is well-justified
4. **Trade-off acknowledgment** - Both docs honestly discuss downsides

### What Needs Improvement

1. **Migration guidance** - Recipe authors need step-by-step instructions
2. **Duplication** - Same examples and code appear in both docs
3. **Syntax consistency** - Option 2B shows syntax that doesn't match the solution
4. **Quick reference** - Add a cookbook-style reference for common patterns
5. **Jargon** - Define technical terms for broader audience

### Priority Recommendations

| Priority | Recommendation |
|----------|---------------|
| High | Add explicit migration guide with before/after examples |
| High | Remove or clarify the `primitives = [...]` syntax in Option 2B |
| Medium | Add quick reference card for common action patterns |
| Medium | Reduce duplication by keeping examples in one canonical location |
| Medium | Define jargon terms (golden files, TOCTOU, content-addressing) |
| Low | Add decision tree for os vs distro |
| Low | Add examples for tap, ppa, fallback fields |

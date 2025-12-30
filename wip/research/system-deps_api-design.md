# API Design Assessment: System Dependency Actions

## Focus: Q1 - Action Granularity

This assessment evaluates three proposed approaches for structuring system dependency actions in tsuku's recipe format.

## Options Under Review

- **Option A**: One action per operation (`apt_install`, `apt_repo`, `brew_cask`)
- **Option B**: One action per manager with sub-operations as fields (`action = "apt"` with `install`, `repo` fields)
- **Option C**: Unified action with manager as a field (`action = "pkg_install"` with `manager = "apt"`)

---

## 1. Consistency and Predictability

**Option A (Granular Actions)** provides the highest consistency. Each action has a single responsibility with a predictable schema. Recipe authors know exactly what `apt_install` does without consulting documentation about which sub-operation modes exist. The naming convention `<manager>_<operation>` is self-documenting.

**Option B (Manager Actions)** introduces polymorphism within a single action type. The `apt` action behaves differently depending on whether `install` or `repo` fields are present. This creates cognitive overhead: authors must remember valid field combinations, and the schema becomes conditional (repo fields only valid when adding a repo, not when installing).

**Option C (Unified Action)** suffers from the same problem that motivated this redesign: a generic container with platform-specific content inside. The schema must enumerate all possible manager-specific fields, leading to many fields that are only valid for certain `manager` values (e.g., `key_url` only makes sense for apt/dnf, not brew).

**Winner: Option A** - Each action has exactly one schema, making validation straightforward and behavior predictable.

---

## 2. Learnability for Recipe Authors

**Option A** has a larger vocabulary but simpler rules. Authors learn individual actions by name. The naming pattern is consistent: once you know `apt_install` and `apt_repo`, you can guess `dnf_install` and `dnf_repo` exist. Tab completion and IDE support work naturally.

**Option B** has fewer action names but complex internal rules. Authors must learn which field combinations are valid. The design document's example shows this complexity: `repo = { url = "...", key_url = "...", key_sha256 = "..." }` nested inside an `apt` action creates a multi-level structure that's harder to remember than a flat `apt_repo` action with top-level fields.

**Option C** appears simple initially but becomes confusing quickly. What happens if you specify `manager = "brew"` with `key_url`? The unified interface papers over real differences between package managers, pushing complexity into runtime validation rather than static schema checks.

**Winner: Option A** - More actions to learn, but each is simpler. The vocabulary scales predictably as new managers are added.

---

## 3. Extensibility for New Package Managers

**Option A** excels here. Adding a new package manager (e.g., `apk` for Alpine, `zypper` for openSUSE) means defining new action types with their own schemas. No existing code or schemas need modification. Each manager's unique features are first-class (e.g., Alpine's `--no-cache`, Nix's `--attr`).

**Option B** requires modifying the polymorphic action to understand new sub-operations if the new manager has operations not covered by existing fields. It's awkward if `apk` needs an operation that `apt` doesn't have.

**Option C** requires adding to an ever-growing union type. The schema becomes increasingly complex, and manager-specific features either bloat the common interface or require escape hatches that defeat the purpose of unification.

**Winner: Option A** - New managers are additive; existing actions remain unchanged.

---

## 4. Error Handling and Validation Implications

**Option A** enables precise error messages: "apt_install requires 'packages' field" is unambiguous. Static analysis tools can validate recipes without understanding conditional logic. Each action can define its own error handling strategy appropriate to its semantics.

**Option B** makes validation harder: "apt action is invalid" requires explaining which mode was detected and why it failed. The validator must first determine intent (repo vs install vs both), then apply mode-specific rules.

**Option C** produces the most confusing errors. If validation fails, was it because the manager doesn't support that field, or because the field value was wrong? The indirection through `manager` obscures the root cause.

From an implementation perspective, Option A maps cleanly to Go's type system: each action becomes a distinct struct with typed fields. Option B requires runtime dispatch based on which fields are present. Option C requires a union type or reflection-heavy validation.

**Winner: Option A** - Cleanest validation path with the most actionable error messages.

---

## 5. Recommendation

**Option A (One action per operation)** is the clear winner across all evaluation criteria.

The proposed vocabulary in the design document is well-structured:
- Clear naming convention: `<manager>_<operation>`
- Consistent field patterns across managers (e.g., all install actions have `packages`)
- Clean separation between package installation, system configuration, and verification

### Additional Recommendations

1. **Establish consistent field naming**: All package installation actions should use `packages` (plural, array). All repo actions should have `url`, `key_url`, `key_sha256`.

2. **Document the taxonomy explicitly**: Group actions by category (installation, configuration, verification, fallback) as done in the proposed vocabulary table.

3. **Consider action aliases for common patterns**: If `brew_tap` + `brew_install` is a frequent combination, consider a convenience action while keeping the primitives available.

4. **Validate early in the design cycle**: Write JSON Schema or Go struct definitions for each action before implementing. This surfaces field naming inconsistencies early.

---

## Summary

Option A provides the best balance of simplicity, extensibility, and maintainability. Its larger vocabulary is offset by consistent naming patterns and single-responsibility actions. The design document's proposed vocabulary already follows this approach and should proceed as specified.

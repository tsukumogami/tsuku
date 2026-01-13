# Issue 772 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-structured-install-guide.md`
- Sibling issues reviewed: #758 (discovery, closed)
- Prior patterns identified:
  - Typed actions implemented: `apt_install`, `apt_repo`, `apt_ppa`, `brew_install`, `brew_cask`, `dnf_install`, `require_command`, `manual`, `group_add`, `service_enable`, `service_start`
  - SystemAction interface established with `Validate()`, `ImplicitConstraint()`, `Describe()` methods
  - Migration scope document created at `wip/migration-scope.md` with 3 recipes identified

## Gap Analysis

### Minor Gaps

1. **Migration scope is small (3 recipes)**: The discovery work (#758) confirmed only 3 recipes need migration:
   - `docker.toml` - macOS: brew_cask, Linux: manual (complex docker.io instructions)
   - `cuda.toml` - Both platforms: manual (NVIDIA-specific, requires hardware)
   - `test-tuples.toml` - macOS: brew_cask, Linux: apt_install

2. **Typed actions are implemented**: All required action types exist in `internal/actions/`:
   - `apt_install`, `apt_repo`, `apt_ppa` (apt_actions.go)
   - `brew_install`, `brew_cask` (brew_actions.go)
   - `dnf_install`, `dnf_repo` (dnf_actions.go)
   - `require_command`, `manual`, `group_add`, `service_enable`, `service_start` (system_config.go)

3. **test-tuples.toml needs special attention**: The migration scope doc notes Linux instruction is "N/A" but the recipe file shows `"linux/amd64" = "sudo apt install docker.io"` - this should migrate to `apt_install` with `when = { os = "linux" }`.

### Moderate Gaps

None identified. The issue spec is complete given the discovery work.

### Major Gaps

None identified. All dependencies are closed (#758, #760, #770, #771) and the implementation path is clear.

## Recommendation

**Proceed**

The issue spec is complete. All dependencies are closed. The discovery work (#758) established the exact scope (3 recipes), and the typed action vocabulary is implemented. Migration can proceed without amendments.

## Implementation Notes

For the migration work, reference these conventions from prior work:

1. **PM actions have implicit constraints**: e.g., `apt_install` implicitly constrains to `linux_family = "debian"` - no explicit `when` clause needed for family filtering.

2. **Use `require_command` for verification**: Add at end of recipe to verify the tool is installed.

3. **Use `manual` for complex instructions**: For docker on Linux and cuda on all platforms, use the `manual` action with clear instructions since fully automated installation is not feasible.

4. **Recipe locations** (from migration-scope.md):
   - `internal/recipe/recipes/c/cuda.toml`
   - `internal/recipe/recipes/d/docker.toml`
   - `internal/recipe/recipes/t/test-tuples.toml`

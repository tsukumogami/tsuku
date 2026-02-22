# Architect Review: DESIGN-sandbox-image-unification

## 1. Problem Statement Specificity

The problem statement is well-grounded. The drift table (lines 87-93) maps directly to verifiable code locations, and I confirmed each claim against the codebase:

- `container_spec.go:43-49` does have `alpine:3.19` and `opensuse/leap:15`
- `recipe-validation-core.yml:131` has `alpine:3.21`, line 127 has `opensuse/tumbleweed`
- `test-checksum-pinning.sh:45` has `fedora:39` while everything else uses `fedora:41`

The problem is concrete, the drift is real, and the scope boundaries are appropriate. One minor gap: the problem statement doesn't mention `DefaultSandboxImage` and `SourceBuildSandboxImage` constants in `requirements.go:15,19` (`debian:bookworm-slim` and `ubuntu:22.04`). These are a fourth location of hardcoded images within the same package. The design should acknowledge whether these constants are in scope or out of scope, since they represent additional drift potential within the sandbox package itself.

## 2. Missing Alternatives

No critical alternatives are missing. The design covers the reasonable solution space: centralized config (chosen), Go-as-source-of-truth, Dockerfile proxy, and full sandbox replacement. The alternatives span the spectrum from minimal change to maximal unification.

One minor option not explicitly considered: a **Go-generated code approach** where a generator reads a config file and produces a Go source file with the map literal. This avoids the `go:embed` + runtime JSON parsing overhead and is a common Go pattern (see `go generate`). It's arguably more idiomatic than embedding JSON, though it introduces a build step. This doesn't need to be an option in the design -- it's a variant of the chosen approach that could be explored during implementation.

## 3. Rejection Rationale

The rejection rationale for each alternative is specific and fair:

- **Go source as source of truth**: Correctly identifies that CI workflows can't easily parse Go source. The rejection is that Renovate would update Go but not the 8+ workflow files, which is the core problem restated. Fair.

- **Dockerfile proxy**: The "confusing to contributors" argument is soft, but the stronger technical reason (Dependabot doesn't understand rolling releases like `opensuse/tumbleweed`) is valid. Fair.

- **Replace all CI with --sandbox**: This is the most detailed rejection, backed by the three specific capability gaps (post-install verification, env passthrough, structured reporting). The design explicitly says this is the right long-term direction but premature today. Not a strawman -- the research findings justify the rejection.

- **Dependabot with proxy Dockerfile** (Decision 2): Correctly identifies the indirection layer problem. Fair.

- **Manual updates with CI drift check** (Decision 2): Interesting that this is rejected as primary but recommended as a safety net. The design should be more explicit that a drift-check CI job should be part of the implementation plan, not just an aside.

- **.env file** (Decision 3): The rejection that `.env` files can't be easily read from Go is slightly overstated (it's a trivial parser), but the Renovate argument is sound.

- **Reusable workflow** (Decision 3): The limitations around matrix strategies are real. Fair.

No option reads as a strawman. Each has genuine advantages that the design acknowledges before explaining why they fall short.

## 4. Unstated Assumptions

### `go:embed` cannot access parent directories (Blocking concern)

The design proposes placing `container-images.json` at the repo root and embedding it from `internal/sandbox/container_spec.go`. This won't work. Go's `//go:embed` directive can only embed files from the same directory or subdirectories of the package containing the directive. It cannot reference files in parent directories or outside the package tree.

The file at the repo root (`container-images.json`) is several directory levels above `internal/sandbox/`. The embed directive would need to be something like `//go:embed ../../../../container-images.json`, which `go:embed` does not support -- it rejects paths with `..` components.

The existing `go:embed` usages in the codebase confirm this pattern: `internal/recipe/embedded.go` embeds `recipes/*.toml` from a `recipes/` subdirectory within `internal/recipe/`, and `internal/builders/llm_integration_test.go` embeds `llm-test-matrix.json` from the same directory.

Workarounds:
1. Place the JSON file inside `internal/sandbox/` (hurts discoverability from the repo root).
2. Create a dedicated package (e.g., `internal/containerimages/`) that embeds the file and exports the map, then import that from sandbox. The JSON file lives inside this package.
3. Place the JSON at the repo root and use a symlink from `internal/sandbox/container-images.json` (symlinks in `go:embed` are explicitly disallowed).
4. Use `go generate` to produce Go source from the root JSON file at build time.
5. Place the embed in `cmd/tsuku/main.go` (which is closer to root but still not at root), pass the data down via dependency injection. This inverts the current dependency direction.

Option 2 or 4 are the most architecturally clean. Option 2 follows the existing pattern (dedicated package owns its embedded data) and avoids a build step. But in either case, the JSON file can't live at the repo root and be embedded from `internal/sandbox/` -- the design needs to address this.

### `ubuntu:22.04` is not in the proposed config

The `SourceBuildSandboxImage` constant (`ubuntu:22.04`) and the PPA fallback to `ubuntu:24.04` in `container_spec.go:135` are not part of the proposed `container-images.json` schema. The JSON maps families to a single image each. But the sandbox code uses two different images per family depending on context (default vs. source build), plus a conditional Ubuntu override for PPAs.

The design should clarify: does the JSON only cover the `familyToBaseImage` map, or does it also cover `DefaultSandboxImage`, `SourceBuildSandboxImage`, and the PPA override? If not, these remain as hardcoded drift candidates.

### `jq` availability in CI

The design assumes `jq` is available in all CI workflow contexts. On `ubuntu-latest` runners, `jq` is pre-installed. But some workflows run steps inside the container images themselves (the `docker run ... sh -c '...'` pattern). Inside `alpine:3.21` or `archlinux:base`, `jq` is not installed by default. The existing workflows already install `curl` and `ca-certificates` inside containers but not `jq`. For the `container-images.json` pattern to work, the JSON must be read before entering the container (on the host runner), which is what the design shows. This works, but should be stated explicitly since the container-vs-host distinction is subtle.

### Flat JSON schema assumes one image per family

The proposed schema maps each family to exactly one image. But CI workflows use family images in different configurations (with different `install_cmd` values for each family, and the `libc` distinction between glibc and musl families). The JSON file handles the image mapping, but the auxiliary data (`install_cmd`, `libc`) would still be hardcoded in workflows. This is fine for the stated goal (image version unification), but the design should note that the install commands themselves are another dimension of per-family configuration that may benefit from centralization later.

## 5. Strawman Analysis

No option is a strawman. The sandbox replacement option (1D) could look like one at first glance -- it's the most ambitious and least feasible. But the design invests the most research effort into it (three specific capability gaps, a full audit table), treats it as the ideal long-term direction, and defers it on practical grounds rather than dismissing it. This is honest evaluation, not strawman setup.

## 6. Architectural Concerns

### `go:embed` path constraint (Blocking)

As detailed in section 4, the `go:embed` directive cannot reach the repo root from `internal/sandbox/`. This is a technical constraint that invalidates the proposed file placement. The design must either move the JSON file or introduce a package boundary to own the embed.

### No parallel pattern risk

The proposed change replaces an existing inline map with an embedded config -- same pattern the codebase already uses for recipe embedding (`internal/recipe/embedded.go`). The consumption via `encoding/json` is standard library. No new parsing infrastructure or config framework is introduced. This respects the existing architecture.

### Drift check CI job should be explicit

The design mentions a CI drift check as a "safety net" in the rejection of manual-only updates (Decision 2), but doesn't include it in the implementation plan. A drift check that verifies no file in the repo contains a container image reference that doesn't match `container-images.json` would catch cases where a contributor adds a new workflow and hardcodes an image. Without this, the single-source-of-truth guarantee depends entirely on contributor discipline. This should be part of the implementation, not an aside.

### Renovate configuration scope

The design proposes Renovate's regex custom manager but doesn't address that Renovate isn't currently configured for this repo. Setting up Renovate involves a `renovate.json` at the repo root and enabling the Renovate GitHub App. This is a non-trivial operational change. The design correctly notes that Renovate is "an optimization, not a dependency," but the implementation effort for Renovate setup should be sized as a separate work item, not bundled with the JSON extraction.

## Summary of Findings

| Finding | Severity | Action Needed |
|---------|----------|---------------|
| `go:embed` cannot access repo root from `internal/sandbox/` | Blocking | Revise file placement or introduce a dedicated embed package |
| `DefaultSandboxImage` and `SourceBuildSandboxImage` not covered | Advisory | Clarify scope -- are these constants part of the unification or not? |
| Drift-check CI job mentioned but not in implementation plan | Advisory | Include as an explicit deliverable |
| Renovate setup is a separate operational effort | Advisory | Size it as its own work item, not bundled with the config extraction |
| `jq` availability is host-only, not in-container | Advisory | Note explicitly that JSON reads happen on the runner, not inside containers |

## Recommendations

1. **Address the `go:embed` constraint before implementation.** Either create a dedicated `internal/containerimages/` package that owns the JSON file and exports the parsed map, or use `go generate` to produce Go source from a root-level JSON. The former is simpler and follows existing patterns in the codebase.

2. **Add a drift-check CI job to the implementation plan.** Without enforcement, the single-source-of-truth guarantee is a convention rather than a constraint.

3. **Scope the Ubuntu images explicitly.** State whether `ubuntu:22.04` (SourceBuildSandboxImage) and the PPA `ubuntu:24.04` override are in or out of this design's scope.

4. **Separate Renovate setup from the config extraction.** The JSON file and Go/CI consumption can ship independently of Renovate. This reduces the blast radius and unblocks the immediate drift fix.

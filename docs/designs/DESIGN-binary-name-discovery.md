---
status: Proposed
problem: |
  Recipe builders discover binary names by fetching source files (Cargo.toml,
  pyproject.toml, gemspec) from GitHub repositories. This fails for workspace
  monorepos where binaries are defined in member packages, not the root
  manifest. The fallback is the package name, which is often wrong (e.g.,
  sqlx-cli produces binary "sqlx"). These errors aren't caught until sandbox
  validation, and the self-repair system only handles verification failures,
  not wrong binary names.
decision: |
  Replace repository-based binary discovery with registry API lookups where
  available. For crates.io, use the bin_names field from the version endpoint
  already being called. For npm, fix the parseBinField() string-type handling.
  For PyPI and RubyGems, add artifact-based discovery that downloads the
  published package and extracts executable metadata. Add a post-generation
  validation step in the orchestrator that cross-checks recipe binary names
  against registry metadata before sandbox validation.
rationale: |
  Registry APIs and published artifacts are the authoritative source for what
  a package installs. Repository source files are unreliable because workspace
  layouts, monorepos, and build-time code generation all create mismatches
  between what's in the repo root and what cargo install or pip install
  actually produces. The crates.io fix requires changing one function call.
  The orchestrator validation catches any builder's mistakes as a safety net.
---

# Deterministic Binary Name Discovery for Recipe Builders

## Status

**Proposed**

## Context and Problem Statement

When `tsuku create` generates a recipe, each builder must determine what executables the package produces. The current approach fetches source manifests from GitHub repositories:

- **Cargo**: fetches root `Cargo.toml`, parses `[[bin]]` sections
- **npm**: uses the `bin` field from the registry API (partially working)
- **PyPI**: fetches `pyproject.toml` from GitHub, parses `[project.scripts]`
- **RubyGems**: fetches gemspec from GitHub, parses `executables` attribute
- **Go**: infers from the last segment of the module import path

This approach breaks for workspace monorepos. When a crate like `sqlx-cli` lives inside a Cargo workspace, the root `Cargo.toml` contains `[workspace]` configuration, not `[[bin]]` sections. The builder falls back to the crate name (`sqlx-cli`), but the actual binary is `sqlx`. The same pattern affects multi-binary crates like `probe-rs-tools`, which produces `probe-rs`, `cargo-flash`, and `cargo-embed`.

These mismatches were caught manually during PR #1869 testing. Manual review doesn't scale as the recipe count grows.

The existing verification self-repair system (DESIGN-verification-self-repair) handles a related but different problem: tools whose `--version` flag doesn't work. It doesn't attempt to repair wrong binary names because exit code 127 (command not found) is treated as unrecoverable.

### Scope

**In scope:**
- Replacing repository-based binary discovery with registry API/artifact lookups
- Adding orchestrator-level validation of binary names
- Covering all five builder ecosystems (Cargo, npm, PyPI, RubyGems, Go)

**Out of scope:**
- LLM-based recipe generation (GitHub Release, Homebrew, Cask builders)
- Changes to the verification self-repair system
- Standalone `tsuku validate --fix` command for batch repair (can follow later)

## Decision Drivers

- **Accuracy over speed**: wrong binary names cause install failures that users see
- **Use authoritative sources**: registry APIs and published artifacts over repository source files
- **Minimal new network calls**: reuse data from API calls already happening
- **Incremental delivery**: fix the easy wins (crates.io, npm) first, tackle harder registries after
- **Safety net**: catch any builder's mistakes before they reach sandbox validation

## Considered Options

### Decision 1: Source of truth for binary names

Each builder currently parses source files from GitHub to find executable names. The question is whether to keep this approach and fix the monorepo edge cases, or switch to a different data source entirely.

Registry APIs and published package artifacts represent what `cargo install` or `pip install` will actually produce, because they contain the resolved, normalized metadata that the package manager uses at install time. Repository source files represent what the developer wrote, which may use workspace inheritance, build scripts, or other indirection that our TOML parser can't evaluate.

#### Chosen: Registry APIs and published artifacts

Use the registry's own metadata as the primary source for binary names. The availability varies by registry:

| Registry | Source | Field | Extra call needed? |
|----------|--------|-------|--------------------|
| crates.io | Version API response | `bin_names` | No (already fetched) |
| npm | Package metadata | `bin` | No (already fetched) |
| PyPI | Wheel artifact | `entry_points.txt` | Yes (download .whl) |
| RubyGems | Gem artifact | `metadata.gz` | Yes (download .gem) |
| Go | None available | N/A | N/A |

For crates.io and npm, the data is already in API responses we're fetching. For PyPI and RubyGems, we'd need to download the published artifact and extract internal metadata files.

#### Alternatives Considered

**Fix repository parsing for monorepos**: For Cargo, detect workspace manifests and follow member paths to find the right sub-crate Cargo.toml. Rejected because workspace layouts vary widely (path dependencies, virtual manifests, nested workspaces), and we'd be reimplementing part of Cargo's workspace resolution. The registry already did this work when the crate was published.

**Use `cargo metadata` or equivalent tools**: Shell out to `cargo metadata --no-deps` to get resolved binary targets. Rejected because it requires the Rust toolchain to be installed (not guaranteed during recipe generation), and doesn't generalize to other registries.

### Decision 2: Validation architecture

Beyond fixing individual builders, the question is whether to add a cross-cutting validation layer that catches binary name errors regardless of which builder produced them. The current orchestrator runs sandbox validation (which catches wrong names via exit code 127) but doesn't attempt repair.

#### Chosen: Pre-sandbox validation in the orchestrator

Add a validation step between recipe generation and sandbox validation. This step cross-checks the recipe's executable list against registry metadata. If there's a mismatch, it corrects the recipe before the sandbox run. This catches errors from any builder, including future ones, without waiting for a full sandbox cycle.

The validation runs only for deterministic builders (not LLM builders, which have their own repair loop). It's a fast check since the registry data was already fetched during generation.

#### Alternatives Considered

**Rely on sandbox validation alone**: Let wrong binary names fail in the sandbox (exit code 127), then repair. Rejected because sandbox runs are slow (container startup, network, build), and the information to prevent the failure is already available. A 100ms API check beats a 60-second container failure.

**Post-sandbox binary name repair**: Add a new self-repair phase that handles exit code 127 by querying registry metadata and retrying. Viable but slower than pre-validation, and doesn't prevent the wasted sandbox run. Could be added later as a defense-in-depth measure.

### Decision 3: Rollout strategy

The five registries have different levels of API support for binary metadata. The question is whether to implement all at once or incrementally.

#### Chosen: Incremental by registry, starting with crates.io and npm

Fix crates.io and npm first because they require no additional network calls -- the binary metadata is already in API responses being fetched. PyPI and RubyGems need artifact downloads, which adds complexity (download size limits, extraction, error handling) that warrants separate implementation work.

Go has no reliable source for binary names beyond the import path heuristic. The best option there may be to detect common patterns (e.g., `cmd/` subdirectories) from the repository, but that's speculative and can wait.

#### Alternatives Considered

**All registries at once**: Implement all five in one pass. Rejected because the crates.io and npm fixes are small, targeted changes that can ship immediately. Bundling them with the more complex PyPI and RubyGems work delays the easy wins.

## Decision Outcome

**Chosen: Registry API lookups + pre-sandbox validation, incremental rollout**

### Summary

Replace `discoverExecutables()` in the Cargo builder with a call that reads the `bin_names` field from the crates.io version API response that `fetchCrateInfo()` already returns. This fixes the workspace monorepo problem without any new network calls, because `bin_names` is included in the same response used for version resolution. Similarly, fix the npm builder's `parseBinField()` to handle the string-type `bin` field (single executable pointing to a file path), which currently returns nil and falls back to the package name.

Add a `ValidateBinaryNames()` step in the orchestrator between generation and sandbox validation. This function takes the generated recipe and the builder's registry metadata, extracts the executable list from both, and flags mismatches. When the recipe's executables don't match the registry metadata, the orchestrator corrects them before the sandbox run. This prevents wasted sandbox cycles and catches errors from any builder.

PyPI and RubyGems don't expose binary metadata through their APIs. Fixing those registries requires downloading the published artifact (`.whl` for PyPI, `.gem` for RubyGems) and extracting internal metadata files (`entry_points.txt` and `metadata.gz` respectively). This is straightforward but adds download/extraction logic that should be implemented separately.

The Go builder's import-path heuristic has no equivalent registry metadata to validate against. Improving Go binary name discovery likely requires source-level analysis (looking for `cmd/` directories in the repository), which is a different class of problem.

### Rationale

The crates.io and npm fixes are small, well-scoped changes that solve the most common failure mode (Cargo workspace monorepos were the source of every binary name error found in PR #1869). The orchestrator validation adds a safety net that catches any builder's mistakes. Doing the easy wins first and deferring artifact downloads keeps the initial scope tight while establishing the pattern for future registry work.

## Solution Architecture

### Component Changes

**1. Cargo builder (`internal/builders/cargo.go`)**

Replace the `discoverExecutables()` function. Instead of constructing a GitHub URL, fetching root Cargo.toml, and parsing `[[bin]]` sections, read `bin_names` directly from the crates.io API response that `fetchCrateInfo()` already returns.

The `cratesIOCrateResponse` struct needs a `BinNames` field added to capture the version-level `bin_names` array. The builder should use `bin_names` from the latest non-yanked version (which is the version it resolves to), falling back to the crate name only if `bin_names` is empty or null (which indicates a library-only crate). The builder must cache the API response from `fetchCrateInfo()` so that `AuthoritativeBinaryNames()` can return the data later when the orchestrator calls it.

Remove `buildCargoTomlURL()`, `fetchCargoTomlExecutables()`, and the `cargoToml`/`cargoTomlBinSection` structs once the new approach is validated.

**2. npm builder (`internal/builders/npm.go`)**

Fix `parseBinField()` to handle the string-type `bin` value. When `bin` is a string (not a map), it means there's a single executable whose name matches the package name. The function should return `[]string{packageName}` in this case instead of `nil`. For scoped packages (`@scope/tool`), the executable name is the unscoped part (`tool`), so the function needs the package name passed in to strip the scope prefix.

**3. Orchestrator (`internal/builders/orchestrator.go`)**

Add a `validateBinaryNames()` method that runs after `buildRecipe()` and before `validateInSandbox()`. The method takes the generated recipe and a `BinaryNameMetadata` interface (implemented by each builder that has registry data) and compares the recipe's executable list against the authoritative source.

When a mismatch is detected, log a warning, emit a telemetry event (following the pattern in `attemptVerifySelfRepair`), and correct the recipe's executables. The telemetry provides signal on how often the safety net fires. The orchestrator should type-assert the `SessionBuilder` to `BinaryNameProvider` in `Create()` before creating the session, since the builder reference isn't retained after session creation.

**4. Builder interface extension**

Add an optional interface that builders can implement to provide authoritative binary name data:

```go
type BinaryNameProvider interface {
    AuthoritativeBinaryNames() []string
}
```

Builders that have registry metadata (Cargo, npm, and eventually PyPI, RubyGems) implement this interface. The orchestrator checks if the builder satisfies it and runs validation when available. Builders without metadata (Go, LLM builders) skip this step.

### Data Flow

```
Builder.Build()
  ├── fetchCrateInfo()          // Already happening
  │     └── bin_names in response  // New: capture this
  ├── discoverExecutables()     // Changed: read bin_names instead of Cargo.toml
  └── generateRecipe()
        └── recipe with executables

Orchestrator.Generate()
  ├── builder.Build()           // Gets recipe
  ├── validateBinaryNames()     // New: cross-check against registry
  │     ├── builder implements BinaryNameProvider?
  │     │     └── compare recipe executables vs authoritative names
  │     └── mismatch? → correct recipe, log warning
  └── validateInSandbox()       // Existing: sandbox validates installation
```

### Future Work: PyPI and RubyGems

PyPI and RubyGems require downloading published artifacts to discover binary names. The pattern is similar for both:

1. Get the artifact URL from the registry API (already available)
2. Download the artifact (`.whl` is a ZIP, `.gem` is a tarball)
3. Extract the metadata file (`entry_points.txt` or `metadata.gz`)
4. Parse executable names from the metadata

This needs download size limits, timeouts, in-memory archive reading (no extraction to disk), and hash verification against registry-provided digests. All extracted binary names must pass through `isValidExecutableName()` and the executable count per package should be bounded. The `BinaryNameProvider` interface makes it easy to add these later without changing the orchestrator.

## Implementation Approach

The work breaks into three phases:

1. **Crates.io + npm fixes**: Change `discoverExecutables()` to use `bin_names`, fix `parseBinField()`. These are isolated builder changes with no new dependencies.

2. **Orchestrator validation**: Add `BinaryNameProvider` interface and `validateBinaryNames()` step. Depends on phase 1 for the first two implementations of the interface.

3. **PyPI + RubyGems artifact discovery**: Download and parse published artifacts. Depends on phase 2 for the integration point but can be developed independently.

Each phase is independently shippable. Phase 1 alone fixes the known failures from PR #1869.

## Security Considerations

### Download verification

No change for crates.io and npm (no new downloads). PyPI and RubyGems artifact downloads in future work should use HTTPS and verify content types. Artifact sizes should be bounded (existing `maxResponseSize` patterns apply).

### Execution isolation

Binary names flow from registry APIs into recipe TOML files, then into verify commands that are interpolated into shell scripts. The existing `isValidExecutableName()` regex (`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`) blocks all shell metacharacters and must be applied to every binary name from `bin_names` before it enters a recipe. This is the same validation applied to the current Cargo.toml parsing path.

### Supply chain risks

Registry APIs are the same ones already used for version resolution. The `bin_names` field comes from the same crates.io response as version data. No new trust boundaries introduced.

### User data exposure

No change. Binary name discovery doesn't access or transmit user data.

## Consequences

### Positive

- Fixes workspace monorepo binary name discovery for Cargo (the most common failure case)
- Fixes single-executable npm packages that use string-type `bin` field
- Adds a safety net that catches any builder's binary name errors before sandbox validation
- Establishes `BinaryNameProvider` pattern for future registry integrations
- No new network calls for crates.io and npm (reuses existing API responses)

### Negative

- Adds a dependency on the `bin_names` field existing in crates.io API responses (field has been present since at least 2016, well-documented in OpenAPI spec)
- Future PyPI and RubyGems work requires downloading artifacts, adding latency to recipe generation
- Go builder has no equivalent metadata source and remains heuristic-based

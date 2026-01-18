---
status: Proposed
problem: The recipe registry separation design requires a validated embedded recipe list, but there's no runtime enforcement that action dependencies can actually be resolved from embedded recipes.
decision: Add a --require-embedded flag to the loader that fails if action dependencies can't be resolved from the embedded registry. Use CI with this flag to iteratively discover and validate the embedded recipe list.
rationale: Runtime validation is the ground truth - it uses the actual loader to verify embedded recipes work. This enables incremental migration with an exclusions file to track known gaps, rather than building a separate static analysis tool.
---

# Embedded Recipe List Validation

## Status

**Proposed**

## Upstream Design Reference

This design implements Stage 0 of [DESIGN-recipe-registry-separation.md](DESIGN-recipe-registry-separation.md).

**Relevant sections:**
- Stage 0: Embedded Recipe List Validation (Prerequisite)
- Implementation Approach: Validation of embedded recipe completeness

## Context and Problem Statement

The recipe registry separation design requires splitting tsuku's 171 recipes into two categories:
- **Embedded**: Action dependencies needed for bootstrap (stay in `internal/recipe/recipes/`)
- **Registry**: All other recipes (move to `recipes/`)

Before migration can proceed, we need confidence that all action dependencies are available as embedded recipes. The current estimate (15-20 recipes) is based on manual analysis, but there's no runtime enforcement that these recipes are actually present and complete.

The key insight is that **runtime validation is the ground truth**. Rather than building a static analysis tool that computes what *should* be embedded, we should validate at runtime that action dependencies *can* be resolved from the embedded registry. This approach:

1. Uses the actual loader code path (no duplicate logic)
2. Catches real failures, not theoretical ones
3. Enables incremental migration with known exclusions
4. Self-documents gaps through CI failures

### Success Criteria

The implementation is complete when:
1. **Runtime flag exists**: `--require-embedded` flag in loader/resolver
2. **CI validation runs**: Recipes using each action type are validated in CI
3. **Exclusions are trackable**: Known gaps are documented in an exclusions file
4. **Failures are actionable**: Clear error messages indicate which recipe needs embedding
5. **EMBEDDED_RECIPES.md exists**: Manual documentation of embedded recipes and rationale

### Scope

**In scope:**
- `--require-embedded` flag for loader/resolver
- CI workflow to validate action dependency resolution
- Exclusions file for known gaps during migration
- Manual EMBEDDED_RECIPES.md documenting the embedded recipe list
- Cleanup of stale TODO comments referencing resolved issue #644

**Out of scope:**
- Static analysis tool to compute embedded list (redundant with runtime validation)
- Automatic generation of EMBEDDED_RECIPES.md
- Recipe migration to new locations (handled in issue #1033)
- Golden file reorganization (handled in issue #1034)

## Decision Drivers

- **Ground truth**: Validation must use actual loader behavior, not a parallel implementation
- **Incremental migration**: Must support known gaps during the migration period
- **Actionable failures**: CI failures must clearly indicate what's missing
- **Simplicity**: Prefer flag in existing code over new tools
- **Maintainability**: Documentation should be manually curated, not generated

## Implementation Context

### Existing Infrastructure

The loader (`internal/recipe/loader.go`) has a priority chain:
1. In-memory cache
2. Local recipes (`$TSUKU_HOME/recipes/`)
3. Embedded recipes (`internal/recipe/recipes/`)
4. Registry (GitHub raw fetch)

The `--require-embedded` flag would restrict resolution to step 3 only for action dependencies.

### Key Actions and Their Tool Dependencies

| Action | Tool Dependency | Platform |
|--------|----------------|----------|
| `go_install`, `go_build` | golang | all |
| `cargo_install`, `cargo_build` | rust | all |
| `npm_install`, `npm_exec` | nodejs | all |
| `pip_install`, `pipx_install` | python-standalone | all |
| `gem_install`, `gem_exec` | ruby | all |
| `cpan_install` | perl | all |
| `homebrew_relocate`, `meson_build` | patchelf | linux |
| `configure_make`, `cmake_build`, `meson_build` | make, zig, pkg-config | all |
| `cmake_build` | cmake | all |
| `meson_build` | meson, ninja | all |

### Transitive Recipe Dependencies

Some embedded recipes depend on other recipes:
- `ruby` → `libyaml`
- `cmake` → `openssl`, `patchelf`
- `openssl` → `zlib`

When `--require-embedded` is set, these transitive dependencies must also resolve from embedded recipes.

## Considered Options

### Decision 1: Validation Approach

#### Option 1A: Static Analysis Tool

Build a tool that parses action code, extracts Dependencies() returns, computes transitive closure, and generates EMBEDDED_RECIPES.md.

**Pros:**
- Automated generation of embedded list
- Can run without building tsuku

**Cons:**
- Parallel implementation of dependency logic
- Could diverge from actual runtime behavior
- Doesn't catch real resolution failures
- Requires maintaining separate codebase

#### Option 1B: Runtime Validation Flag

Add `--require-embedded` flag that restricts the loader to embedded recipes only when resolving action dependencies.

**Pros:**
- Uses actual loader code (ground truth)
- Catches real failures
- No duplicate logic to maintain
- Validates what actually matters

**Cons:**
- Requires building and running tsuku
- Can't generate documentation automatically
- Initial list must be created manually

### Decision 2: CI Enforcement

#### Option 2A: Full Validation (Fail on Any Gap)

CI fails if any action dependency can't be resolved from embedded recipes.

**Pros:**
- Strict enforcement
- Clear signal when something is wrong

**Cons:**
- Blocks all PRs during migration
- Can't incrementally improve

#### Option 2B: Validation with Exclusions

CI validates with an exclusions file that lists known gaps. Failures outside exclusions fail the build.

**Pros:**
- Enables incremental migration
- Documents known gaps explicitly
- Can shrink exclusions over time
- Non-blocking for unrelated PRs

**Cons:**
- Exclusions could become stale
- Requires discipline to shrink list

### Decision 3: Documentation Format

#### Option 3A: Generated EMBEDDED_RECIPES.md

Tool generates markdown from analysis.

**Pros:**
- Always accurate (if tool is correct)
- No manual maintenance

**Cons:**
- Requires static analysis tool
- Can't include human rationale

#### Option 3B: Manual EMBEDDED_RECIPES.md

Manually maintain documentation with rationale for each embedded recipe.

**Pros:**
- Can include context and reasoning
- Updated when actual changes happen
- Simple to maintain

**Cons:**
- Could drift from reality
- Requires discipline

### Evaluation Against Decision Drivers

| Driver | 1A (Static Analysis) | 1B (Runtime Flag) |
|--------|---------------------|-------------------|
| Ground truth | Poor | Good |
| Incremental migration | Fair | Good |
| Simplicity | Poor | Good |
| Maintainability | Poor | Good |

| Driver | 2A (Full Validation) | 2B (With Exclusions) |
|--------|---------------------|---------------------|
| Incremental migration | Poor | Good |
| Actionable failures | Good | Good |

| Driver | 3A (Generated) | 3B (Manual) |
|--------|----------------|-------------|
| Accuracy | Good (if tool works) | Fair |
| Simplicity | Poor | Good |

## Decision Outcome

**Chosen: 1B (Runtime Flag) + 2B (Exclusions) + 3B (Manual Documentation)**

### Summary

Add a `--require-embedded` flag to tsuku that fails if action dependencies can't be resolved from the embedded registry. CI runs validation for representative recipes using each action type, with an exclusions file for known gaps. EMBEDDED_RECIPES.md is manually maintained with rationale.

### Rationale

**Runtime validation (1B) chosen because:**
- Uses the actual loader - no parallel implementation to maintain
- Catches real failures, not theoretical ones
- Ground truth for "does this actually work?"
- Simpler than building a static analysis tool

**Exclusions (2B) chosen because:**
- Enables incremental migration without blocking all PRs
- Explicitly documents known gaps
- Provides a path to zero exclusions
- Failures outside exclusions are real regressions

**Manual documentation (3B) chosen because:**
- Can include human reasoning ("ruby is embedded because gem_install needs it")
- Runtime validation is the enforcement mechanism, not documentation
- Simple to maintain alongside recipe changes
- No tool generation means no tool maintenance

**Static analysis tool rejected because:**
- Redundant with runtime validation
- Would require maintaining parallel dependency logic
- Could diverge from actual loader behavior
- More complex with no additional benefit

### Trade-offs Accepted

1. **Manual documentation could drift**: EMBEDDED_RECIPES.md might not perfectly match reality.
   - *Acceptable because*: Runtime validation is the enforcement mechanism. Documentation is for humans.

2. **Must build tsuku for CI validation**: Can't validate without compiling.
   - *Acceptable because*: CI already builds tsuku; this adds minimal overhead.

3. **Initial exclusions list may be large**: During migration, many gaps may exist.
   - *Acceptable because*: This is temporary; exclusions shrink as we migrate recipes back.

## Solution Architecture

### Overview

The solution adds a flag to the loader and a CI workflow:

1. **`--require-embedded` flag**: When set, action dependencies must resolve from embedded registry
2. **CI validation workflow**: Runs `tsuku eval` with the flag for representative recipes
3. **Exclusions file**: Lists known gaps that won't fail CI
4. **EMBEDDED_RECIPES.md**: Manual documentation of embedded recipes

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    Loader (internal/recipe/loader.go)           │
│                                                                 │
│  LoadRecipe(name, opts)                                         │
│      │                                                          │
│      ├─► if opts.RequireEmbedded && isActionDependency(name)   │
│      │       └─► Only check embedded FS                         │
│      │       └─► Fail if not found                              │
│      │                                                          │
│      └─► else: normal priority chain                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    CI Workflow                                   │
│                                                                 │
│  for recipe in test_recipes:                                    │
│      tsuku eval $recipe --require-embedded                      │
│      if failure && recipe not in exclusions:                    │
│          FAIL                                                    │
└─────────────────────────────────────────────────────────────────┘
```

### Key Interfaces

**LoaderOptions struct** (new or extended):
```go
type LoaderOptions struct {
    RequireEmbedded bool // If true, action deps must come from embedded FS
    // ... existing options
}
```

**CLI flag**:
```
tsuku eval <recipe> --require-embedded
tsuku install <recipe> --require-embedded
```

**Error message** (when embedded recipe not found):
```
Error: action dependency "rust" requires embedded recipe, but "rust" is not in embedded registry.

This error occurs because --require-embedded is set and the action "cargo_install"
depends on "rust", which must be available without network access.

To fix: ensure "rust" recipe exists in internal/recipe/recipes/r/rust.toml
```

**Exclusions file** (`embedded-validation-exclusions.json`):
```json
{
  "exclusions": [
    {
      "recipe": "some-recipe",
      "reason": "Depends on X which is being migrated in #1234",
      "issue": "#1234"
    }
  ]
}
```

### Data Flow

1. **User runs**: `tsuku eval cargo-watch --require-embedded`
2. **Loader resolves recipe**: `cargo-watch` loads normally (it's the target, not an action dep)
3. **Plan generation**: `cargo_install` action declares dependency on `rust`
4. **Dependency resolution**: Loader sees `RequireEmbedded=true` for action deps
5. **Embedded check**: Loader looks for `rust` in embedded FS only
6. **If found**: Continue normally
7. **If not found**: Error with actionable message

For transitive dependencies:
- `rust` recipe has `dependencies = ["some-lib"]`
- When loading `rust`, its dependencies also use `RequireEmbedded` mode
- Any missing transitive dependency fails with clear indication

## Implementation Approach

### Step 1: Add Flag to Loader

Modify `internal/recipe/loader.go`:

```go
type LoaderOptions struct {
    RequireEmbedded bool
}

func (l *Loader) LoadRecipe(name string, opts LoaderOptions) (*Recipe, error) {
    // When RequireEmbedded is set for action dependencies,
    // only check embedded FS, skip local and registry
    if opts.RequireEmbedded {
        recipe, err := l.loadFromEmbedded(name)
        if err != nil {
            return nil, fmt.Errorf(
                "action dependency %q requires embedded recipe, but not found in embedded registry: %w",
                name, err,
            )
        }
        return recipe, nil
    }

    // Normal priority chain for non-action-dep or when flag not set
    return l.loadWithPriorityChain(name)
}
```

### Step 2: Add CLI Flag

Modify `cmd/tsuku/eval.go` and `cmd/tsuku/install.go`:

```go
var requireEmbedded bool

func init() {
    evalCmd.Flags().BoolVar(&requireEmbedded, "require-embedded", false,
        "Require action dependencies to resolve from embedded registry")
}
```

### Step 3: Create Initial EMBEDDED_RECIPES.md

Manually create documentation based on known action dependencies:

```markdown
# Embedded Recipes

This file documents recipes that must remain in `internal/recipe/recipes/`
because they are dependencies of tsuku's actions.

## Toolchain Recipes

| Recipe | Required By | Rationale |
|--------|-------------|-----------|
| golang | go_install, go_build | Go toolchain for building Go packages |
| rust | cargo_install, cargo_build | Rust toolchain for building crates |
| nodejs | npm_install, npm_exec | Node.js for npm packages |
| python-standalone | pip_install, pipx_install | Python for pip/pipx packages |
| ruby | gem_install, gem_exec | Ruby for gem packages |
| perl | cpan_install | Perl for CPAN modules |

## Build Tool Recipes

| Recipe | Required By | Rationale |
|--------|-------------|-----------|
| make | configure_make, cmake_build, meson_build | GNU Make |
| cmake | cmake_build | CMake build system |
| meson | meson_build | Meson build system |
| ninja | meson_build | Ninja build tool |
| zig | configure_make, cmake_build, meson_build | Zig CC for cross-compilation |
| pkg-config | configure_make, cmake_build, meson_build | Package configuration |
| patchelf | homebrew_relocate, meson_build (Linux) | ELF binary patching |

## Library Recipes (Transitive Dependencies)

| Recipe | Required By | Rationale |
|--------|-------------|-----------|
| libyaml | ruby | YAML parsing for Ruby |
| zlib | openssl, libcurl | Compression library |
| openssl | cmake, libcurl | TLS/crypto library |

## Notes

- This list is validated by CI using `--require-embedded` flag
- Gaps are tracked in `embedded-validation-exclusions.json`
- To add a recipe: add to `internal/recipe/recipes/`, update this file
```

### Step 4: Create Exclusions File

Create `embedded-validation-exclusions.json`:

```json
{
  "description": "Recipes excluded from --require-embedded validation during migration",
  "exclusions": []
}
```

### Step 5: Add CI Workflow

Create `.github/workflows/validate-embedded-deps.yml`:

```yaml
name: Validate Embedded Dependencies

on:
  push:
    paths:
      - 'internal/recipe/recipes/**'
      - 'internal/actions/**'
      - 'internal/recipe/loader.go'
      - 'embedded-validation-exclusions.json'
  pull_request:
    paths:
      - 'internal/recipe/recipes/**'
      - 'internal/actions/**'
      - 'internal/recipe/loader.go'
      - 'embedded-validation-exclusions.json'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Validate embedded action dependencies
        run: |
          # Test recipes that exercise each action type
          declare -A test_recipes=(
            ["go_install"]="gofumpt"
            ["cargo_install"]="cargo-audit"
            ["npm_install"]="netlify-cli"
            ["pipx_install"]="ruff"
            ["gem_install"]="bundler"
            ["cpan_install"]="ack"
            ["homebrew"]="make"
          )

          # Load exclusions
          exclusions=$(jq -r '.exclusions[].recipe' embedded-validation-exclusions.json 2>/dev/null || echo "")

          failed=0
          for action in "${!test_recipes[@]}"; do
            recipe="${test_recipes[$action]}"

            # Skip if excluded
            if echo "$exclusions" | grep -q "^${recipe}$"; then
              echo "SKIP: $recipe (excluded)"
              continue
            fi

            echo "TEST: $recipe (exercises $action)"
            if ! ./tsuku eval "$recipe" --require-embedded 2>&1; then
              echo "FAIL: $recipe"
              failed=1
            fi
          done

          exit $failed
```

### Step 6: Iterative Migration Process

1. **Start**: All recipes in `internal/recipe/recipes/`, exclusions empty
2. **Migrate**: Move non-essential recipes to `recipes/`
3. **CI fails**: Identifies missing embedded recipes
4. **Add exclusion**: Temporarily exclude the failing recipe
5. **Fix**: Either migrate recipe back to embedded or update exclusion with issue link
6. **Repeat**: Until exclusions list is empty or contains only documented exceptions

## Consequences

### Positive

- **Ground truth validation**: Uses actual loader, not parallel logic
- **Incremental migration**: Exclusions enable gradual improvement
- **Clear failures**: Error messages indicate exactly what's missing
- **No new tools**: Flag added to existing commands
- **Self-documenting**: Exclusions file shows exactly what's incomplete

### Negative

- **Manual documentation**: EMBEDDED_RECIPES.md must be maintained by hand
- **Exclusions discipline**: Team must actively shrink exclusions list
- **Build required**: Can't validate without compiling tsuku

### Mitigations

- **Documentation drift**: Runtime validation is the enforcement; docs are supplementary
- **Exclusions growth**: CI job can warn when exclusions exceed threshold
- **Build time**: Module cache makes builds fast; validation runs only on relevant changes

## Security Considerations

### Download Verification

**Not applicable** - This feature restricts where recipes are loaded from; it doesn't add new download paths. When `--require-embedded` is set, recipes load from the embedded FS only (no downloads).

### Execution Isolation

**Low risk** - The flag restricts behavior, not expands it:
- Limits recipe sources to embedded FS
- Fails fast rather than falling back to network
- No new permissions required

### Supply Chain Risks

**Improves security** - The primary purpose is to ensure bootstrap integrity:
- Action dependencies are guaranteed to be embedded (no network required)
- Reduces attack surface during installation
- Offline installation becomes more reliable

### User Data Exposure

**Not applicable** - This feature:
- Does not access user data
- Does not transmit data externally
- Only affects recipe loading source

### Security Summary

| Dimension | Risk Level | Notes |
|-----------|------------|-------|
| Download Verification | N/A | Restricts downloads, doesn't add them |
| Execution Isolation | Low | Restricts behavior |
| Supply Chain | Positive | Improves bootstrap integrity |
| User Data | N/A | No user data involved |

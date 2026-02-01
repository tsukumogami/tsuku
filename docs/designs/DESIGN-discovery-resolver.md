---
status: Accepted
problem: tsuku requires --from flags on every create invocation, but users expect tsuku install <tool> to just work without knowing the source.
decision: Three-stage resolver (embedded registry, parallel ecosystem probe, LLM fallback) behind a unified tsuku install entry point, with disambiguation via registry overrides and popularity ranking.
rationale: A registry handles the top ~500 tools instantly without API keys, ecosystem probes cover the middle ground under 3 seconds, and LLM discovery handles the long tail. This layered approach degrades gracefully when keys are missing or APIs are down.
---

# DESIGN: Discovery Resolver

**Status**: Proposed

## Context and Problem Statement

tsuku v0.4.x has eight recipe builders (cargo, gem, pypi, npm, go, cpan, github, homebrew) and a growing recipe registry. But every `tsuku create` invocation requires the user to specify `--from github:owner/repo` or `--from crates.io`. Users also need to know whether a recipe already exists before choosing between `tsuku install` and `tsuku create`.

For new users, this is a poor first experience. Users expect `tsuku install stripe-cli` to work without knowing that stripe-cli is distributed as a GitHub release. They don't care about the source, and they shouldn't need to distinguish between "install an existing recipe" and "generate a new recipe."

The problem breaks into four parts:

1. **Command convergence**: `tsuku install` should be the single entry point, falling back to recipe generation when no recipe exists.
2. **Source resolution**: Given a tool name, figure out where it comes from (which builder and which source argument).
3. **Disambiguation**: When multiple ecosystem registries claim to have "bat", pick the right one.
4. **Graceful degradation**: Work without API keys for registry and ecosystem tools; degrade to clear error messages when LLM isn't available.

### Scope

**In scope:**
- `tsuku install` convergence with `tsuku create`
- Resolver interface and `DiscoveryResult` type
- Discovery registry format, scope, bootstrap, and update mechanism
- Parallel ecosystem probe with timeout and filtering
- Disambiguation strategy (registry overrides, popularity ranking, user prompt)
- LLM discovery as last resort (web search, user confirmation, prompt injection defense)
- Error and fallback UX for every failure mode
- Integration points with existing builder and recipe infrastructure
- Post-launch evolution path toward a unified tool index

**Out of scope:**
- Documentation builder (separate design: DESIGN-documentation-builder.md)
- Recipe contribution workflow (separate design: DESIGN-recipe-contribution.md)
- Checksum verification (separate issue)
- Individual builder improvements
- Windows support

## Decision Drivers

- **First-impression reliability**: The top ~500 developer tools must resolve without LLM, instantly
- **No API key requirement for common tools**: Deterministic and ecosystem builders should work without API keys
- **Latency budget**: Registry hits under 100ms, ecosystem probes under 3 seconds, LLM discovery under 15 seconds
- **Disambiguation correctness**: Wrong resolution (installing npm's `bat` instead of sharkdp/bat) destroys trust
- **Build on existing infrastructure**: Eight builders, sandbox validation, repair loop, recipe loader all exist
- **Graceful degradation**: Each resolver stage should fail independently without blocking subsequent stages
- **Minimal maintenance burden**: The discovery registry should stay small and shrink as recipe coverage grows

## Implementation Context

### Existing Patterns

**Recipe Loader** (`internal/recipe/loader.go`): Already implements a priority chain (cache → local → embedded → registry). The discovery resolver follows a similar pattern but resolves `{builder, source}` pairs instead of recipes.

**Builder Registry** (`internal/builders/registry.go`): Thread-safe map of `SessionBuilder` implementations with `Get(name)` lookup. The discovery result's `Builder` field maps directly to this registry.

**SessionBuilder interface** (`internal/builders/builder.go`): Defines `CanBuild(ctx, req)` which validates that a package exists on the ecosystem. The ecosystem probe can use this directly.

**Version Provider Factory** (`internal/version/provider_factory.go`): Strategy-based provider selection with priorities. The resolver uses a similar priority pattern.

**`parseFromFlag()`** in `cmd/tsuku/create.go`: Parses `builder:arg` syntax. The discovery result produces the same pair that `--from` would parse to.

### Conventions to Follow

- New packages go in `internal/discover/` (parallel to `internal/builders/`, `internal/recipe/`)
- Interfaces defined in a `resolver.go` file, implementations in separate files per stage
- Context-aware with `context.Context` for cancellation and timeouts
- Errors wrap with `fmt.Errorf("...: %w", err)` for chain inspection

## Considered Options

### Decision 1: Resolver Architecture

How should the resolver chain be organized?

#### Option A: Sequential Chain with Short-Circuit

Each resolver stage runs in sequence. If a stage returns a confident match, stop. If it returns nothing, try the next stage. The registry is checked first (instant), then ecosystem probe (up to 3s), then LLM (up to 15s).

**Pros:**
- Simple to reason about: deterministic ordering, predictable latency
- Easy to test each stage in isolation
- Short-circuit means common tools are fast
- Matches the recipe loader's existing priority chain pattern

**Cons:**
- Ecosystem probe always waits for registry miss (though registry is instant, so minimal impact)
- Adding new stages requires deciding where they go in the chain

#### Option B: Parallel Resolution with Priority Merge

All stages run simultaneously. Results are collected and the highest-priority match wins. Registry results beat ecosystem results, which beat LLM results.

**Pros:**
- Lowest possible latency for every query
- Ecosystem probe runs while registry lookup happens (though registry is <1ms)

**Cons:**
- Wastes API calls: ecosystem APIs get queried even for tools in the registry
- LLM gets invoked even when ecosystem finds the tool, wasting money
- More complex cancellation logic
- Harder to test because stages interact

#### Option C: Registry-then-Parallel with LLM Gating

Registry lookup runs first (instant). On miss, ecosystem probe runs. On miss, LLM runs. But ecosystem probe queries all ecosystems in parallel (not sequentially).

This is effectively Option A, but clarifies that "ecosystem probe" itself is internally parallel while the three stages are sequential.

**Pros:**
- No wasted API calls (ecosystem only runs on registry miss)
- No wasted LLM cost (LLM only runs on ecosystem miss)
- Ecosystem probing is still fast because it's internally parallel
- Simple top-level flow, parallel only where it matters

**Cons:**
- Slightly higher latency than full parallel for ecosystem-resolved tools (registry miss adds ~0ms, so negligible)
- Requires two levels of concurrency (stage sequencing + ecosystem parallelism)

### Decision 2: Discovery Registry Storage

Where does the discovery registry live and how is it updated?

#### Option A: Embedded in Binary

Ship the registry JSON as an embedded Go file (using `//go:embed`). Updated with each tsuku release. No separate update mechanism.

**Pros:**
- Zero network requests for registry lookup
- Works offline and in air-gapped environments
- Integrity guaranteed by binary signing
- Simplest implementation

**Cons:**
- Registry only updates with new tsuku releases
- Can't fix a bad registry entry without releasing a new binary

#### Option B: Embedded with Registry-Sync Override

Embed a baseline registry in the binary, but allow `tsuku update-registry` to fetch a newer version from the recipes repository. The local override takes precedence over the embedded version.

**Pros:**
- Fast default (embedded)
- Can update registry between binary releases
- Follows the same pattern as recipe registry updates
- Graceful fallback to embedded when network is unavailable

**Cons:**
- Two sources of truth (embedded vs. local override)
- Need integrity verification for downloaded registry
- More complex loader logic

#### Option C: Remote-Only Registry

No embedded registry. Always fetch from the recipes repository (cached locally).

**Pros:**
- Always up to date
- Single source of truth

**Cons:**
- First run requires network access
- Breaks offline use
- Adds latency to first invocation

### Decision 3: Ecosystem Probe Filtering

How aggressively should ecosystem probe results be filtered to exclude typosquats and abandoned packages?

#### Option A: Threshold Filtering (Age + Downloads)

Reject ecosystem matches below minimum thresholds: >90 days old AND >1000 downloads/month. These thresholds are conservative enough to exclude typosquats while keeping legitimate tools.

**Pros:**
- Simple, deterministic rules
- Catches most typosquats (newly registered, low download)
- Easy to explain to users

**Cons:**
- New legitimate tools fail the age check for 90 days
- Download thresholds vary wildly across ecosystems (1000/month is huge on CPAN, tiny on npm)
- Thresholds need per-ecosystem tuning

#### Option B: Ecosystem-Specific Thresholds

Each ecosystem gets its own thresholds based on its scale. npm might require 10K downloads/month, CPAN might require 100.

**Pros:**
- More accurate filtering per ecosystem
- Fewer false negatives for smaller ecosystems

**Cons:**
- More configuration to maintain
- Thresholds become stale as ecosystems grow
- Hard to justify specific numbers

#### Option C: No Filtering, Rely on Disambiguation UX

Don't filter results. Instead, show all matches to the user with metadata (downloads, age, description) and let them pick. For single matches, auto-select but show metadata.

**Pros:**
- No false negatives from over-filtering
- User makes the final call
- Simpler implementation

**Cons:**
- Typosquats appear as options (with metadata that should reveal them)
- More prompting friction for the user
- Automated/scripted usage becomes harder

### Uncertainties

- **Ecosystem API reliability**: We haven't measured probe latency under real-world conditions across all seven ecosystems simultaneously
- **Popularity data availability**: Not all ecosystem APIs return download counts in the same format; some may not expose it at all
- **Name collision frequency**: We don't know how often tool names collide across ecosystems in practice
- **Registry bootstrap quality**: One-time import from external registries may include stale or incorrect entries
- **LLM web search reliability**: Web search tool accuracy for finding official tool sources hasn't been validated

## Decision Outcome

**Chosen: 1C + 2C + 3A**

### Summary

Sequential resolver chain (registry → parallel ecosystem probe → LLM) with a remote-fetched discovery registry (cached locally) and threshold-based ecosystem filtering. The three stages run sequentially at the top level, but the ecosystem probe queries all registries in parallel internally.

### Rationale

**1C over 1A/1B**: Option C is the clearest decomposition. The top-level flow is sequential (no wasted API calls or LLM cost), while ecosystem probing is internally parallel (keeping latency under 3 seconds). Option B's full parallelism wastes resources on tools already in the registry. Option A is essentially the same as 1C once you clarify that "ecosystem probe" means parallel queries.

**2C over 2A/2B**: Remote-only keeps things simple: one source of truth, always up to date, no embedded-vs-local precedence logic. The registry is fetched on first use and cached locally, following the same pattern as the recipe registry. Option 2B (embedded + sync) adds surface area for little value when the recipe registry already proves the remote-fetch-and-cache pattern works. Option 2A is too rigid. The trade-off is that first run requires network access, but tsuku already requires network for recipe fetching and tool installation.

**3A over 3B/3C**: Threshold filtering is simple and catches the obvious cases. Per-ecosystem thresholds (3B) add maintenance burden for marginal improvement. No filtering (3C) exposes users to typosquats. We can start with conservative global thresholds and add per-ecosystem tuning later if collision data shows it's needed.

### Trade-offs Accepted

- The discovery registry requires curation. Its scope is narrow (~500 entries) and shrinks as recipe coverage grows via batch generation.
- First run requires network access for registry fetch. This matches tsuku's existing behavior for recipe registry fetching.
- Global thresholds will occasionally filter out legitimate new tools. Users can bypass discovery with `--from` for these cases.
- The sequential chain means ecosystem-resolved tools pay ~0ms extra for the registry miss. This is negligible.

## Solution Architecture

### Overview

The discovery resolver sits between `tsuku install` and the existing builder infrastructure. When a user runs `tsuku install <tool>` and no recipe exists, the resolver determines which builder and source to use, then delegates to the existing `tsuku create` pipeline.

### Command Convergence

`tsuku install` gains a fallback path:

```
tsuku install <tool>
    |
    v
Recipe exists? --yes--> Install directly (existing behavior)
    |
    no
    |
    v
DiscoveryResolver.Resolve(tool)
    |
    v
DiscoveryResult{Builder, Source, Confidence}
    |
    v
Existing create pipeline (builder → sandbox → install)
```

`tsuku create` remains available for explicit use: `--from` overrides discovery entirely, `--force` regenerates even when a recipe exists. The typical user never needs `create`.

`tsuku install` also accepts `--from` to override discovery without switching to `create`. Internally, `install --from` forwards to the create pipeline, keeping `install` as the single entry point even for power users.

`tsuku install` inherits the `--deterministic-only` flag from `create`. When set, the resolver skips the LLM discovery stage entirely, and if the selected builder requires LLM (`RequiresLLM() == true`), the create pipeline fails with an actionable error rather than silently falling back. This is distinct from the "no API key" case: `--deterministic-only` is an explicit choice to avoid LLM, while a missing key is a configuration gap.

### Components

```
internal/discover/
├── resolver.go           # Resolver interface and DiscoveryResult type
├── chain.go              # ChainResolver orchestrating the three stages
├── registry.go           # Registry data types and JSON loading
├── registry_lookup.go    # Stage 1: registry lookup
├── ecosystem_probe.go    # Stage 2: parallel ecosystem API queries
├── llm_discovery.go      # Stage 3: LLM web search fallback
├── disambiguation.go     # Popularity ranking and user prompting
└── normalize.go          # Input normalization and homoglyph detection
```

### Key Interfaces

```go
// DiscoveryResult describes where a tool can be sourced from.
type DiscoveryResult struct {
    Builder    string   // Builder name (maps to builders.Registry)
    Source     string   // Builder-specific source arg (e.g., "owner/repo")
    Confidence string   // "registry", "ecosystem", or "llm"
    Reason     string   // Human-readable explanation for display
    Metadata   Metadata // Optional: stars, downloads, age (for confirmation UX)
}

// Resolver resolves a tool name to a source.
type Resolver interface {
    Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error)
}
```

Each stage implements `Resolver`. `ChainResolver` wraps them:

```go
type ChainResolver struct {
    stages []Resolver // registry, ecosystem, llm - tried in order
}

func (c *ChainResolver) Resolve(ctx context.Context, name string) (*DiscoveryResult, error) {
    for _, stage := range c.stages {
        result, err := stage.Resolve(ctx, name)
        if err != nil {
            // Hard errors (context cancelled, budget exhausted) stop the chain
            if ctx.Err() != nil || isFatalError(err) {
                return nil, err
            }
            // Soft errors (timeout, API error) log and continue
            continue
        }
        if result != nil {
            return result, nil
        }
    }
    return nil, &NotFoundError{Tool: name}
}
```

Errors fall into two categories:
- **Soft errors** (stage couldn't find the tool, API timeout, rate limit on a single ecosystem): log and try the next stage
- **Hard errors** (context cancelled, LLM budget exhausted, invalid tool name): stop the chain and return the error

Tool name input is normalized before resolution: Unicode homoglyph detection and lowercasing prevent confusable-character attacks (e.g., `kubеctl` with a Cyrillic "е").
```

### Discovery Registry Format

```json
{
  "schema_version": 1,
  "tools": {
    "kubectl": {"builder": "github", "source": "kubernetes/kubernetes", "binary": "kubectl"},
    "ripgrep": {"builder": "github", "source": "BurntSushi/ripgrep"},
    "bat": {"builder": "github", "source": "sharkdp/bat"},
    "jq": {"builder": "github", "source": "jqlang/jq"},
    "stripe-cli": {"builder": "github", "source": "stripe/stripe-cli"}
  }
}
```

The registry is a JSON file fetched from the recipes repository and cached locally at `$TSUKU_HOME/registry/discovery.json`. It's updated via `tsuku update-registry` (same mechanism as recipe registry updates). Fields:

- `builder` (required): Builder name from the builder registry
- `source` (required): Builder-specific source argument
- `binary` (optional): Binary name when it differs from the tool name

Two categories of entries:
1. **GitHub-release tools** not in any ecosystem registry (kubectl, terraform, stripe-cli, jq)
2. **Disambiguation overrides** for tools whose names collide across ecosystems (bat, fd, serve)

Target size: ~500 entries at launch, bootstrapped from a one-time import of external registry data.

### Ecosystem Probe

The probe queries all ecosystem registries in parallel with a shared 3-second timeout:

```go
type EcosystemProbe struct {
    builders []builders.SessionBuilder
    timeout  time.Duration // 3 seconds
}

func (p *EcosystemProbe) Resolve(ctx context.Context, name string) (*DiscoveryResult, error) {
    ctx, cancel := context.WithTimeout(ctx, p.timeout)
    defer cancel()

    // Query all builders in parallel using CanBuild()
    // Collect matches, filter by thresholds, disambiguate
}
```

The probe runs against **ecosystem builders only** (cargo, gem, pypi, npm, go, cpan, cask). The github and homebrew builders are excluded because they require a source argument (`owner/repo` or formula name) that discovery is trying to find.

Each ecosystem builder's `CanBuild()` checks whether a package exists, but returns only `bool`. The ecosystem probe needs metadata (downloads, age) for filtering and disambiguation. This requires a new optional interface:

```go
// EcosystemProber extends SessionBuilder with metadata for discovery.
type EcosystemProber interface {
    SessionBuilder
    Probe(ctx context.Context, name string) (*ProbeResult, error)
}

type ProbeResult struct {
    Exists    bool
    Downloads int    // Monthly downloads (0 if unavailable)
    Age       int    // Days since first publish (0 if unavailable)
    Source    string // Builder-specific source arg
}
```

Builders that implement `EcosystemProber` participate in discovery. Others are skipped. This avoids changing the existing `SessionBuilder` interface.

**Filtering**: Matches below the age/download threshold (>90 days, >1000 downloads/month) are discarded. These thresholds are noise reduction, not a security boundary. Patient attackers can bypass them. The real security comes from registry overrides for the top 500 tools, disambiguation UX showing metadata, and user confirmation for ambiguous results.

### Disambiguation Strategy

When multiple ecosystems match:

1. **Registry wins**: If the tool is in the discovery registry, use that (no ambiguity)
2. **Edit-distance check**: Flag results whose name is suspiciously close to a registry entry (Levenshtein distance <=2). Display a warning: `"Did you mean 'ripgrep'? Found 'rigrep' on npm (23 downloads/month)."`
3. **Single match**: If only one ecosystem passes filters, use it
4. **Popularity ranking**: Rank by download count. If the top result has >10x the runner-up's downloads, auto-select it
5. **User prompt**: If the top two are close, prompt the user to choose
6. **Ecosystem priority tiebreaker**: When popularity data is unavailable, fall back to: Homebrew > crates.io > PyPI > npm > RubyGems > Go > CPAN

Display on auto-select: `"Found bat (sharkdp/bat via crates.io, 45K downloads/day). Also available: npm (bat-cli, 200 downloads/day). Use --from to override."`

**Non-interactive mode** (piped stdin or `--yes`): Auto-select the top match by popularity. If the top two are within 10x, error with a message listing the options and suggesting `--from`.

### LLM Discovery

When registry and ecosystem probe both miss:

1. Invoke LLM with web search tool to find the official source
2. LLM returns a structured JSON response (`extract_source` tool call) with builder, source, and reasoning
3. **Verify the source**: For GitHub sources, query the GitHub API to confirm the repo exists, isn't archived, and matches the expected tool. This catches prompt injection attempts that steer the LLM toward non-existent or malicious repos.
4. Display confirmation with metadata: repo age, star count, last commit, owner
5. User must confirm unless `--yes` is set
6. On confirmation, delegate to the recommended builder

The LLM can recommend any builder type (including the documentation builder once available).

### Error and Fallback UX

| Scenario | Message |
|----------|---------|
| No match anywhere | `Could not find 'foo'. Try tsuku install foo --from github:owner/repo if you know the source.` |
| LLM not configured | `No match in registry or ecosystems. Set ANTHROPIC_API_KEY to enable web search discovery, or use --from to specify the source directly.` |
| `--deterministic-only`, no ecosystem match | `No deterministic source found for 'foo'. Remove --deterministic-only to enable LLM discovery, or use --from to specify the source.` |
| `--deterministic-only`, builder requires LLM | `'foo' resolved to GitHub releases (owner/repo), which requires LLM for recipe generation. Remove --deterministic-only or wait for a recipe to be contributed.` |
| Ecosystem probe timeout | Silently fall through to LLM. Show timeout warning in `--verbose` mode. |
| LLM rate limit/budget | `LLM discovery unavailable (rate limit). Try --from <source> to specify the source directly.` |
| Ecosystem ambiguity (close) | Interactive prompt listing matches with metadata |

### Data Flow: Integration with Existing Code

**`cmd/tsuku/install.go`**: After recipe lookup fails, call `discover.NewChainResolver().Resolve(ctx, toolName)`. On success, convert `DiscoveryResult` to a `builders.BuildRequest` and delegate to the existing create pipeline.

**`cmd/tsuku/create.go`**: When `--from` is omitted, call the resolver instead of erroring. The resolver result produces the same `{builder, source}` pair that `parseFromFlag()` would.

**`internal/builders/registry.go`**: No changes. `DiscoveryResult.Builder` maps to `registry.Get(name)`.

**`internal/recipe/loader.go`**: No changes to the loader. Discovery only runs after the loader confirms no recipe exists.

### Post-Launch Evolution

The discovery registry and the batch pipeline's priority queue serve related purposes: both map tool names to sources. Post-launch, they can converge into a unified tool index:

1. **Automated scraping** of ecosystem registries produces a catalog of tool names and sources
2. **Offline disambiguation** resolves name collisions (batch LLM, not real-time)
3. **The index replaces both** the curated discovery registry and the batch pipeline's seed list

This convergence is a future concern. The current design anticipates it by keeping the registry format simple and the `Resolver` interface stable.

## Implementation Approach

### Phase 1: Resolver Interface and Registry Lookup

Define the `Resolver` interface, `DiscoveryResult` type, and `ChainResolver`. Implement the registry lookup stage with embedded JSON. This is the foundation everything else builds on.

- Define `internal/discover/` package with core types
- Implement registry loading from remote JSON (cached locally, same pattern as recipe registry)
- Implement `RegistryLookup` resolver
- Implement `ChainResolver` with soft/hard error distinction
- Add input normalization (Unicode homoglyph detection, lowercasing)
- Extend `tsuku update-registry` to fetch discovery registry alongside recipe registry

### Phase 2: Discovery Registry Bootstrap

Populate the registry with ~500 entries. This must happen before convergence so the install fallback has data to resolve against.

- One-time import from external registry data
- Validate entries against actual GitHub repos (existence, not archived)
- Add disambiguation overrides for known collisions
- Embed in binary

### Phase 3: Install/Create Convergence

Make `tsuku install` the universal entry point by adding the discovery fallback path.

- Modify `tsuku install` to fall back to discovery + create when no recipe exists
- Add `--from` flag to `tsuku install` (forwards to create pipeline)
- Keep `tsuku create --from` as explicit override
- Display discovery confidence to user
- Handle `--yes` and non-interactive (piped) mode

### Phase 4: Ecosystem Probe

Add parallel ecosystem probing as the second resolver stage.

- Define `EcosystemProber` interface (extends `SessionBuilder` with `Probe()` method)
- Implement `EcosystemProbe` resolver with parallel queries and 3-second timeout
- Implement threshold filtering (age, downloads)
- Implement disambiguation logic (edit-distance check, popularity ranking, user prompt, non-interactive mode)
- Wire into the chain resolver

### Phase 5: LLM Discovery

Add web search and LLM analysis as the final resolver fallback.

- Implement `LLMDiscovery` using existing LLM client
- Add web search tool for the LLM conversation
- Structured JSON output from LLM (`extract_source` tool call)
- GitHub API verification of LLM-recommended sources
- Rich confirmation display (stars, age, owner)
- User confirmation requirement (unless `--yes`)
- Prompt injection defenses (HTML stripping, URL validation)

### Phase 6: Error UX and Verbose Mode

Polish the error messages and add `--verbose` debugging.

- Implement all error/fallback messages from the table above
- Add `--verbose` output showing resolver chain progress
- Add telemetry events for discovery usage patterns

## Security Considerations

### Download Verification

The discovery resolver doesn't download artifacts itself. It determines `{builder, source}` which the existing builder pipeline then uses. However, **discovery misdirection is equivalent to download misdirection**: pointing a user at the wrong source is as dangerous as corrupting a download. The resolver's integrity is part of the supply chain.

Mitigation: The registry is fetched from the recipes repository over HTTPS and cached locally, following the same trust model as recipe registry updates. Ecosystem results are filtered by age/download thresholds. LLM results require user confirmation with repo metadata.

### Execution Isolation

The resolver runs read-only queries:
- Registry lookup: file read (no network)
- Ecosystem probe: HTTP GET to public APIs (crates.io, PyPI, npm, etc.)
- LLM discovery: API call to LLM provider + web search

No code execution happens during discovery. Generated recipes go through existing sandbox validation before installation.

### Supply Chain Risks

**Discovery misdirection / dependency confusion.** An attacker could register a tool name on an ecosystem registry to hijack discovery. Mitigations:
- Registry entries override ecosystem results for the top ~500 tools
- Ecosystem probe filters by age (>90 days) and download count (>1000/month), excluding newly-registered typosquats
- Disambiguation shows multiple matches with metadata, letting the user spot imposters
- LLM discovery requires explicit user confirmation with repo age, stars, and owner
- Sandbox validation catches recipes with unexpected behavior

**LLM prompt injection.** Web pages fetched during LLM discovery could contain hidden text steering the LLM toward malicious sources. Mitigations:
- Strip hidden HTML elements before passing to LLM
- Validate LLM-extracted URLs against expected patterns (e.g., `github.com/owner/repo`)
- User confirmation required for all LLM-discovered sources
- Sandbox validation as defense in depth

**Registry integrity.** The discovery registry controls what gets installed for the top ~500 tools. It's fetched over HTTPS from the recipes repository and cached locally, following the same trust model as the recipe registry. An attacker who compromises the repository or performs a MITM attack on the HTTPS connection could modify registry entries. Mitigation: the registry lives in the same repository as recipes, so its integrity is covered by the same review and access controls. The HTTPS transport prevents tampering in transit.

**Registry staleness.** A registry entry pointing to a compromised repo continues directing users there until updated. Mitigations:
- `tsuku update-registry` fetches the latest registry
- Automated freshness checks (verifying repos still exist and aren't archived) can run as a periodic CI job
- Registry entries don't pin ownership: a transferred GitHub repo could become malicious. Post-launch, freshness checks should verify that the repo owner hasn't changed since the entry was created

### User Data Exposure

**Ecosystem API queries**: Tool names are sent to crates.io, PyPI, npm, etc. during ecosystem probing. This is equivalent to searching those registries directly. No authentication is required.

**LLM queries**: When LLM discovery is used, the tool name is sent as a web search query via the LLM provider. Users who don't set an API key never trigger this. The existing telemetry opt-out controls tsuku's own telemetry but doesn't affect data sent to the LLM provider.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Discovery misdirection | Curated registry for top 500, edit-distance checking, disambiguation UX, sandbox | Sophisticated typosquatting with clean sandbox behavior |
| Typosquatting via ecosystem | Age/download thresholds (noise reduction), edit-distance vs. registry entries | Patient attacker with gameable downloads |
| LLM prompt injection | HTML stripping, structured JSON output, GitHub API verification, user confirmation | Novel injection via visible text in search results |
| Registry staleness | update-registry, freshness checks, ownership change detection | Zero-day repo compromise |
| LLM recommends malicious source | Rich confirmation display, GitHub API verification, sandbox validation | User ignores warning signs |
| Registry integrity | HTTPS transport, same repo access controls as recipes | Repository compromise |
| Unicode/homoglyph confusion | Input normalization before resolution | Novel Unicode attacks |
| Ecosystem API abuse | 3-second timeout, no authentication required | API-side rate limiting |

## Consequences

### Positive

- `tsuku install <tool>` becomes the single entry point, matching user expectations
- The top ~500 tools resolve instantly from the embedded registry without API keys
- Ecosystem probing covers thousands of tools under 3 seconds
- LLM discovery handles the long tail for users with API keys
- The resolver interface is stable and extensible for future stages
- Discovery registry scope is narrow and shrinks as recipe coverage grows

### Negative

- Discovery adds complexity to the install path (encapsulated behind the `Resolver` interface)
- The registry requires curation (~500 entries, shrinking over time)
- Global filtering thresholds won't be perfect for every ecosystem
- `install`/`create` convergence changes a user-visible interface (mitigated by keeping `create` as explicit command)
- Users without API keys can't discover tools outside the registry and ecosystem registries

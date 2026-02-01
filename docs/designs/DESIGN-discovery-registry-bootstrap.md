---
status: Proposed
problem: |
  The discovery registry has 1 entry but needs ~500 for the resolver to deliver value.
  The current schema conflates tool identity (where it's maintained, what it does) with
  install instructions (which builder to use). This blocks future evolution: an LLM builder
  could infer install paths from metadata, and richer data improves disambiguation UX.
  The bootstrap must populate entries that serve today's resolver while collecting metadata
  for future builders and features.
decision: |
  Evolve the registry schema to v2: keep builder+source as required fields (today's resolver
  works unchanged), add optional metadata fields (repo, homepage, description, disambiguation
  flag). A Go CLI tool reads curated seed lists, validates via builder-specific API checks,
  enriches with metadata from the same API responses, detects ecosystem name collisions, and
  outputs discovery.json. CI runs weekly freshness checks. Schema v3 (optional install fields)
  is deferred until an LLM builder exists to consume metadata-only entries.
rationale: |
  Separating tool identity from install path is a low-cost, backward-compatible schema change
  that collects useful data now without requiring consumers. Keeping builder+source required
  avoids premature abstraction. Curated seed lists focus manual effort on the high-value
  decision (correct source for each tool) while automating mechanical work (description,
  homepage, validation). The Go tool reuses patterns from existing seed-queue infrastructure.
---

# DESIGN: Discovery Registry Bootstrap

## Status

Proposed

## Upstream Design Reference

This design implements Phase 2 of [DESIGN-discovery-resolver.md](DESIGN-discovery-resolver.md), and proposes evolving the discovery registry schema from install-only mappings to a richer tool metadata index.

**Relevant upstream sections:**
- Discovery Registry Format (current): `{builder, source, binary?}`
- Two entry categories: GitHub-release tools and disambiguation overrides
- Target size: ~500 entries at launch
- Registry is fetched from recipes repository, cached locally

**Related designs:**
- [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md): Batch recipe generation from priority queue. Shares data sourcing and validation concerns.
- [DESIGN-batch-recipe-generation.md](DESIGN-batch-recipe-generation.md): CI pipeline that generates recipes. As recipes get generated, tools graduate out of the discovery registry.

## Context and Problem Statement

The discovery resolver's registry lookup stage maps tool names to install instructions so `tsuku install <tool>` can resolve tools without ecosystem probing or LLM calls. The registry has a single `jq` entry added during skeleton work. It needs ~500 entries before the resolver delivers value.

The current schema conflates two different things: what a tool *is* (its identity, where it's maintained, what it does) and how to *install* it (which builder to use, what arguments to pass). A tool like `jq` is maintained on GitHub at `jqlang/jq`, documented at `https://jqlang.github.io/jq/`, and installable via Homebrew bottles. These are separate facts. The current schema — `{"builder": "homebrew", "source": "jq"}` — captures only the install path and loses everything else.

This matters for three reasons:

1. **Today's resolver needs install instructions.** The `RegistryLookup` stage must return a `DiscoveryResult` with builder and source. That's the minimum viable entry.

2. **Future builders could infer install paths from metadata.** An LLM-based builder (not yet implemented) could take a tool's homepage, repository URL, and description and figure out how to install it. If the registry captured this metadata, entries wouldn't need explicit builder+source — they could work with builders that don't exist yet.

3. **Richer data improves disambiguation and UX.** When multiple ecosystems claim a name, having the tool's description, homepage, and repo URL helps both automated disambiguation and user-facing display (`"Found bat: a cat clone with syntax highlighting (github.com/sharkdp/bat). Also available: npm bat (testing framework)."`).

The bootstrap challenge is populating ~500 entries with enough data to serve today's resolver while collecting metadata that enables future evolution.

### Scope

**In scope:**
- Discovery registry schema evolution (metadata fields, optional install info)
- Data sources for populating ~500 entries
- Go tool for generating and validating entries
- Disambiguation entries for known name collisions
- CI validation of the registry file
- Updates to parent design (DESIGN-discovery-resolver.md) for schema changes
- Contributor workflow for adding entries

**Out of scope:**
- Resolver chain architecture (covered by parent design)
- Ecosystem probe and LLM discovery implementation
- LLM-based builder that infers install paths from metadata (future work)
- Recipe format changes

## Decision Drivers

- **Serve today's resolver**: Entries with builder+source must work with the current `RegistryLookup` code
- **Enable future evolution**: Schema should accommodate richer metadata without breaking changes
- **Separate identity from install path**: Where a tool is maintained is distinct from how to install it
- **Entry accuracy**: Every install instruction must resolve to a working builder+source pair
- **Minimal required fields**: Only `name` and install info should be required; everything else optional
- **Repeatable process**: Population method must be re-runnable as tools change
- **Contributor-friendly**: External contributors should be able to add entries via PR
- **Reuse infrastructure**: Leverage existing seed-queue patterns and priority queue data

## Considered Options

### Decision 1: Registry Schema

The current schema is a flat map of tool name to install instructions: `{"builder": "github", "source": "BurntSushi/ripgrep"}`. The question is how to evolve it to support richer metadata while keeping backward compatibility with the existing `RegistryLookup` code.

The resolver code (`internal/discover/registry.go`) unmarshals `RegistryEntry` with `builder`, `source`, and `binary` fields. Adding optional fields to this struct is backward-compatible — old entries still work, new entries carry more data. Making `builder`+`source` optional is a bigger change but enables future builders that infer install paths from metadata.

#### Chosen: Required Install Info + Optional Metadata Fields

Keep `builder` and `source` as required fields for now. Add optional metadata fields to the same entry struct. This delivers value today (resolver works) while collecting data for the future (metadata is available when needed).

The schema becomes:

```json
{
  "schema_version": 2,
  "tools": {
    "ripgrep": {
      "builder": "github",
      "source": "BurntSushi/ripgrep",
      "homepage": "https://github.com/BurntSushi/ripgrep",
      "description": "Fast line-oriented search tool"
    },
    "jq": {
      "builder": "homebrew",
      "source": "jq",
      "repo": "https://github.com/jqlang/jq",
      "homepage": "https://jqlang.github.io/jq/",
      "description": "Lightweight command-line JSON processor"
    }
  }
}
```

Schema version bumps from 1 to 2. The loading code accepts both versions: v1 entries have only `builder`/`source`/`binary`, v2 entries may include metadata fields. The `RegistryLookup` resolver ignores metadata — it just reads `builder` and `source`. Other consumers (future LLM builder, `tsuku info`, `tsuku search`, disambiguation UX) can use the metadata when available.

#### Alternatives Considered

**Separate install info into a nested object**: Structure entries as `{"install": {"builder": "github", "source": "..."}, "repo": "...", "homepage": "..."}`. Cleaner separation but breaks the existing `RegistryEntry` struct and all code that reads `entry.Builder`. The refactor cost isn't justified when the flat approach works and is forward-compatible. Rejected because the migration cost is high for a cosmetic difference.

**Make builder+source optional now**: Allow entries with only metadata and no install info, for future LLM builder consumption. Premature — the LLM builder doesn't exist, and entries without install info provide zero value to today's resolver. The resolver would need to handle `nil` install fields, adding complexity for no immediate gain. Rejected as future work: when the LLM builder lands, a schema v3 can make install fields optional.

### Decision 2: Data Sourcing Strategy

The registry needs ~500 entries covering the most popular developer tools. The challenge is finding tool-to-source mappings at scale with enough metadata (homepage, description, repo URL) to populate the new schema fields.

#### Chosen: Go CLI Tool with Curated Seed Lists

Build a `cmd/seed-discovery` Go tool that reads from curated seed list files, validates each entry, enriches it with metadata from GitHub/ecosystem APIs, and outputs `discovery.json`. Seed lists provide the human-curated mapping (tool name → source), and the tool handles validation and metadata enrichment automatically.

Seed lists require only the minimum: tool name, builder, and source. The tool queries APIs to fill in optional metadata (description, homepage, star count, etc.) during generation. This keeps the curation effort focused on accuracy (is this the right source?) rather than data collection (what's the homepage?).

Sources for populating seed lists:
- Homebrew analytics top-500 (filtered to tools with GitHub release binaries)
- Well-known tool categories (HashiCorp, Kubernetes, cloud CLIs)
- Tools referenced in tsuku issues and existing recipes
- Priority queue entries that map to GitHub-release tools

#### Alternatives Considered

**LLM-assisted bulk generation**: Use an LLM to generate tool-to-repo mappings. Fast but unreliable — hallucinated repo names pass no validation, and the cost of fixing errors exceeds manual curation. Rejected because accuracy is non-negotiable.

**Scrape awesome-lists and aggregate**: Parse curated GitHub lists (awesome-cli-apps, etc.) for tool names and repos. Good coverage but noisy — many entries are libraries, and URLs go stale. Rejected as primary source but useful as input when building seed lists.

### Decision 3: Validation and Enrichment

Every entry must point to a real, active source. Beyond validation, the tool should enrich entries with metadata from APIs — description, homepage, stars, etc. The question is how aggressive to be with enrichment and how to handle stale data.

#### Chosen: Builder-Aware Validation + API Enrichment + CI Freshness

The `seed-discovery` tool does three things per entry:

1. **Validates** via builder-specific checks (GitHub: repo exists, not archived, has binary releases in last 24 months; Homebrew: formula exists; ecosystem builders: package exists).
2. **Enriches** with metadata from the same API response (description from GitHub API, star count, homepage from repo metadata). Enrichment is best-effort — missing metadata doesn't fail the entry.
3. **Detects collisions** via lightweight HTTP checks against ecosystem registries (npm, crates.io, PyPI, RubyGems).

CI runs a weekly freshness check (`seed-discovery --validate-only`) that verifies entries still resolve and flags ownership changes.

#### Alternatives Considered

**Validation only, no enrichment**: Validate entries but don't collect metadata — let contributors add it manually. Rejected because GitHub API responses already include description and homepage for free; throwing away data we've already fetched is wasteful.

**CI validation on every PR**: Validate all 500 entries when `discovery.json` changes. Too slow (rate limits, 500+ API calls per PR). Weekly is sufficient. Rejected for PR-time; weekly catches staleness without blocking contributors.

### Decision 4: Disambiguation Approach

Some tool names exist in multiple ecosystems. The registry needs override entries so the resolver returns the correct tool. The question is how to identify collisions and choose the right resolution.

#### Chosen: Automated Detection + Manual Resolution

The `seed-discovery` tool queries ecosystem registries for each entry name. When a collision is found, it logs the conflict with metadata from both sides. A human reviews and marks the entry as a disambiguation override if needed.

Disambiguation entries are preserved in `discovery.json` even when a recipe exists for the tool, because they prevent the ecosystem probe from resolving to the wrong package.

#### Alternatives Considered

**Fully automated by popularity**: Auto-resolve by download count. Wrong in practice — npm's `bat` has more downloads than sharkdp/bat has GitHub stars, but CLI users want sharkdp/bat. Rejected because disambiguation correctness is a core decision driver.

### Uncertainties

- **GitHub API rate limits during bootstrap**: 500 entries with 2-3 calls each = 1000-1500 requests. Within the 5000/hour authenticated limit, but enrichment adds calls.
- **Actual collision rate**: Could be 10 or 50 tools with name collisions across ecosystems.
- **Metadata staleness**: Descriptions and homepages change. Weekly freshness checks catch broken repos but not updated descriptions. Acceptable for now — stale descriptions don't break installs.

## Decision Outcome

**Chosen: Schema v2 with optional metadata + Go CLI tool + CI freshness**

### Summary

The discovery registry schema evolves from a flat install mapping to a tool metadata index. Each entry still requires `builder` and `source` (so today's resolver works unchanged), but gains optional fields: `repo`, `homepage`, `description`, `binary`, and a `disambiguation` flag. Schema version bumps to 2 with backward compatibility for v1 entries.

A new `cmd/seed-discovery` Go tool reads curated seed lists (tool name, builder, source), validates each entry via builder-specific API checks, enriches it with metadata from the same API responses (description, homepage, stars), detects name collisions against ecosystem registries, and outputs `discovery.json`. The tool also checks for typosquatting (Levenshtein distance between entry names) and cross-references existing recipes (excluding entries where a recipe already exists, unless marked as disambiguation overrides).

Seed lists require only the install mapping plus a verification URL. The tool handles metadata enrichment automatically. CI runs weekly freshness checks. Disambiguation entries are resolved by human review and preserved regardless of recipe existence.

### Rationale

Keeping `builder`+`source` required avoids premature abstraction — the LLM builder that could infer install paths from metadata doesn't exist yet. Adding optional metadata fields is a low-cost, backward-compatible change that collects useful data for disambiguation UX, `tsuku info`/`search`, and future builder evolution. The schema can move to v3 (optional install fields) when a builder exists that can use metadata-only entries.

The Go tool with seed lists balances accuracy (human-curated install mappings) with automation (API validation, metadata enrichment, collision detection). The manual effort is focused on the high-value decision (which source is correct for this tool name?) while mechanical work (description, homepage, validation) is automated.

### Trade-offs Accepted

- **builder+source still required**: Entries without install info provide no value to today's resolver. This means we can't add metadata-only entries yet — acceptable since no consumer exists for them.
- **Manual seed list curation**: ~500 tool-to-source mappings must be compiled by hand. The accuracy requirement justifies this; the work is one-time with incremental additions afterward.
- **Schema version bump**: Existing v1 entries still load, but the loading code needs to handle both versions. Minor code change.

## Solution Architecture

### Overview

Three components: an evolved registry schema, a Go CLI tool for population, and a CI workflow for freshness.

### Registry Schema (v2)

```json
{
  "schema_version": 2,
  "tools": {
    "ripgrep": {
      "builder": "github",
      "source": "BurntSushi/ripgrep",
      "description": "Fast line-oriented search tool",
      "homepage": "https://github.com/BurntSushi/ripgrep",
      "repo": "https://github.com/BurntSushi/ripgrep"
    },
    "jq": {
      "builder": "homebrew",
      "source": "jq",
      "description": "Lightweight command-line JSON processor",
      "homepage": "https://jqlang.github.io/jq/",
      "repo": "https://github.com/jqlang/jq"
    },
    "bat": {
      "builder": "github",
      "source": "sharkdp/bat",
      "description": "A cat clone with syntax highlighting",
      "homepage": "https://github.com/sharkdp/bat",
      "repo": "https://github.com/sharkdp/bat",
      "disambiguation": true
    },
    "kubectl": {
      "builder": "github",
      "source": "kubernetes/kubernetes",
      "binary": "kubectl",
      "description": "Kubernetes command-line tool",
      "homepage": "https://kubernetes.io/docs/reference/kubectl/",
      "repo": "https://github.com/kubernetes/kubernetes"
    }
  }
}
```

**Required fields:**
- `builder`: Builder name (`github`, `homebrew`, `cargo`, `npm`, `pypi`, `gem`, `go`, `cpan`, `cask`)
- `source`: Builder-specific source argument (`owner/repo` for github, formula name for homebrew, crate name for cargo, etc.)

**Optional fields:**
- `binary`: Binary name when it differs from the tool name
- `description`: Short description of the tool (enriched from API)
- `homepage`: URL to the tool's documentation or marketing page
- `repo`: URL to the source code repository (may differ from install source — e.g., jq's repo is GitHub but builder is homebrew)
- `disambiguation`: When `true`, entry is a collision override preserved even when a recipe exists

**Builder selection guidance:**

The `builder` field describes how to install the tool, not where its source code lives. A tool maintained on GitHub might install best via Homebrew, cargo, or a direct GitHub release download. The choice depends on the installation path:

1. **`github`**: Tool publishes pre-built platform binaries in GitHub releases. Fastest, most reliable — no build step needed.
2. **`homebrew`**: Tool has Homebrew bottles or requires complex build dependencies that Homebrew handles.
3. **Ecosystem builders** (`cargo`, `npm`, `pypi`, `gem`, `go`, `cpan`): Tool is primarily distributed through an ecosystem registry. Mainly used for disambiguation overrides.
4. **`cask`**: macOS application distributed as a .dmg or .pkg.

### Go Code Changes

Update `internal/discover/registry.go`:

```go
type RegistryEntry struct {
    Builder        string `json:"builder"`
    Source         string `json:"source"`
    Binary         string `json:"binary,omitempty"`
    Description    string `json:"description,omitempty"`
    Homepage       string `json:"homepage,omitempty"`
    Repo           string `json:"repo,omitempty"`
    Disambiguation bool   `json:"disambiguation,omitempty"`
}
```

Update `ParseRegistry` to accept schema versions 1 and 2. The `RegistryLookup` resolver is unchanged — it reads `Builder` and `Source` as before. New fields are available for other consumers.

Update the parent design (DESIGN-discovery-resolver.md) to reflect the v2 schema.

### Components

```
data/discovery-seeds/          # Curated seed lists (input)
├── cloud-cli.json             # AWS, GCP, Azure CLI tools
├── dev-tools.json             # General development tools
├── kubernetes.json            # k8s ecosystem tools
├── security.json              # Security and audit tools
├── hashicorp.json             # HashiCorp tools
└── disambiguations.json       # Known name collisions

cmd/seed-discovery/            # Go CLI tool (processor)
└── main.go

recipes/discovery.json         # Output: validated registry (v2 schema)

.github/workflows/
└── discovery-freshness.yml    # Weekly CI validation
```

### Seed List Format

Seed lists are the human-curated input. They contain the minimum needed for validation — the tool handles enrichment:

```json
{
  "category": "dev-tools",
  "description": "General development CLI tools",
  "entries": [
    {"name": "ripgrep", "builder": "github", "source": "BurntSushi/ripgrep"},
    {"name": "fd", "builder": "github", "source": "sharkdp/fd", "disambiguation": true},
    {"name": "bat", "builder": "github", "source": "sharkdp/bat", "disambiguation": true},
    {"name": "jq", "builder": "homebrew", "source": "jq", "repo": "https://github.com/jqlang/jq"},
    {"name": "kubectl", "builder": "github", "source": "kubernetes/kubernetes", "binary": "kubectl"}
  ]
}
```

Required per entry: `name`, `builder`, `source`. Optional: `binary`, `disambiguation`, `repo` (when repo differs from install source — the tool can't infer it). The tool enriches `description`, `homepage`, and `repo` (for github builder entries) from API responses.

### Processing Pipeline

```
Read seed lists (data/discovery-seeds/*.json)
    |
    v
Merge all entries (deduplicate by name; last-seen wins on conflicts,
    logged as warning for review)
    |
    v
For each entry:
  1. Builder-specific validation:
     - github: repo exists? not archived? API full_name matches source
       (detect renamed/redirected repos)? has a non-draft release in
       last 24 months with at least one asset matching platform patterns
       (linux|darwin|windows combined with amd64|arm64|x86_64)?
     - homebrew: formula exists via Homebrew API?
     - cargo/npm/pypi/gem/go/cpan: package exists via ecosystem API?
     All API calls use exponential backoff with 3 retries. Responses
     cached in memory for the duration of the run to avoid duplicate
     requests (e.g., collision detection reusing validation responses).
  2. Metadata enrichment (from the same API response):
     - github: description, homepage, stars, repo URL
     - homebrew: description, homepage
     - ecosystem: description (where available)
  3. Cross-reference: recipe already exists in recipes/? (case-insensitive,
     normalized name match against recipe filenames)
  4. Collision detection: name exists on npm, crates.io, PyPI, or RubyGems?
  5. Typosquatting check: Levenshtein distance <=2 from another entry?
  6. Unicode normalization: reject names with mixed-script characters
    |
    v
Output:
  - Valid entries -> recipes/discovery.json (v2 schema)
  - Skipped (has recipe, not disambiguation) -> logged, excluded
  - Skipped (has recipe, is disambiguation) -> logged, INCLUDED
  - Failed validation -> logged with reason, excluded
  - Collisions detected -> logged for human review
  - Typosquatting warnings -> logged for human review
```

### CLI Interface

```
Usage: seed-discovery [flags]

Flags:
  -seeds-dir      Seed list directory (default: data/discovery-seeds)
  -output         Output path (default: recipes/discovery.json)
  -recipes-dir    Recipe directory for cross-reference (default: recipes)
  -validate-only  Validate existing discovery.json without regenerating
  -verbose        Show detailed validation and enrichment output
```

### CI Freshness Workflow

```yaml
name: Discovery Registry Freshness
on:
  schedule:
    - cron: '0 6 * * 1'  # Weekly Monday 6 AM UTC
  workflow_dispatch: {}

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go build -o seed-discovery ./cmd/seed-discovery
      - run: ./seed-discovery --validate-only --verbose
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

On failure, creates an issue listing stale entries with reasons.

### Data Flow

```
Seed lists (data/discovery-seeds/*.json)
    |  human-curated: name, builder, source
    v
cmd/seed-discovery
    |  validates + enriches via APIs
    v
recipes/discovery.json (~500 entries, v2 schema with metadata)
    |  committed to repo
    v
tsuku update-registry (fetches to $TSUKU_HOME/registry/discovery.json)
    |  cached locally
    v
RegistryLookup (reads builder+source, O(1) lookup)
    |  future: other consumers read metadata fields
    v
DiscoveryResult -> create pipeline -> install
```

### Relationship to Batch Pipeline

| Aspect | Discovery Registry | Priority Queue |
|--------|-------------------|----------------|
| Purpose | Name resolution for `tsuku install` | Batch recipe generation |
| Builder | Any (github, homebrew, cargo, etc.) | Primarily `homebrew` |
| Lifecycle | Entry removed when recipe exists (unless disambiguation) | Entry status → `success` when recipe generated |
| Tooling | `cmd/seed-discovery` | `cmd/seed-queue` |
| Output | `recipes/discovery.json` | `data/priority-queue.json` |

Shared patterns: GitHub API client, JSON I/O, recipe cross-referencing. Not shared code — different tools with different concerns.

### Future Evolution

**Schema v3 (when LLM builder exists):** Make `builder`+`source` optional. Entries with only metadata (`repo`, `homepage`, `description`) can be resolved by an LLM builder that infers the install path. The registry becomes a general tool knowledge base rather than just an install index.

**Automated population:** Once ecosystem probe metadata is collected at scale, new entries could be generated automatically from probe results + disambiguation review.

**Convergence with priority queue:** Both registries map tool names to sources. They could merge into a unified tool index that serves both discovery (name resolution) and batch generation (recipe queue).

## Implementation Approach

### Phase 1: Schema Evolution and Go Tool

Update the registry schema to v2, modify Go code to accept new fields, and build `cmd/seed-discovery`. Start with 50-100 entries to validate end-to-end.

**Files:**
- `internal/discover/registry.go` (add optional fields, accept schema v1 and v2)
- `internal/discover/registry_test.go` (update tests)
- `cmd/seed-discovery/main.go`
- `data/discovery-seeds/dev-tools.json` (initial seed list)
- `recipes/discovery.json` (updated output)
- `docs/designs/DESIGN-discovery-resolver.md` (update schema section)

### Phase 2: Scale to ~500 Entries

Expand seed lists across categories. Run the tool, review collision reports, add disambiguation entries.

**Files:**
- `data/discovery-seeds/*.json` (all categories)
- `data/discovery-seeds/disambiguations.json`
- `recipes/discovery.json` (~500 entries)

### Phase 3: CI Freshness

Add weekly validation workflow.

**Files:**
- `.github/workflows/discovery-freshness.yml`

## Security Considerations

### Download Verification

The `seed-discovery` tool doesn't download binaries. It queries APIs over HTTPS (GitHub, Homebrew, ecosystem registries) to verify metadata and enrich entries. No artifacts are downloaded or executed.

The entries it produces are consumed by the discovery resolver, which delegates to builders with their own verification (checksums, HTTPS). The accuracy of the registry entry — pointing to the correct source for the correct builder — is the security concern.

### Execution Isolation

The tool runs locally or in CI. Read-only API calls, writes a JSON file. No code execution, no sandbox, no elevated permissions.

### Supply Chain Risks

**Registry poisoning via seed lists.** A malicious PR could map a tool name to a compromised source (e.g., `kubernetes-tools/kubernetes` instead of `kubernetes/kubernetes`). Mitigations:
- PR review: changes to `data/discovery-seeds/` require CODEOWNERS review
- Reviewer checklist: verify source matches official tool, check for typosquatting
- Automated validation: tool verifies source exists and has legitimate release history
- Redirect detection: compare GitHub API's `full_name` response against the seed list's `source` field to catch renamed repos claimed by attackers
- Typosquatting detection: Levenshtein distance check flags near-miss entry names
- Unicode normalization: reject entry names with mixed-script characters
- PR size limit: PRs adding more than 100 entries require additional review
- Weekly freshness: detects ownership transfers

**Stale entries pointing to transferred repos.** Mitigations:
- Weekly CI freshness check compares current repo owner against entry
- The parent design's ownership verification applies

**Disambiguation errors.** Wrong override sends users to wrong tool. Mitigations:
- Disambiguation entries are flagged for extra review
- Collision detection surfaces both sides with metadata
- Each disambiguation must be justified in the seed list

**Metadata poisoning.** Enriched fields (description, homepage) come from API responses controlled by repo owners. A malicious repo could set misleading metadata. Mitigations:
- Metadata is informational only — it doesn't affect install behavior
- The `builder`+`source` fields (which control what gets installed) come from the human-curated seed list, not from API enrichment

### User Data Exposure

The bootstrap tool sends tool names and source identifiers to GitHub and ecosystem APIs. Equivalent to browsing those sites. No user data collected or transmitted. Published `discovery.json` contains tool names, public repo identifiers, and publicly available metadata.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Registry poisoning via PR | CODEOWNERS review, automated validation, typosquatting detection | Sophisticated attack with legitimate-looking source |
| Stale entry to transferred repo | Weekly freshness with ownership check | 7-day window between transfer and detection |
| Wrong disambiguation | Collision detection + human review + documented rationale | Obscure collisions not in seed lists |
| Metadata poisoning | Metadata is informational only; install fields from curated seed list | Misleading description/homepage (no install impact) |
| Typosquatting | Levenshtein distance check, review checklist | Novel Unicode attacks (handled by resolver's normalize.go) |

## Consequences

### Positive

- Discovery resolver has ~500 entries for instant tool resolution
- Schema v2 captures tool identity (repo, homepage, description) separately from install path
- Metadata enables richer disambiguation UX and `tsuku info`/`search` output
- Schema is forward-compatible with future LLM builder (v3 can make install fields optional)
- Automated enrichment reduces manual effort — curators only specify name+builder+source
- CI freshness keeps entries current

### Negative

- Schema version bump requires code changes to the registry loader
- Initial seed list curation requires manual effort (~500 entries)
- Metadata enrichment adds API calls during generation (acceptable for one-time bootstrap)
- Weekly freshness has up to 7-day staleness window

### Mitigations

- Schema v1 entries still load without changes (backward compatible)
- Seed lists organized by category for parallel curation
- Enrichment API calls are cached and rate-limited
- Critical tools could have more frequent freshness checks if needed

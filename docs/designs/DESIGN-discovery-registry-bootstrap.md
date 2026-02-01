---
status: Proposed
problem: |
  The discovery resolver's registry lookup stage has 1 entry but needs ~500 to make
  `tsuku install <tool>` useful for popular developer tools. Without populated data,
  every tool resolution falls through to slower ecosystem probes or LLM discovery.
  The entries cover two categories: GitHub-release tools not in ecosystem registries
  (kubectl, terraform, stripe-cli) and disambiguation overrides for name collisions
  across ecosystems (bat, fd, serve).
decision: |
  A Go CLI tool (`cmd/seed-discovery`) reads from curated seed lists organized by
  category, validates each entry via the GitHub API (repo exists, not archived, has
  binary release assets within 24 months), detects ecosystem name collisions via
  direct HTTP checks against npm/crates.io/PyPI/RubyGems, and outputs a validated
  `discovery.json`. CI runs weekly freshness checks. Disambiguation entries are
  preserved even when recipes exist. Seed lists require verification URLs and
  CODEOWNERS review for supply chain safety.
rationale: |
  Curated seed lists (rather than LLM generation or web scraping) ensure accuracy
  for the initial ~500 entries. A Go tool gives type safety, testability, and
  reusable patterns from the existing seed-queue infrastructure. Keeping discovery
  tooling separate from the batch pipeline respects their different purposes while
  sharing conventions. CI freshness checks address the staleness risk without
  blocking PR workflows.
---

# DESIGN: Discovery Registry Bootstrap

## Status

Proposed

## Upstream Design Reference

This design implements Phase 2 of [DESIGN-discovery-resolver.md](DESIGN-discovery-resolver.md).

**Relevant sections:**
- Discovery Registry Format: `{builder, source, binary?}` schema
- Two entry categories: GitHub-release tools and disambiguation overrides
- Target size: ~500 entries at launch
- Registry is fetched from recipes repository, cached locally

**Related designs:**
- [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md): Batch recipe generation from priority queue. Shares data sourcing and validation concerns — both need to verify GitHub repos exist and aren't archived. The priority queue (`data/priority-queue.json`) contains 204 Homebrew entries that overlap with potential discovery entries.
- [DESIGN-batch-recipe-generation.md](DESIGN-batch-recipe-generation.md): CI pipeline that generates recipes. As recipes get generated and merged, discovery registry entries for those tools become redundant (the recipe exists, so the resolver never reaches the registry stage). The bootstrap should account for this shrinkage.

## Context and Problem Statement

The discovery resolver's registry lookup stage maps tool names to `{builder, source}` pairs so `tsuku install <tool>` can find tools without ecosystem probing or LLM calls. The registry currently has a single `jq` entry added during skeleton work. It needs ~500 real entries before the resolver delivers value to users.

The entries fall into two categories:

1. **GitHub-release tools** not discoverable through ecosystem registries. Tools like kubectl, terraform, and stripe-cli distribute binaries via GitHub releases but don't appear in crates.io, npm, or PyPI. Without registry entries, these tools fall through to LLM discovery — slow, requires API keys, and unreliable.

2. **Disambiguation overrides** for tools whose names collide across ecosystems. "bat" exists on npm (a testing framework) and as a GitHub release (sharkdp/bat, a cat replacement). Without an override, the ecosystem probe might resolve to the wrong tool.

Three challenges make this non-trivial:

1. **Data sourcing at scale.** Manually curating 500 entries is tedious and error-prone. Automated approaches risk importing stale or incorrect mappings. The right answer is likely a hybrid: seed from existing data sources, validate automatically, review manually.

2. **Validation reliability.** Every entry must point to a real, active GitHub repository (or a valid ecosystem source). Repos get archived, transferred, or deleted. A one-time validation isn't enough — entries need periodic freshness checks.

3. **Overlap with batch pipeline.** The priority queue and discovery registry both map tool names to builders. As the batch pipeline generates recipes, those tools no longer need discovery entries. The bootstrap process should be aware of this lifecycle.

### Scope

**In scope:**
- Data sources for populating ~500 entries
- Go tool for generating and validating entries
- Disambiguation entries for known name collisions
- CI validation of the registry file
- Process for adding new entries (contributor workflow)
- Relationship between discovery registry and priority queue / batch pipeline

**Out of scope:**
- Resolver architecture (covered by DESIGN-discovery-resolver.md)
- Ecosystem probe implementation
- LLM discovery implementation
- Registry format changes (schema is defined in parent design)

## Decision Drivers

- **Entry accuracy**: Every entry must resolve to a working builder+source pair
- **Repeatable process**: The population method must be re-runnable as tools change
- **Reuse existing infrastructure**: The seed-queue tool and priority queue data already exist
- **Minimal manual effort**: 500 entries can't be hand-curated efficiently
- **Contributor-friendly**: External contributors should be able to add entries via PR
- **Shrinkage awareness**: As recipes get generated, discovery entries become redundant
- **Builder coverage**: Most entries will use `github` builder since ecosystem tools are discoverable via probe

## Considered Options

### Decision 1: Data Sourcing Strategy

The registry needs ~500 entries covering the most popular developer tools not already discoverable through ecosystem registries. The challenge is finding a reliable source of tool-name-to-GitHub-repo mappings at scale. Manual curation is accurate but slow. Automated scraping is fast but may import incorrect data. The priority queue already has 204 Homebrew entries, but those map to the `homebrew` builder — discovery entries for these tools would use the `github` builder only if the tool distributes pre-built binaries via GitHub releases.

The key insight is that the discovery registry's primary purpose is covering GitHub-release tools that ecosystem probes can't find. Ecosystem tools (cargo, npm, pypi, etc.) are handled by the probe stage. So the data source should focus on popular tools that distribute via GitHub releases.

#### Chosen: Go CLI Tool with Multiple Data Sources

Build a `cmd/seed-discovery` Go tool (parallel to the existing `cmd/seed-queue`) that:

1. **Reads from curated seed lists** — static JSON/TOML files containing known tool-name-to-repo mappings, organized by category (cloud CLI tools, development tools, security tools, etc.)
2. **Validates via GitHub API** — for each entry, verifies the repo exists, isn't archived, and has recent releases with binary assets
3. **Cross-references existing recipes** — skips tools that already have recipes in `recipes/`
4. **Cross-references priority queue** — notes overlap with `data/priority-queue.json` entries
5. **Outputs validated discovery.json** — merges validated entries into the existing registry

The seed lists are the manual curation step, but they're one-time work that produces a reusable artifact. Sources for the seed lists:
- Homebrew analytics top-500 formulas (filtered to those with GitHub release binaries)
- Curated lists of popular CLI tools (terraform, kubectl, gh, etc.)
- Tools already mentioned in tsuku issues and documentation

#### Alternatives Considered

**LLM-assisted bulk generation**: Use an LLM to generate tool-to-repo mappings from prompts like "list the 50 most popular Kubernetes CLI tools." Fast for initial generation but unreliable — LLMs hallucinate repo names, and validation becomes the bottleneck. Rejected because the accuracy requirement is high (every entry must resolve to a real repo) and the cost of fixing LLM errors exceeds the cost of manual curation.

**Extend cmd/seed-queue to output discovery entries**: Modify the existing seed-queue tool to emit discovery.json entries alongside priority queue entries. Architecturally clean for reuse, but the tools serve different purposes: seed-queue populates a generation queue (homebrew builder), while discovery maps names to GitHub release sources. Forcing them together conflates the concerns. Rejected as primary approach, but the `seed-discovery` tool should reuse `internal/seed` package utilities where applicable (HTTP client, retry logic, JSON I/O patterns).

**Scrape awesome-cli-apps and similar lists**: Parse GitHub awesome-lists to extract tool names and repos. Good coverage but noisy — many entries are libraries, not CLI tools, and repo URLs may be stale. Rejected as primary source but useful as input for seed lists.

### Decision 2: Validation Approach

Every registry entry must point to a real, active source. The question is whether validation happens once during bootstrap, continuously in CI, or both.

Stale entries are a real risk: GitHub repos get archived, transferred to new owners, or deleted. A tool that worked when bootstrapped may break months later. The parent design notes that "Registry staleness" is a security concern — a transferred repo could become malicious.

#### Chosen: Build-Time Validation in Go Tool + CI Freshness Check

Two validation layers:

1. **Build-time validation** (in `cmd/seed-discovery`): When generating entries, validate each one via GitHub API — repo exists, not archived, has releases with binary assets, and owner matches expected. Failures are logged and the entry is excluded.

2. **CI freshness check** (GitHub Actions workflow): A weekly scheduled workflow runs `cmd/seed-discovery --validate-only` against the existing `discovery.json`, checking that all entries still resolve. Failures open an issue or PR removing stale entries.

This separates the initial population (fast, runs locally) from ongoing maintenance (automated, runs in CI).

#### Alternatives Considered

**One-time script with no CI**: Validate during bootstrap only. Simple but entries go stale silently. Rejected because the parent design explicitly calls for freshness checks.

**CI-only validation on every PR**: Validate all 500 entries on every PR that touches discovery.json. Thorough but slow (GitHub API rate limits at 5000 requests/hour for authenticated users; 500 entries with multiple API calls each could hit limits). Rejected for PR-time validation; weekly is sufficient.

### Decision 3: Disambiguation Data

Some tool names exist in multiple ecosystems. The discovery registry needs override entries so the resolver returns the "correct" tool without ecosystem probing. The question is how to identify which names collide and which resolution is correct.

#### Chosen: Automated Collision Detection + Manual Resolution

1. **Detection**: The `seed-discovery` tool makes lightweight HTTP GET requests to ecosystem registries (crates.io, npm, PyPI, RubyGems — the four most common collision sources) for each entry in the seed list. This avoids coupling to the `internal/builders` package. When a tool name exists in both the discovery registry and an ecosystem registry, flag it as a potential collision.

2. **Resolution**: A human reviews flagged collisions and adds an explicit disambiguation entry. The entry goes in the seed list with a `disambiguation: true` field so it's preserved even if the tool gets a recipe later.

3. **Known collisions**: Start with a manually curated list of known collisions (bat, fd, serve, delta, etc.) based on tsuku issue history and common developer tools.

This catches collisions that a manual review would miss, while keeping humans in the loop for the resolution decision.

#### Alternatives Considered

**Fully manual curation**: Maintain a hand-written list of disambiguations. Catches obvious cases but misses obscure collisions. Rejected because at 500 entries, manual cross-referencing against 7 ecosystem registries is impractical.

**Fully automated resolution by popularity**: Auto-resolve collisions by picking the most-downloaded option. Fast but wrong in edge cases — npm's `bat` has more downloads than sharkdp/bat has GitHub stars, but sharkdp/bat is what CLI users want. Rejected because disambiguation correctness is a key decision driver.

### Uncertainties

- **GitHub API rate limits during bootstrap**: 500 entries with 2-3 API calls each = 1000-1500 requests. Within the 5000/hour authenticated limit, but may need pagination for repos with many releases.
- **Actual collision rate**: We don't know how many of the ~500 tools have name collisions across ecosystems. Could be 10 or 50.
- **Homebrew overlap**: Many popular tools exist in both Homebrew and as GitHub releases. The right discovery entry depends on whether the homebrew or github builder produces better results for that tool.

## Decision Outcome

**Chosen: Go CLI tool + CI freshness + automated collision detection**

### Summary

A new `cmd/seed-discovery` Go tool reads from curated seed lists (static files containing tool-name-to-repo mappings), validates each entry via the GitHub API (repo exists, not archived, has binary release assets), cross-references against existing recipes and the priority queue, detects ecosystem name collisions by probing builder registries, and outputs a validated `discovery.json`. The seed lists are organized by category (cloud tools, dev tools, security tools) and maintained as committed files in the repository.

CI runs a weekly freshness check against the published `discovery.json`, verifying all entries still resolve. Stale entries get flagged via automated issue or PR. Disambiguation entries are identified via automated collision detection and resolved by a human reviewer. Known collisions (bat, fd, serve, delta) are pre-seeded in the seed lists with explicit `disambiguation: true` markers.

The tool reuses patterns from the existing `cmd/seed-queue` and `internal/seed` packages: HTTP client with retry, JSON I/O, merge-with-deduplication. It doesn't share code directly with the batch pipeline but follows the same conventions.

### Rationale

A Go tool (rather than a shell script) gives type safety, testability, and access to the existing `internal/builders` package for collision detection. Curated seed lists (rather than LLM generation or web scraping) ensure accuracy for the initial ~500 entries — the manual effort is front-loaded but produces a reusable, auditable artifact. CI freshness checks address the staleness risk identified in the parent design without blocking PR workflows.

Keeping the discovery tool separate from the seed-queue tool (rather than extending it) respects the different purposes: seed-queue populates a generation queue for batch recipe creation, while seed-discovery populates a name-resolution index for the install command. They share patterns but not concerns.

### Trade-offs Accepted

- **Manual seed list curation**: Building the initial seed lists requires human effort to compile ~500 tool-to-repo mappings. This is acceptable because the accuracy requirement is high and the work is one-time (future additions are incremental).
- **GitHub API dependency**: Validation requires GitHub API access, which has rate limits. Acceptable because authenticated requests get 5000/hour and the bootstrap runs once.
- **No automated population growth**: The registry doesn't automatically discover new tools — entries are added manually or via PR. Acceptable because the registry is intentionally curated, and the ecosystem probe handles the long tail.

## Solution Architecture

### Overview

The bootstrap system has three components: seed lists (data), a Go CLI tool (processor), and a CI workflow (maintenance).

### Components

```
data/discovery-seeds/          # Curated seed lists
├── cloud-cli.json             # AWS, GCP, Azure CLI tools
├── dev-tools.json             # General development tools
├── kubernetes.json            # k8s ecosystem tools
├── security.json              # Security and audit tools
├── hashicorp.json             # HashiCorp tools
└── disambiguations.json       # Known name collisions with explicit resolution

cmd/seed-discovery/            # Go CLI tool
├── main.go                    # Entry point, flag parsing
└── (uses internal/seed/ patterns)

recipes/discovery.json         # Output: validated registry

.github/workflows/
└── discovery-freshness.yml    # Weekly CI validation
```

### Builder Selection Rules

Discovery entries exist for two reasons: (1) the tool isn't discoverable through ecosystem probes, or (2) the tool needs a disambiguation override because its name collides across ecosystems. The builder for each entry should be chosen based on the best installation path for that tool, not just "whatever hosts the source code."

Many tools are maintained on GitHub but distributed through Homebrew, cargo, npm, or other ecosystems. The GitHub repo is the source of truth for development, but the best installation path may be an ecosystem package. For example, `jq` is maintained on GitHub (jqlang/jq) but its Homebrew bottle is the most reliable install method — so its discovery entry uses the `homebrew` builder.

**Selection order (prefer earlier options):**

1. **Use `github`** when the tool publishes pre-built platform binaries in GitHub releases. This is the fastest and most reliable path — no build step, no ecosystem-specific toolchain. Examples: kubectl, terraform, gh, stripe-cli.
2. **Use `homebrew`** when the tool has Homebrew bottles but no GitHub release binaries, or when Homebrew handles complex build dependencies that other builders struggle with. Examples: jq, ffmpeg, imagemagick.
3. **Use an ecosystem builder** (`cargo`, `npm`, `pypi`, etc.) as a disambiguation override when the tool name collides and the correct resolution is the ecosystem package. Examples: if `serve` should resolve to the npm package rather than a GitHub release.

**What about tools not on GitHub?**

The discovery registry format supports any builder — entries like `{"builder": "homebrew", "source": "formula-name"}` or `{"builder": "cargo", "source": "crate-name"}` are valid. The seed list format and validation tool should handle non-GitHub entries:

- For `homebrew` builder: validate the formula exists via Homebrew API
- For ecosystem builders (`cargo`, `npm`, etc.): validate the package exists via the ecosystem's API
- For `github` builder: validate the repo exists, isn't archived, and has binary release assets

At bootstrap time, the majority of entries will use `github` since that's the largest gap (ecosystem tools are already discoverable via the probe stage). But the design doesn't limit future entries to GitHub-only — the seed list format and validation are builder-aware.

### Seed List Format

```json
{
  "category": "dev-tools",
  "description": "General development CLI tools",
  "entries": [
    {"name": "ripgrep", "builder": "github", "source": "BurntSushi/ripgrep", "verification": "https://github.com/BurntSushi/ripgrep"},
    {"name": "fd", "builder": "github", "source": "sharkdp/fd", "disambiguation": true, "verification": "https://github.com/sharkdp/fd"},
    {"name": "bat", "builder": "github", "source": "sharkdp/bat", "disambiguation": true, "verification": "https://github.com/sharkdp/bat"},
    {"name": "jq", "builder": "homebrew", "source": "jq", "verification": "https://github.com/jqlang/jq"},
    {"name": "yq", "builder": "github", "source": "mikefarah/yq", "verification": "https://github.com/mikefarah/yq"},
    {"name": "delta", "builder": "github", "source": "dandavison/delta", "disambiguation": true, "verification": "https://github.com/dandavison/delta"},
    {"name": "kubectl", "builder": "github", "source": "kubernetes/kubernetes", "binary": "kubectl", "verification": "https://kubernetes.io/docs/tasks/tools/"}
  ]
}
```

Fields:
- `name` (required): Tool name as users would type it
- `builder` (required): Builder name (`github`, `homebrew`, `cargo`, `npm`, etc.)
- `source` (required): Builder-specific source argument (`owner/repo` for github, formula name for homebrew, crate name for cargo, etc.)
- `binary` (optional): Binary name when it differs from tool name
- `disambiguation` (optional): When `true`, this entry is a collision override and should be preserved even after a recipe exists
- `verification` (required): URL to the tool's official page or repository proving this is the canonical source. Reviewers must verify this link matches the entry

### Go Tool: cmd/seed-discovery

```
Usage: seed-discovery [flags]

Flags:
  -seeds-dir     Directory containing seed list JSON files (default: data/discovery-seeds)
  -output        Output discovery.json path (default: recipes/discovery.json)
  -recipes-dir   Recipe directory to cross-reference (default: recipes)
  -queue          Priority queue to cross-reference (default: data/priority-queue.json)
  -validate-only  Validate existing discovery.json without regenerating
  -github-token   GitHub API token (default: $GITHUB_TOKEN)
  -verbose        Show detailed validation output
```

Processing pipeline:

```
Read seed lists
    |
    v
Merge all entries (deduplicate by name)
    |
    v
For each entry:
  1. Builder-specific validation:
     - github: repo exists? not archived? owner matches? release in last 24 months? binary assets?
     - homebrew: formula exists via Homebrew API?
     - cargo/npm/pypi/gem: package exists via ecosystem API?
  2. Cross-reference: recipe already exists in recipes/?
  3. Cross-reference: entry in priority queue?
  4. Collision check: name exists on npm, crates.io, PyPI, or RubyGems?
  5. Typosquatting check: Levenshtein distance <=2 from another entry?
    |
    v
Output:
  - Valid entries -> discovery.json
  - Skipped (has recipe, not disambiguation) -> logged, excluded
  - Skipped (has recipe, is disambiguation) -> logged, INCLUDED
  - Failed validation -> logged with reason, excluded
  - Collisions detected -> logged for human review
  - Typosquatting warnings -> logged for human review
```

**Validation criteria for "has binary releases":** A release qualifies if it has at least one asset that is not an auto-generated source archive (GitHub's automatic `Source code (zip)` and `Source code (tar.gz)` don't count). The asset name should contain a platform identifier (linux, darwin, windows, amd64, arm64, etc.) suggesting it's a pre-built binary.

**Entry lifecycle:** Regular entries are excluded from `discovery.json` when a recipe exists in `recipes/`. Disambiguation entries (marked `disambiguation: true`) are always included regardless of recipe existence, because they prevent the ecosystem probe from resolving to the wrong tool.

### Output Format

The tool outputs `recipes/discovery.json` matching the existing schema:

```json
{
  "schema_version": 1,
  "tools": {
    "ripgrep": {"builder": "github", "source": "BurntSushi/ripgrep"},
    "bat": {"builder": "github", "source": "sharkdp/bat"},
    "kubectl": {"builder": "github", "source": "kubernetes/kubernetes", "binary": "kubectl"}
  }
}
```

The `builder` field is always `"github"` for most entries (the primary use case). Disambiguation overrides may use other builders (e.g., `"cargo"` for a Rust tool that should be installed from crates.io rather than GitHub releases).

### CI Freshness Workflow

```yaml
# .github/workflows/discovery-freshness.yml
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

On validation failure, the workflow creates an issue listing stale entries with the reason (archived, deleted, no releases).

### Data Flow

```
Seed lists (data/discovery-seeds/*.json)
    |
    v
cmd/seed-discovery (Go tool)
    |--- validates via GitHub API
    |--- cross-references recipes/ and priority queue
    |--- detects ecosystem collisions
    |
    v
recipes/discovery.json (~500 entries)
    |
    v
tsuku update-registry (fetches to $TSUKU_HOME/registry/discovery.json)
    |
    v
RegistryLookup resolver (instant O(1) lookup)
```

### Relationship to Batch Pipeline

The discovery registry and batch pipeline serve different lifecycle stages:

| Aspect | Discovery Registry | Priority Queue |
|--------|-------------------|----------------|
| Purpose | Name resolution for `tsuku install` | Batch recipe generation |
| Builder | Primarily `github` | Primarily `homebrew` |
| Lifecycle | Entry becomes redundant when recipe exists | Entry moves to `success` when recipe generated |
| Update | `cmd/seed-discovery` | `cmd/seed-queue` |
| Output | `recipes/discovery.json` | `data/priority-queue.json` |

Shared patterns (not shared code):
- GitHub API client with retry and rate limiting
- JSON file load/save with schema validation
- Cross-referencing against existing recipes

As the batch pipeline generates recipes for popular tools, those tools' discovery entries become redundant (the recipe exists, so the resolver short-circuits before reaching the registry stage). The `seed-discovery` tool's cross-reference step handles this: it skips tools that already have recipes, so re-running the tool naturally shrinks the registry.

## Implementation Approach

### Phase 1: Seed Lists and Go Tool

Create the seed list files and `cmd/seed-discovery` tool. Start with 50-100 entries from the most obvious sources (Kubernetes tools, HashiCorp tools, popular GitHub CLI tools) to validate the pipeline end-to-end.

**Files:**
- `data/discovery-seeds/*.json` (seed lists)
- `cmd/seed-discovery/main.go`
- `recipes/discovery.json` (updated output)

### Phase 2: Scale to 500 Entries

Expand the seed lists to cover ~500 tools. Run the tool, review collision reports, add disambiguation entries.

**Files:**
- `data/discovery-seeds/*.json` (expanded)
- `data/discovery-seeds/disambiguations.json` (collision overrides)
- `recipes/discovery.json` (500 entries)

### Phase 3: CI Freshness

Add the weekly freshness workflow.

**Files:**
- `.github/workflows/discovery-freshness.yml`

## Security Considerations

### Download Verification

The `seed-discovery` tool doesn't download binaries. It queries the GitHub API over HTTPS to verify repo metadata (existence, archived status, release assets). No artifacts are downloaded or executed during the bootstrap process.

The entries it produces are consumed by the discovery resolver, which delegates to builders that have their own download verification (checksums, HTTPS). The accuracy of the registry entry (pointing to the correct repo) is the security concern here, not download integrity.

### Execution Isolation

The tool runs locally or in CI. It makes read-only GitHub API calls and writes a JSON file. No code execution, no sandbox needed, no elevated permissions required.

### Supply Chain Risks

**Registry poisoning via seed lists.** A malicious PR could add an entry pointing to a compromised repo (e.g., `kubernetes-tools/kubernetes` instead of `kubernetes/kubernetes`). Mitigations:
- Every seed list entry requires a `verification` URL linking to the tool's official page
- PR reviewers must verify the `verification` URL matches the `repo` field
- The tool validates that repos exist and have legitimate release history (24+ months of releases)
- Weekly freshness checks detect repos that change ownership or get transferred
- Automated typosquatting detection flags entries with names close to existing entries (Levenshtein distance <=2)

**PR review requirements for seed list changes:**
- Changes to `data/discovery-seeds/` require review from a CODEOWNERS-designated reviewer
- Reviewer checklist: (1) verify `verification` URL is the tool's official page, (2) verify `repo` matches the official repository, (3) check for typosquatting against existing entries, (4) for disambiguation entries, verify the chosen resolution is correct

**Stale entries pointing to transferred repos.** A legitimate repo transferred to a new owner could become malicious. Mitigations:
- Weekly CI freshness check detects ownership changes (compare owner in entry vs. current GitHub owner)
- The parent design's ownership verification applies here

**Typosquatting via seed lists.** An entry like `kubeclt` (one character off from `kubectl`) could redirect users to a malicious repo. Mitigations:
- The `seed-discovery` tool computes Levenshtein distance between all entry names and flags pairs with distance <=2
- Flagged entries require explicit justification in the seed list or are rejected

**Disambiguation errors.** A wrong disambiguation override sends users to the wrong tool. Mitigations:
- Disambiguation entries are flagged in seed lists (`disambiguation: true`) for extra review attention
- Collision detection automates discovery; human review ensures correctness
- Each disambiguation entry must document the rationale (which tool is the "correct" one for the name)

### User Data Exposure

The bootstrap tool sends tool names and repo identifiers to the GitHub API and ecosystem registries (crates.io, npm, PyPI, RubyGems) during collision detection. This is equivalent to browsing those sites. No user data is collected or transmitted. The published `discovery.json` contains only tool names and public repo identifiers.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Registry poisoning via PR | Verification URLs, CODEOWNERS review, automated validation | Sophisticated attack with matching verification page |
| Typosquatting via seed lists | Levenshtein distance check, review checklist | Novel Unicode or homoglyph attacks (handled by resolver's normalize.go) |
| Stale entry to transferred repo | Weekly freshness check with ownership comparison | 7-day window between transfer and detection |
| Wrong disambiguation | Automated collision detection + human review + documented rationale | Obscure collisions not in seed lists |
| GitHub API compromise | HTTPS transport, standard API authentication | GitHub infrastructure compromise |

## Consequences

### Positive

- Discovery resolver has data to work with (~500 tools resolve instantly)
- Repeatable process for ongoing maintenance
- Automated collision detection catches disambiguations humans would miss
- CI freshness keeps entries current
- Contributor-friendly: add an entry to a seed list, run the tool, submit PR

### Negative

- Initial seed list curation requires manual effort (~500 entries)
- GitHub API rate limits constrain validation speed
- Weekly freshness has up to 7-day staleness window

### Mitigations

- Seed lists are organized by category for parallel curation by multiple contributors
- Authenticated GitHub API provides 5000 requests/hour (sufficient for 500 entries)
- Critical tools (top 50) could have more frequent freshness checks if needed

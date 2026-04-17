# Lead: Curation criteria

## Findings

### Tiering already exists — in two separate, not-yet-unified places

The batch pipeline has a concrete three-level tier definition, written in `data/queues/priority-queue-homebrew.json`:

- Tier 1: "Critical - manually curated high-impact tools"
- Tier 2: "Popular - >10K weekly downloads (>40K/30d)"
- Tier 3: "Standard - all other packages"

This tiering drives which queue entries the batch generator processes first (`-tier` flag, where 1=critical, 2=popular, 3=all). In `priority-queue-homebrew.json`, tier 1 tools include: `gh`, `neovim`, `node`, `tmux`. Tier 2 includes `awscli`, `uv`, `jq`, `ripgrep`, `bat`, `exa`, `yq`, and many more. Tier 3 is everything else.

However, the **recipe TOML files have their own `tier` field**, and it is almost universally `0` across the 1,405 recipes that exist. Only two recipes use non-zero values (`nix-portable.toml` uses `tier = 3`, `hello-nix.toml` uses `tier = 3`). The `tier` field in recipe TOML appears to be a placeholder that the batch generator wrote during early scaffolding but hasn't been meaningfully populated.

### The `confidence` field is the actual curated/auto distinction

In `data/queues/priority-queue.json`, each queue entry carries a `confidence` field:
- `"curated"` — the source (e.g., `github:cli/cli`) was manually selected
- `"auto"` — the source was resolved by the disambiguation pipeline

The curated list lives in `data/disambiguations/curated.jsonl` and contains 20 tools: `bat`, `fd`, `rg`, `delta`, `dust`, `sd`, `age`, `sk`, `fzf`, `jq`, `yq`, `just`, `task`, `gh`, `hub`, `gum`, `dive`, `step`, `buf`, `ko`. These are modern CLI tools with unambiguous GitHub releases.

In `internal/seed/freshness.go`, curated entries are explicitly excluded from re-disambiguation (`IsCurated` returns true, skips the entry). Curated sources are only validated via HTTP HEAD to confirm they still exist.

### `llm_validation` carries a different meaning than expected

The `llm_validation` field is present in ~1,218 of 1,405 recipes and always set to `"skipped"`. It is not a signal of quality or human review — it appears to be an artifact of the early batch generator that marked LLM-assisted validation as not yet run. Recipes without this field (about 187 files) are handcrafted recipes that predate the batch generator: `gh.toml`, `golang.toml`, `golangci-lint.toml`, `just.toml`, `goreleaser.toml` and similar. These clean, well-formed recipes have no `tier`, no `llm_validation`, and no `requires_sudo` fields — just the fields they actually need.

### Handcrafted vs. auto-generated is visually distinct

Handcrafted recipes (no `tier`, no `llm_validation`, no `requires_sudo`):
- `gh.toml` — correct `os_mapping`, `arch_mapping`, split by OS with `when`
- `golang.toml` — custom download URL with pattern substitution, dual binary
- `golangci-lint.toml` — clean github_archive, pattern match in verify
- `just.toml` — correct musl/darwin arch strings, compact format

Batch-generated recipes with `tier = 0`, `llm_validation = "skipped"`:
- `gitui.toml` — uses `homebrew` action with many blank version fields
- `goreleaser.toml` — same scaffold pattern; all version fields blank
- `jq.toml` — `unsupported_platforms` added post-hoc by constraint derivation

The batch-generated recipes trend toward the `homebrew` action (bottle extraction), while handcrafted ones use `github_archive` or `download_archive` for direct binary downloads.

### How tools enter the pipeline

Tools enter `priority-queue.json` via two paths:
1. **Seed queue** (`cmd/seed-queue`): Probes Homebrew, Cargo, npm, PyPI, RubyGems weekly. Ranks by download volume and assigns priority 1/2/3. Disambiguation selects the best source across ecosystems.
2. **Curated overrides** (`data/disambiguations/curated.jsonl`): Manually authored JSON; these entries get `confidence: curated` and are never re-disambiguated.

The batch generator (`cmd/batch-generate`) processes queue entries, generates recipe TOML via an LLM, runs local validation, then sandbox-validates across 11 platform environments (5 Linux x86_64 families, 4 Linux arm64, 2 macOS) before merging.

### Claude is absent from the registry

There is no `claude.toml` or any AI coding assistant recipe in `recipes/c/`. The claude CLI tool is a concrete gap in the curated list, and its disambiguation is non-trivial: `claude` is a common name that could resolve to multiple packages.

## Implications

### 5 concrete criteria for curated status

1. **Name ambiguity requires human judgment.** Tools with names that collide across ecosystems (e.g., `claude`, `sd`, `bat`) need a human to confirm the correct source. The `confidence: curated` mechanism already captures this but is limited to 20 tools.

2. **Users install it by name, not by searching.** Tools like `gh`, `jq`, `fzf`, `claude` are typed verbatim by developers who expect them to work immediately. Installation failure on these creates the strongest negative impression. Download rank is a proxy, but intent (typed-by-name) is the better signal.

3. **Platform coverage must be complete.** A handcrafted recipe for a tier-1 tool should work on all 11 validated platform environments, not be auto-constrained to a subset. The batch pipeline derives constraints from validation results; a curated recipe should explicitly target all relevant platforms from the start.

4. **The install mechanism is non-standard.** If a tool's canonical install path doesn't go through a single GitHub release tarball (e.g., claude installs via npm but also ships native binaries; golang uses go.dev/dl not GitHub releases), the LLM-generated recipe will likely be wrong or incomplete. Human authorship adds correctness that auto-generation can't reliably provide.

5. **The tool has high failure cost.** Some tools are load-bearing for developer workflows (git, fzf, jq, claude). A broken recipe for these causes more support load than a broken recipe for an obscure library. Periodic re-verification — not just source validation via HTTP HEAD — is warranted.

### Where curated recipes live

The current design has no structural distinction between handcrafted and batch-generated recipes in `recipes/` — they coexist in the same alphabetical subdirectories. The real distinction is:
- The `confidence` field in the queue (curated vs. auto)
- The absence of `tier = 0` / `llm_validation = "skipped"` scaffolding

A "curated tier" doesn't require a separate directory. It could be enforced by adding a `handcrafted = true` field to the recipe `[metadata]` and gating it to human-authored PRs only. Alternatively, the existing `tier` field (currently always 0) could be repurposed: tier 1 = curated + periodically re-verified, tier 2 = auto-generated + validated, tier 3 = auto-generated + constrained.

## Surprises

1. **The `tier` field in recipe TOML is essentially unused.** It was always `0` except for two nix recipes (`nix-portable` and `hello-nix` which use `3`). The tiering concept exists only in the queue infrastructure, not in the installed recipes themselves. The recipe consumer never reads `tier`.

2. **`llm_validation = "skipped"` is universal, not selective.** I expected this field to distinguish validated vs. unvalidated recipes. Instead, it marks every batch-generated recipe as "skipped" — meaning the LLM validation phase was never run (or never enforced). It's a dead metadata field.

3. **Only 20 tools are in the curated list**, and most are Unix-style CLI tools (`bat`, `fd`, `rg`, etc.). Modern AI tools (`claude`, `aider`, `continue`) are entirely absent, despite being exactly the kind of tools tsuku users would want installed immediately.

4. **Handcrafted recipes are identifiable by absence.** The ~187 recipes without `tier`/`llm_validation`/`requires_sudo` fields form a natural curated set — they just aren't labeled as such.

5. **The batch pipeline has a robust security gate** (excludes any recipe with `run_command` action) but no quality gate that distinguishes curated from auto-generated. All recipes that pass platform validation get merged with equal status.

## Open Questions

1. Should `tier` in recipe TOML be repurposed (1 = curated, 2 = auto-validated) or dropped in favor of a new `handcrafted = true` boolean?

2. Does "periodically re-verified" for curated recipes mean a separate CI workflow that installs and runs the tool on real hardware, or is the existing sandbox validation sufficient?

3. For tools like `claude` that install via npm but also have native binaries, should the curated recipe use the direct binary path (faster, no Node dependency) or the npm path (official channel)? This is the concrete claude disambiguation bug.

4. The curated.jsonl source-selection list and the handcrafted recipe authoring are currently separate concerns. Should they be unified — i.e., if a recipe is handcrafted, it's automatically promoted to `confidence: curated` in the queue?

5. What is the right threshold for "typed by name"? Download volume is measurable (Homebrew tracks 30-day installs); intent is harder. Could telemetry from `tsuku search` or `tsuku install` fill this gap over time?

## Summary (3 sentences)

The batch pipeline already has a three-tier priority concept (critical / popular / standard) in queue infrastructure, but the corresponding `tier` field in recipe TOML is universally set to 0 and ignored — the real curated/auto distinction lives in the queue's `confidence` field, with only 20 manually curated tools. Handcrafted recipes are identifiable by negative space: they lack the `tier = 0`, `llm_validation = "skipped"`, and blank-version-field scaffolding that batch-generated recipes carry, and they use direct binary download actions rather than the `homebrew` bottle action. A curated recipe for a tool like `claude` should meet five criteria: name requires disambiguation, users install it by exact name, platform coverage must be complete, the install mechanism is non-standard, and a failed install has high cost — criteria that map cleanly onto the existing priority-1 concept but need to be formalized as a recipe-level label and periodic re-verification workflow.

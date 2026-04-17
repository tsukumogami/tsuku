# Exploration Findings: install-claude

## Core Question

Which major developer tools lack high-quality handcrafted recipes in the tsuku registry, what criteria distinguish a tool that warrants one, and where should these curated recipes live relative to auto-generated pipeline output? The claude disambiguation bug is the concrete first case — its solution should be consistent with the broader system design.

## Round 1

### Key Insights

- **The gap is large and systematic.** ~20 high-profile tools are missing entirely (node, AWS CLI, neovim, cmake, claude, gcloud, all AI coding assistants), and another 8-10 have weak batch-generated recipes that only work on Linux (kubectl, helm, ripgrep, fd, eza, zoxide, podman, bazel). Coverage quality correlates inversely with install complexity: tools using direct GitHub release tarballs are well-covered; tools requiring npm, custom CDNs, or complex platform variants are absent or broken. [coverage-gap]

- **kubectl and helm are actively misleading.** They appear in search results but only install on Linux. A macOS user who searches for kubectl, finds it, tries to install it, and gets a failure has a worse experience than if the recipe didn't exist. ~571K and comparable Homebrew download counts suggest heavy usage. [coverage-gap]

- **AI coding tools have zero handcrafted coverage.** Despite tsuku-llm (a tsuku internal tool) being among the most sophisticated handcrafted recipes, the entire category — claude, aider, gemini-cli, ollama, opencode — has no recipes at all. These are precisely the tools tsuku's target users install first. [coverage-gap]

- **Discovery entries without recipes are worse than nothing.** Neovim (541K downloads), bat, starship, cmake (1.58M downloads), and aws (2.2M downloads) appear in `tsuku search` but fail on `tsuku install`. This generates failed install attempts where "not found" would be more honest. [coverage-gap]

- **A three-tier priority concept already exists in the queue, but is invisible to recipes.** `priority-queue.json` has tier-1 (critical), tier-2 (popular), tier-3 (standard) — but the `tier` field in recipe TOML is always 0 and has no runtime effect. The real curated/auto distinction lives in the queue's `confidence` field (`curated` vs `auto`), with only 20 tools in `curated.jsonl`. [curation-criteria]

- **Handcrafted recipes are identifiable by absence.** The ~184 existing handcrafted recipes have no `tier`, no `llm_validation`, and no `requires_sudo` fields. The ~1,218 batch-generated recipes all have `tier = 0` and `llm_validation = "skipped"`. These fields are dead as quality signals (every generated recipe has them) but reliable as provenance markers. [curation-criteria, recipe-placement]

- **The embedded slot is not the answer.** `internal/recipe/recipes/` is a sealed contract for build-time action dependencies (go, rust, nodejs, cmake, etc.). Every update to an embedded recipe requires a binary release. Unsuitable for frequently-updated user tools. [recipe-placement]

- **`curated = true` (flag) vs `recipes/core/` (directory) is the main placement tension.** The metadata flag is zero infrastructure change and backward-compatible with 184 existing handcrafted recipes. The dedicated directory provides an unambiguous file-system signal and enables separate CI gates, but requires changes to URL construction in `registry.go` and a new provider priority slot. [recipe-placement]

- **The nightly execute-sample is alphabetically random, not importance-weighted.** ~26 recipes get actual install testing per night — one per letter directory. A tool like `terraform` only gets tested if it happens to be the first alphabetically-unexcluded recipe in `t/`. Silent rot is possible: a recipe that passes PR CI, then neither its TOML nor Go source changes, receives no further install test. [periodic-testing]

- **`ci.curated` in `test-matrix.json` is the natural implementation path.** The `ci.scheduled` array already models an opt-in set for heavier nightly tests. A `ci.curated` array in the same file declares which recipes get periodic full-install testing across a representative platform matrix, with automatic issue creation on failure. [periodic-testing]

- **Claude's fix requires both a handcrafted recipe AND a discovery entry, and neither alone is sufficient.** The `NpmBuilder.Build()` method uses `req.Package` (the tool name, "claude") not `req.SourceArg` to fetch npm metadata, so auto-generation would query the wrong npm package. A discovery entry without a recipe routes through `tryDiscoveryFallback` → `runCreate` → `NpmBuilder.Build("claude")` → wrong metadata. The handcrafted recipe (`recipes/c/claude.toml`) must explicitly set `package = "@anthropic-ai/claude-code"`. [claude-resolution]

- **Ecosystem curation patterns (Homebrew, nixpkgs, mise) converge on the same shape:** explicit inclusion criteria + multi-platform automated testing + strong provenance signal. mise's approach (curated registry → backend mapping, preferring aqua for supply-chain security) is the closest analog to tsuku's model. [ecosystem-curation]

### Tensions

- **`curated = true` flag (lowest friction) vs `recipes/core/` directory (strongest signal).** The flag can be adopted today with zero infrastructure change; the directory provides unambiguous file-system discoverability but requires provider chain changes. Likely a two-phase decision: flag now, directory when the registry URL architecture is revisited.

- **Recipe name = tool name (`claude`) vs package name (`claude-code`).** Research clearly favors tool name: the installed binary is `claude`, the wrangler.toml precedent (recipe name = command) confirms it. Not a real tension — the answer is `claude`.

- **Fix NpmBuilder to use `sourceArg` when set vs accept that scoped-package tools need handcrafted recipes.** A targeted NpmBuilder fix would unblock auto-generation for tools like gemini-cli. But it's scope expansion; the handcrafted path is sufficient and correct for the initial problem.

### Gaps

- The ecosystem curation agent (Homebrew/nixpkgs/mise research) couldn't write its file; findings were captured in the summary notification only. Sufficient for this round.
- The exact priority ordering of the first batch of curated recipes to author (beyond the top 13 identified) was not fully resolved. The coverage-gap findings provide strong signal but no finalized ranked list.

### Open Questions

- Should the near-term solution be `curated = true` flag or `recipes/core/` directory? The flag is lower risk; the directory is cleaner. This is the key design decision.
- Should `NpmBuilder` be fixed to use `sourceArg` when it's set? This would make auto-generation work for other scoped npm tools (gemini-cli, etc.) without requiring handcrafted recipes. Probably yes, but separate scope.
- What is the right cadence for curated recipe periodic tests — daily (same as nightly) or weekly?
- Should the curated recipe set start small (20-30 tools) or attempt broad coverage (50-100) from the start?

## Accumulated Understanding

The problem is larger than the claude disambiguation bug — it's a structural gap between tsuku's automation-focused pipeline and the small set of tools users actually reach for first. The pipeline is optimized for breadth (1,405 recipes, 823 discovery entries) but the most important 20-50 tools are poorly served: either missing, limited to Linux, or using Homebrew bottles that fail on non-Homebrew-capable platforms.

Three interrelated decisions need to be made:

1. **Curated recipe identity**: How does a recipe declare itself as handcrafted and worth periodic verification? Current answer by convention (absence of `tier`/`llm_validation`); proposed answer is a `curated = true` flag or `recipes/core/` directory.

2. **Periodic testing**: How do curated recipes get ongoing install verification? Current answer: random alphabetical nightly sample. Proposed: `ci.curated` array in `test-matrix.json`, extending the existing nightly workflow.

3. **Initial coverage**: What tools get handcrafted recipes first, and in what order? Research identified 13 critical gaps. The full design doc should include a prioritized authoring queue.

The claude recipe is the concrete first case and validates the pattern. The solution — handcrafted `recipes/c/claude.toml` + discovery entry `recipes/discovery/c/cl/claude.json` — is fully specified and can be implemented immediately. The broader system design (curated flag/directory, periodic testing) can be designed in parallel.

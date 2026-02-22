# Security Review: DESIGN-system-lib-backfill.md

## Scope

This review evaluates the Security Considerations section (lines 315-341) of the system library backfill design against four required dimensions: download verification, execution isolation, supply chain risks, and user data exposure. It also assesses whether `satisfies` metadata and automatic requeue-on-merge create new attack vectors.

---

## Dimension 1: Download Verification

**Design claim** (line 318-319): "The `homebrew` action downloads pre-built bottles from Homebrew's CDN with the same integrity checks used for all Homebrew-based recipes. No new download paths are introduced."

**Codebase verification**: Confirmed. `internal/actions/homebrew.go:106` calls `verifySHA256(bottlePath, blobSHA)` which computes SHA256 over the downloaded bottle and compares it against the blob SHA from the GHCR manifest (lines 334-352). The SHA comes from Homebrew's OCI manifest annotations (`sh.brew.bottle.digest`), not from the recipe TOML. The formula name is validated against path traversal and injection at line 140-157.

**Assessment**: Adequate. The verification chain is: GHCR manifest -> blob SHA -> downloaded file SHA256 comparison. This is the same path used for all 22 existing library recipes and all homebrew-based tool recipes. No new download path is introduced.

**One nuance the design doesn't mention**: The SHA comes from Homebrew's GHCR manifest fetched at install time, not from a checksum declared in the recipe TOML. This means verification depends on GHCR availability and integrity at download time, not on a pre-committed hash. This is the existing homebrew action's trust model and isn't changed by this design, but it differs from static checksum verification used by `github_archive` or `download` actions.

---

## Dimension 2: Execution Isolation

**Design claim** (lines 322-324): "Library recipes install shared objects (`.so`, `.dylib`) and headers into `$TSUKU_HOME/libs/`. These files are not executable -- they're loaded by other tools via dynamic linking."

**Codebase verification**: Partially accurate. `internal/install/library.go:30` confirms libraries install to `$TSUKU_HOME/libs/{name}-{version}/`. The `install_binaries` action with `install_mode = "directory"` is used by library recipes (confirmed in `recipes/l/libcurl.toml:17`).

However, the claim that library files "are not executable" is misleading. Shared objects are loaded via `dlopen`/dynamic linking, which executes code in them. A malicious `.so` file will execute its constructor functions and any called exported symbols. The design should not call these "not executable" -- they are not standalone executables, but they execute code when loaded.

Additionally, `internal/actions/set_rpath.go:401-449` validates that RPATH entries stay within `$TSUKU_HOME/libs/`, which prevents library recipes from injecting RPATH entries pointing to attacker-controlled directories. This is a meaningful isolation mechanism the design doesn't mention.

**Assessment**: The isolation claim understates the actual execution risk. Shared libraries execute code when loaded. The practical mitigation is that library recipes go through the same PR review as tool recipes, and the homebrew action only fetches from Homebrew's GHCR (not arbitrary URLs). The RPATH validation in `set_rpath.go` is a real security boundary worth mentioning.

---

## Dimension 3: Supply Chain Risks

**Design claim** (lines 326-330): "`satisfies` metadata is declared in recipe TOML files reviewed via PR, not derived automatically. This prevents an attacker from claiming to satisfy a popular library name without review."

**Codebase verification**: Confirmed with strong supporting evidence.

1. **CI-time duplicate detection**: `scripts/generate-registry.py:321-345` validates that no two recipes claim the same satisfies package name, and that no satisfies entry collides with an existing canonical recipe name. This runs on every PR that touches recipes.

2. **Structural validation**: `internal/recipe/validate.go:74-109` checks for self-referential entries, malformed ecosystem names, and empty package names at parse time.

3. **Loader priority**: `internal/recipe/loader.go:80-143` resolves exact recipe names before falling to the satisfies index. A satisfies entry cannot shadow an existing canonical recipe.

4. **Anti-recursion**: `internal/recipe/loader.go:136-139` uses `loadDirect` to prevent infinite recursion from cyclic satisfies entries.

**Assessment**: The satisfies system has defense-in-depth. The main residual risk the design correctly identifies is "correctly-named but malicious library content" -- if an attacker submits a PR for a legitimate-sounding library that passes review but contains malicious shared objects. This is inherent to any package manager that accepts community contributions and isn't specific to this design.

**One gap**: The design says "CI validates satisfies entries, cross-recipe duplicate detection" but doesn't mention the collision check against canonical recipe names (line 340-345 of `generate-registry.py`). That check is actually the more important one -- it prevents a satisfies entry from creating a confusing alias for an existing tool recipe.

---

## Dimension 4: User Data Exposure

**Design claim** (lines 332-333): "Library recipes don't access or transmit user data. They install static binary artifacts (shared libraries, headers, pkg-config files) into `$TSUKU_HOME/libs/`. No telemetry or network activity occurs after installation."

**Codebase verification**: The library installation path (`internal/install/library.go`) copies files from work directory to `$TSUKU_HOME/libs/` and records checksums and usage tracking in state.json. No network calls post-installation. State.json tracks `used_by` relationships (which tools use which library) but this is local state.

**Assessment**: Adequate. The one caveat is that state.json tracks library usage relationships, but this is the same pattern used for tool installations and doesn't expose user data externally.

---

## Satisfies Metadata: New Attack Vectors

**Question**: Does the satisfies metadata create any new attack vectors?

**Analysis**: The satisfies system introduces a name-aliasing mechanism. Potential attack vectors:

1. **Typosquatting via satisfies**: An attacker submits a recipe that claims `satisfies.homebrew = ["opensl"]` (typo of "openssl"). If a recipe has a typo in its dependency list, the malicious recipe resolves as the dependency. **Mitigation**: PR review catches this. The CI validation (`generate-registry.py`) checks for duplicates but not for similar-looking names. This is a residual risk but not introduced by this design -- it exists for any recipe in the registry.

2. **Satisfies entry that shadows a future canonical recipe**: An attacker claims `satisfies.homebrew = ["future-tool"]` before anyone creates `recipes/f/future-tool.toml`. When someone later creates the canonical recipe, the loader's exact-match priority means the canonical recipe wins. **No persistent risk.**

3. **Index poisoning via manifest cache**: The satisfies index includes entries from the registry manifest (`internal/recipe/loader.go:400-417`). If an attacker could corrupt the cached manifest file, they could redirect satisfies lookups. **Mitigation**: The manifest is fetched from the registry (same trust model as recipes themselves) and embedded recipes take priority.

**Verdict**: No new attack vectors beyond those inherent to any name-resolution system with aliases. The existing validation layers (CI duplicate detection, collision with canonical names, PR review) provide adequate coverage.

---

## Automatic Requeue-on-Merge: New Risks

**Question**: Does the automatic requeue-on-merge create any new risks?

**Analysis** (based on `.github/workflows/update-queue-status.yml`):

1. **Requeue mechanism**: When a recipe merges to `main`, the workflow detects changed recipe files, matches them to queue entries by name, and sets status to "success". Then `queue-maintain` runs to requeue blocked entries.

2. **Source verification**: The workflow extracts the source type from the merged recipe (lines 71-128) and compares it to the queue entry's source. If they don't match, it still sets "success" but marks `confidence = "curated"`. This means a recipe could be created for a different source than what the queue expected.

3. **Potential risk -- premature requeue**: If a library recipe is merged but is incomplete (e.g., missing a platform step), tools blocked on that library will be requeued and immediately fail again when the pipeline processes them. This isn't a security risk -- it's an efficiency waste that results in another blocked cycle. The design acknowledges this implicitly in Decision 5 ("Match the Blocking Platform").

4. **Potential risk -- CI credential scope**: The workflow uses a GitHub App token with `contents: write` to push queue changes. If an attacker could trigger this workflow with a malicious recipe, they could modify queue state. **Mitigation**: The workflow only triggers on pushes to `main` (requires PR approval), not on PR creation.

5. **No recipe execution**: The requeue workflow does not execute recipes. It only changes queue entry status fields in a JSON file. Actual recipe execution happens in the separate `batch-generate.yml` workflow.

**Verdict**: The requeue mechanism doesn't create new security risks. The worst case is wasted pipeline cycles from incomplete library recipes, which the design's platform-matching strategy (Decision 5) already addresses.

---

## Residual Risk Assessment: Honesty Check

The design's residual risk table (lines 337-341) lists three risks:

| Risk | Residual Risk Listed | Assessment |
|------|---------------------|------------|
| Malicious library recipe via PR | "Reviewer must verify formula name matches intent" | Honest but incomplete. Reviewer must also verify the formula is a legitimate Homebrew formula and that the `satisfies` entries are correct. For library recipes that fail deterministic generation and get manual fixes, the review surface is larger (the manual changes need extra scrutiny). |
| Satisfies metadata claiming wrong name | "Correctly-named but malicious library content" | Honest. This is the fundamental trust-the-source risk inherent to Homebrew bottles. |
| Requeue triggers retry of poisoned recipe | "None beyond standard recipe validation" | Honest. Requeue only changes status; it doesn't bypass validation. |

**One residual risk not listed**: The design proposes creating library recipes at scale (potentially dozens in a short timeframe). Volume creates review fatigue. When 15 library recipes are submitted in a batch, the 14th may get less scrutiny than the 1st. This is an operational risk, not a technical one, but it's relevant to the backfill context.

---

## Summary of Findings

### Adequate

- **Download verification**: Correctly delegates to the existing homebrew action's GHCR SHA256 verification. No new download paths.
- **Supply chain -- satisfies metadata**: CI validation, structural validation, and loader priority provide defense-in-depth against satisfies abuse.
- **User data exposure**: No external data transmission. Local state tracking matches existing patterns.
- **Requeue-on-merge**: No new security risks. Only modifies queue status JSON, doesn't execute recipes.

### Needs Improvement

1. **Execution isolation claim is misleading** (lines 323-324): "These files are not executable" is incorrect. Shared libraries execute code when loaded. The design should say "not standalone executables" or "loaded by tools at runtime" and acknowledge that a malicious `.so` would execute code. The RPATH validation in `set_rpath.go` that constrains paths to `$TSUKU_HOME/libs/` is a real isolation boundary worth mentioning.

2. **Missing residual risk: review fatigue at scale**: The backfill process will submit many library recipes in batches. The security section doesn't address the operational risk of reduced review quality under volume. Consider noting that library recipes from deterministic generation need lighter review than manual fixes, and manual fixes should get tagged for extra scrutiny.

3. **Understated: the trust model for homebrew bottles**: The verification section says "same integrity checks" but doesn't make explicit that the trust anchor is Homebrew's GHCR infrastructure, not a pre-committed hash. If GHCR were compromised, all homebrew-based recipes (not just libraries) would be affected. This isn't a gap in this design -- it's the existing trust model -- but the security section would be more complete if it named the trust anchor explicitly.

### Not a Concern

- The satisfies metadata does not create new attack vectors beyond those inherent to name aliasing systems.
- The requeue automation does not bypass recipe validation or create privilege escalation paths.

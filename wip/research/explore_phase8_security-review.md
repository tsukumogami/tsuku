# Security Review: DESIGN-recipe-ci-batching.md

## Scope

This review evaluates the security posture of the recipe CI batching design. The design changes how GitHub Actions CI jobs are grouped (batching N recipes per runner instead of 1 per runner) for `test-changed-recipes.yml` and `validate-golden-recipes.yml`. No Go code changes. No new scripts. Only workflow YAML modifications.

The review is grounded in the actual codebase at commit `985791af` on `main`.

---

## Threat Model Context

tsuku is a tool that downloads and executes binaries. Recipes are TOML files that define download URLs, shell commands, and verification steps. In CI, recipes come from PR branches -- meaning any external contributor can submit a recipe that:

1. Downloads arbitrary binaries from the internet
2. Executes arbitrary shell commands via `run_command` action (see `internal/actions/run_command.go:104` -- passes command to `sh -c`)
3. Installs binaries that become available on `$PATH`

This is the **existing** threat model. The batching design doesn't change it, but it does change the blast radius of a malicious recipe within a CI job.

---

## Security Section Assessment

### 1. "Download Verification: Not Applicable" -- Correct

The design correctly identifies this as not applicable. The same `tsuku install` command runs with the same verification pipeline:

- `download.go:350-353`: HTTPS enforced for all downloads
- `download_file.go:51-54`: Checksum required for plan-based installs
- `signature.go:171-209`: PGP signature verification when configured
- `httputil/client.go:57-109`: Secure HTTP client with SSRF protection, redirect validation, decompression bomb protection
- `httputil/ssrf.go:18-38`: Blocks private, loopback, link-local, multicast IPs

The number of invocations per runner is irrelevant to verification correctness. **No issue.**

### 2. "Execution Isolation" -- Partially Addressed, One Gap

The design correctly identifies the `TSUKU_HOME` isolation pattern: each recipe gets its own `TSUKU_HOME` directory, following the macOS aggregated job pattern already in production (`test-changed-recipes.yml:239-242`).

**What's addressed:**
- Filesystem isolation via per-recipe `TSUKU_HOME`
- Shared download cache via symlink (read-through cache, not shared state)

**Gap: Cross-recipe interference via shared runner state outside TSUKU_HOME.**

The design acknowledges this: "if a recipe modifies global state outside `TSUKU_HOME` (e.g., writes to `/tmp` or modifies system paths), subsequent recipes in the batch could be affected."

This is an accurate assessment, but the risk is **larger in the batched model than the per-recipe model** and the design should be explicit about why this is acceptable:

- **Per-recipe model (current Linux):** A malicious recipe can only affect its own runner. No cross-contamination.
- **Batched model (proposed):** A malicious recipe runs first in a batch, then 14 more recipes execute on the same runner. The malicious recipe could:
  - Plant a trojan in `/usr/local/bin` that shadows a legitimate binary
  - Modify `$GITHUB_PATH` (via `$GITHUB_PATH` file append) to inject a PATH entry that later recipes use
  - Write to `/tmp` where subsequent recipes might read
  - Leak `GITHUB_TOKEN` to an external service (already possible in per-recipe model, unchanged)

**Risk level:** Low in practice. The macOS aggregated path has been running this exact pattern without incident. All recipes in a batch come from the same PR, so a malicious contributor controls all recipes in their PR anyway. The threat is really "accidental interference" not "malicious escalation." But the design should state this explicitly rather than handwaving with "hasn't been a problem."

**Recommendation:** Add a sentence to the design noting that cross-recipe interference is bounded by the fact that all recipes in a batch originate from the same PR author, so there's no privilege escalation -- a malicious author already controls all the recipes they're submitting.

### 3. "Supply Chain Risks: Not Applicable" -- Correct

The design doesn't change recipe sourcing, parsing, or validation. Recipes are read from the PR branch in both the current and proposed models. The `git diff` detection logic is unchanged. The jq batching step operates on the already-validated recipe list.

**One nuance worth noting (not a finding):** The design's jq batching code operates on the detection job's output, which is already constructed from `git diff` results. The batching step doesn't re-read recipe files or introduce new trust boundaries. The trust boundary is the same: "recipes from a PR branch are untrusted inputs that run on ephemeral CI runners."

**No issue.**

### 4. "User Data Exposure: Not Applicable" -- Correct

CI runners are ephemeral. No user data. Batching doesn't change the data profile. **No issue.**

---

## Attack Vectors Not Considered in the Design

### 5. GitHub Actions Expression Injection

**Existing risk, slightly changed surface.**

The current workflow has this pattern at `test-changed-recipes.yml:200`:

```yaml
run: ./tsuku install --force --recipe ${{ matrix.recipe.path }}
```

This is a **script injection vulnerability** if `matrix.recipe.path` contains shell metacharacters. The `path` value comes from `git diff` output, which is constructed from filenames. In the current codebase, recipe filenames are constrained to `recipes/{letter}/{name}.toml` and `internal/recipe/recipes/{name}.toml`, so the practical risk is minimal.

In the batched model, the design proposes an inner loop:

```bash
for recipe in $(echo "$RECIPES" | jq -r '.[].path'); do
  tool=$(echo "$recipe" | xargs basename -s .toml)
  ...
  if ! ./tsuku install --force --recipe "$recipe"; then
```

This is actually **slightly safer** because the recipe path is properly quoted inside the script (`"$recipe"`) rather than being interpolated directly by GitHub Actions' expression engine. The design's proposed pattern avoids the expression injection that exists in the current per-recipe model.

**Finding: Not a new risk. The batched model's inner-loop approach is marginally safer than the current `${{ matrix.recipe.path }}` direct interpolation pattern. No action needed for the batching design, but the existing `test-linux` job should eventually quote its matrix variables via environment variables.**

### 6. Batch Size Manipulation via `workflow_dispatch`

The design proposes `batch_size` as a `workflow_dispatch` input:

```yaml
BATCH_SIZE="${{ inputs.batch_size || 15 }}"
```

For `pull_request`-triggered runs, `inputs.batch_size` will be empty, so the default of 15 applies. For `workflow_dispatch`, any repository collaborator can set an arbitrary batch size.

**Risk:** Setting `batch_size=1` restores per-recipe behavior (no harm). Setting `batch_size=99999` puts all recipes in one batch, creating a very long-running job. This is a denial-of-service against CI resources, but only by someone with write access to the repo (workflow_dispatch requires write permissions).

**Finding: Acceptable risk.** The `batch_size` input should be validated (minimum 1, maximum ~50) via a shell guard in the detection step, but this is an operational concern, not a security concern. Repository collaborators already have far more destructive capabilities.

### 7. GITHUB_TOKEN Scope in Batched Context

The current per-recipe job exposes `GITHUB_TOKEN` to each recipe's install step (`test-changed-recipes.yml:198`). The macOS aggregated job also exposes it (`test-changed-recipes.yml:224`). The batched Linux model will do the same.

**Risk:** A malicious recipe could exfiltrate `GITHUB_TOKEN` during `tsuku install`. This token typically has `contents: read` and `metadata: read` for `pull_request`-triggered workflows.

**Finding: No change.** This risk is identical in both models. The token scope is already minimal for PR-triggered workflows. The design doesn't change token exposure.

### 8. Shared Download Cache as Attack Vector

The design proposes shared download caches between recipes in a batch (following the macOS pattern at `test-changed-recipes.yml:228-242`):

```bash
CACHE_DIR="${{ runner.temp }}/tsuku-cache/downloads"
ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads"
```

**Risk:** If recipe A downloads and caches a file, recipe B could theoretically use that cached file. However, the download cache (`internal/actions/download_cache.go`) uses the download URL as the cache key, and `download_file` action requires checksum verification (`download_file.go:51-54`). A cache poisoning attack would require a malicious recipe to:

1. Write a file to the cache directory with the exact filename that a later recipe's download would produce
2. Have the checksum match (impossible without knowing the expected checksum in advance)

**Finding: No issue.** The checksum verification in `download_file` prevents cache poisoning. The `download` action (composite) computes checksums at plan time, and plan-based execution (`--plan`) uses `download_file` which requires checksums. For direct `--recipe` execution (used in `test-changed-recipes.yml`), the `download` action downloads fresh and can optionally verify via `checksum_url`.

---

## Residual Risk Assessment

| Risk | Severity | Changed by Design? | Action |
|------|----------|---------------------|--------|
| Malicious recipe executes arbitrary commands on CI runner | High | No | Existing risk. Ephemeral runners mitigate. |
| Cross-recipe interference via filesystem (/tmp, PATH) | Low | Increased (1 recipe -> 15 recipes per runner) | Document in design that all batched recipes share a PR author. |
| Expression injection in workflow YAML | Low | Decreased (inner loop uses shell quoting) | No action for this design. |
| Long-running jobs via large batch_size | Low | New (workflow_dispatch only) | Add input validation (guard clause). |
| GITHUB_TOKEN exfiltration | Medium | No | Existing risk. Token scope is minimal. |

---

## "Not Applicable" Justification Audit

| Section | Justification | Verdict |
|---------|---------------|---------|
| Download Verification | "Changes how CI jobs are grouped, not how recipes are downloaded or verified" | **Correct.** Verified against `download.go`, `download_file.go`, `signature.go`. |
| Supply Chain Risks | "Doesn't change where recipes come from or how they're validated" | **Correct.** The `git diff` detection and recipe parsing are unchanged. |
| User Data Exposure | "CI workflows don't access user data" | **Correct.** Ephemeral GitHub-hosted runners. |

All three "not applicable" justifications are valid.

---

## Recommendations

1. **Add explicit rationale for cross-recipe interference acceptance.** The design acknowledges the risk but doesn't explain why it's acceptable. Add: "All recipes in a batch originate from the same PR, so a malicious contributor already controls all recipes being tested. Cross-recipe interference within a batch doesn't grant additional capabilities."

2. **Validate batch_size input.** In the detection step, clamp the value to a reasonable range (e.g., 1-50) to prevent accidental or intentional creation of extremely large batches via `workflow_dispatch`.

3. **Note the expression injection improvement.** The design's inner-loop pattern (`"$recipe"` with shell quoting) is safer than the existing direct `${{ matrix.recipe.path }}` interpolation. This is a minor security improvement that should be called out as a positive consequence.

---

## Summary

The design's security analysis is accurate in its "not applicable" assessments and correct in identifying `TSUKU_HOME` isolation as the primary concern. The one gap is that cross-recipe interference risk is acknowledged but not explained as acceptable. There are no unaddressed attack vectors that would change the security posture of CI relative to the current model.

The design does not introduce new trust boundaries, does not change recipe parsing or execution logic, and does not modify secret handling. The residual risks are either pre-existing (malicious recipes on ephemeral runners) or low-impact (batch size manipulation by repository collaborators).

**Verdict: No blocking security issues. Two advisory recommendations.**

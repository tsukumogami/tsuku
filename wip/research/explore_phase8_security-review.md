# Security Review: DESIGN-structured-error-subcategories

**Reviewer role**: Maintainer reviewer (security focus)
**Date**: 2026-02-21
**Design**: `docs/designs/DESIGN-structured-error-subcategories.md`

## Executive Summary

The design's security section dismisses all four categories as "not applicable." Three of those dismissals hold up. One does not: the **Supply Chain Risks** assessment misses that the new `subcategory` field is populated from error classification logic and then consumed by shell scripts in CI pipelines. While the risk is low severity, it deserves explicit treatment rather than a blanket "not applicable."

The design also has an unacknowledged output-contract risk: the `--json` output is already parsed by `jq` in CI workflows and by the Go-based orchestrator. Adding a field that downstream consumers will key on introduces a subtle trust boundary that the security section should address.

## Detailed Findings

### Finding 1: Subcategory Values Flow Into Shell Context (Low Severity)

**"Not applicable" claim**: "No new external dependencies or data sources are introduced."

**Why it's partially wrong**: The claim is true in isolation -- the subcategory is derived from `errors.As()` typed checks, not from external input. But the claim elides an important downstream fact. The subcategory string will be written to JSONL files that are:

1. Committed to the repository via CI (`data/failures/*.jsonl`)
2. Parsed by `jq` in GitHub Actions workflows (`.github/workflows/batch-generate.yml:886-896`)
3. Used by `scripts/requeue-unblocked.sh` which constructs file paths from failure record fields

Today the `category` field (which follows the same path) uses a closed set of hardcoded strings in `categoryFromExitCode()`. The subcategory taxonomy in the design also uses a closed set. The risk is that a future developer adds a subcategory derived from user-controlled input (e.g., a recipe name or error message text) without realizing the value ends up in shell-parsed JSON.

**Current mitigation already in place**: `isValidDependencyName()` in `internal/batch/orchestrator.go:545` validates `blocked_by` values against path traversal. But no equivalent validation exists for category or subcategory strings.

**Recommendation**: The design should:
- State that subcategory values MUST come from a closed enumeration (not derived from error message text or user input)
- Note that downstream consumers (CI scripts, `requeue-unblocked.sh`) parse these fields with `jq` and the values should be treated as an API contract
- This is not a blocking issue for the current design since all proposed values are hardcoded strings, but the security section should acknowledge it explicitly rather than saying "not applicable"

### Finding 2: Error Message Leakage Via Subcategory Mismatch (Informational)

The design proposes that `classifyInstallError()` returns `(exitCode, subcategory)` using `errors.As()`. The current `classifyInstallError()` at `cmd/tsuku/install.go:300` does not handle `ExitVerifyFailed` (7), `ExitVersionNotFound` (4), or `ExitDeterministicFailed` (9). These exit codes are assigned elsewhere:

- `ExitVerifyFailed` is used in `cmd/tsuku/verify.go:1031,1054`
- `ExitVersionNotFound` is defined in `exitcodes.go:23` but never appears in `classifyInstallError()`

The design's subcategory taxonomy (lines 159-172) maps these exit codes to subcategories (`verify_failed`, `verify_pattern_mismatch`, `version_not_found`, `deterministic_failed`). But `classifyInstallError()` would return `ExitInstallFailed` (6) as the catch-all for errors that don't match its type checks. This means:

- The subcategory for a verify failure might say `verify_failed` while the exit code says `6`
- Or the implementation would need to refactor verify/version/deterministic error handling to go through `classifyInstallError()`

This is not a security issue per se, but the mismatch between the taxonomy table and the actual error flow means the implementation could produce inconsistent (category, subcategory, exit_code) tuples. If downstream consumers use subcategory for retry/skip logic (as the CI workflow does with exit codes today), an incorrect subcategory could cause a recipe to be retried when it shouldn't be, or skipped when it should be retried.

**Recommendation**: The design should document which error paths actually flow through `handleInstallError()` -> `classifyInstallError()` and which bypass it. The verify and deterministic paths appear to call `exitWithCode()` directly, so they won't get subcategories unless the implementation changes their error flow.

### Finding 3: Divergent `categoryFromExitCode()` Functions (Confirmed Acceptable)

Two `categoryFromExitCode()` functions exist:
- `cmd/tsuku/install.go:339` -- user-facing categories
- `internal/batch/orchestrator.go:483` -- pipeline categories

Both files have extensive cross-referencing comments explaining the intentional divergence. The design proposes adding subcategories to the CLI's JSON output, which the orchestrator's `parseInstallJSON()` will extract. This is fine: the subcategory travels through the structured JSON field, not through exit code re-mapping.

The security concern would be if the orchestrator started deriving subcategories from its own `categoryFromExitCode()` (which maps differently). The design correctly avoids this -- the orchestrator extracts the subcategory from the CLI's JSON output for the `validate()` path, and from bracketed tags for the `generate()` path.

**No action needed.**

### Finding 4: The "Execution Isolation" Dismissal Holds

The design says execution isolation is "not applicable" because the subcategory is just a classification label. Confirmed: the subcategory value is never used to make execution decisions within the CLI process. It's written to stdout as JSON and consumed downstream. No privilege escalation or isolation boundary is affected.

### Finding 5: The "Download Verification" Dismissal Holds

Confirmed: the subcategory field doesn't change the download, checksum verification, or extraction pipeline. It's purely error classification metadata.

### Finding 6: The "User Data Exposure" Assessment Is Adequate

The design correctly notes that subcategory values are classification labels, not user data. The `message` field (which can contain paths) is unchanged. One nuance: the design says the subcategory "contains the same kind of information already present in `category` and `exit_code`." This is true for the proposed taxonomy. It would stop being true if someone added a subcategory like `missing_dep:<dependency-name>` that embeds user-controlled data. But the current design doesn't propose this.

### Finding 7: Backward Compatibility Creates a Trust Transition Window

During the fallback period (old records use heuristic parsing, new records use structured field), the dashboard shows a mix of authoritative and heuristic subcategories. The design acknowledges this.

The security angle: if an attacker could craft error messages with specific bracketed tags (e.g., inject `[no_bottles]` into a recipe that actually failed for a different reason), the heuristic parser would misclassify the failure. The structured field from the CLI is immune to this because it uses typed error checks, not text parsing.

**However**: who controls the error message text? For the `generate()` path, the message comes from `tsuku create` stdout/stderr -- which in turn may include output from upstream APIs (Homebrew, GitHub, etc.). An upstream API that returned a response containing `[no_bottles]` would be misclassified by the heuristic parser.

This is an existing vulnerability, not introduced by this design. The design actually mitigates it by preferring structured fields. But the security section should acknowledge that the transition period preserves this existing risk for old records.

## Attack Vectors Not Considered

### 1. Subcategory Injection via Upstream API Responses

**Vector**: A compromised or malicious upstream (e.g., a GitHub release with crafted release notes, or a Homebrew formula with a manipulated description) includes text like `[no_bottles]` in output that gets captured as an error message.

**Current impact**: The heuristic `extractSubcategory()` uses bracketed tags as highest-priority classification. A crafted `[api_error]` tag in error output could change how the dashboard displays and how retry logic behaves.

**Post-design impact**: Reduced. New records use the structured CLI field, which is immune to this. Old records in the transition period remain vulnerable.

**Severity**: Low. The dashboard is an internal operator tool. Misclassification affects visibility, not execution. The retry logic in CI workflows uses exit codes, not subcategories.

### 2. JSONL Record Injection

**Vector**: If an attacker could commit a crafted JSONL file to `data/failures/`, they could inject arbitrary subcategory/category values that appear in the dashboard.

**Current protection**: JSONL files are written by CI workflows and committed via automated PRs. Write access requires GitHub repo permissions.

**Post-design change**: No change. The subcategory field is just another string in the JSONL record.

**Severity**: Not a concern unless repo access is compromised, at which point there are bigger problems.

### 3. JSON Output Consumed by Future External Tools

The `--json` flag is documented as a CLI feature. If users or third-party tools start parsing `tsuku install --json` output, the subcategory field becomes part of a public API contract. The `omitempty` tag means the field silently disappears when empty, which is the right default. But the design should note that subcategory values, once shipped, become a stability contract.

## Residual Risks to Escalate

**None require escalation.** The risks identified are low-severity and either:
- Already present before this design (heuristic parsing vulnerability)
- Actively mitigated by this design (structured field replaces heuristic)
- Adequately handled by existing controls (JSONL write access, closed-set values)

## Recommendations for the Design Document

1. **Change "Supply Chain Risks: Not applicable" to a brief acknowledgment**: "Subcategory values are drawn from a closed enumeration of hardcoded strings. They must not be derived from error message text or user-controlled input, because downstream consumers (CI workflows, shell scripts) parse these values from JSONL records."

2. **Add a note under Consequences > Negative**: "The `subcategory` field in `--json` output becomes part of the CLI's machine-readable API. Adding, renaming, or removing subcategory values is a contract change that affects the orchestrator's `parseInstallJSON()` and CI workflow `jq` filters."

3. **Document the error flow gap**: Not all exit codes flow through `classifyInstallError()`. The verify path (`ExitVerifyFailed`) and version-not-found path (`ExitVersionNotFound`) appear to call `exitWithCode()` directly. The implementation plan should specify whether these paths will be refactored to go through `handleInstallError()` or whether they'll set subcategories through a different mechanism.

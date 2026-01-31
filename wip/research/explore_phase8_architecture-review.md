# Architecture and Security Review: DESIGN-batch-recipe-generation

## Architecture Review

### 1. Is the architecture clear enough to implement?

**Mostly yes, with gaps.** The four-job structure (preflight, generate, validate-macos, merge) is well-defined. The pseudocode, YAML snippets, and data flow diagram give implementers a clear picture. However, several details are underspecified:

- **Artifact passing between jobs.** The design doesn't specify how generated recipe files and failure records move between jobs. GitHub Actions requires explicit artifact upload/download steps. The generate jobs produce files that the merge job consumes, but the mechanism (workflow artifacts, output variables, shared filesystem) is unstated. This is a blocking ambiguity for implementers.

- **`tsuku create --from <eco>:<pkg> --deterministic` doesn't exist yet.** The design references this CLI command in the data flow but doesn't mention it as new work. If the existing builder API is invoked through Go code in a shell script, the design should say so. If a new CLI flag is needed, it belongs in the implementation phases.

- **Concurrency within a generation job.** The design shows sequential per-package generation with sleep-based rate limiting. For a batch of 25 packages at 1 req/sec, that's 25+ seconds minimum per ecosystem -- reasonable. But it's unclear whether the generation loop is in Go code or a shell script wrapping CLI calls. The pseudocode is Go, but the implementation files reference `scripts/batch-generate.sh`.

- **Priority queue consumption.** The preflight job reads `data/priority-queue.json` but the design doesn't specify how consumed packages are marked (removed from queue, marked as processed, etc.). Without this, re-runs process the same packages.

### 2. Are there missing components or interfaces?

**Yes, three notable ones:**

1. **Queue consumption tracking.** No mechanism prevents re-processing packages already attempted. The design needs a "processed packages" list or queue pointer that advances after each batch.

2. **`run_command` detection logic.** The merge job gates auto-merge on `run_command` absence, but the detection mechanism isn't specified. Is it grep on the TOML? A `tsuku validate` flag? Recipe parsing in a shell script? The existing `run_command.go` action exists, so detection via `tsuku validate` with a `--no-run-command` flag would be clean, but this needs to be stated.

3. **batch-control.json write safety.** The merge job updates `batch-control.json` with circuit breaker state and budget metrics, and also commits failure JSONL files. If two batch runs overlap (e.g., operator triggers cargo and npm simultaneously), both merge jobs write to `batch-control.json` and could conflict. The design mentions per-ecosystem JSONL files to avoid conflicts there, but `batch-control.json` is shared.

### 3. Are the implementation phases correctly sequenced?

**Phase sequencing has a dependency issue.** Phase 1 (workflow + generation) and Phase 2 (Linux validation) are presented as separate phases, but the generation flow in the pseudocode already includes validation as part of the success path. A recipe that generates but isn't validated has no value -- you can't merge it.

Better sequencing:
- Phase 1: Workflow + generation + Linux validation (minimum viable pipeline)
- Phase 2: Merge automation with `run_command` gate
- Phase 3: macOS progressive validation
- Phase 4: Metrics and circuit breaker

This front-loads the end-to-end path (generate -> validate -> merge) so the pipeline produces value from Phase 2 onward.

### 4. Are there simpler alternatives we overlooked?

**The design is already fairly lean.** One simplification worth considering:

- **Skip the preflight job.** The preflight job reads `batch-control.json` and the priority queue, then outputs package lists. This could be the first step of each generate job instead of a separate job. It eliminates one job boundary and the artifact-passing complexity for package lists. The circuit breaker check can be a conditional step in each ecosystem's matrix entry.

- **Use GitHub Actions matrix for per-package validation** instead of a loop inside the generate job. This gives per-package visibility in the Actions UI and automatic parallelism. The rate limiting concern is real, but GitHub's `max-parallel` on the matrix handles it. However, this creates many matrix entries (25 per ecosystem), so the current loop approach may be more practical.

## Security Review

### 1. Are there attack vectors we haven't considered?

**Yes, two significant ones:**

**A. Recipe content injection beyond `run_command`.** The design gates auto-merge solely on `run_command` absence. But recipes contain other fields that could be abused:
- `download.url` pointing to a malicious binary (the checksum is verified against the registry, but if the registry is compromised, both URL and checksum are attacker-controlled)
- `extract.strip_prefix` or `install_binaries.rename` with path traversal values (e.g., `../../bin/sudo`)
- Template variables or string interpolation in recipe fields that could inject shell commands

The sandbox catches many of these at validation time, but the auto-merge gate should also validate that URLs point to expected registry domains, and that paths don't contain traversal sequences.

**B. GITHUB_TOKEN permission scope.** The merge job creates a PR and auto-merges it. This requires `contents: write` and potentially `pull-requests: write` permissions. The design doesn't specify the workflow's permission block. If the workflow inherits default repo permissions (which may include `actions: write`), a compromised generation step could modify other workflows. The workflow should declare minimal permissions explicitly.

**C. Concurrent batch manipulation.** If an attacker can trigger `workflow_dispatch` (anyone with write access), they could launch many simultaneous batches to overwhelm the circuit breaker tracking, cause `batch-control.json` merge conflicts that skip safety checks, or exhaust CI budgets. The design should specify who can trigger the workflow (e.g., restrict to maintainers via environment protection rules).

### 2. Are the mitigations sufficient for the risks identified?

**Partially.** The mitigations are appropriate for the risks they address, but coverage is incomplete:

- **`run_command` gate: Sufficient** for its stated purpose. The existing `RunCommandAction.RequiresNetwork()` returning `true` means sandbox runs with `run_command` need `network=host`, which is a weaker isolation boundary. The gate correctly forces human review.

- **Sandbox with `--network=none`: Insufficient alone.** The sandbox uses `--network=none` only when `RequiresNetwork` is false. Recipes with actions that set `RequiresNetwork=true` (including `run_command`, `npm_install`, `cargo_build`) run with network access in the sandbox. The design says validation uses `--network=none` but this contradicts the actual sandbox behavior for recipes requiring network. The design should clarify which recipe types are expected in batch generation -- if they're all binary downloads (deterministic mode), they shouldn't need network during validation. This assumption should be stated explicitly.

- **Checksum verification: Necessary but not sufficient.** Checksums protect against download corruption and MITM, but not against a compromised registry serving matching hash + payload. This is acknowledged as residual risk (zero-day compromise) which is appropriate -- there's no practical mitigation beyond the registry's own security.

- **Circuit breaker: Sufficient for availability, insufficient for security.** The circuit breaker detects high failure rates (>50% over window) but a targeted attack might introduce 1-2 malicious recipes per batch, staying well under the circuit breaker threshold. The circuit breaker is a reliability mechanism, not a security one. The design correctly doesn't position it as a security control, but the risk table lists "batch poisoning" with circuit breaker as the mitigation, which overstates its security value.

### 3. Is there residual risk we should escalate?

**One item warrants escalation:**

**Auto-merge of binary-downloading recipes without human review.** Even without `run_command`, a recipe that downloads and installs a binary is executing a supply chain action. If the upstream registry is compromised (or a popular package is taken over), the auto-merge pipeline becomes an automated distribution channel for malicious binaries. The time from upstream compromise to user installation could be as short as: batch trigger -> generation -> validation -> auto-merge -> user update.

This is the fundamental tension of the system: automation enables scale but removes human judgment from the supply chain. The design's `run_command` gate addresses the most obvious vector but doesn't address compromised-but-otherwise-normal binaries.

**Recommendation:** Consider a "quarantine" period where auto-merged recipes are merged to a staging branch and promoted to main after a delay (e.g., 24 hours). This gives time for upstream compromises to be detected. Alternatively, recipes for tier-1 (critical) packages should always require human review regardless of `run_command` status.

### 4. Are any "not applicable" justifications actually applicable?

The design doesn't explicitly mark anything as N/A. However, there's an implicit assumption worth challenging:

**"No additional verification is needed because the pipeline reuses existing builder download paths."** This is stated under Download Verification. While true that the builders verify checksums, the batch pipeline changes the trust model: individual interactive installs have a human in the loop who chose to install a specific package. Batch generation removes that human decision. The verification is technically the same, but the risk profile is different because the attack surface (number of packages processed without human review) is much larger. The existing verification is necessary but the justification for not adding more should acknowledge this changed risk profile.

## Summary of Recommendations

### Architecture (must-fix before implementation)

1. **Specify artifact passing between jobs** -- how recipe files and failure records move from generate to merge jobs.
2. **Define queue consumption tracking** -- how processed packages are marked to prevent re-processing.
3. **Resequence phases** to deliver end-to-end value sooner (merge generation + Linux validation into Phase 1).
4. **Clarify the generation invocation** -- is it a new CLI command, a Go binary, or shell script wrapping existing commands?

### Architecture (should-fix)

5. **Specify `batch-control.json` write concurrency handling** for overlapping batch runs.
6. **Consider merging preflight into generate jobs** to reduce job boundary complexity.

### Security (must-fix before implementation)

7. **Declare explicit minimal workflow permissions** (`contents: write`, `pull-requests: write`, nothing else).
8. **Restrict workflow trigger access** via environment protection rules or branch protection.
9. **Add URL domain allowlist check** to the auto-merge gate (recipes should only download from known registry domains).
10. **Clarify sandbox network mode** for batch-generated recipes -- if deterministic mode means binary-only downloads, state that validation always uses `--network=none` and any recipe requiring network fails the gate.

### Security (should-fix)

11. **Add path traversal validation** to the auto-merge gate for `strip_prefix`, `rename`, and similar fields.
12. **Reframe the circuit breaker** in the risk table as a reliability control, not a security mitigation for batch poisoning.
13. **Consider a quarantine period or staging branch** for auto-merged recipes, especially tier-1 packages.
14. **Acknowledge the changed risk profile** in Download Verification -- batch processing without human review is different from interactive installation.

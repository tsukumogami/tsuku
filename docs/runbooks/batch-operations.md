# Batch Operations Runbook

Operational procedures for the batch recipe generation pipeline. Each section follows a standard structure: Decision Tree, Investigation Steps, Resolution, and Escalation.

## Incident Severity Classification

| Severity | Criteria | Response Time | Example |
|----------|----------|---------------|---------|
| Critical | Security compromise suspected | < 1 hour | Checksum drift, supply chain attack |
| High | Batch affecting >50% recipes | < 4 hours | Validation pipeline bug |
| Medium | Single ecosystem degraded | < 24 hours | Upstream API outage |
| Low | Transient failures | Next batch | Network timeout |

---

## 1. Batch Success Rate Drop

### Decision Tree

- Single ecosystem affected: check ecosystem-specific issues first
- All ecosystems affected: check infrastructure (GitHub Actions, network)
- During deployment window: likely a deployment issue
- Circuit breaker already tripped: automatic recovery in progress, monitor

### Investigation Steps

1. Check the pipeline dashboard health panel at `/pipeline/` for a quick overview. It shows circuit breaker state per ecosystem, last run timestamps, runs since last success, and a warning if no batch has run in >2 hours. This is the fastest way to assess the situation before deeper investigation.

2. Check which ecosystems have failures:

   ```
   jq '.circuit_breaker | to_entries | map(select(.value.failures > 0))' batch-control.json
   ```

   Expected output (healthy):
   ```json
   []
   ```

   Expected output (failures present):
   ```json
   [
     {
       "key": "homebrew",
       "value": {
         "state": "closed",
         "failures": 2,
         "last_failure": "2026-01-28T03:15:00Z"
       }
     }
   ]
   ```

3. Check circuit breaker state for a specific ecosystem:

   ```
   ./scripts/check_breaker.sh homebrew
   ```

   Expected output (proceeding):
   ```
   state=closed
   skip=false
   ```

   Expected output (tripped):
   ```
   state=open
   skip=true
   ```

4. Review recent workflow runs:

   ```
   gh run list --workflow=batch-operations.yml --limit=5
   ```

5. If network errors dominate, check upstream API status pages.

6. If validation errors dominate, check recent CI changes:

   ```
   git log --oneline -10 -- .github/workflows/
   ```

### Resolution

- **Transient failures**: Wait for circuit breaker auto-recovery. The breaker transitions from `open` to `half-open` after its timeout period, then back to `closed` on the next success.
- **Persistent failures**: Disable the affected ecosystem via the control file (see Emergency Stop below) and investigate root cause.

### Escalation

- If >50% recipes failing across multiple ecosystems: **High severity**. Disable batch processing entirely and investigate.
- If security-related failure detected: **Critical severity**. Escalate immediately (see Security Incident section).

---

## 2. Emergency Stop

### Decision Tree

- Single ecosystem problematic: disable that ecosystem only
- All ecosystems problematic: disable batch processing entirely
- Security incident: disable entirely and follow Security Incident procedure

### Investigation Steps

1. Confirm current batch processing state:

   ```
   jq '{enabled: .enabled, disabled_ecosystems: .disabled_ecosystems}' batch-control.json
   ```

   Expected output (all enabled):
   ```json
   {
     "enabled": true,
     "disabled_ecosystems": []
   }
   ```

### Resolution

**Disable a single ecosystem:**

```bash
jq '.disabled_ecosystems += ["homebrew"]' batch-control.json > tmp.json && mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore: disable homebrew batch processing

reason: <description of issue>
incident_url: <link to issue or alert>"
git push
```

**Disable all batch processing:**

```bash
jq '.enabled = false |
    .reason = "Emergency stop: <description>" |
    .disabled_by = "<your-github-username>" |
    .disabled_at = (now | todate) |
    .expected_resume = "<expected resume date>"' batch-control.json > tmp.json && mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore: emergency stop batch processing

reason: <description of issue>
incident_url: <link to issue or alert>"
git push
```

**Re-enable after resolution:**

```bash
jq '.enabled = true |
    .reason = "" |
    .disabled_ecosystems = [] |
    .disabled_by = "" |
    .disabled_at = "" |
    .expected_resume = ""' batch-control.json > tmp.json && mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore: re-enable batch processing

reason: <resolution summary>"
git push
```

### Escalation

- If the issue requires disabling all ecosystems for >24 hours, open a tracking issue.
- If the root cause is unclear after initial investigation, escalate to a second operator.

---

## 3. Batch Rollback

### Decision Tree

- Known batch_id with problematic recipes: execute rollback directly
- Unknown batch_id: find it from recent git history first
- Partial rollback needed: manually select files instead of using the script

### Investigation Steps

1. Find recent batch commits:

   ```
   git log --all --oneline --grep="batch_id:"
   ```

   Expected output:
   ```
   a1b2c3d chore: add homebrew recipes batch_id: 2026-01-28-001
   e4f5g6h chore: add cargo recipes batch_id: 2026-01-28-001
   ```

2. Identify the batch_id to roll back. Confirm which files are affected:

   ```
   git log --all --name-only --grep="batch_id: 2026-01-28-001" --format="" | grep '^recipes/' | sort -u
   ```

   Expected output:
   ```
   recipes/homebrew/cmake.toml
   recipes/homebrew/ninja.toml
   ```

### Resolution

**Execute the rollback:**

```bash
./scripts/rollback-batch.sh 2026-01-28-001
```

Expected output:
```
Finding recipes from batch 2026-01-28-001...
Found 2 recipes to rollback:
recipes/homebrew/cmake.toml
recipes/homebrew/ninja.toml
Rollback branch created: rollback-batch-2026-01-28-001
Review changes with: git diff main...rollback-batch-2026-01-28-001
Create PR with: gh pr create --title 'Rollback batch 2026-01-28-001'
```

After the script runs, review and create a PR:

```bash
git diff main...rollback-batch-2026-01-28-001
gh pr create --title "Rollback batch 2026-01-28-001" --body "Rollback of batch 2026-01-28-001 due to <reason>"
```

### Escalation

- If rollback affects >10 recipes, have a second operator review the PR before merge.
- If rollback does not resolve the issue, escalate: the problem may not be recipe-related.

---

## 4. Budget Threshold Alert

### Decision Tree

- Usage at 80%: reduce batch size and enable sampling
- Usage at 90%+: stop non-critical batches, preserve budget for critical ecosystems
- Budget already exhausted: wait for weekly reset

### Investigation Steps

1. Check current budget usage:

   ```
   jq '.budget' batch-control.json
   ```

   Expected output:
   ```json
   {
     "macos_minutes_used": 450,
     "linux_minutes_used": 1200,
     "week_start": "2026-01-27",
     "sampling_active": false
   }
   ```

2. Check when the budget week resets (resets weekly from `week_start`).

3. Review which ecosystems consume the most minutes by checking recent workflow run durations:

   ```
   gh run list --workflow=batch-operations.yml --limit=10 --json databaseId,conclusion,updatedAt
   ```

### Resolution

**At 80% threshold -- enable sampling:**

```bash
jq '.budget.sampling_active = true' batch-control.json > tmp.json && mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore: enable budget sampling at 80% threshold"
git push
```

**At 90%+ threshold -- disable non-critical ecosystems:**

```bash
jq '.disabled_ecosystems += ["<low-priority-ecosystem>"] |
    .reason = "Budget threshold exceeded"' batch-control.json > tmp.json && mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore: disable non-critical ecosystems for budget control"
git push
```

**After weekly reset:**

Re-enable ecosystems and disable sampling:

```bash
jq '.budget.sampling_active = false |
    .budget.macos_minutes_used = 0 |
    .budget.linux_minutes_used = 0 |
    .budget.week_start = (now | todate | split("T")[0]) |
    .disabled_ecosystems = [] |
    .reason = ""' batch-control.json > tmp.json && mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore: reset budget for new week"
git push
```

### Escalation

- If budget is repeatedly exhausted before week end, open an issue to review batch frequency or CI runner allocation.
- If a single ecosystem consumes >50% of total budget, consider reducing its batch frequency.

---

## 5. Security Incident

### Decision Tree

- Checksum drift detected (file changed without version bump): **Critical**. Stop immediately.
- Unexpected recipe source URL: **Critical**. Stop and investigate supply chain.
- Validation failure with unusual pattern: **High**. Investigate before continuing.

### Investigation Steps

1. **Immediately disable all batch processing** (see Emergency Stop, "Disable all" procedure).

2. Identify affected recipes. If a checksum drift monitoring workflow flagged the issue:

   ```
   gh run view <run-id> --log
   ```

3. Check git history for unexpected modifications:

   ```
   git log --all --oneline --diff-filter=M -- recipes/
   ```

   Expected output (normal -- only batch commits):
   ```
   a1b2c3d chore: add homebrew recipes batch_id: 2026-01-28-001
   ```

   Suspicious output (manual or unauthorized changes):
   ```
   z9y8x7w Update cmake.toml
   ```

4. Verify checksums for affected recipes by re-downloading and comparing:

   ```
   # For a specific recipe, download the artifact and compare checksum
   curl -sL "<url-from-recipe>" | sha256sum
   ```

5. Check if the problematic change was introduced through a PR or direct push:

   ```
   git log --all --format="%H %ae %s" -- recipes/<affected-file>
   ```

### Resolution

1. If supply chain compromise confirmed:
   - Roll back all recipes from the affected batch (see Batch Rollback)
   - Audit all recipes added in the same time window
   - Rotate any secrets that may have been exposed

2. If checksum drift is benign (upstream re-published artifact):
   - Update the recipe with the new checksum
   - Document the drift in the commit message
   - Re-enable batch processing

### Escalation

- **All security incidents are Critical severity** until proven otherwise.
- If supply chain compromise is confirmed, notify all downstream users.
- Open a security advisory if published recipes contained compromised artifacts.

---

## 6. Seeding Pipeline Operations

### Decision Tree

- Weekly run succeeded (exit 0): no action needed
- Partial failure (exit 2): check `sources_failed` and `errors` in the summary
- Fatal failure (exit 1): check workflow logs for queue read/write errors
- Source change issues created: review and close after resolution

### Source Change Review

The weekly `seed-queue.yml` workflow creates GitHub issues with the `seeding:review` label when re-disambiguation selects a different source for priority 1-2 packages. These are not auto-accepted because changing the source for high-priority tools requires human verification.

1. List open source change issues:

   ```bash
   gh issue list --label seeding:review --state open
   ```

2. For each issue, verify the proposed source change makes sense. The issue body includes the package name, old source, new source, and priority level.

3. To accept a source change, update the queue entry's source and reset its failure state:

   ```bash
   # Find the entry and update its source
   jq '(.entries[] | select(.name == "<package>")).source = "<new-source>"' \
     data/queues/priority-queue.json > tmp.json && mv tmp.json data/queues/priority-queue.json
   ```

4. Close the issue after applying or rejecting the change.

### Failure Investigation

When the seeding workflow exits with code 2 (partial failure), check the summary:

```bash
# Download the summary artifact from the workflow run
gh run download <run-id> -n seeding-summary

# Check which sources failed
jq '.sources_failed' seeding-summary.json

# Check error details
jq '.errors' seeding-summary.json
```

Common causes:
- **Ecosystem API outage**: The affected ecosystem's API returned errors. The other ecosystems still processed successfully. No action needed unless the outage persists across multiple runs.
- **Rate limiting**: An ecosystem hit its rate limit. The command respects per-ecosystem rate limits internally but external throttling can still occur. Typically resolves on the next run.
- **Network timeout**: Transient connectivity issue. Check if the next scheduled run succeeds.

### Curated Source Validation

Entries with `confidence: "curated"` are never re-disambiguated, but the workflow validates that their sources still exist via HTTP HEAD requests. Invalid curated sources appear in the summary:

```bash
jq '.curated_invalid' seeding-summary.json
```

If a curated source is invalid (the upstream package was removed or renamed), update the queue entry manually:

```bash
# Check what the entry currently looks like
jq '.entries[] | select(.name == "<package>")' data/queues/priority-queue.json

# Either remove the entry or update its source
```

### Interpreting the Seeding Summary

The JSON summary written to stdout contains these fields:

| Field | Type | Description |
|-------|------|-------------|
| `sources_processed` | string[] | Ecosystems that completed successfully |
| `sources_failed` | string[] | Ecosystems that encountered errors |
| `new_packages` | number | New packages added to the queue |
| `stale_refreshed` | number | Stale entries that were re-disambiguated |
| `source_changes` | object[] | Entries where re-disambiguation selected a different source |
| `curated_skipped` | number | Curated entries that were skipped |
| `curated_invalid` | string[] | Curated entries with invalid upstream sources |
| `errors` | string[] | Error messages from failed operations |

Each `source_changes` entry contains: `package`, `old_source`, `new_source`, `priority`, and `auto_accepted` (true for priority 3, false for priority 1-2).

### Escalation

- If the same ecosystem fails across 3+ consecutive weekly runs, open an issue to investigate the upstream API.
- If `curated_invalid` grows beyond 10 entries, review whether the curation list needs a bulk update.

---

## 7. Bootstrap Phase B: Multi-Ecosystem Disambiguation

### Context

Bootstrap Phase B is a one-time local procedure that re-disambiguates all existing homebrew-sourced queue entries. After the initial seeding (Phase A, which populated the queue from Homebrew analytics), most entries have `homebrew:` sources because no other ecosystem was checked. Phase B runs full 8-ecosystem disambiguation for every entry, updating sources where a better option exists (e.g., `cargo:ripgrep` instead of `homebrew:ripgrep`).

This is the most expensive seeding operation (~5K entries, each requiring 8 ecosystem probes at ~2.5 seconds). It only runs once. After Phase B, the weekly seed-queue workflow handles incremental updates.

### Prerequisites

- Go toolchain installed (version matching `go.mod`)
- Network access to ecosystem APIs (crates.io, npm, PyPI, RubyGems, Homebrew, GitHub)
- The unified queue at `data/queues/priority-queue.json` is up to date

### Procedure

1. Build the seed-queue command:

   ```bash
   go build -o seed-queue ./cmd/seed-queue
   ```

2. Run disambiguation for all entries. Setting `-limit 0` skips new package discovery and `-freshness 0` forces re-disambiguation of every entry regardless of its `disambiguated_at` timestamp:

   ```bash
   ./seed-queue \
     -source homebrew \
     -limit 0 \
     -freshness 0 \
     -queue data/queues/priority-queue.json \
     -audit-dir data/disambiguations/audit \
     -verbose \
     > bootstrap-summary.json
   ```

   Expected runtime: 3-4 hours (5,137 homebrew entries at ~2.5 seconds per 8-ecosystem probe).

3. Review the summary output:

   ```bash
   # Total source changes
   jq '.source_changes | length' bootstrap-summary.json

   # Source changes for priority 1-2 packages (need manual review)
   jq '.source_changes[] | select(.priority <= 2)' bootstrap-summary.json

   # Auto-accepted changes (priority 3)
   jq '[.source_changes[] | select(.auto_accepted == true)] | length' bootstrap-summary.json

   # Any errors during the run
   jq '.errors' bootstrap-summary.json
   ```

4. Create a PR with the updated queue and audit files:

   ```bash
   git checkout -b bootstrap-phase-b
   git add data/queues/priority-queue.json data/disambiguations/audit/
   git commit -m "chore(batch): bootstrap Phase B disambiguation"
   gh pr create \
     --title "Bootstrap Phase B: multi-ecosystem disambiguation" \
     --body "Re-disambiguated all homebrew entries using ecosystem probers.

   See bootstrap-summary.json for full details."
   ```

### Expected Results

Of ~5,137 homebrew entries, approximately 200-400 should get better sources based on the 10x popularity threshold. Common outcomes:

- Rust CLI tools (ripgrep, fd, bat, eza, hyperfine) move from `homebrew:` to `cargo:` or `github:` sources
- Node.js CLI tools with dedicated GitHub releases move from `homebrew:` to `github:` sources
- The remaining ~4,700 entries stay as `homebrew:` because Homebrew is genuinely their best source

Priority 1-2 source changes are not auto-accepted. The summary JSON lists them under `source_changes` with `auto_accepted: false`. Review these individually before applying.

### Verification

After the PR merges, confirm the queue is valid:

```bash
jq '.entries | length' data/queues/priority-queue.json
jq '[.entries[] | .source | split(":")[0]] | group_by(.) | map({(.[0]): length}) | add' data/queues/priority-queue.json
```

The second command shows the source distribution. After Phase B, the `homebrew` count should decrease and `cargo`, `github`, `npm`, and other sources should increase.

### Troubleshooting

- **Rate limiting (429 errors)**: The seed-queue command handles per-ecosystem rate limits internally. If you see repeated 429 errors, wait 30 minutes and rerun. The command processes only entries that still need disambiguation (those without a fresh `disambiguated_at`).
- **Partial completion**: If the run is interrupted, rerun the same command. Entries that were already disambiguated (with a recent `disambiguated_at`) are skipped automatically unless `-freshness 0` is used again.
- **API outages**: Check the summary for `sources_failed` and `errors`. The command continues processing other ecosystems if one fails (exit code 2 = partial failure).

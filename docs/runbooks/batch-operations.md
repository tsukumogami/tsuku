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

1. Check which ecosystems have failures:

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

2. Check circuit breaker state for a specific ecosystem:

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

3. Review recent workflow runs:

   ```
   gh run list --workflow=batch-operations.yml --limit=5
   ```

4. If network errors dominate, check upstream API status pages.

5. If validation errors dominate, check recent CI changes:

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

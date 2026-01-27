# R2 Golden Storage Operational Runbook

This runbook documents operational procedures for the R2 golden storage system used by tsuku's CI validation.

**Design Reference**: [DESIGN-r2-golden-storage.md](designs/current/DESIGN-r2-golden-storage.md)

## Overview

R2 golden storage holds JSON plan files used to validate recipe installations. The system consists of:

- **Storage**: Cloudflare R2 bucket `tsuku-golden-registry`
- **Access**: Read-only and write tokens stored as GitHub Secrets
- **Monitoring**: Automated health, cost, and cleanup workflows

### Architecture

```
recipes/ (git) → CI generates plans → R2 storage → CI validates plans
                                          ↓
                              Monitoring workflows
                              (health, cost, cleanup)
```

## Ownership

| Role | Responsibility |
|------|----------------|
| tsuku maintainers | Credential rotation, incident response |
| Cloudflare account | dangazineu (primary) |
| GitHub repository | tsukumogami/tsuku |

## Credential Management

### Token Inventory

| Secret Name | Permission | Usage |
|-------------|------------|-------|
| `R2_BUCKET_URL` | N/A | Bucket endpoint URL |
| `R2_ACCESS_KEY_ID_READONLY` | Object Read | Validation workflows |
| `R2_SECRET_ACCESS_KEY_READONLY` | Object Read | Validation workflows |
| `R2_ACCESS_KEY_ID_WRITE` | Object Read & Write | Upload, cleanup workflows |
| `R2_SECRET_ACCESS_KEY_WRITE` | Object Read & Write | Upload, cleanup workflows |

All tokens are scoped to the `tsuku-golden-registry` bucket only, not account-wide.

### Rotation Schedule

Tokens rotate every 90 days (quarterly). Set calendar reminders for:
- January 1
- April 1
- July 1
- October 1

### Rotation Procedure

**Prerequisites**: Cloudflare dashboard access and GitHub admin access.

1. **Generate new tokens in Cloudflare**:
   ```
   Cloudflare Dashboard → R2 → Manage R2 API Tokens → Create API Token
   ```
   - Create one read-only token
   - Create one read-write token
   - Note both Access Key ID and Secret Access Key for each

2. **Update GitHub Secrets**:
   ```bash
   # Update read-only tokens
   gh secret set R2_ACCESS_KEY_ID_READONLY
   gh secret set R2_SECRET_ACCESS_KEY_READONLY

   # Update write tokens
   gh secret set R2_ACCESS_KEY_ID_WRITE
   gh secret set R2_SECRET_ACCESS_KEY_WRITE
   ```

3. **Verify new tokens work**:
   ```bash
   # Trigger health check to verify read access
   gh workflow run r2-health-monitor.yml

   # Wait for completion and check result
   gh run list --workflow=r2-health-monitor.yml --limit=1
   ```

4. **Verify write access** (optional, use with caution):
   ```bash
   # Trigger cleanup in dry-run mode
   gh workflow run r2-cleanup.yml -f dry_run=true
   ```

5. **Revoke old tokens** (only after verification):
   ```
   Cloudflare Dashboard → R2 → Manage R2 API Tokens → Delete old tokens
   ```

6. **Document rotation**:
   - Update rotation tracking issue or internal log
   - Note date and any issues encountered

### Rotation Tracking

**Workflow**: `.github/workflows/r2-credential-rotation-reminder.yml`
**Schedule**: Quarterly (Jan 1, Apr 1, Jul 1, Oct 1) at 9 AM UTC
**Issue Label**: `maintenance`

The rotation reminder workflow automatically creates a GitHub issue with a rotation checklist at the start of each quarter.

**Manual trigger** (for testing):
```bash
gh workflow run r2-credential-rotation-reminder.yml -f quarter="Q1 2026"
```

## Monitoring

### Health Monitoring

**Workflow**: `.github/workflows/r2-health-monitor.yml`
**Schedule**: Every 6 hours
**Issue Label**: `r2-degradation`

The health monitor checks R2 availability by making a HEAD request to `health/ping.json`.

**Thresholds**:
- Success: HTTP 200, latency < 2000ms
- Degraded: HTTP 200, latency >= 2000ms
- Failure: Timeout, error, or non-200 response

**When issues are created**:
- Failure or degraded status creates/updates issue with `r2-degradation` label
- Recovery adds comment and closes issue

**Manual trigger**:
```bash
gh workflow run r2-health-monitor.yml
```

### Cost Monitoring

**Workflow**: `.github/workflows/r2-cost-monitoring.yml`
**Schedule**: Weekly (Monday 6 AM UTC)
**Issue Label**: `r2-cost-alert`

The cost monitor calculates total storage by listing all objects.

**Thresholds**:
- Free tier limit: 10 GB
- Alert threshold: 80% (8 GB)

**When issues are created**:
- Storage exceeds 80% of 10 GB limit
- Issue includes usage breakdown and cleanup recommendations

**Manual trigger**:
```bash
gh workflow run r2-cost-monitoring.yml
```

## Troubleshooting

### Health Check Failures

**Symptoms**: `r2-degradation` issue created, workflows failing.

**Diagnosis**:
```bash
# Check recent health check runs
gh run list --workflow=r2-health-monitor.yml --limit=5

# View specific run logs
gh run view <run-id> --log

# Manual health check (requires credentials)
export R2_BUCKET_URL="..."
export R2_ACCESS_KEY_ID="..."
export R2_SECRET_ACCESS_KEY="..."
./scripts/r2-health-check.sh
```

**Common causes**:
1. **Cloudflare outage**: Check [Cloudflare Status](https://www.cloudflarestatus.com/)
2. **Credential expiration**: Verify tokens haven't been revoked
3. **Network issues**: Transient, usually self-resolves

**Resolution**:
- For Cloudflare outages: Wait for resolution, monitor status page
- For credential issues: Follow rotation procedure
- For persistent failures: Check Cloudflare dashboard for bucket status

### Upload/Download Failures

**Symptoms**: Post-merge workflow fails, validation can't fetch golden files.

**Diagnosis**:
```bash
# Check recent upload workflow runs
gh run list --workflow=publish-golden-to-r2.yml --limit=5

# Check for write permission issues
gh run view <run-id> --log | grep -i "error\|failed\|denied"
```

**Common causes**:
1. **Environment protection**: Write workflow needs `registry-write` approval
2. **Token permissions**: Write token may be read-only
3. **Bucket permissions**: Bucket policy may have changed

**Resolution**:
- Verify `registry-write` environment is configured correctly
- Verify write tokens have Object Read & Write permission
- Check Cloudflare bucket settings

### Checksum Mismatches

**Symptoms**: Validation fails with checksum error, corrupted file warnings.

**Diagnosis**:
```bash
# Run consistency check
./scripts/r2-consistency-check.sh
```

**Resolution**:
1. Re-run post-merge workflow to regenerate affected file
2. If persistent, quarantine and regenerate:
   ```bash
   gh workflow run r2-cleanup.yml -f dry_run=false
   gh workflow run publish-golden-to-r2.yml -f recipes=<affected-recipe>
   ```

### Manifest Inconsistencies

**Symptoms**: Files exist but aren't in manifest, or manifest references missing files.

**Diagnosis**:
```bash
./scripts/r2-consistency-check.sh --verbose
```

**Resolution**:
1. Regenerate manifest:
   ```bash
   gh workflow run publish-golden-to-r2.yml -f update_manifest=true
   ```
2. For orphaned files, run cleanup:
   ```bash
   gh workflow run r2-cleanup.yml -f dry_run=false
   ```

## Degradation Response

### When to Investigate vs Wait

| Situation | Action |
|-----------|--------|
| Single health check failure | Wait 6 hours for next check |
| Two consecutive failures | Check Cloudflare status |
| Cloudflare reports outage | Wait for resolution |
| No Cloudflare outage reported | Investigate credentials/config |
| Extended outage (> 24 hours) | Escalate |

### Manual Validation Trigger

If automated validation is blocked, trigger manually after R2 recovers:

```bash
# Trigger nightly validation
gh workflow run nightly-registry-validation.yml
```

### Escalation Path

1. **Cloudflare issues**: Contact Cloudflare support via dashboard
2. **GitHub Actions issues**: Check [GitHub Status](https://www.githubstatus.com/)
3. **Repository issues**: Open issue in tsukumogami/tsuku

## Maintenance

### Cleanup Workflow

**Workflow**: `.github/workflows/r2-cleanup.yml`
**Schedule**: Weekly (Sunday 4 AM UTC)

The cleanup workflow:
1. Detects orphaned files (deleted recipes)
2. Detects excess versions (retention policy: 2 per recipe/platform)
3. Soft deletes to `quarantine/{date}/` prefix
4. Hard deletes quarantined files older than 7 days

**Manual trigger (dry-run)**:
```bash
gh workflow run r2-cleanup.yml -f dry_run=true
```

**Manual trigger (execute)**:
```bash
gh workflow run r2-cleanup.yml -f dry_run=false
```

**Hard delete quarantined files**:
```bash
gh workflow run r2-cleanup.yml -f dry_run=false -f hard_delete=true
```

### Local Script Usage

All scripts require R2 credentials in environment:

```bash
export R2_BUCKET_URL="https://<account>.r2.cloudflarestorage.com"
export R2_ACCESS_KEY_ID="<key>"
export R2_SECRET_ACCESS_KEY="<secret>"
```

| Script | Purpose |
|--------|---------|
| `scripts/r2-health-check.sh` | Check bucket availability |
| `scripts/r2-upload.sh` | Upload golden files |
| `scripts/r2-download.sh` | Download golden files |
| `scripts/r2-orphan-detection.sh` | Find orphaned files |
| `scripts/r2-retention-check.sh` | Find excess versions |
| `scripts/r2-cleanup.sh` | Orchestrate cleanup |
| `scripts/r2-consistency-check.sh` | Verify manifest consistency |

## Environment Protection

### `registry-write` Environment

Write operations use the `registry-write` GitHub Environment which requires:
- **Branch restriction**: Only `main` branch
- **Reviewer approval**: One reviewer must approve deployment

This prevents accidental or malicious writes to R2.

### Approval Workflow

When a workflow needs write access:
1. Workflow pauses at environment deployment step
2. Designated reviewers receive notification
3. Reviewer approves in GitHub UI
4. Workflow continues with write tokens

### Emergency Access

For urgent situations requiring immediate write access:

1. **Temporary bypass** (not recommended):
   - Repository admin can temporarily disable environment protection
   - Re-enable immediately after emergency action
   - Document the bypass in an issue

2. **Direct Cloudflare access**:
   - Use Cloudflare dashboard to directly modify bucket
   - Only for critical recovery situations
   - Document all changes made

## Reference

### Workflows

| Workflow | Schedule | Purpose |
|----------|----------|---------|
| `r2-health-monitor.yml` | Every 6 hours | Health check |
| `r2-cost-monitoring.yml` | Weekly (Mon 6 AM) | Storage usage |
| `r2-cleanup.yml` | Weekly (Sun 4 AM) | Orphan/retention cleanup |
| `r2-credential-rotation-reminder.yml` | Quarterly | Rotation reminder |
| `publish-golden-to-r2.yml` | On merge | Upload golden files |

### Issue Labels

| Label | Created By | Purpose |
|-------|------------|---------|
| `r2-degradation` | r2-health-monitor.yml | Health issues |
| `r2-cost-alert` | r2-cost-monitoring.yml | Storage alerts |
| `automation` | r2-cleanup.yml | Cleanup reports |
| `maintenance` | r2-credential-rotation-reminder.yml | Rotation reminders |

### Useful Commands

```bash
# Check R2 health
gh workflow run r2-health-monitor.yml

# Check storage usage
gh workflow run r2-cost-monitoring.yml

# Dry-run cleanup
gh workflow run r2-cleanup.yml -f dry_run=true

# View recent workflow runs
gh run list --workflow=r2-health-monitor.yml --limit=5

# View open R2-related issues
gh issue list --label=r2-degradation --state=open
gh issue list --label=r2-cost-alert --state=open
```

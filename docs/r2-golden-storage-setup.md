# R2 Golden Storage Setup Guide

This guide documents the setup of Cloudflare R2 storage for registry golden files. Follow these steps to provision the infrastructure required for automated golden file storage.

## Prerequisites

- Cloudflare account with R2 enabled
- Admin access to the tsuku GitHub repository
- Access to repository secrets and environments settings

## Step 1: Create R2 Bucket

1. Log into the Cloudflare dashboard
2. Navigate to **R2 Object Storage** in the left sidebar
3. Click **Create bucket**
4. Configure the bucket:
   - **Bucket name**: `tsuku-golden-registry`
   - **Location**: Automatic (or select a region close to GitHub Actions runners)
5. Click **Create bucket**

### Verification

After creation, the bucket should appear in your R2 dashboard with 0 objects.

## Step 2: Create API Tokens

Create two API tokens with different permission levels.

### 2.1 Read-Only Token

1. In the Cloudflare dashboard, go to **R2 Object Storage**
2. Click on the `tsuku-golden-registry` bucket
3. Click **Settings** tab, then **Manage R2 API Tokens**
4. Click **Create API token**
5. Configure:
   - **Token name**: `tsuku-golden-readonly`
   - **Permissions**: Object Read only
   - **Specify bucket(s)**: Select `tsuku-golden-registry` only
   - **TTL**: No expiration (we'll rotate manually every 90 days)
6. Click **Create API Token**
7. **Important**: Copy and save the **Access Key ID** and **Secret Access Key** securely. These will not be shown again.

### 2.2 Read-Write Token

1. Click **Create API token** again
2. Configure:
   - **Token name**: `tsuku-golden-readwrite`
   - **Permissions**: Object Read & Write
   - **Specify bucket(s)**: Select `tsuku-golden-registry` only
   - **TTL**: No expiration (we'll rotate manually every 90 days)
3. Click **Create API Token**
4. **Important**: Copy and save the **Access Key ID** and **Secret Access Key** securely.

### Verification

You should now have two API tokens listed:
- `tsuku-golden-readonly` (Object Read)
- `tsuku-golden-readwrite` (Object Read & Write)

## Step 3: Note R2 Endpoint URL

The R2 endpoint URL follows this pattern:
```
https://<ACCOUNT_ID>.r2.cloudflarestorage.com
```

Find your Account ID:
1. In the Cloudflare dashboard, click on your domain or go to **Account Home**
2. The Account ID is shown on the right sidebar under "API"
3. Or navigate to **R2** and note the endpoint URL shown in bucket settings

## Step 4: Configure GitHub Secrets

Add the following secrets to the GitHub repository:

1. Go to **Settings** > **Secrets and variables** > **Actions**
2. Click **New repository secret** for each:

| Secret Name | Value |
|-------------|-------|
| `R2_ACCOUNT_ID` | Your Cloudflare account ID |
| `R2_ACCESS_KEY_ID_READONLY` | Access Key ID from read-only token |
| `R2_SECRET_ACCESS_KEY_READONLY` | Secret Access Key from read-only token |
| `R2_ACCESS_KEY_ID_WRITE` | Access Key ID from read-write token |
| `R2_SECRET_ACCESS_KEY_WRITE` | Secret Access Key from read-write token |

### Verification

All 5 secrets should appear in the repository secrets list.

## Step 5: Create Protected Environment

Create a GitHub Environment to protect write operations:

1. Go to **Settings** > **Environments**
2. Click **New environment**
3. Name it: `registry-write`
4. Configure protection rules:
   - **Required reviewers**: Add at least one reviewer (recommended: repository maintainers)
   - **Deployment branches**: Select "Selected branches" and add `main`
5. Click **Save protection rules**

### Verification

The `registry-write` environment should appear in the Environments list with protection rules configured.

## Step 6: Create Health Check Object

Create a sentinel object for health checks:

1. In the Cloudflare R2 dashboard, open the `tsuku-golden-registry` bucket
2. Click **Upload**
3. Create a file named `health/ping.json` with content:
   ```json
   {"status": "ok", "created": "2026-01-24"}
   ```
4. Upload the file

Alternatively, use the AWS CLI (with R2 credentials):
```bash
echo '{"status": "ok", "created": "'$(date -I)'"}' | \
  aws s3 cp - s3://tsuku-golden-registry/health/ping.json \
  --endpoint-url https://<ACCOUNT_ID>.r2.cloudflarestorage.com
```

### Verification

The file should be visible at path `health/ping.json` in the bucket.

## Step 7: Verify Complete Setup

Run the following verification steps:

### 7.1 Test Read Access

Using the read-only credentials:
```bash
export AWS_ACCESS_KEY_ID=<R2_ACCESS_KEY_ID_READONLY>
export AWS_SECRET_ACCESS_KEY=<R2_SECRET_ACCESS_KEY_READONLY>
export AWS_ENDPOINT_URL=https://<ACCOUNT_ID>.r2.cloudflarestorage.com

aws s3 ls s3://tsuku-golden-registry/
aws s3 cp s3://tsuku-golden-registry/health/ping.json -
```

Expected: Lists bucket contents and prints the ping.json content.

### 7.2 Test Write Access

Using the read-write credentials:
```bash
export AWS_ACCESS_KEY_ID=<R2_ACCESS_KEY_ID_WRITE>
export AWS_SECRET_ACCESS_KEY=<R2_SECRET_ACCESS_KEY_WRITE>
export AWS_ENDPOINT_URL=https://<ACCOUNT_ID>.r2.cloudflarestorage.com

echo '{"test": "write-verification"}' | \
  aws s3 cp - s3://tsuku-golden-registry/test/verification.json

aws s3 rm s3://tsuku-golden-registry/test/verification.json
```

Expected: File uploads and deletes successfully.

### 7.3 Verify GitHub Environment

1. Go to repository **Actions** tab
2. The `registry-write` environment should be available for workflow selection

## Credential Rotation Schedule

Tokens should be rotated every 90 days (quarterly).

### Rotation Procedure

1. Create a new API token with the same permissions
2. Update the GitHub secret with the new credentials
3. Verify workflows still function correctly
4. Revoke the old token in Cloudflare dashboard

### Rotation Tracking

Set a recurring reminder (calendar event or scheduled issue) for:
- **Next rotation**: 90 days from setup date
- **Repeat**: Every 90 days

## Troubleshooting

### "Access Denied" errors

- Verify the token has correct permissions (Object Read vs Object Read & Write)
- Confirm the token is scoped to the correct bucket
- Check that secrets are correctly named in GitHub

### "Bucket not found" errors

- Verify the bucket name is exactly `tsuku-golden-registry`
- Confirm the endpoint URL uses the correct account ID

### Environment protection not triggering

- Verify the workflow uses `environment: registry-write`
- Confirm the branch pushing the workflow is `main`
- Check that required reviewers are configured

## Summary

After completing this guide, you should have:

- [x] R2 bucket `tsuku-golden-registry` created
- [x] Read-only API token created and configured
- [x] Read-write API token created and configured
- [x] 5 GitHub secrets configured
- [x] `registry-write` environment with protection rules
- [x] Health check object `health/ping.json` uploaded
- [x] Read and write access verified
- [x] Rotation reminder set for 90 days

This infrastructure enables the CI workflows in subsequent issues (#1094+) to upload and download golden files from R2.

## Problem

The `Update Coverage Dashboard` workflow (`coverage-update.yml`) fails on every run with:

```
remote: Permission to tsukumogami/tsuku.git denied to github-actions[bot].
fatal: unable to access 'https://github.com/tsukumogami/tsuku/': The requested URL returned error: 403
```

It has never succeeded -- all 7 runs since the workflow was introduced (2026-02-10) have failed. The `website/coverage/coverage.json` file has never been auto-updated.

## Root Cause

The workflow has two problems compared to the working push workflows (`update-dashboard.yml`, `batch-generate.yml`, `sync-disambiguations.yml`):

### 1. Missing `permissions: contents: write`

The workflow has no top-level `permissions` block. With the repository's default restrictive token permissions, `GITHUB_TOKEN` has read-only access.

| Workflow | Has `permissions: contents: write`? |
|----------|-------------------------------------|
| `update-dashboard.yml` | Yes |
| `batch-generate.yml` | Yes |
| `sync-disambiguations.yml` | Yes |
| **`coverage-update.yml`** | **Missing** |

### 2. Uses `GITHUB_TOKEN` instead of a GitHub App token

The working workflows all generate a GitHub App token via `actions/create-github-app-token` and pass it to `actions/checkout`. The coverage workflow uses `secrets.GITHUB_TOKEN` instead.

| Workflow | Authentication |
|----------|---------------|
| `update-dashboard.yml` | GitHub App token |
| `batch-generate.yml` | GitHub App token |
| `sync-disambiguations.yml` | GitHub App token |
| **`coverage-update.yml`** | **`GITHUB_TOKEN`** |

## Affected File

`.github/workflows/coverage-update.yml` -- added in PR #1529 (2026-02-09), never modified since.

## Impact

Every push to `main` touching `recipes/**/*.toml` triggers this workflow and it fails. The coverage dashboard on the website shows stale data.

## Suggested Fix

Model the fix after `sync-disambiguations.yml` (simplest analogous workflow):

1. Add `permissions: contents: write` at top level
2. Add a `concurrency` block to prevent push race conditions
3. Add a GitHub App token generation step using `TSUKU_BATCH_GENERATOR_APP_ID` / `TSUKU_BATCH_GENERATOR_APP_PRIVATE_KEY`
4. Pass the App token to `actions/checkout`
5. Add retry logic on `git push` with `git pull --rebase` on failure (all other push workflows have this)

## Reproduction

Check any recent run of the `Update Coverage Dashboard` workflow -- they all fail at the push step.

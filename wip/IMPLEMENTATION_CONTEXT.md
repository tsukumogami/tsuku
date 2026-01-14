The checksum pinning tests in `integration-tests.yml` sporadically fail with GitHub API rate limit errors despite `GITHUB_TOKEN` being configured in the workflow.

## Problem

The workflow passes `GITHUB_TOKEN` to the test script:

```yaml
# .github/workflows/integration-tests.yml:39-42
- name: Run checksum pinning tests
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: ./test/scripts/test-checksum-pinning.sh ${{ matrix.family }}
```

But the script runs tsuku inside Docker containers without forwarding the token:

```bash
# test/scripts/test-checksum-pinning.sh:125-126
RESULT=$(docker run --rm "$IMAGE_TAG" bash -c '
    ./tsuku install fzf --force 2>&1
    ...
')
```

The container makes unauthenticated GitHub API requests (60 req/hour limit). Since all 5 matrix jobs (debian, rhel, arch, alpine, suse) run in parallel from the same runner IP, they collectively exhaust the unauthenticated rate limit.

## Error observed

```
Warning: version resolution failed: GitHub API rate limit exceeded while resolving tool versions from GitHub: 60/60 requests used (unauthenticated), resets at 12:08AM
```

## Fix options

**Option A: Pass token to containers**

```bash
RESULT=$(docker run --rm -e GITHUB_TOKEN="$GITHUB_TOKEN" "$IMAGE_TAG" bash -c '
    ./tsuku install fzf --force 2>&1
')
```

**Option B: Use pre-generated plan**

The script already generates a plan on the host (with token):
```bash
./tsuku eval fzf --os linux --linux-family "$FAMILY" --install-deps > "fzf-checksum-$FAMILY.json"
```

But then ignores it inside the container. Could copy the plan into the container and use `--plan`:
```bash
docker run --rm -v "$(pwd)/fzf-checksum-$FAMILY.json:/home/testuser/plan.json" "$IMAGE_TAG" bash -c '
    ./tsuku install --plan /home/testuser/plan.json --force 2>&1
'
```

Option B avoids passing secrets into containers but requires ensuring the plan file is accessible.

## Files

- `.github/workflows/integration-tests.yml` - workflow definition
- `test/scripts/test-checksum-pinning.sh` - test script with Docker runs

# Issue 757 Implementation Plan

## Summary

Create a GitHub Actions workflow that builds and publishes multi-arch container images to GHCR on sandbox/ changes, manual dispatch, and release tags.

## Approach

Create a new workflow file `.github/workflows/container-build.yml` following existing patterns in the repo (using actions/checkout@v4, proper permissions, etc.). Use docker/build-push-action with QEMU for multi-arch builds.

### Alternatives Considered

- **Using separate jobs per architecture**: More complex, harder to maintain. QEMU-based multi-arch build is simpler and the standard approach.
- **Building only on release tags**: Would delay container availability. Building on sandbox/ changes ensures images are ready when dependent issues need them.

## Files to Create

- `.github/workflows/container-build.yml` - The container build workflow

## Files to Modify

None - this is a new workflow

## Implementation Steps

- [x] Create the container-build.yml workflow with:
  - Triggers: push to main (sandbox/** changes), workflow_dispatch, release tags (v*)
  - Multi-arch builds using docker/setup-qemu-action and docker/setup-buildx-action
  - Login to GHCR using docker/login-action
  - Build and push using docker/build-push-action with caching
  - Tag images as ghcr.io/tsukumogami/tsuku-sandbox:latest (and version tags on release)
- [x] Add GHCR packages write permission

## Testing Strategy

- Workflow syntax validated by actionlint CI
- Full workflow test when sandbox/Dockerfile.minimal is added (issue #767)
- Manual dispatch can test the workflow without Dockerfile present (will fail gracefully)

## Risks and Mitigations

- **Risk**: Dockerfile doesn't exist yet - workflow will fail if triggered
  - **Mitigation**: Add path filter on sandbox/** so it only triggers when sandbox/ changes exist
- **Risk**: Multi-arch builds are slow
  - **Mitigation**: Enable build cache using gha cache type

## Success Criteria

- [ ] Workflow file exists at `.github/workflows/container-build.yml`
- [ ] Workflow triggers on: push to main with sandbox/ changes, workflow_dispatch, release tags
- [ ] Workflow builds linux/amd64 and linux/arm64 architectures
- [ ] Workflow publishes to ghcr.io/tsukumogami/tsuku-sandbox
- [ ] Build cache is enabled
- [ ] actionlint passes on the workflow

## Open Questions

None - requirements are clear from the issue.

# Lead: What does retiring llm-release.yml look like, and are there consumers of the separate tsuku-llm-v* tags?

## Findings

### No external consumers of tsuku-llm-v* tags
The `llm-release.yml` workflow triggers on tags matching `tsuku-llm-v*` and produces artifacts, but no tags exist in the current repository history, no external documentation points to them, and no CI workflows depend on separate llm releases.

### Recipe resolves from separate repo
The tsuku-llm recipe (`recipes/t/tsuku-llm.toml`) resolves versions from `tsukumogami/tsuku-llm`, not from the main tsuku repo. This is the key reference that would need updating.

### Pipeline produces 6 platform-specific binaries
The `llm-release.yml` workflow builds for darwin-arm64, darwin-amd64, linux-amd64-cuda, linux-amd64-vulkan, linux-arm64-cuda, and linux-arm64-vulkan, then uploads to a separate GitHub release per tag. The recipe's asset patterns (`tsuku-llm-v{version}-{platform}`) are hardcoded to expect this naming convention.

### No documentation references
No README, installation guide, or external docs point users to the separate llm release flow.

## Implications

Safe to retire immediately. The main effort isn't removal but:
1. Moving tsuku-llm binary builds into the unified `release.yml` pipeline
2. Updating the recipe's version provider and asset URL patterns to use unified `v*` tags

## Surprises

No `tsuku-llm-v*` tags have ever been pushed to the repo -- the workflow exists but has never been triggered. This makes retirement even simpler since there are no existing releases to maintain compatibility with.

## Open Questions

- Should the old workflow be deleted outright or kept with a "deprecated" comment during transition?
- Does the `tsukumogami/tsuku-llm` repo (if it exists separately) need any cleanup?

## Summary
No consumers of `tsuku-llm-v*` tags exist -- no tags have been pushed, no docs reference them, and the recipe points to a separate repo. The workflow can be deleted outright. The real work is moving builds into `release.yml` and updating recipe version resolution.

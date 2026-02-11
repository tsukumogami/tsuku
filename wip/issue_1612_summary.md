# Issue 1612 Summary

## What Was Implemented

Fork detection for LLM Discovery's GitHub verification. The system now identifies forked repositories, fetches parent metadata, and prevents auto-selection of forks to protect users from installing tools from abandoned forks.

## Changes Made

- `internal/discover/resolver.go`: Added fork-related fields to Metadata struct (IsFork, ParentRepo, ParentStars)
- `internal/discover/llm_discovery.go`: Extended ghRepo struct with Parent parsing, populated fork metadata in verifyGitHubRepo, updated passesQualityThreshold to reject forks
- `internal/discover/llm_discovery_test.go`: Added tests for fork detection, missing parent handling, and quality threshold fork exclusion

## Key Decisions

- **Inline fork detection**: Added fork handling within verifyGitHubRepo rather than a separate function, since the parent data is already in the API response
- **Pointer type for Parent struct**: Used `*struct{}` for Parent to handle null gracefully when parent data is missing
- **Graceful degradation**: When parent metadata is unavailable (null parent), IsFork is still set to true but ParentRepo and ParentStars remain zero-valued

## Trade-offs Accepted

- **No separate parent API call**: We rely on the parent data included in the fork's API response rather than making a second request to the parent repo. This reduces API calls but means we don't get real-time parent star count if it changed since the fork was created.

## Test Coverage

- New tests added: 4
  - TestVerifyGitHubRepo_Fork
  - TestVerifyGitHubRepo_ForkWithMissingParent
  - TestVerifyGitHubRepo_NotAFork
  - TestPassesQualityThreshold_RejectsForks (with 4 subtests)
- All tests pass

## Known Limitations

- Parent star count comes from the fork's API response cache, not a real-time query to the parent repository
- Edge case of archived/deleted parent repos is not explicitly handled (relies on API returning null parent)

## Future Improvements

- #1613 will add rate limit handling for the verification flow
- #1615 will add fork warning display in confirmation UX with "10x more stars" messaging

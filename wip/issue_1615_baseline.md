# Issue #1615 Baseline

## Branch
Reusing `feature/1612-fork-detection` branch (continuing from #1612 implementation).

## Test Status
- `go test ./internal/discover/...` - PASS
- `go test ./cmd/tsuku/...` - PASS

## Current State

### Metadata struct (resolver.go)
Current fields:
- Downloads, AgeDays, Stars, Description (original)
- IsFork, ParentRepo, ParentStars (added in #1612)

Missing for #1615:
- LastCommitDays (or PushedAt) for AC6
- OwnerType for AC7
- CreatedAt (actual date string) for AC5

### confirmLLMDiscovery (create.go:154-176)
Current behavior:
- Fails in non-interactive mode with unhelpful error
- Shows: Source, Stars, Description, Reason
- No --yes handling
- No fork warning display
- No age/last commit display

### GitHub API parsing (llm_discovery.go:199-210)
Current fields parsed:
- stargazers_count, forks_count, archived, description, created_at, fork, parent

Missing:
- pushed_at (for last commit)
- owner.login, owner.type (for owner info)

## Implementation Scope
1. Extend Metadata struct with new fields
2. Extend GitHub API parsing to extract new fields
3. Update confirmLLMDiscovery to:
   - Accept autoApprove parameter
   - Display enhanced metadata
   - Show fork warnings
   - Handle --yes flag
4. Wire --yes through the call chain

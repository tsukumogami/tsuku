# Issue 278 Summary

## What Was Implemented

Created `internal/llm/` package with a Claude client that supports multi-turn tool use for recipe generation. The client handles conversations where Claude can call tools to fetch files, inspect archives, and ultimately return discovered asset patterns.

## Changes Made

- `go.mod`, `go.sum`: Added `github.com/anthropics/anthropic-sdk-go` dependency
- `internal/llm/client.go`: Main Client struct with GenerateRecipe method implementing multi-turn conversation
- `internal/llm/client_test.go`: Comprehensive tests including unit tests and integration test
- `internal/llm/cost.go`: Usage tracking with Cost() and String() for Claude Sonnet 4 pricing
- `internal/llm/cost_test.go`: Unit tests for cost calculation
- `internal/llm/tools.go`: Tool schema definitions for fetch_file, inspect_archive, extract_pattern

## Key Decisions

- **Used official Anthropic SDK**: Handles auth, request/response serialization, and retries
- **Injected HTTP client for tools**: Allows testing fetch_file without real network calls
- **Stub for inspect_archive**: Full archive inspection deferred to Slice 2; LLM can still infer patterns from asset names
- **Max 5 turns limit**: Prevents infinite loops and runaway costs

## Trade-offs Accepted

- **inspect_archive is a stub**: Acceptable for Slice 1 as the LLM can infer structure from naming conventions
- **No provider abstraction yet**: Slice 3 will add the provider interface; Slice 1 focuses on proving the concept with Claude

## Test Coverage

- New tests added: 12 tests across 2 test files
- Unit tests: cost calculation, prompt building, HTTP client
- Integration test: skipped unless ANTHROPIC_API_KEY is set (real API calls)
- All tests pass, build succeeds

## Known Limitations

- `inspect_archive` tool returns a placeholder message
- No streaming support (not needed for this use case)
- Hardcoded to Claude Sonnet 4 model

## Future Improvements (Slice 2+)

- Implement actual archive inspection with tar.gz/zip support
- Add container validation for generated recipes
- Add repair loop when validation fails
- Add second provider (Gemini) behind unified interface

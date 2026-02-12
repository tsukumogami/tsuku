# Issue 1611 Summary

## What Was Implemented

Added HTML stripping and URL validation as defense layers against prompt injection in LLM discovery. These functions sanitize web search results before the LLM processes them, removing hidden text that could be used to steer the model toward malicious sources.

## Changes Made

- `internal/discover/sanitize.go`: Created with `StripHTML()` and `ValidateGitHubURL()` functions
- `internal/discover/sanitize_test.go`: Comprehensive tests including injection scenarios
- `internal/discover/llm_discovery.go`: Integrated sanitization in `handleWebSearch()` and `handleExtractSource()`
- `internal/discover/llm_discovery_test.go`: Updated to use `ValidateGitHubURL` instead of old regex validation

## Key Decisions

- **Use golang.org/x/net/html for HTML parsing**: Provides robust handling of malformed HTML per the HTML5 parsing algorithm
- **Insert spaces when skipping dangerous elements**: Prevents adjacent text from merging (e.g., "Start<script>x</script>End" becomes "Start End" not "StartEnd")
- **Accept both URL and owner/repo formats**: `ValidateGitHubURL` handles both `https://github.com/owner/repo` and `owner/repo` for flexibility
- **Replace githubSourceRegex with ValidateGitHubURL**: The new function provides more comprehensive validation including port, credentials, and path traversal checks

## Trade-offs Accepted

- **Zero-width character removal doesn't add spaces**: Removing U+200B from "End\u200BText" produces "EndText" not "End Text" - this is intentional as zero-width chars are invisible and shouldn't affect word spacing
- **Whitespace collapsing**: Multiple whitespace characters are collapsed to single spaces, which may change formatting but improves readability for the LLM

## Test Coverage

- New tests added: 17 test functions covering HTML stripping and URL validation
- Key injection scenarios tested:
  - Hidden prompts in HTML comments
  - Prompts in script/style/noscript tags
  - Zero-width character obfuscation
  - URL credential injection
  - Path traversal attempts
  - Non-standard port rejection

## Known Limitations

- **Visible text injection not addressed**: HTML stripping removes hidden elements but cannot detect SEO-optimized attack pages with malicious visible content. This is a known limitation documented in the design doc.
- **Performance on large HTML documents**: No explicit size limit, though `strings.Fields()` for whitespace collapsing is O(n)

## Future Improvements

- Consider multi-source verification (official docs → repo link) to address visible text injection
- Add size limits if performance becomes an issue with large HTML documents
- Consider adding more zero-width Unicode characters as new attack patterns emerge

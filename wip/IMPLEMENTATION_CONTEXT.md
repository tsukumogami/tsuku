---
summary:
  constraints:
    - Must sanitize content BEFORE it reaches the LLM (defense layer 2 of 6)
    - HTML stripping must handle malformed HTML gracefully without crashes
    - URL validation uses allowlist approach - only accept known-good patterns
    - No user input passed to shell commands or eval
    - Error messages must not leak internal paths
  integration_points:
    - internal/discover/llm_discovery.go - sanitization runs before LLM sees search results
    - internal/search/provider.go - FormatForLLM() may need to call sanitization
    - The existing githubSourceRegex in llm_discovery.go validates owner/repo format
  risks:
    - Nested/malformed HTML could bypass stripping if not handled correctly
    - Zero-width characters could sneak through if incomplete Unicode handling
    - URL parsing edge cases (encoded characters, unusual schemes)
    - Performance impact if sanitization is slow on large HTML documents
  approach_notes: |
    Create internal/discover/sanitize.go with two main functions:
    1. StripHTML(content string) string - removes dangerous elements
    2. ValidateGitHubURL(rawURL string) error - validates URL patterns

    For HTML stripping:
    - Use golang.org/x/net/html for robust parsing (handles malformed HTML)
    - Remove script, style, noscript tags and their contents
    - Remove HTML comments
    - Remove zero-width Unicode chars (U+200B, U+200C, U+200D, U+FEFF, U+2060)
    - Extract text content only

    For URL validation:
    - Parse with net/url
    - Reject credentials (User != nil)
    - Reject non-standard ports
    - Reject path traversal (../)
    - Validate owner/repo against safe character set (extend existing regex)
---

# Implementation Context: Issue #1611

**Source**: docs/designs/DESIGN-llm-discovery-implementation.md

## Design Excerpt (Key Sections)

### Defense Layers

From the design doc, issue #1611 implements defense layers 2 and 3:

2. **HTML Stripping** (new)
   - Remove script, style, noscript tags and HTML comments
   - Remove zero-width Unicode characters
   - Convert to plain text before LLM processing

3. **URL Validation** (new)
   - GitHub URLs must match `github.com/{owner}/{repo}` pattern
   - Owner/repo names restricted to safe character sets
   - Reject credentials, non-standard ports, path traversal

### Security Considerations

**Prompt injection via web content**: Malicious pages could contain hidden text attempting to steer the LLM toward bad sources.

Mitigations:
1. HTML stripping removes hidden elements, zero-width characters
2. URL validation rejects malformed patterns
3. GitHub API verification confirms repository exists
4. User confirmation shows metadata for manual review
5. Sandbox validation catches bad recipes post-discovery

### Existing Related Code

`internal/discover/llm_discovery.go:481`:
```go
var githubSourceRegex = regexp.MustCompile(`^[a-zA-Z0-9][-a-zA-Z0-9]*/[a-zA-Z0-9._-]+$`)
```

This regex validates the owner/repo format after extraction. URL validation should complement this by validating the full URL structure.

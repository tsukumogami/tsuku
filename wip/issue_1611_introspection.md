# Issue 1611 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-llm-discovery-implementation.md`
- Sibling issues reviewed: #1610 (closed - DDG retry logic merged in PR #1619)
- Prior patterns identified:
  - Search providers follow `search.Provider` interface
  - Test fixtures in `internal/search/testdata/`
  - HTTP response mocking via httptest

## Gap Analysis

### Minor Gaps

1. **Integration point location**: Issue mentions "internal/discover/" but from the design doc and existing code, the sanitization should:
   - Create `internal/discover/sanitize.go` for StripHTML and ValidateGitHubURL functions
   - Call sanitization from `FormatForLLM()` in `internal/search/provider.go` OR
   - Call sanitization in `handleWebSearch()` in `internal/discover/llm_discovery.go:428`

   Based on design doc section "Defense Layers", sanitization should happen "before LLM processing", so the call site should be in `handleWebSearch()` after search results are returned but before `FormatForLLM()`.

2. **Existing regex validation**: The `githubSourceRegex` at `llm_discovery.go:481` already validates owner/repo format. The new `ValidateGitHubURL` function should complement this by validating full URL structure (scheme, host, port, credentials, path traversal) before extracting owner/repo.

3. **Zero-width Unicode characters to remove**: Design doc specifies: U+200B (ZWSP), U+200C (ZWNJ), U+200D (ZWJ), U+FEFF (BOM), U+2060 (WJ). Implementation should include all of these.

4. **Test fixture pattern**: Follow the pattern from PR #1619 - use recorded responses in `testdata/` and httptest for mocking.

### Moderate Gaps

None identified - the issue spec is complete and aligns with prior work.

### Major Gaps

None identified - no conflicts with prior implementation decisions.

## Recommendation

Proceed with implementation. The minor gaps are resolvable from design doc and prior work context.

## Implementation Notes

1. Create `internal/discover/sanitize.go` with:
   - `StripHTML(content string) string` - uses golang.org/x/net/html
   - `ValidateGitHubURL(rawURL string) error` - uses net/url
   - `removeZeroWidthChars(s string) string` - helper

2. Call sanitization in `llm_discovery.go` inside `handleWebSearch()`:
   ```go
   // After getting search results, sanitize before formatting
   for i := range resp.Results {
       resp.Results[i].Title = StripHTML(resp.Results[i].Title)
       resp.Results[i].Snippet = StripHTML(resp.Results[i].Snippet)
   }
   ```

3. URL validation should be called in `handleExtractSource()` before accepting the source:
   ```go
   // Validate the source as a valid GitHub URL pattern
   if builder == "github" {
       if err := ValidateGitHubURL(source); err != nil {
           return nil, fmt.Errorf("extract_source: %w", err)
       }
   }
   ```

4. Note: The source in extract_source is already in `owner/repo` format, not a full URL. The ValidateGitHubURL function should accept either format and validate accordingly.

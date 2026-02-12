package discover

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// zeroWidthChars are Unicode characters that have no visual width and can be
// used to hide text from human readers while still being processed by LLMs.
var zeroWidthChars = []rune{
	'\u200B', // Zero Width Space
	'\u200C', // Zero Width Non-Joiner
	'\u200D', // Zero Width Joiner
	'\uFEFF', // Byte Order Mark / Zero Width No-Break Space
	'\u2060', // Word Joiner
	'\u200E', // Left-to-Right Mark
	'\u200F', // Right-to-Left Mark
}

// dangerousTags are HTML tags whose content should be completely removed
// as they may contain hidden text used for prompt injection.
var dangerousTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
}

// StripHTML removes dangerous HTML elements and converts to plain text.
// It removes script, style, noscript tags and their contents, HTML comments,
// and zero-width Unicode characters before content reaches the LLM.
//
// The function handles malformed HTML gracefully by using golang.org/x/net/html
// which implements the HTML5 parsing algorithm with error recovery.
func StripHTML(content string) string {
	if content == "" {
		return ""
	}

	// Parse the HTML document
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		// If parsing fails completely, fall back to removing zero-width chars only
		return removeZeroWidthChars(content)
	}

	// Extract text from the parsed tree
	var sb strings.Builder
	extractText(doc, &sb)
	text := sb.String()

	// Remove zero-width characters from the extracted text
	text = removeZeroWidthChars(text)

	// Collapse multiple whitespace into single spaces and trim
	text = collapseWhitespace(text)

	return text
}

// extractText recursively walks the HTML tree and extracts text content,
// skipping dangerous elements and comments.
func extractText(n *html.Node, sb *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		sb.WriteString(n.Data)
	case html.CommentNode:
		// Skip comments - they may contain injection prompts
		// Add a space to prevent adjacent text from merging
		sb.WriteString(" ")
		return
	case html.ElementNode:
		// Skip dangerous tags entirely (including their children)
		if dangerousTags[n.Data] {
			// Add a space to prevent adjacent text from merging
			sb.WriteString(" ")
			return
		}
	}

	// Recursively process children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, sb)
	}
}

// removeZeroWidthChars removes all zero-width Unicode characters from a string.
func removeZeroWidthChars(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))

	for _, r := range s {
		if !isZeroWidthChar(r) {
			sb.WriteRune(r)
		}
	}

	return sb.String()
}

// isZeroWidthChar returns true if the rune is a zero-width character.
func isZeroWidthChar(r rune) bool {
	for _, zw := range zeroWidthChars {
		if r == zw {
			return true
		}
	}
	return false
}

// collapseWhitespace collapses multiple whitespace characters into single spaces
// and trims leading/trailing whitespace.
func collapseWhitespace(s string) string {
	// Split on whitespace and rejoin with single spaces
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// URL validation errors
var (
	ErrURLCredentials     = errors.New("URL contains credentials")
	ErrURLNonStandardPort = errors.New("URL has non-standard port")
	ErrURLPathTraversal   = errors.New("URL contains path traversal")
	ErrURLNotGitHub       = errors.New("URL host must be github.com")
	ErrURLInvalidOwner    = errors.New("invalid owner name in URL")
	ErrURLInvalidRepo     = errors.New("invalid repo name in URL")
	ErrURLMalformed       = errors.New("malformed URL")
)

// ownerRepoRegex validates owner and repo names use only safe characters.
// This extends the existing githubSourceRegex pattern to full URL context.
// Allowed: alphanumeric, hyphen (not at start for owner), underscore, period
var ownerRepoRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateGitHubURL validates a GitHub URL against security constraints.
// Returns an error if the URL:
// - Contains credentials (user:pass@)
// - Has a non-standard port
// - Contains path traversal (..)
// - Is not hosted on github.com
// - Has owner/repo names with invalid characters
//
// The function accepts both full URLs (https://github.com/owner/repo) and
// owner/repo format for convenience.
func ValidateGitHubURL(rawURL string) error {
	if rawURL == "" {
		return ErrURLMalformed
	}

	// If it's just owner/repo format (no scheme), validate directly
	if !strings.Contains(rawURL, "://") {
		return validateOwnerRepo(rawURL)
	}

	// Parse the full URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrURLMalformed, err)
	}

	// Check for credentials
	if u.User != nil {
		return ErrURLCredentials
	}

	// Check for non-standard port
	// url.Port() returns empty string if no port is specified
	if u.Port() != "" {
		return ErrURLNonStandardPort
	}

	// Check host is github.com
	host := strings.ToLower(u.Host)
	if host != "github.com" && host != "www.github.com" {
		return ErrURLNotGitHub
	}

	// Check for path traversal
	if strings.Contains(u.Path, "..") {
		return ErrURLPathTraversal
	}

	// Also check the raw path for encoded traversal attempts
	if strings.Contains(u.RawPath, "..") || strings.Contains(rawURL, "%2e%2e") || strings.Contains(rawURL, "%2E%2E") {
		return ErrURLPathTraversal
	}

	// Extract and validate owner/repo from path
	// Path should be /owner/repo or /owner/repo/...
	path := strings.TrimPrefix(u.Path, "/")
	return validateOwnerRepo(path)
}

// validateOwnerRepo validates the owner/repo portion of a GitHub reference.
func validateOwnerRepo(path string) error {
	// Split on / and validate first two components
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return fmt.Errorf("%w: expected owner/repo format", ErrURLMalformed)
	}

	owner := parts[0]
	repo := parts[1]

	// Owner validation
	if owner == "" {
		return ErrURLInvalidOwner
	}
	if !ownerRepoRegex.MatchString(owner) {
		return ErrURLInvalidOwner
	}

	// Repo validation
	if repo == "" {
		return ErrURLInvalidRepo
	}
	if !ownerRepoRegex.MatchString(repo) {
		return ErrURLInvalidRepo
	}

	return nil
}

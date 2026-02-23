package builders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// downloadArtifactOptions configures the shared artifact download helper.
type downloadArtifactOptions struct {
	// MaxSize is the maximum number of bytes to download. The download is
	// aborted if the response body exceeds this limit.
	MaxSize int64

	// ExpectedSHA256 is an optional hex-encoded SHA256 digest. When non-empty,
	// the downloaded bytes are verified against this hash and an error is
	// returned on mismatch.
	ExpectedSHA256 string

	// ExpectedContentTypes is a list of acceptable Content-Type prefixes.
	// If non-empty, the response Content-Type must match at least one.
	ExpectedContentTypes []string
}

// downloadArtifact downloads a URL in-memory and returns the response body as
// a byte slice. It enforces:
//   - HTTPS-only URLs
//   - Configurable maximum download size
//   - Optional Content-Type verification
//   - Optional SHA256 hash verification
//   - User-Agent header matching builder convention
//
// The caller's HTTP client is used for the request, so timeout behavior matches
// the builder's existing 60-second client timeout.
func downloadArtifact(
	ctx context.Context,
	client *http.Client,
	url string,
	opts downloadArtifactOptions,
) ([]byte, error) {
	// Enforce HTTPS
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("artifact download requires HTTPS: %s", url)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact request: %w", err)
	}
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artifact download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifact download returned status %d", resp.StatusCode)
	}

	// Verify content type if required
	if len(opts.ExpectedContentTypes) > 0 {
		ct := resp.Header.Get("Content-Type")
		matched := false
		for _, expected := range opts.ExpectedContentTypes {
			if strings.HasPrefix(ct, expected) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("unexpected artifact content-type: %s", ct)
		}
	}

	// Read with size limit. We add 1 byte beyond the limit so we can detect
	// whether the response was truncated.
	limitedReader := io.LimitReader(resp.Body, opts.MaxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact body: %w", err)
	}
	if int64(len(data)) > opts.MaxSize {
		return nil, fmt.Errorf("artifact exceeds maximum size of %d bytes", opts.MaxSize)
	}

	// Verify SHA256 if provided
	if opts.ExpectedSHA256 != "" {
		hash := sha256.Sum256(data)
		actual := hex.EncodeToString(hash[:])
		if actual != opts.ExpectedSHA256 {
			return nil, fmt.Errorf("artifact SHA256 mismatch: expected %s, got %s", opts.ExpectedSHA256, actual)
		}
	}

	return data, nil
}

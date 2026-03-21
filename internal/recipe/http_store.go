package recipe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HTTPStore is a BackingStore that fetches files via HTTP with a built-in
// DiskCache. It handles conditional requests (ETag/If-Modified-Since) and
// stale-if-error fallback transparently.
type HTTPStore struct {
	baseURL string
	cache   *DiskCache
	client  *http.Client
}

// HTTPStoreConfig holds parameters for creating an HTTPStore.
type HTTPStoreConfig struct {
	// BaseURL is the URL prefix for fetching files. The key passed to Get
	// is appended to this URL with a "/" separator.
	BaseURL string

	// CacheDir is the root directory for the disk cache.
	CacheDir string

	// TTL controls how long cached entries are considered fresh.
	TTL time.Duration

	// MaxSize is the maximum cache size in bytes.
	MaxSize int64

	// Eviction is the eviction strategy when cache exceeds MaxSize.
	Eviction EvictionStrategy

	// MaxStale is the maximum staleness for stale-if-error fallback.
	// Set to 0 to disable stale fallback.
	MaxStale time.Duration

	// Client is the HTTP client to use. If nil, http.DefaultClient is used.
	Client *http.Client
}

// NewHTTPStore creates an HTTPStore with the given configuration.
func NewHTTPStore(cfg HTTPStoreConfig) *HTTPStore {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}

	cache := NewDiskCache(DiskCacheConfig{
		Dir:      cfg.CacheDir,
		TTL:      cfg.TTL,
		MaxSize:  cfg.MaxSize,
		Eviction: cfg.Eviction,
		MaxStale: cfg.MaxStale,
	})

	return &HTTPStore{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		cache:   cache,
		client:  client,
	}
}

// Get retrieves bytes at the given path, using the cache when fresh.
//
// Behavior:
//   - Cache hit + fresh: returns cached content
//   - Cache hit + expired: sends conditional request (ETag/If-Modified-Since)
//   - 304 Not Modified: refreshes metadata, returns cached content
//   - 200 OK: updates cache with new content
//   - Network error + stale-if-error: returns stale content if within MaxStale
//   - Cache miss: fetches from HTTP
func (s *HTTPStore) Get(ctx context.Context, path string) ([]byte, error) {
	key := path // The cache key is the path itself

	// Check cache
	data, meta, err := s.cache.Get(key)
	if err != nil {
		// Cache read error: try network directly
		return s.fetchAndCache(ctx, key)
	}

	if data != nil && meta != nil && s.cache.IsFresh(meta) {
		return data, nil
	}

	// Expired or missing: fetch from HTTP
	if data != nil && meta != nil {
		// Have stale data: try conditional request
		freshData, fetchErr := s.conditionalFetch(ctx, key, meta)
		if fetchErr != nil {
			return s.handleStaleError(data, meta, fetchErr)
		}
		return freshData, nil
	}

	// No cached data at all
	freshData, fetchErr := s.fetchAndCache(ctx, key)
	if fetchErr != nil {
		return nil, fetchErr
	}
	return freshData, nil
}

// List returns recipe file paths from the cached directory.
// It returns keys that end in ".toml" from the disk cache.
func (s *HTTPStore) List(_ context.Context) ([]string, error) {
	keys, err := s.cache.Keys()
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, key := range keys {
		if strings.HasSuffix(key, ".toml") {
			paths = append(paths, key)
		}
	}
	return paths, nil
}

// Cache returns the underlying DiskCache for introspection.
func (s *HTTPStore) Cache() *DiskCache {
	return s.cache
}

// CacheStats returns cache statistics. Implements CacheIntrospectable
// when attached to a RegistryProvider.
func (s *HTTPStore) CacheStats() (*DiskCacheStats, error) {
	return s.cache.Stats()
}

// fetchAndCache performs an unconditional HTTP GET, caches the result, and returns it.
func (s *HTTPStore) fetchAndCache(ctx context.Context, key string) ([]byte, error) {
	url := s.baseURL + "/" + key

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if err := checkHTTPResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	meta := s.cache.NewMeta(body)
	meta.ETag = resp.Header.Get("ETag")
	meta.LastModified = resp.Header.Get("Last-Modified")

	if putErr := s.cache.Put(key, body, meta); putErr != nil {
		// Cache write failed but we have data
		return body, nil
	}

	return body, nil
}

// conditionalFetch sends a conditional request using ETag/If-Modified-Since.
// On 304 Not Modified, it refreshes metadata and returns cached data.
// On 200 OK, it updates the cache with new data.
func (s *HTTPStore) conditionalFetch(ctx context.Context, key string, meta *CacheMeta) ([]byte, error) {
	url := s.baseURL + "/" + key

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}

	if meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	if meta.LastModified != "" {
		req.Header.Set("If-Modified-Since", meta.LastModified)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		// Content unchanged: re-read cached content and refresh metadata to extend TTL.
		// We read the content file directly to avoid the Get() path which updates
		// last-access and re-reads metadata.
		data, readErr := os.ReadFile(filepath.Join(s.cache.Dir(), key))
		if readErr != nil {
			// Cache was evicted between our check and this read; fall through
			// to treat as a full fetch by returning an error so the caller retries.
			return nil, fmt.Errorf("cached content disappeared after 304: %w", readErr)
		}

		refreshed := s.cache.NewMeta(data)
		refreshed.ETag = meta.ETag
		refreshed.LastModified = meta.LastModified
		if newETag := resp.Header.Get("ETag"); newETag != "" {
			refreshed.ETag = newETag
		}
		_ = s.cache.Put(key, data, refreshed)
		return data, nil
	}

	if err := checkHTTPResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	newMeta := s.cache.NewMeta(body)
	newMeta.ETag = resp.Header.Get("ETag")
	newMeta.LastModified = resp.Header.Get("Last-Modified")

	if putErr := s.cache.Put(key, body, newMeta); putErr != nil {
		return body, nil
	}

	return body, nil
}

// handleStaleError checks whether stale cached data can be returned
// when a fetch fails (stale-if-error pattern).
func (s *HTTPStore) handleStaleError(data []byte, meta *CacheMeta, fetchErr error) ([]byte, error) {
	if s.cache.IsStaleUsable(meta) {
		return data, nil
	}
	return nil, fetchErr
}

// HTTPError represents an HTTP-level error from the remote server.
type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
	RetryAfter time.Duration // Populated for 429 responses
}

func (e *HTTPError) Error() string {
	if e.StatusCode == http.StatusTooManyRequests {
		msg := fmt.Sprintf("rate limited by %s (HTTP %d)", e.URL, e.StatusCode)
		if e.RetryAfter > 0 {
			msg += fmt.Sprintf("; retry after %s", e.RetryAfter.Round(time.Second))
		}
		return msg
	}
	return fmt.Sprintf("HTTP %d from %s: %s", e.StatusCode, e.URL, e.Status)
}

// checkHTTPResponse returns an error for non-2xx status codes.
func checkHTTPResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	httpErr := &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		URL:        resp.Request.URL.String(),
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				httpErr.RetryAfter = time.Duration(secs) * time.Second
			} else if t, err := http.ParseTime(ra); err == nil {
				httpErr.RetryAfter = time.Until(t)
			}
		}
	}

	return httpErr
}

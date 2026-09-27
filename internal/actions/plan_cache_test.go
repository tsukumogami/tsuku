package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// These cases cover plan generation's use of the download cache: an artifact already in
// the cache costs no request, and only bytes that still hash to what was saved count.

func newPlanCache(t *testing.T) *DownloadCache {
	t.Helper()
	return NewDownloadCache(filepath.Join(t.TempDir(), "downloads"))
}

// resolveAndSave mirrors the plan-time call sites: resolve, then save only a download.
func resolveAndSave(t *testing.T, d *scriptedDownloader, cache *DownloadCache, sources ...string) (*DownloadResult, bool) {
	t.Helper()
	result, servingURL, fromCache, err := ResolveFirstAvailable(context.Background(), d, cache, sources)
	if err != nil {
		t.Fatalf("ResolveFirstAvailable: %v", err)
	}
	if !fromCache {
		if err := cache.Save(servingURL, result.AssetPath, result.Checksum); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	_ = result.Cleanup()
	return result, fromCache
}

func TestResolveFirstAvailable_SecondResolveMakesNoRequest(t *testing.T) {
	const url = "https://a.example/tool.tar.gz"
	d := &scriptedDownloader{serve: map[string]string{url: "tool bytes"}}
	cache := newPlanCache(t)

	first, fromCache := resolveAndSave(t, d, cache, url)
	if fromCache {
		t.Fatal("first resolve reported a cache hit on an empty cache")
	}
	second, fromCache := resolveAndSave(t, d, cache, url)
	if !fromCache {
		t.Fatal("second resolve did not use the cache")
	}
	if len(d.attempts) != 1 {
		t.Errorf("downloader called %d times, want 1: %v", len(d.attempts), d.attempts)
	}
	if second.Checksum != first.Checksum || second.Size != first.Size {
		t.Errorf("cached result %s/%d differs from downloaded %s/%d", second.Checksum, second.Size, first.Checksum, first.Size)
	}
	if second.AssetPath != "" {
		t.Errorf("cache hit AssetPath = %q, want empty so Cleanup cannot touch the cache", second.AssetPath)
	}
}

func TestResolveFirstAvailable_TamperedBytesAreAMiss(t *testing.T) {
	const url = "https://a.example/tool.tar.gz"
	d := &scriptedDownloader{serve: map[string]string{url: "tool bytes"}}
	cache := newPlanCache(t)
	resolveAndSave(t, d, cache, url)

	// Same size, different content: only the hash can tell.
	filePath, _ := cache.cachePaths(url)
	if err := os.WriteFile(filePath, []byte("TOOL BYTES"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, fromCache := resolveAndSave(t, d, cache, url); fromCache {
		t.Fatal("tampered cache entry was used")
	}
	if len(d.attempts) != 2 {
		t.Errorf("downloader called %d times, want 2 (the tampered entry must be refetched)", len(d.attempts))
	}
}

func TestResolveFirstAvailable_HitsUnderAFallbackSource(t *testing.T) {
	const primary, mirror = "https://a.example/tool.tar.gz", "https://mirror.example/tool.tar.gz"
	d := &scriptedDownloader{serve: map[string]string{mirror: "tool bytes"}}
	cache := newPlanCache(t)

	resolveAndSave(t, d, cache, primary, mirror) // primary fails, mirror serves and is cached
	d.attempts = nil
	if _, fromCache := resolveAndSave(t, d, cache, primary, mirror); !fromCache {
		t.Fatal("entry cached under the mirror was not found")
	}
	if len(d.attempts) != 0 {
		t.Errorf("downloader called %v, want no requests", d.attempts)
	}
}

func TestDownloadCacheLookup_EntryWithoutRecordedHashIsAMiss(t *testing.T) {
	const url = "https://a.example/tool.tar.gz"
	cache := newPlanCache(t)
	src := filepath.Join(t.TempDir(), "tool.tar.gz")
	if err := os.WriteFile(src, []byte("tool bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(url, src, ""); err != nil {
		t.Fatal(err)
	}
	_, metaPath := cache.cachePaths(url)
	meta, err := cache.readMeta(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	meta.ActualHash = ""
	if err := cache.writeMeta(metaPath, meta); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := cache.Lookup(url); err != nil || hit {
		t.Errorf("Lookup = hit %v, err %v; want a miss for an entry with no recorded hash", hit, err)
	}
}

func TestResolveFirstAvailable_NilCacheDownloads(t *testing.T) {
	const url = "https://a.example/tool.tar.gz"
	d := &scriptedDownloader{serve: map[string]string{url: "tool bytes"}}
	result, _, fromCache, err := ResolveFirstAvailable(context.Background(), d, nil, []string{url})
	if err != nil || fromCache || len(d.attempts) != 1 {
		t.Errorf("got fromCache %v, err %v, attempts %v; want one download", fromCache, err, d.attempts)
	}
	_ = result.Cleanup()
}

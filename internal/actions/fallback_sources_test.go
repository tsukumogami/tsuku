package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// scriptedDownloader answers for the URLs in serve and fails for everything
// else, recording the order it was asked in. That order is the thing most of
// these tests are really asserting: fallback is only correct if it walks the
// declared order and stops at the first source that answers.
type scriptedDownloader struct {
	serve    map[string]string // URL -> file contents
	attempts []string
}

func (d *scriptedDownloader) Download(ctx context.Context, url string) (*DownloadResult, error) {
	d.attempts = append(d.attempts, url)

	body, ok := d.serve[url]
	if !ok {
		return nil, fmt.Errorf("host unreachable: %s", url)
	}

	dir, err := os.MkdirTemp("", "scripted-download-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, filepath.Base(url))
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return nil, err
	}
	sum, err := computeSHA256(path)
	if err != nil {
		return nil, err
	}
	return &DownloadResult{AssetPath: path, Checksum: sum, Size: int64(len(body))}, nil
}

func TestDownloadSources(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   []string
	}{
		{
			name:   "url only yields a one-element list",
			params: map[string]interface{}{"url": "https://a.example/x.tar.gz"},
			want:   []string{"https://a.example/x.tar.gz"},
		},
		{
			name: "primary comes first, then declaration order",
			params: map[string]interface{}{
				"url": "https://a.example/x.tar.gz",
				FallbackURLsParam: []interface{}{
					"https://b.example/x.tar.gz",
					"https://c.example/x.tar.gz",
				},
			},
			want: []string{
				"https://a.example/x.tar.gz",
				"https://b.example/x.tar.gz",
				"https://c.example/x.tar.gz",
			},
		},
		{
			name:   "no url and no fallbacks yields nothing",
			params: map[string]interface{}{},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DownloadSources(tt.params)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("source %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDownloadFirstAvailable_FallsThroughToLaterSource(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{
		"https://c.example/x.tar.gz": "archive bytes",
	}}

	result, servingURL, err := DownloadFirstAvailable(context.Background(), d, []string{
		"https://a.example/x.tar.gz",
		"https://b.example/x.tar.gz",
		"https://c.example/x.tar.gz",
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	defer func() { _ = result.Cleanup() }()

	if servingURL != "https://c.example/x.tar.gz" {
		t.Errorf("serving URL: got %q, want the third source", servingURL)
	}
	if len(d.attempts) != 3 {
		t.Errorf("expected all three sources attempted in order, got %v", d.attempts)
	}
}

func TestDownloadFirstAvailable_StopsAtFirstSuccess(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{
		"https://a.example/x.tar.gz": "archive bytes",
		"https://b.example/x.tar.gz": "archive bytes",
	}}

	result, servingURL, err := DownloadFirstAvailable(context.Background(), d, []string{
		"https://a.example/x.tar.gz",
		"https://b.example/x.tar.gz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = result.Cleanup() }()

	if servingURL != "https://a.example/x.tar.gz" {
		t.Errorf("serving URL: got %q, want the primary", servingURL)
	}
	// R14: no extra network cost when the first source answers.
	if len(d.attempts) != 1 {
		t.Errorf("expected exactly one attempt when the primary answers, got %v", d.attempts)
	}
}

func TestDownloadFirstAvailable_ExhaustionNamesEverySource(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{}}

	_, _, err := DownloadFirstAvailable(context.Background(), d, []string{
		"https://a.example/x.tar.gz",
		"https://b.example/x.tar.gz",
	})
	if err == nil {
		t.Fatal("expected an error when every source fails")
	}

	// R4: a maintainer reading CI logs must be able to tell an outage from a
	// retention gap from a typo, which needs every source named.
	for _, want := range []string{"a.example", "b.example", "all 2 download sources failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestDownloadFirstAvailable_SingleSourceErrorIsUnchanged(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{}}

	_, _, err := DownloadFirstAvailable(context.Background(), d, []string{"https://a.example/x.tar.gz"})
	if err == nil {
		t.Fatal("expected an error")
	}
	// A single-source recipe's failure must read exactly as it did before
	// fallback existed — no aggregate wrapper.
	if got, want := err.Error(), "host unreachable: https://a.example/x.tar.gz"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDownloadFirstAvailable_CanceledContextStopsTheWalk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := &scriptedDownloader{serve: map[string]string{}}
	_, _, err := DownloadFirstAvailable(ctx, d, []string{
		"https://a.example/x.tar.gz",
		"https://b.example/x.tar.gz",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	// A canceled context means the caller gave up, not that the source failed.
	if len(d.attempts) != 1 {
		t.Errorf("expected the walk to stop after the first attempt, got %v", d.attempts)
	}
}

func TestDownloadFileHTTPWithFallback(t *testing.T) {
	t.Run("empty source list is an error, not a silent success", func(t *testing.T) {
		err := downloadFileHTTPWithFallback(context.Background(), nil,
			filepath.Join(t.TempDir(), "out"), progress.NoopReporter{})
		if err == nil {
			t.Fatal("expected an error for an empty source list")
		}
	})

	t.Run("non-HTTPS sources are rejected per source", func(t *testing.T) {
		err := downloadFileHTTPWithFallback(context.Background(),
			[]string{"http://a.example/x", "http://b.example/x"},
			filepath.Join(t.TempDir(), "out"), progress.NoopReporter{})
		if err == nil {
			t.Fatal("expected an error")
		}
		// SECURITY: HTTPS enforcement applies to every source, not only the
		// primary.
		for _, want := range []string{"a.example", "b.example", "HTTPS"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})
}

// TestDownloadCache_CheckAny covers the miss this whole mechanism would
// otherwise introduce: plan generation saves under whichever source answered,
// so an install that probed only the primary's key would re-download an
// archive it already has on disk.
func TestDownloadCache_CheckAny(t *testing.T) {
	const (
		primary   = "https://a.example/tool-1.0.0.tar.gz"
		alternate = "https://b.example/tool-1.0.0.tar.gz"
		body      = "archive bytes"
	)

	newCacheWithEntryUnder := func(t *testing.T, url string) (*DownloadCache, string) {
		t.Helper()
		cacheDir := filepath.Join(t.TempDir(), "downloads")
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			t.Fatal(err)
		}
		cache := NewDownloadCache(cacheDir)
		cache.SetSkipSecurityChecks(true)

		src := filepath.Join(t.TempDir(), "tool-1.0.0.tar.gz")
		if err := os.WriteFile(src, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		checksum, err := computeSHA256(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Save(url, src, checksum); err != nil {
			t.Fatal(err)
		}
		return cache, checksum
	}

	t.Run("entry saved under an alternate is a hit for the whole list", func(t *testing.T) {
		cache, checksum := newCacheWithEntryUnder(t, alternate)
		dest := filepath.Join(t.TempDir(), "out.tar.gz")

		found, err := cache.CheckAny([]string{primary, alternate}, dest, checksum, "sha256")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected a cache hit under the alternate's key")
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Errorf("cached bytes: got %q, want %q", got, body)
		}
	})

	t.Run("single-source probe behaves as a direct Check", func(t *testing.T) {
		cache, checksum := newCacheWithEntryUnder(t, primary)
		dest := filepath.Join(t.TempDir(), "out.tar.gz")

		found, err := cache.CheckAny([]string{primary}, dest, checksum, "sha256")
		if err != nil || !found {
			t.Fatalf("expected a hit under the primary's key, got found=%v err=%v", found, err)
		}
	})

	t.Run("checksum mismatch is still a miss, per candidate", func(t *testing.T) {
		cache, _ := newCacheWithEntryUnder(t, alternate)
		dest := filepath.Join(t.TempDir(), "out.tar.gz")

		// SECURITY: CheckAny only changes which keys are probed. The checksum
		// verification inside Check is what decides whether cached bytes are
		// used, and it is unchanged.
		wrong := strings.Repeat("0", 64)
		found, err := cache.CheckAny([]string{primary, alternate}, dest, wrong, "sha256")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected a miss when the cached bytes do not match the pinned checksum")
		}
	})

	t.Run("no candidate hits", func(t *testing.T) {
		cache, checksum := newCacheWithEntryUnder(t, "https://z.example/other.tar.gz")
		dest := filepath.Join(t.TempDir(), "out.tar.gz")

		found, err := cache.CheckAny([]string{primary, alternate}, dest, checksum, "sha256")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected a miss when no candidate key is cached")
		}
	})
}

// TestDownloadFileAction_Execute_ExhaustsEverySource is the install-time face
// of R4: when the plan records several sources and all are unreachable, the
// error must name each one rather than only the last.
func TestDownloadFileAction_Execute_ExhaustsEverySource(t *testing.T) {
	action := &DownloadFileAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}

	err := action.Execute(ctx, map[string]interface{}{
		"url":      "https://first.invalid/tool-1.0.0.tar.gz",
		"checksum": strings.Repeat("a", 64),
		FallbackURLsParam: []interface{}{
			"https://second.invalid/tool-1.0.0.tar.gz",
		},
	})
	if err == nil {
		t.Fatal("expected an error when every source is unreachable")
	}
	for _, want := range []string{"first.invalid", "second.invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestAllSourcesFailedError_UnwrapsToPrimary(t *testing.T) {
	sentinel := errors.New("primary failed")
	err := newAllSourcesFailedError([]sourceFailure{
		{URL: "https://a.example/x", Err: sentinel},
		{URL: "https://b.example/x", Err: errors.New("secondary failed")},
	})

	// Callers that checked the primary's error before fallback existed keep
	// working.
	if !errors.Is(err, sentinel) {
		t.Errorf("expected the aggregate error to unwrap to the primary failure")
	}
}

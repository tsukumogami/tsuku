package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/version"
)

// fallbackMockServer creates an httptest server that mimics the GitHub API for
// version fallback tests. It serves:
//   - /rate_limit                                   → 5000/4999 remaining
//   - /repos/<repo>/releases/tags/<tag>             → dispatch per releaseAssets map
//   - /repos/<repo>/tags                            → tagList in order
func fallbackMockServer(t *testing.T, repo string, releaseAssets map[string][]string, tagList []string) *httptest.Server {
	t.Helper()
	owner, repoName, _ := strings.Cut(repo, "/")

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// The go-github enterprise client adds an /api/v3/ prefix.
		releasesTagsPrefix := fmt.Sprintf("/api/v3/repos/%s/%s/releases/tags/", owner, repoName)
		tagsPath := fmt.Sprintf("/api/v3/repos/%s/%s/tags", owner, repoName)

		switch {
		case strings.HasSuffix(r.URL.Path, "/rate_limit"):
			resetTime := time.Now().Add(time.Hour).Unix()
			fmt.Fprintf(w, `{"resources":{"core":{"limit":5000,"remaining":4999,"reset":%d,"used":1}}}`, resetTime)

		case strings.HasPrefix(r.URL.Path, releasesTagsPrefix):
			tag := strings.TrimPrefix(r.URL.Path, releasesTagsPrefix)
			assets, ok := releaseAssets[tag]
			if !ok {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
				return
			}
			type releaseAsset struct {
				Name string `json:"name"`
			}
			type release struct {
				Assets []releaseAsset `json:"assets"`
			}
			rel := release{}
			for _, a := range assets {
				rel.Assets = append(rel.Assets, releaseAsset{Name: a})
			}
			_ = json.NewEncoder(w).Encode(rel)

		case r.URL.Path == tagsPath:
			type tagEntry struct {
				Name string `json:"name"`
			}
			tags := make([]tagEntry, len(tagList))
			for i, t := range tagList {
				tags[i] = tagEntry{Name: t}
			}
			_ = json.NewEncoder(w).Encode(tags)

		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
}

// TestGitHubArchiveAction_Decompose_VersionFallback verifies that when the
// requested version has no matching asset, Decompose falls back to the preceding
// version, returns its steps, and fires Warn with the "version_fallback:" prefix.
func TestGitHubArchiveAction_Decompose_VersionFallback(t *testing.T) {
	t.Parallel()

	repo := "testowner/tool-fallback-test"
	// v2.0.0 has assets that don't match the pattern (different OS suffix)
	// v1.0.0 has the matching asset
	// v2.0.0 release exists but has only a darwin asset — won't match the linux pattern.
	// v1.0.0 has the linux asset we need.
	server := fallbackMockServer(t, repo,
		map[string][]string{
			"v2.0.0": {"tool-2.0.0-darwin-amd64.tar.gz"},
			"v1.0.0": {"tool-1.0.0-linux-amd64.tar.gz"},
		},
		[]string{"v2.0.0", "v1.0.0"},
	)
	defer server.Close()

	resolver := version.New(version.WithGitHubBaseURL(server.URL+"/", server.URL+"/"))
	reporter := progress.NewInboxReporter("tool-fallback-test", t.TempDir())

	action := &GitHubArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "2.0.0",
		VersionTag: "v2.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Resolver:   resolver,
		Reporter:   reporter,
	}

	// assetPattern expands to "tool-2.0.0-linux*.tar.gz" — wildcard triggers resolver path.
	steps, err := action.Decompose(ctx, map[string]interface{}{
		"repo":          repo,
		"asset_pattern": "tool-{version}-{os}*.tar.gz",
		"binaries":      []interface{}{"tool-fallback-test"},
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("Decompose() returned no steps")
	}

	// The download URL must use the fallback version tag (v1.0.0), not v2.0.0.
	downloadURL, _ := steps[0].Params["url"].(string)
	if !strings.Contains(downloadURL, "v1.0.0") {
		t.Errorf("download URL should contain fallback version v1.0.0, got %q", downloadURL)
	}
	if strings.Contains(downloadURL, "v2.0.0") {
		t.Errorf("download URL must not contain requested version v2.0.0, got %q", downloadURL)
	}

	// The asset name should be the fallback asset.
	assetName, _ := steps[1].Params["archive"].(string)
	if assetName != "tool-1.0.0-linux-amd64.tar.gz" {
		t.Errorf("archive = %q, want %q", assetName, "tool-1.0.0-linux-amd64.tar.gz")
	}

	// Reporter must have received a version_fallback warn.
	reporter.Stop()
}

// TestGitHubArchiveAction_Decompose_NilReporterNoPanic verifies that Decompose
// does not panic when EvalContext.Reporter is nil (version fallback path).
func TestGitHubArchiveAction_Decompose_NilReporterNoPanic(t *testing.T) {
	t.Parallel()

	repo := "testowner/tool-nil-reporter-test"
	// Same setup as the VersionFallback test: v2.0.0 has darwin-only assets.
	server := fallbackMockServer(t, repo,
		map[string][]string{
			"v2.0.0": {"tool-2.0.0-darwin-amd64.tar.gz"},
			"v1.0.0": {"tool-1.0.0-linux-amd64.tar.gz"},
		},
		[]string{"v2.0.0", "v1.0.0"},
	)
	defer server.Close()

	resolver := version.New(version.WithGitHubBaseURL(server.URL+"/", server.URL+"/"))

	action := &GitHubArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "2.0.0",
		VersionTag: "v2.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Resolver:   resolver,
		Reporter:   nil, // explicitly nil
	}

	// Must not panic; GetReporter() returns NoopReporter when Reporter is nil.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Decompose panicked with nil Reporter: %v", r)
		}
	}()

	_, _ = action.Decompose(ctx, map[string]interface{}{
		"repo":          repo,
		"asset_pattern": "tool-{version}-{os}*.tar.gz",
		"binaries":      []interface{}{"tool-nil-reporter-test"},
	})
}

// TestEvalContext_GetReporter_NilReturnsNoop verifies GetReporter returns
// NoopReporter{} when Reporter is nil.
func TestEvalContext_GetReporter_NilReturnsNoop(t *testing.T) {
	ctx := &EvalContext{}
	r := ctx.GetReporter()
	if _, ok := r.(progress.NoopReporter); !ok {
		t.Errorf("GetReporter() = %T, want progress.NoopReporter", r)
	}
}

// TestEvalContext_GetReporter_ReturnsSetReporter verifies GetReporter returns
// the Reporter that was set.
func TestEvalContext_GetReporter_ReturnsSetReporter(t *testing.T) {
	reporter := progress.NewInboxReporter("tool", t.TempDir())
	ctx := &EvalContext{Reporter: reporter}
	if ctx.GetReporter() != reporter {
		t.Error("GetReporter() should return the set reporter")
	}
}

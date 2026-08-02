package actions

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func evalCtxWith(d Downloader) *EvalContext {
	return &EvalContext{
		Context:    context.Background(),
		Version:    "0.14.1",
		VersionTag: "0.14.1",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
		Downloader: d,
	}
}

func downloadStepFrom(t *testing.T, steps []Step) Step {
	t.Helper()
	for _, s := range steps {
		if s.Action == "download_file" {
			return s
		}
	}
	t.Fatal("no download_file step in decomposition")
	return Step{}
}

// TestDownloadArchiveDecompose_SingleSourceIsUnchanged is the byte-identity
// guard (PRD R13). A recipe with no fallback_urls must produce a params map
// with no fallback_urls key, because that map is serialized straight into the
// generated plan and 96 golden files depend on it not moving.
func TestDownloadArchiveDecompose_SingleSourceIsUnchanged(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{
		"https://a.example/tool-0.14.1-linux-amd64.tar.gz": "archive bytes",
	}}

	action := &DownloadArchiveAction{}
	steps, err := action.Decompose(evalCtxWith(d), map[string]interface{}{
		"url":      "https://a.example/tool-{version}-{os}-{arch}.tar.gz",
		"binaries": []interface{}{"tool"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := downloadStepFrom(t, steps)
	if _, present := step.Params[FallbackURLsParam]; present {
		t.Error("a single-source recipe must not emit fallback_urls into the plan")
	}

	wantKeys := []string{"url", "dest", "checksum", "checksum_algo"}
	gotKeys := make([]string, 0, len(step.Params))
	for k := range step.Params {
		gotKeys = append(gotKeys, k)
	}
	if len(gotKeys) != len(wantKeys) {
		t.Errorf("params keys: got %v, want exactly %v", gotKeys, wantKeys)
	}
}

// TestDownloadArchiveDecompose_RecordsExpandedFallbacks covers R1, R5 and the
// expansion half of R2: alternates get the same placeholder and mapping
// treatment as url, and the whole ordered list lands in the plan.
func TestDownloadArchiveDecompose_RecordsExpandedFallbacks(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{
		"https://a.example/tool-0.14.1-linux-x86_64.tar.gz": "archive bytes",
	}}

	action := &DownloadArchiveAction{}
	steps, err := action.Decompose(evalCtxWith(d), map[string]interface{}{
		"url": "https://a.example/tool-{version}-{os}-{arch}.tar.gz",
		FallbackURLsParam: []interface{}{
			"https://b.example/tool-{version}-{os}-{arch}.tar.gz",
			"https://c.example/tool-{version}-{os}-{arch}.tar.gz",
		},
		"arch_mapping": map[string]interface{}{"amd64": "x86_64"},
		"binaries":     []interface{}{"tool"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := downloadStepFrom(t, steps)
	got, ok := GetStringSlice(step.Params, FallbackURLsParam)
	if !ok {
		t.Fatal("expected fallback_urls in the emitted step")
	}
	want := []string{
		"https://b.example/tool-0.14.1-linux-x86_64.tar.gz",
		"https://c.example/tool-0.14.1-linux-x86_64.tar.gz",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fallback_urls: got %v, want %v", got, want)
	}
}

// TestDownloadArchiveDecompose_FallsThroughAtPlanTime is the CI-survives-a-
// dead-first-source journey: the plan that comes out must be the plan the
// first source would have produced, so nothing downstream can tell which
// source answered.
func TestDownloadArchiveDecompose_FallsThroughAtPlanTime(t *testing.T) {
	const body = "archive bytes"

	params := func() map[string]interface{} {
		return map[string]interface{}{
			"url":             "https://dead.example/tool-{version}.tar.gz",
			FallbackURLsParam: []interface{}{"https://live.example/tool-{version}.tar.gz"},
			"binaries":        []interface{}{"tool"},
		}
	}

	action := &DownloadArchiveAction{}

	// First source dead, second alive.
	degraded := &scriptedDownloader{serve: map[string]string{
		"https://live.example/tool-0.14.1.tar.gz": body,
	}}
	degradedSteps, err := action.Decompose(evalCtxWith(degraded), params())
	if err != nil {
		t.Fatalf("expected plan generation to survive a dead first source, got %v", err)
	}

	// Both sources alive.
	healthy := &scriptedDownloader{serve: map[string]string{
		"https://dead.example/tool-0.14.1.tar.gz": body,
		"https://live.example/tool-0.14.1.tar.gz": body,
	}}
	healthySteps, err := action.Decompose(evalCtxWith(healthy), params())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	degradedStep := downloadStepFrom(t, degradedSteps)
	healthyStep := downloadStepFrom(t, healthySteps)

	if !reflect.DeepEqual(degradedStep.Params, healthyStep.Params) {
		t.Errorf("plan differs by which source answered:\n  degraded: %v\n  healthy:  %v",
			degradedStep.Params, healthyStep.Params)
	}
	if degradedStep.Checksum != healthyStep.Checksum {
		t.Errorf("checksum differs by which source answered: %q vs %q",
			degradedStep.Checksum, healthyStep.Checksum)
	}
	// The primary is what "url" names, whether or not it served. Rewriting it
	// to the source that answered would make plans depend on network weather.
	if url, _ := GetString(degradedStep.Params, "url"); url != "https://dead.example/tool-0.14.1.tar.gz" {
		t.Errorf("url was rewritten to the serving source: %q", url)
	}
}

func TestDownloadArchiveDecompose_AllSourcesFail(t *testing.T) {
	d := &scriptedDownloader{serve: map[string]string{}}

	action := &DownloadArchiveAction{}
	_, err := action.Decompose(evalCtxWith(d), map[string]interface{}{
		"url":             "https://a.example/tool-{version}.tar.gz",
		FallbackURLsParam: []interface{}{"https://b.example/tool-{version}.tar.gz"},
		"binaries":        []interface{}{"tool"},
	})
	if err == nil {
		t.Fatal("expected plan generation to fail when every source fails")
	}
	for _, want := range []string{"a.example", "b.example"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestDownloadArchivePreflight_FallbackURLs(t *testing.T) {
	action := &DownloadArchiveAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		wantErrors  []string
		wantWarning string
	}{
		{
			name: "non-HTTPS alternate is an error",
			params: map[string]interface{}{
				"url":             "https://a.example/tool-{version}.tar.gz",
				FallbackURLsParam: []interface{}{"http://b.example/tool-{version}.tar.gz"},
			},
			wantErrors: []string{"fallback_urls[0] must use HTTPS"},
		},
		{
			name: "empty alternate is an error",
			params: map[string]interface{}{
				"url":             "https://a.example/tool-{version}.tar.gz",
				FallbackURLsParam: []interface{}{""},
			},
			wantErrors: []string{"fallback_urls[0] is empty"},
		},
		{
			name: "static alternate against a version-templated url warns",
			params: map[string]interface{}{
				"url":             "https://a.example/tool-{version}.tar.gz",
				FallbackURLsParam: []interface{}{"https://b.example/tool-0.14.1.tar.gz"},
			},
			wantWarning: "no {version} placeholder",
		},
		{
			name: "duplicate of the primary warns",
			params: map[string]interface{}{
				"url":             "https://a.example/tool-{version}.tar.gz",
				FallbackURLsParam: []interface{}{"https://a.example/tool-{version}.tar.gz"},
			},
			wantWarning: "duplicates an earlier source",
		},
		{
			// `tsuku validate --strict` promotes warnings to errors and CI
			// validates every recipe that way, so a warning here would make
			// upstream anchoring mandatory in practice -- which is what the
			// design rejected, because zig cannot satisfy it.
			name: "alternates without an upstream anchor produce no diagnostics",
			params: map[string]interface{}{
				"url":             "https://a.example/tool-{version}.tar.gz",
				FallbackURLsParam: []interface{}{"https://b.example/tool-{version}.tar.gz"},
			},
		},
		{
			name: "a single-source recipe gets no fallback diagnostics at all",
			params: map[string]interface{}{
				"url": "https://a.example/tool-{version}.tar.gz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)

			for _, want := range tt.wantErrors {
				if !anyContains(result.Errors, want) {
					t.Errorf("expected an error containing %q, got %v", want, result.Errors)
				}
			}
			if tt.wantWarning != "" && !anyContains(result.Warnings, tt.wantWarning) {
				t.Errorf("expected a warning containing %q, got %v", tt.wantWarning, result.Warnings)
			}
			if len(tt.wantErrors) == 0 && tt.wantWarning == "" {
				if anyContains(result.Errors, "fallback_urls") || anyContains(result.Warnings, "fallback_urls") {
					t.Errorf("single-source recipe produced fallback diagnostics: %v %v",
						result.Errors, result.Warnings)
				}
			}
		})
	}
}

func anyContains(messages []string, needle string) bool {
	for _, m := range messages {
		if strings.Contains(m, needle) {
			return true
		}
	}
	return false
}

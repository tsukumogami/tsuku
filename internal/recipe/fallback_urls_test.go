package recipe

import (
	"strings"
	"testing"
)

func fallbackStepResult(t *testing.T, fallbacks interface{}) *ValidationResult {
	t.Helper()
	result := &ValidationResult{}
	step := &Step{
		Action: "download_archive",
		Params: map[string]interface{}{
			"url":           "https://a.example/tool-{version}.tar.gz",
			"fallback_urls": fallbacks,
		},
	}
	validatePathParams(result, "steps[0]", step)
	return result
}

// TestValidateFallbackURLs covers PRD R9: a rule that holds for url holds for
// every alternate. Without this the validator quietly stops applying to the
// entries nobody looks at until an outage.
func TestValidateFallbackURLs(t *testing.T) {
	tests := []struct {
		name      string
		fallbacks interface{}
		wantError string
	}{
		{
			name:      "valid https alternates pass",
			fallbacks: []interface{}{"https://b.example/tool-1.0.tar.gz"},
		},
		{
			name:      "an ftp alternate is rejected the same way an ftp url is",
			fallbacks: []interface{}{"ftp://b.example/tool-1.0.tar.gz"},
			wantError: "URL scheme must be http or https",
		},
		{
			name:      "a non-array value is rejected",
			fallbacks: "https://b.example/tool-1.0.tar.gz",
			wantError: "must be an array of URL strings",
		},
		{
			name:      "a non-string entry is rejected",
			fallbacks: []interface{}{42},
			wantError: "entries must be strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fallbackStepResult(t, tt.fallbacks)

			if tt.wantError == "" {
				if len(result.Errors) != 0 {
					t.Errorf("expected no errors, got %v", result.Errors)
				}
				return
			}

			found := false
			for _, e := range result.Errors {
				if strings.Contains(e.Message, tt.wantError) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected an error containing %q, got %v", tt.wantError, result.Errors)
			}
		})
	}
}

// TestDetectHardcodedVersions_FallbackURLs covers the list-shaped version rule:
// a hardcoded version hiding in the third alternate must be reported against
// that alternate, not silently skipped because the field is an array.
func TestDetectHardcodedVersions_FallbackURLs(t *testing.T) {
	r := &Recipe{
		Steps: []Step{
			{
				Action: "download_archive",
				Params: map[string]interface{}{
					"url": "https://a.example/tool-{version}.tar.gz",
					"fallback_urls": []interface{}{
						"https://b.example/tool-{version}.tar.gz",
						"https://c.example/tool-1.2.3.tar.gz",
					},
				},
			},
		},
	}

	detected := DetectHardcodedVersions(r)
	if len(detected) != 1 {
		t.Fatalf("expected exactly one detection, got %v", detected)
	}
	if detected[0].Field != "fallback_urls[1]" {
		t.Errorf("field: got %q, want fallback_urls[1]", detected[0].Field)
	}
	if detected[0].Value != "1.2.3" {
		t.Errorf("value: got %q, want 1.2.3", detected[0].Value)
	}
}

// A single-source recipe must not acquire new diagnostics — the version rules
// gained a list-shaped entry, and a nil params lookup must stay a no-op.
func TestDetectHardcodedVersions_SingleSourceUnaffected(t *testing.T) {
	r := &Recipe{
		Steps: []Step{
			{
				Action: "download_archive",
				Params: map[string]interface{}{
					"url": "https://a.example/tool-{version}.tar.gz",
				},
			},
		},
	}

	if detected := DetectHardcodedVersions(r); len(detected) != 0 {
		t.Errorf("expected no detections for a single-source recipe, got %v", detected)
	}
}

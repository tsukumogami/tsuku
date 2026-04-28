package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gcloudFixture mimics https://dl.google.com/dl/cloudsdk/channels/rapid/components-2.json
// at the relevant top-level field.
const gcloudFixture = `{"version":"566.0.0","components":[],"revision":"20260424041539"}`

// hashicorpCheckpointFixture mimics https://checkpoint-api.hashicorp.com/v1/check/<product>
const hashicorpCheckpointFixture = `{"product":"terraform","current_version":"1.20.4","current_release":1700000000}`

// adoptiumAvailableFixture mimics https://api.adoptium.net/v3/info/available_releases
const adoptiumAvailableFixture = `{"available_releases":[8,11,17,21,25],"available_lts_releases":[8,11,17,21],"most_recent_lts":21,"most_recent_feature_release":25}`

// adoptiumAssetsFixture mimics https://api.adoptium.net/v3/assets/latest/<n>/hotspot
const adoptiumAssetsFixture = `[{"version":{"openjdk_version":"21.0.4+7"}}]`

func TestHTTPJSONProvider_ResolveLatest(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		path        string
		wantVersion string
	}{
		{"gcloud", gcloudFixture, "version", "566.0.0"},
		{"hashicorp checkpoint", hashicorpCheckpointFixture, "current_version", "1.20.4"},
		{"adoptium most recent LTS", adoptiumAvailableFixture, "most_recent_lts", "21"},
		{"adoptium binary version", adoptiumAssetsFixture, "[0].version.openjdk_version", "21.0.4+7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.fixture))
			}))
			defer server.Close()

			resolver := New()
			provider, err := NewHTTPJSONProvider(resolver, server.URL, tt.path)
			if err != nil {
				t.Fatalf("NewHTTPJSONProvider failed: %v", err)
			}

			info, err := provider.ResolveLatest(context.Background())
			if err != nil {
				t.Fatalf("ResolveLatest failed: %v", err)
			}
			if info.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", info.Version, tt.wantVersion)
			}
			if info.Tag != tt.wantVersion {
				t.Errorf("Tag = %q, want %q", info.Tag, tt.wantVersion)
			}
		})
	}
}

func TestHTTPJSONProvider_ErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		path      string
		wantError string
	}{
		{
			name: "non-200 status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			path:      "version",
			wantError: "returned status 404",
		},
		{
			name: "invalid JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("not json"))
			},
			path:      "version",
			wantError: "failed to parse JSON",
		},
		{
			name: "path miss",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"different_field":"1.0"}`))
			},
			path:      "version",
			wantError: "key not found",
		},
		{
			name: "leaf is object",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"version":{"major":1}}`))
			},
			path:      "version",
			wantError: "expected string or number at leaf",
		},
		{
			name: "empty string version",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"version":""}`))
			},
			path:      "version",
			wantError: "resolved to empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			resolver := New()
			provider, err := NewHTTPJSONProvider(resolver, server.URL, tt.path)
			if err != nil {
				t.Fatalf("NewHTTPJSONProvider failed: %v", err)
			}

			_, err = provider.ResolveLatest(context.Background())
			if err == nil {
				t.Fatalf("ResolveLatest succeeded; want error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestHTTPJSONProvider_ResponseSizeCap(t *testing.T) {
	// Respond with maxHTTPJSONResponseSize+1 bytes of valid JSON to trip the cap.
	// Valid-shaped JSON to make the failure unambiguous (we want the size cap
	// error, not a parse error).
	big := strings.Repeat("a", maxHTTPJSONResponseSize)
	body := `{"version":"` + big + `"}` // > maxHTTPJSONResponseSize bytes

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	resolver := New()
	provider, err := NewHTTPJSONProvider(resolver, server.URL, "version")
	if err != nil {
		t.Fatalf("NewHTTPJSONProvider failed: %v", err)
	}

	_, err = provider.ResolveLatest(context.Background())
	if err == nil {
		t.Fatal("ResolveLatest succeeded; want size-cap error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %q, want substring %q", err.Error(), "exceeds")
	}
}

func TestHTTPJSONProvider_BadPathRejectedAtConstruction(t *testing.T) {
	resolver := New()
	_, err := NewHTTPJSONProvider(resolver, "https://example.com/manifest.json", "")
	if err == nil {
		t.Fatal("NewHTTPJSONProvider with empty path succeeded; want error")
	}
	if !strings.Contains(err.Error(), "invalid version_path") {
		t.Errorf("error = %q, want substring %q", err.Error(), "invalid version_path")
	}
}

func TestHTTPJSONProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider, err := NewHTTPJSONProvider(resolver, "https://example.com/manifest.json", "version")
	if err != nil {
		t.Fatalf("NewHTTPJSONProvider failed: %v", err)
	}
	got := provider.SourceDescription()
	want := "http_json:https://example.com/manifest.json"
	if got != want {
		t.Errorf("SourceDescription = %q, want %q", got, want)
	}
}

package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Compile-time interface assertions.
var (
	_ EcosystemProber = (*CargoBuilder)(nil)
	_ EcosystemProber = (*PyPIBuilder)(nil)
	_ EcosystemProber = (*NpmBuilder)(nil)
	_ EcosystemProber = (*GemBuilder)(nil)
	_ EcosystemProber = (*GoBuilder)(nil)
	_ EcosystemProber = (*CPANBuilder)(nil)
	_ EcosystemProber = (*CaskBuilder)(nil)
	_ EcosystemProber = (*HomebrewBuilder)(nil)
)

func TestCargoBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/ripgrep" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"crate":{"name":"ripgrep","recent_downloads":5000000,"repository":"https://github.com/BurntSushi/ripgrep"},"versions":[{},{},{},{},{},{},{},{},{},{}]}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists with metadata", func(t *testing.T) {
		result, err := builder.Probe(ctx, "ripgrep")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "ripgrep" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "ripgrep")
		}
		if result.Downloads != 5000000 {
			t.Errorf("Probe() Downloads = %d, want 5000000", result.Downloads)
		}
		if result.VersionCount != 10 {
			t.Errorf("Probe() VersionCount = %d, want 10", result.VersionCount)
		}
		if !result.HasRepository {
			t.Error("Probe() HasRepository = false, want true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-crate-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestCargoBuilder_Probe_NoRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crate":{"name":"squatter","recent_downloads":50},"versions":[{},{}]}`))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Probe(context.Background(), "squatter")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil")
	}
	if result.HasRepository {
		t.Error("Probe() HasRepository = true, want false")
	}
	if result.Downloads != 50 {
		t.Errorf("Probe() Downloads = %d, want 50", result.Downloads)
	}
	if result.VersionCount != 2 {
		t.Errorf("Probe() VersionCount = %d, want 2", result.VersionCount)
	}
}

func TestPyPIBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pypi/black/json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"black","project_urls":{"Homepage":"https://github.com/psf/black","Repository":"https://github.com/psf/black"}},"releases":{"20.8b1":[],"21.4b0":[],"22.1.0":[],"23.1.0":[],"24.1.0":[]}}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists with metadata", func(t *testing.T) {
		result, err := builder.Probe(ctx, "black")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "black" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "black")
		}
		if result.VersionCount != 5 {
			t.Errorf("Probe() VersionCount = %d, want 5", result.VersionCount)
		}
		if !result.HasRepository {
			t.Error("Probe() HasRepository = false, want true")
		}
		if result.Downloads != 0 {
			t.Errorf("Probe() Downloads = %d, want 0 (PyPI doesn't expose downloads)", result.Downloads)
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-pkg-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestPyPIBuilder_Probe_NoProjectURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"info":{"name":"squatter"},"releases":{"0.0.1":[]}}`))
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Probe(context.Background(), "squatter")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil")
	}
	if result.HasRepository {
		t.Error("Probe() HasRepository = true, want false")
	}
	if result.VersionCount != 1 {
		t.Errorf("Probe() VersionCount = %d, want 1", result.VersionCount)
	}
}

func TestNpmBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prettier":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"prettier","repository":{"type":"git","url":"git+https://github.com/prettier/prettier.git"},"versions":{"1.0.0":{},"2.0.0":{},"3.0.0":{}}}`))
		case "/downloads/point/last-week/prettier":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"downloads":25000000}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists with metadata", func(t *testing.T) {
		result, err := builder.Probe(ctx, "prettier")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "prettier" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "prettier")
		}
		if result.Downloads != 25000000 {
			t.Errorf("Probe() Downloads = %d, want 25000000", result.Downloads)
		}
		if result.VersionCount != 3 {
			t.Errorf("Probe() VersionCount = %d, want 3", result.VersionCount)
		}
		if !result.HasRepository {
			t.Error("Probe() HasRepository = false, want true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-pkg-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestNpmBuilder_Probe_DownloadsAPIFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/some-pkg":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"some-pkg","versions":{"1.0.0":{},"2.0.0":{}}}`))
		case "/downloads/point/last-week/some-pkg":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Probe(context.Background(), "some-pkg")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil")
	}
	if result.Downloads != 0 {
		t.Errorf("Probe() Downloads = %d, want 0 (API failed)", result.Downloads)
	}
	if result.VersionCount != 2 {
		t.Errorf("Probe() VersionCount = %d, want 2", result.VersionCount)
	}
}

func TestGemBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/gems/rubocop.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"rubocop"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "rubocop")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "rubocop" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "rubocop")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-gem-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestGoBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/github.com/spf13/cobra/@latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Version":"v1.8.0","Time":"2023-11-01T12:00:00Z"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "github.com/spf13/cobra")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "github.com/spf13/cobra" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "github.com/spf13/cobra")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "github.com/nonexistent/module")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestCPANBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/App-Ack" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"distribution":"App-Ack","version":"3.7.0"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "App-Ack")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "App-Ack" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "App-Ack")
		}
	})

	t.Run("module notation", func(t *testing.T) {
		result, err := builder.Probe(ctx, "App::Ack")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil (:: should normalize to -)")
		}
		if result.Source != "App-Ack" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "App-Ack")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "Nonexistent-Module")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestCaskBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cask/visual-studio-code.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"visual-studio-code","version":"1.96.4","homepage":"https://code.visualstudio.com/"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCaskBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists with metadata", func(t *testing.T) {
		result, err := builder.Probe(ctx, "visual-studio-code")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "visual-studio-code" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "visual-studio-code")
		}
		if !result.HasRepository {
			t.Error("Probe() HasRepository = false, want true (has homepage)")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-cask-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestHomebrewBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/jq.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"jq","full_name":"jq","desc":"Lightweight JSON processor","homepage":"https://jqlang.github.io/jq/","versions":{"stable":"1.7.1","bottle":true},"deprecated":false,"disabled":false,"analytics":{"install":{"365d":{"jq":638284}}}}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewHomebrewBuilder(WithHomebrewAPIURL(server.URL))
	ctx := context.Background()

	t.Run("exists with metadata", func(t *testing.T) {
		result, err := builder.Probe(ctx, "jq")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result == nil {
			t.Fatal("Probe() returned nil, want non-nil")
		}
		if result.Source != "jq" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "jq")
		}
		if !result.HasRepository {
			t.Error("Probe() HasRepository = false, want true (has homepage)")
		}
		if result.Downloads != 638284 {
			t.Errorf("Probe() Downloads = %d, want 638284", result.Downloads)
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-formula-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result != nil {
			t.Errorf("Probe() = %+v, want nil", result)
		}
	})
}

func TestProbe_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()

	builders := []struct {
		name    string
		builder EcosystemProber
	}{
		{"cargo", NewCargoBuilderWithBaseURL(nil, server.URL)},
		{"pypi", NewPyPIBuilderWithBaseURL(nil, server.URL)},
		{"npm", NewNpmBuilderWithBaseURL(nil, server.URL)},
		{"gem", NewGemBuilderWithBaseURL(nil, server.URL)},
		{"go", NewGoBuilderWithBaseURL(nil, server.URL)},
		{"cpan", NewCPANBuilderWithBaseURL(nil, server.URL)},
		{"cask", NewCaskBuilderWithBaseURL(nil, server.URL)},
		{"homebrew", NewHomebrewBuilder(WithHomebrewAPIURL(server.URL))},
	}

	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			result, err := b.builder.Probe(ctx, "some-package")
			if err != nil {
				t.Fatalf("Probe() returned error for %s: %v", b.name, err)
			}
			if result != nil {
				t.Errorf("Probe() = %+v for %s on 500, want nil", result, b.name)
			}
		})
	}
}

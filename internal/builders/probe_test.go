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
)

func TestCargoBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/ripgrep" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"crate":{"name":"ripgrep"}}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "ripgrep")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
		}
		if result.Source != "ripgrep" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "ripgrep")
		}
		if result.Downloads != 0 {
			t.Errorf("Probe() Downloads = %d, want 0", result.Downloads)
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-crate-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
		}
	})
}

func TestPyPIBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pypi/black/json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"black"}}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "black")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
		}
		if result.Source != "black" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "black")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-pkg-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
		}
	})
}

func TestNpmBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prettier" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"prettier"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "prettier")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
		}
		if result.Source != "prettier" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "prettier")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-pkg-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
		}
	})
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
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
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
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
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

	t.Run("exists with age", func(t *testing.T) {
		result, err := builder.Probe(ctx, "github.com/spf13/cobra")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
		}
		if result.Source != "github.com/spf13/cobra" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "github.com/spf13/cobra")
		}
		if result.Age <= 0 {
			t.Errorf("Probe() Age = %d, want > 0", result.Age)
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "github.com/nonexistent/module")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
		}
	})

	t.Run("invalid time", func(t *testing.T) {
		badTimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Version":"v1.0.0","Time":"not-a-date"}`))
		}))
		defer badTimeServer.Close()

		b := NewGoBuilderWithBaseURL(nil, badTimeServer.URL)
		result, err := b.Probe(ctx, "github.com/example/mod")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
		}
		if result.Age != 0 {
			t.Errorf("Probe() Age = %d, want 0 for invalid time", result.Age)
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
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
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
		if !result.Exists {
			t.Error("Probe() Exists = false, want true (:: should normalize to -)")
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
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
		}
	})
}

func TestCaskBuilder_Probe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cask/visual-studio-code.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"visual-studio-code","version":"1.96.4"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCaskBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		result, err := builder.Probe(ctx, "visual-studio-code")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if !result.Exists {
			t.Error("Probe() Exists = false, want true")
		}
		if result.Source != "visual-studio-code" {
			t.Errorf("Probe() Source = %q, want %q", result.Source, "visual-studio-code")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, err := builder.Probe(ctx, "nonexistent-cask-xyz")
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if result.Exists {
			t.Error("Probe() Exists = true, want false")
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
	}

	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			result, err := b.builder.Probe(ctx, "some-package")
			if err != nil {
				t.Fatalf("Probe() returned error for %s: %v", b.name, err)
			}
			if result.Exists {
				t.Errorf("Probe() Exists = true for %s on 500, want false", b.name)
			}
		})
	}
}

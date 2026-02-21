package seed

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/builders"
)

// mockProber implements builders.EcosystemProber for testing.
type mockProber struct {
	name   string
	result *builders.ProbeResult
	err    error
}

func (m *mockProber) Name() string      { return m.name }
func (m *mockProber) RequiresLLM() bool { return false }
func (m *mockProber) CanBuild(_ context.Context, _ builders.BuildRequest) (bool, error) {
	return false, nil
}
func (m *mockProber) NewSession(_ context.Context, _ builders.BuildRequest, _ *builders.SessionOptions) (builders.BuildSession, error) {
	return nil, nil
}
func (m *mockProber) Probe(_ context.Context, _ string) (*builders.ProbeResult, error) {
	return m.result, m.err
}

func TestDisambiguator_ResolvesSingleMatch(t *testing.T) {
	probers := []builders.EcosystemProber{
		&mockProber{name: "npm", result: nil},
		&mockProber{name: "pypi", result: &builders.ProbeResult{
			Source: "flask", Downloads: 50000, VersionCount: 30,
		}},
	}

	d := NewDisambiguator(probers, 5*time.Second)
	rr, err := d.Resolve(context.Background(), "flask")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Selected == nil {
		t.Fatal("expected selected result")
	}
	if rr.Selected.Builder != "pypi" {
		t.Errorf("expected builder pypi, got %s", rr.Selected.Builder)
	}
	if len(rr.AllProbes) != 2 {
		t.Errorf("expected 2 probes, got %d", len(rr.AllProbes))
	}
}

func TestDisambiguator_ForceDeterministic(t *testing.T) {
	// Close matches: forceDeterministic should select first ranked.
	probers := []builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{
			Source: "serve", Downloads: 1000, VersionCount: 10, HasRepository: true,
		}},
		&mockProber{name: "crates.io", result: &builders.ProbeResult{
			Source: "serve", Downloads: 500, VersionCount: 10, HasRepository: true,
		}},
	}

	d := NewDisambiguator(probers, 5*time.Second)
	rr, err := d.Resolve(context.Background(), "serve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Selected == nil {
		t.Fatal("expected selected result (deterministic fallback)")
	}
	// Should get a result rather than AmbiguousMatchError.
	if rr.Selected.Metadata.SelectionReason != "priority_fallback" {
		t.Errorf("expected priority_fallback, got %s", rr.Selected.Metadata.SelectionReason)
	}
}

func TestDisambiguator_NoMatch(t *testing.T) {
	probers := []builders.EcosystemProber{
		&mockProber{name: "npm", result: nil},
	}

	d := NewDisambiguator(probers, 5*time.Second)
	rr, err := d.Resolve(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Selected != nil {
		t.Fatalf("expected nil selected, got %+v", rr.Selected)
	}
}

func TestDisambiguator_AllErrors(t *testing.T) {
	probers := []builders.EcosystemProber{
		&mockProber{name: "npm", err: fmt.Errorf("fail")},
		&mockProber{name: "pypi", err: fmt.Errorf("fail")},
	}

	d := NewDisambiguator(probers, 5*time.Second)
	_, err := d.Resolve(context.Background(), "tool")
	if err == nil {
		t.Fatal("expected error when all probers fail")
	}
}

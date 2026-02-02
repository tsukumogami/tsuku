package discover

import (
	"context"
	"errors"
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
	delay  time.Duration
}

func (m *mockProber) Name() string      { return m.name }
func (m *mockProber) RequiresLLM() bool { return false }
func (m *mockProber) CanBuild(_ context.Context, _ builders.BuildRequest) (bool, error) {
	return false, nil
}
func (m *mockProber) NewSession(_ context.Context, _ builders.BuildRequest, _ *builders.SessionOptions) (builders.BuildSession, error) {
	return nil, nil
}

func (m *mockProber) Probe(ctx context.Context, _ string) (*builders.ProbeResult, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.result, m.err
}

func TestEcosystemProbe_ZeroResults(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: false}},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Exists: false}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
}

func TestEcosystemProbe_SingleResult(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: false}},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Exists: true, Source: "flask", Downloads: 1000}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "flask")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Builder != "pypi" {
		t.Errorf("expected builder pypi, got %s", result.Builder)
	}
	if result.Confidence != ConfidenceEcosystem {
		t.Errorf("expected confidence %s, got %s", ConfidenceEcosystem, result.Confidence)
	}
}

func TestEcosystemProbe_MultipleResults_PriorityRanking(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: true, Source: "serve"}},
		&mockProber{name: "cask", result: &builders.ProbeResult{Exists: true, Source: "serve"}},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Exists: true, Source: "serve"}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "serve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// cask has priority 1 (highest)
	if result.Builder != "cask" {
		t.Errorf("expected cask (highest priority), got %s", result.Builder)
	}
}

func TestEcosystemProbe_NameMismatch(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: true, Source: "other-tool"}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "my-tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for name mismatch, got %+v", result)
	}
}

func TestEcosystemProbe_NameMatchCaseInsensitive(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: true, Source: "Flask"}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "flask")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result for case-insensitive match, got nil")
	}
}

func TestEcosystemProbe_AllFailures(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", err: fmt.Errorf("network error")},
		&mockProber{name: "pypi", err: fmt.Errorf("timeout")},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error when all probers fail")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
}

func TestEcosystemProbe_SoftErrors(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", err: fmt.Errorf("network error")},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Exists: true, Source: "flask"}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "flask")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result despite partial failure")
	}
	if result.Builder != "pypi" {
		t.Errorf("expected pypi, got %s", result.Builder)
	}
}

func TestEcosystemProbe_Timeout(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", delay: 5 * time.Second, result: &builders.ProbeResult{Exists: true, Source: "tool"}},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Exists: true, Source: "tool"}},
	}, 100*time.Millisecond)

	start := time.Now()
	result, err := probe.Resolve(context.Background(), "tool")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still get pypi result even though npm timed out
	if result == nil {
		t.Fatal("expected result from fast prober")
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout not enforced, took %v", elapsed)
	}
}

func TestEcosystemProbe_EmptyProbers(t *testing.T) {
	probe := NewEcosystemProbe(nil, 5*time.Second)
	result, err := probe.Resolve(context.Background(), "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %+v", result)
	}
}

func TestEcosystemProbe_MetadataPassthrough(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "pypi", result: &builders.ProbeResult{
			Exists: true, Source: "flask", Downloads: 50000, Age: 365,
		}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "flask")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata.Downloads != 50000 {
		t.Errorf("expected downloads 50000, got %d", result.Metadata.Downloads)
	}
	if result.Metadata.AgeDays != 365 {
		t.Errorf("expected age 365, got %d", result.Metadata.AgeDays)
	}
}

// Chain integration tests: EcosystemProbe wired into ChainResolver.

func TestChain_RegistryMissFallsToEcosystemProbe(t *testing.T) {
	registryMiss := &mockResolver{result: nil, err: nil}
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "crates.io", result: &builders.ProbeResult{Exists: true, Source: "ripgrep"}},
	}, 5*time.Second)

	chain := NewChainResolver(registryMiss, probe)
	result, err := chain.Resolve(context.Background(), "ripgrep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result from ecosystem probe")
	}
	if result.Builder != "crates.io" {
		t.Errorf("expected builder crates.io, got %s", result.Builder)
	}
	if result.Confidence != ConfidenceEcosystem {
		t.Errorf("expected confidence %s, got %s", ConfidenceEcosystem, result.Confidence)
	}
}

func TestChain_EcosystemProbeMissFallsThrough(t *testing.T) {
	registryMiss := &mockResolver{result: nil, err: nil}
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: false}},
	}, 5*time.Second)
	llmStub := &mockResolver{result: nil, err: nil}

	chain := NewChainResolver(registryMiss, probe, llmStub)
	_, err := chain.Resolve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected NotFoundError")
	}
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestChain_EcosystemProbeErrorIsSoft(t *testing.T) {
	registryMiss := &mockResolver{result: nil, err: nil}
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", err: fmt.Errorf("all registries down")},
	}, 5*time.Second)
	llmResult := &DiscoveryResult{Builder: "llm", Source: "fallback", Confidence: ConfidenceLLM}
	llmStage := &mockResolver{result: llmResult}

	chain := NewChainResolver(registryMiss, probe, llmStage)
	result, err := chain.Resolve(context.Background(), "sometool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "llm" {
		t.Errorf("expected fallback to llm, got %s", result.Builder)
	}
}

func TestChain_RegistryHitSkipsEcosystemProbe(t *testing.T) {
	registryResult := &DiscoveryResult{Builder: "github", Source: "cli/cli", Confidence: ConfidenceRegistry}
	registryHit := &mockResolver{result: registryResult}
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Exists: true, Source: "gh"}},
	}, 5*time.Second)

	chain := NewChainResolver(registryHit, probe)
	result, err := chain.Resolve(context.Background(), "gh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != ConfidenceRegistry {
		t.Errorf("expected registry confidence, got %s", result.Confidence)
	}
}

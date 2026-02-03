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
		&mockProber{name: "npm", result: nil},
		&mockProber{name: "pypi", result: nil},
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
		&mockProber{name: "npm", result: nil},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Source: "flask", Downloads: 1000, VersionCount: 10}},
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
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "serve", Downloads: 1000, VersionCount: 10}},
		&mockProber{name: "cask", result: &builders.ProbeResult{Source: "serve"}},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Source: "serve", VersionCount: 10}},
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
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "other-tool"}},
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
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "Flask", Downloads: 1000, VersionCount: 10}},
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
		&mockProber{name: "pypi", result: &builders.ProbeResult{Source: "flask", VersionCount: 10}},
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
		&mockProber{name: "npm", delay: 5 * time.Second, result: &builders.ProbeResult{Source: "tool", Downloads: 1000, VersionCount: 10}},
		&mockProber{name: "pypi", result: &builders.ProbeResult{Source: "tool", VersionCount: 10}},
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
			Source: "flask", Downloads: 50000, VersionCount: 30,
		}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "flask")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Metadata.Downloads != 50000 {
		t.Errorf("expected downloads 50000, got %d", result.Metadata.Downloads)
	}
}

func TestEcosystemProbe_QualityFilter(t *testing.T) {
	t.Run("rejects low quality crates.io match", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "crates.io", result: &builders.ProbeResult{
				Source: "prettier", Downloads: 87, VersionCount: 3,
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "prettier")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil (filtered), got %+v", result)
		}
	})

	t.Run("accepts high download crates.io match", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "crates.io", result: &builders.ProbeResult{
				Source: "ripgrep", Downloads: 5000000, VersionCount: 50,
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "ripgrep")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
	})

	t.Run("no filter for unconfigured builders", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "cask", result: &builders.ProbeResult{
				Source: "prettier",
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "prettier")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result (no filter for cask), got nil")
		}
	})
}

// Chain integration tests: EcosystemProbe wired into ChainResolver.

func TestChain_RegistryMissFallsToEcosystemProbe(t *testing.T) {
	registryMiss := &mockResolver{result: nil, err: nil}
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "crates.io", result: &builders.ProbeResult{
			Source: "ripgrep", Downloads: 5000000, VersionCount: 50,
		}},
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
		&mockProber{name: "npm", result: nil},
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
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "gh"}},
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

// Integration tests for quality filtering with realistic squatter scenarios.
// These tests verify that quality filtering plus priority ranking resolves
// tools to the correct registry when squatters exist.

func TestQualityFiltering_PrettierSquatter(t *testing.T) {
	// Scenario: prettier exists on npm (legitimate, high downloads) and
	// crates.io (squatter, low downloads/versions).
	// Expected: npm should win because crates.io squatter is filtered out.

	probe := NewEcosystemProbe([]builders.EcosystemProber{
		// crates.io squatter - realistic metrics from actual squatter
		&mockProber{name: "crates.io", result: &builders.ProbeResult{
			Source:       "prettier",
			Downloads:    87,
			VersionCount: 3,
		}},
		// npm legitimate package - high downloads and versions
		&mockProber{name: "npm", result: &builders.ProbeResult{
			Source:       "prettier",
			Downloads:    45000000,
			VersionCount: 300,
		}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "prettier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// npm should win because crates.io squatter is filtered out
	if result.Builder != "npm" {
		t.Errorf("expected npm (legitimate), got %s (squatter not filtered)", result.Builder)
	}

	// Verify metadata passthrough
	if result.Metadata.Downloads != 45000000 {
		t.Errorf("expected downloads 45000000, got %d", result.Metadata.Downloads)
	}
}

func TestQualityFiltering_HttpieSquatter(t *testing.T) {
	// Scenario: httpie exists on PyPI (legitimate) and crates.io (squatter).
	// Expected: PyPI should win because crates.io squatter is filtered out.

	probe := NewEcosystemProbe([]builders.EcosystemProber{
		// crates.io squatter
		&mockProber{name: "crates.io", result: &builders.ProbeResult{
			Source:       "httpie",
			Downloads:    45,
			VersionCount: 2,
		}},
		// PyPI legitimate package
		&mockProber{name: "pypi", result: &builders.ProbeResult{
			Source:       "httpie",
			Downloads:    0, // PyPI doesn't expose downloads directly
			VersionCount: 50,
		}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "httpie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// PyPI should win (crates.io squatter filtered, pypi passes version threshold)
	if result.Builder != "pypi" {
		t.Errorf("expected pypi (legitimate), got %s", result.Builder)
	}
}

func TestQualityFiltering_PriorityRankingAfterFilter(t *testing.T) {
	// Scenario: Tool exists on multiple registries, all passing quality filter.
	// Expected: Priority ranking should select the highest-priority registry.

	probe := NewEcosystemProbe([]builders.EcosystemProber{
		// crates.io - high quality (would pass filter)
		&mockProber{name: "crates.io", result: &builders.ProbeResult{
			Source:       "serve",
			Downloads:    1000000,
			VersionCount: 50,
		}},
		// npm - high quality (would pass filter)
		&mockProber{name: "npm", result: &builders.ProbeResult{
			Source:       "serve",
			Downloads:    5000000,
			VersionCount: 100,
		}},
		// pypi - high quality (would pass filter)
		&mockProber{name: "pypi", result: &builders.ProbeResult{
			Source:       "serve",
			Downloads:    0,
			VersionCount: 25,
		}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "serve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// crates.io has priority 3, pypi has 4, npm has 5
	// crates.io should win by priority ranking
	if result.Builder != "crates.io" {
		t.Errorf("expected crates.io (highest priority), got %s", result.Builder)
	}
}

func TestQualityFiltering_AllSquattersFiltered(t *testing.T) {
	// Scenario: Tool name exists on multiple registries but all are squatters.
	// Expected: No result should be returned (all filtered out).

	probe := NewEcosystemProbe([]builders.EcosystemProber{
		// crates.io squatter
		&mockProber{name: "crates.io", result: &builders.ProbeResult{
			Source:       "coolname",
			Downloads:    10,
			VersionCount: 1,
		}},
		// npm squatter
		&mockProber{name: "npm", result: &builders.ProbeResult{
			Source:       "coolname",
			Downloads:    5,
			VersionCount: 1,
		}},
		// pypi squatter
		&mockProber{name: "pypi", result: &builders.ProbeResult{
			Source:       "coolname",
			Downloads:    0,
			VersionCount: 1,
		}},
	}, 5*time.Second)

	result, err := probe.Resolve(context.Background(), "coolname")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (all squatters filtered), got builder=%s", result.Builder)
	}
}

func TestQualityFiltering_GracefulDegradation(t *testing.T) {
	// Scenario: Secondary API calls fail, so downloads/versions are 0.
	// Expected: Filter should use available signals gracefully.

	t.Run("version count alone passes filter", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "npm", result: &builders.ProbeResult{
				Source:       "tool",
				Downloads:    0, // downloads API failed
				VersionCount: 10,
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result (version count passes), got nil")
		}
	})

	t.Run("downloads alone passes filter", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "crates.io", result: &builders.ProbeResult{
				Source:       "tool",
				Downloads:    500,
				VersionCount: 0, // versions API failed
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result (downloads passes), got nil")
		}
	})

	t.Run("both signals fail but unconfigured builder passes", func(t *testing.T) {
		// Cask has no threshold configured (fail-open)
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "cask", result: &builders.ProbeResult{
				Source:       "tool",
				Downloads:    0,
				VersionCount: 0,
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result (cask has no threshold), got nil")
		}
	})
}

func TestQualityFiltering_RejectionReason(t *testing.T) {
	// Test that the filter provides a reason for rejection.
	// This is important for debugging quality filtering issues.

	f := NewQualityFilter()

	// Low-quality crates.io package
	result := &builders.ProbeResult{
		Source:       "squatter",
		Downloads:    50,
		VersionCount: 2,
	}

	ok, reason := f.Accept("crates.io", result)
	if ok {
		t.Fatal("expected rejection, got acceptance")
	}
	if reason == "" {
		t.Error("expected non-empty rejection reason")
	}

	// Verify reason contains useful debugging info
	if !contains(reason, "downloads") || !contains(reason, "version") {
		t.Errorf("expected reason to mention downloads and versions, got: %s", reason)
	}
}

func TestQualityFiltering_CpanRiverMetrics(t *testing.T) {
	// CPAN uses river.total (downstream dependency count) instead of downloads.
	// Test that CPAN filtering works correctly.

	t.Run("cpan squatter filtered", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "cpan", result: &builders.ProbeResult{
				Source:       "coolname",
				Downloads:    0, // river.total = 0
				VersionCount: 1,
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "coolname")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Error("expected nil (cpan squatter filtered)")
		}
	})

	t.Run("cpan legitimate passes river threshold", func(t *testing.T) {
		probe := NewEcosystemProbe([]builders.EcosystemProber{
			&mockProber{name: "cpan", result: &builders.ProbeResult{
				Source:       "App-Ack",
				Downloads:    42, // river.total = 42
				VersionCount: 2,
			}},
		}, 5*time.Second)

		result, err := probe.Resolve(context.Background(), "App-Ack")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result (cpan passes river threshold)")
		}
	})
}

// Helper function for checking substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

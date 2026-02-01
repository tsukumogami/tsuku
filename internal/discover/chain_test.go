package discover

import (
	"context"
	"errors"
	"testing"
)

// mockResolver returns a fixed result or error.
type mockResolver struct {
	result *DiscoveryResult
	err    error
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (*DiscoveryResult, error) {
	return m.result, m.err
}

func TestChainResolver_FirstHit(t *testing.T) {
	expected := &DiscoveryResult{Builder: "github", Source: "cli/cli", Confidence: ConfidenceRegistry}
	chain := NewChainResolver(
		&mockResolver{result: expected},
		&mockResolver{result: &DiscoveryResult{Builder: "npm", Source: "gh"}},
	)
	result, err := chain.Resolve(context.Background(), "gh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
}

func TestChainResolver_MissThenHit(t *testing.T) {
	expected := &DiscoveryResult{Builder: "crates.io", Source: "", Confidence: ConfidenceEcosystem}
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil}, // miss
		&mockResolver{result: expected},
	)
	result, err := chain.Resolve(context.Background(), "ripgrep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "crates.io" {
		t.Errorf("got builder %q, want %q", result.Builder, "crates.io")
	}
}

func TestChainResolver_AllMiss(t *testing.T) {
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: nil, err: nil},
	)
	_, err := chain.Resolve(context.Background(), "nonexistent")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
	if notFound.Tool != "nonexistent" {
		t.Errorf("got tool %q, want %q", notFound.Tool, "nonexistent")
	}
}

func TestChainResolver_SoftErrorContinues(t *testing.T) {
	expected := &DiscoveryResult{Builder: "github", Source: "foo/bar", Confidence: ConfidenceEcosystem}
	chain := NewChainResolver(
		&mockResolver{err: errors.New("timeout")}, // soft error
		&mockResolver{result: expected},
	)
	result, err := chain.Resolve(context.Background(), "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
}

func TestChainResolver_CancelledContextStopsChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	chain := NewChainResolver(
		&mockResolver{err: ctx.Err()},
		&mockResolver{result: &DiscoveryResult{Builder: "should-not-reach"}},
	)
	_, err := chain.Resolve(ctx, "anything")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestChainResolver_NormalizesInput(t *testing.T) {
	var captured string
	capturingResolver := &capturingMock{captured: &captured}
	chain := NewChainResolver(capturingResolver)
	_, _ = chain.Resolve(context.Background(), "  MyTool  ")
	if captured != "mytool" {
		t.Errorf("got %q, want %q", captured, "mytool")
	}
}

type capturingMock struct {
	captured *string
}

func (m *capturingMock) Resolve(_ context.Context, name string) (*DiscoveryResult, error) {
	*m.captured = name
	return nil, nil
}

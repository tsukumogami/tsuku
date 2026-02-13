package discover

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/builders"
)

func TestRankProbeResults(t *testing.T) {
	priority := map[string]int{
		"crates.io": 1,
		"npm":       2,
		"pypi":      3,
	}

	tests := []struct {
		name     string
		matches  []probeOutcome
		expected []string // expected order of builder names
	}{
		{
			name: "sort by downloads DESC",
			matches: []probeOutcome{
				{builderName: "npm", result: &builders.ProbeResult{Source: "npm-pkg", Downloads: 100}},
				{builderName: "crates.io", result: &builders.ProbeResult{Source: "crate", Downloads: 1000}},
				{builderName: "pypi", result: &builders.ProbeResult{Source: "pypi-pkg", Downloads: 500}},
			},
			expected: []string{"crates.io", "pypi", "npm"},
		},
		{
			name: "equal downloads - sort by version count DESC",
			matches: []probeOutcome{
				{builderName: "npm", result: &builders.ProbeResult{Source: "npm-pkg", Downloads: 100, VersionCount: 5}},
				{builderName: "crates.io", result: &builders.ProbeResult{Source: "crate", Downloads: 100, VersionCount: 20}},
			},
			expected: []string{"crates.io", "npm"},
		},
		{
			name: "equal downloads and versions - sort by priority",
			matches: []probeOutcome{
				{builderName: "npm", result: &builders.ProbeResult{Source: "npm-pkg", Downloads: 100, VersionCount: 5}},
				{builderName: "crates.io", result: &builders.ProbeResult{Source: "crate", Downloads: 100, VersionCount: 5}},
			},
			expected: []string{"crates.io", "npm"}, // crates.io has lower priority number
		},
		{
			name: "unknown builder gets lowest priority",
			matches: []probeOutcome{
				{builderName: "unknown", result: &builders.ProbeResult{Source: "unknown-pkg", Downloads: 100}},
				{builderName: "crates.io", result: &builders.ProbeResult{Source: "crate", Downloads: 100}},
			},
			expected: []string{"crates.io", "unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rankProbeResults(tt.matches, priority)
			for i, expectedName := range tt.expected {
				if tt.matches[i].builderName != expectedName {
					t.Errorf("position %d: got %s, want %s", i, tt.matches[i].builderName, expectedName)
				}
			}
		})
	}
}

func TestIsClearWinner(t *testing.T) {
	tests := []struct {
		name     string
		first    probeOutcome
		second   probeOutcome
		expected bool
	}{
		{
			name: "clear winner - 10x downloads with secondary signals",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     10000,
					VersionCount:  5,
					HasRepository: true,
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     500,
					VersionCount:  3,
					HasRepository: true,
				},
			},
			expected: true,
		},
		{
			name: "exactly 10x - still clear winner",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     1000,
					VersionCount:  3,
					HasRepository: true,
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     100,
					VersionCount:  2,
					HasRepository: true,
				},
			},
			expected: true,
		},
		{
			name: "close matches - less than 10x",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     900,
					VersionCount:  5,
					HasRepository: true,
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     100,
					VersionCount:  3,
					HasRepository: true,
				},
			},
			expected: false,
		},
		{
			name: "missing download data on first",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     0, // no data
					VersionCount:  5,
					HasRepository: true,
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     100,
					VersionCount:  3,
					HasRepository: true,
				},
			},
			expected: false,
		},
		{
			name: "missing download data on second",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     10000,
					VersionCount:  5,
					HasRepository: true,
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     0, // no data
					VersionCount:  3,
					HasRepository: true,
				},
			},
			expected: false,
		},
		{
			name: "version count below threshold",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     10000,
					VersionCount:  2, // below threshold of 3
					HasRepository: true,
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     100,
					VersionCount:  3,
					HasRepository: true,
				},
			},
			expected: false,
		},
		{
			name: "no repository link",
			first: probeOutcome{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     10000,
					VersionCount:  5,
					HasRepository: false, // missing
				},
			},
			second: probeOutcome{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     100,
					VersionCount:  3,
					HasRepository: true,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClearWinner(tt.first, tt.second)
			if got != tt.expected {
				t.Errorf("isClearWinner() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDisambiguate(t *testing.T) {
	priority := map[string]int{
		"crates.io": 1,
		"npm":       2,
		"pypi":      3,
	}

	tests := []struct {
		name          string
		toolName      string
		matches       []probeOutcome
		expectError   bool
		expectBuilder string
	}{
		{
			name:     "empty matches returns nil",
			toolName: "foo",
			matches:  []probeOutcome{},
		},
		{
			name:     "single match - auto-select without threshold checks",
			toolName: "bat",
			matches: []probeOutcome{
				{
					builderName: "crates.io",
					result: &builders.ProbeResult{
						Source:        "bat",
						Downloads:     0, // no downloads, but single match
						VersionCount:  1, // below threshold, but single match
						HasRepository: false,
					},
				},
			},
			expectBuilder: "crates.io",
		},
		{
			name:     "clear winner - auto-select",
			toolName: "bat",
			matches: []probeOutcome{
				{
					builderName: "crates.io",
					result: &builders.ProbeResult{
						Source:        "bat",
						Downloads:     10000,
						VersionCount:  10,
						HasRepository: true,
					},
				},
				{
					builderName: "npm",
					result: &builders.ProbeResult{
						Source:        "bat-cli",
						Downloads:     100,
						VersionCount:  3,
						HasRepository: true,
					},
				},
			},
			expectBuilder: "crates.io",
		},
		{
			name:     "close matches - return AmbiguousMatchError",
			toolName: "bat",
			matches: []probeOutcome{
				{
					builderName: "crates.io",
					result: &builders.ProbeResult{
						Source:        "bat",
						Downloads:     500,
						VersionCount:  5,
						HasRepository: true,
					},
				},
				{
					builderName: "npm",
					result: &builders.ProbeResult{
						Source:        "bat-cli",
						Downloads:     100,
						VersionCount:  3,
						HasRepository: true,
					},
				},
			},
			expectError: true,
		},
		{
			name:     "missing download data - return AmbiguousMatchError",
			toolName: "bat",
			matches: []probeOutcome{
				{
					builderName: "crates.io",
					result: &builders.ProbeResult{
						Source:        "bat",
						Downloads:     0, // missing
						VersionCount:  5,
						HasRepository: true,
					},
				},
				{
					builderName: "npm",
					result: &builders.ProbeResult{
						Source:        "bat-cli",
						Downloads:     100,
						VersionCount:  3,
						HasRepository: true,
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := disambiguate(tt.toolName, tt.matches, priority, nil)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ambErr, ok := err.(*AmbiguousMatchError)
				if !ok {
					t.Fatalf("expected AmbiguousMatchError, got %T", err)
				}
				if ambErr.Tool != tt.toolName {
					t.Errorf("AmbiguousMatchError.Tool = %q, want %q", ambErr.Tool, tt.toolName)
				}
				if len(ambErr.Matches) != len(tt.matches) {
					t.Errorf("AmbiguousMatchError.Matches length = %d, want %d", len(ambErr.Matches), len(tt.matches))
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(tt.matches) == 0 {
				if result != nil {
					t.Errorf("expected nil result for empty matches, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Builder != tt.expectBuilder {
				t.Errorf("result.Builder = %q, want %q", result.Builder, tt.expectBuilder)
			}
			if result.Confidence != ConfidenceEcosystem {
				t.Errorf("result.Confidence = %q, want %q", result.Confidence, ConfidenceEcosystem)
			}
		})
	}
}

func TestAmbiguousMatchError(t *testing.T) {
	err := &AmbiguousMatchError{
		Tool: "bat",
		Matches: []DiscoveryMatch{
			{Builder: "crates.io", Source: "bat", Downloads: 10000},
			{Builder: "npm", Source: "bat-cli", Downloads: 100},
		},
	}

	expected := "multiple sources found for 'bat': use --from to specify"
	if got := err.Error(); got != expected {
		t.Errorf("Error() = %q, want %q", got, expected)
	}
}

func TestConfirmDisambiguationCallback(t *testing.T) {
	priority := map[string]int{
		"crates.io": 1,
		"npm":       2,
	}

	closeMatches := []probeOutcome{
		{
			builderName: "crates.io",
			result: &builders.ProbeResult{
				Source:        "bat",
				Downloads:     500,
				VersionCount:  5,
				HasRepository: true,
			},
		},
		{
			builderName: "npm",
			result: &builders.ProbeResult{
				Source:        "bat-cli",
				Downloads:     100,
				VersionCount:  3,
				HasRepository: false,
			},
		},
	}

	t.Run("callback invoked with correct data", func(t *testing.T) {
		var receivedMatches []ProbeMatch
		callback := func(matches []ProbeMatch) (int, error) {
			receivedMatches = matches
			return 0, nil
		}

		_, err := disambiguate("bat", closeMatches, priority, callback)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(receivedMatches) != 2 {
			t.Fatalf("expected 2 matches, got %d", len(receivedMatches))
		}

		// Verify first match (crates.io should be first after ranking)
		if receivedMatches[0].Builder != "crates.io" {
			t.Errorf("first match builder = %q, want %q", receivedMatches[0].Builder, "crates.io")
		}
		if receivedMatches[0].Source != "bat" {
			t.Errorf("first match source = %q, want %q", receivedMatches[0].Source, "bat")
		}
		if receivedMatches[0].Downloads != 500 {
			t.Errorf("first match downloads = %d, want %d", receivedMatches[0].Downloads, 500)
		}
		if receivedMatches[0].VersionCount != 5 {
			t.Errorf("first match version count = %d, want %d", receivedMatches[0].VersionCount, 5)
		}
		if !receivedMatches[0].HasRepository {
			t.Error("first match should have repository")
		}

		// Verify second match
		if receivedMatches[1].Builder != "npm" {
			t.Errorf("second match builder = %q, want %q", receivedMatches[1].Builder, "npm")
		}
		if receivedMatches[1].HasRepository {
			t.Error("second match should not have repository")
		}
	})

	t.Run("callback selection honored - select first", func(t *testing.T) {
		callback := func(matches []ProbeMatch) (int, error) {
			return 0, nil // select first
		}

		result, err := disambiguate("bat", closeMatches, priority, callback)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Builder != "crates.io" {
			t.Errorf("result.Builder = %q, want %q", result.Builder, "crates.io")
		}
	})

	t.Run("callback selection honored - select second", func(t *testing.T) {
		callback := func(matches []ProbeMatch) (int, error) {
			return 1, nil // select second
		}

		result, err := disambiguate("bat", closeMatches, priority, callback)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Builder != "npm" {
			t.Errorf("result.Builder = %q, want %q", result.Builder, "npm")
		}
	})

	t.Run("callback error propagates", func(t *testing.T) {
		expectedErr := &disambiguateTestError{msg: "user canceled"}
		callback := func(matches []ProbeMatch) (int, error) {
			return 0, expectedErr
		}

		_, err := disambiguate("bat", closeMatches, priority, callback)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("out of range index returns AmbiguousMatchError", func(t *testing.T) {
		callback := func(matches []ProbeMatch) (int, error) {
			return 99, nil // invalid index
		}

		_, err := disambiguate("bat", closeMatches, priority, callback)
		if err == nil {
			t.Fatal("expected error for out of range index")
		}

		_, ok := err.(*AmbiguousMatchError)
		if !ok {
			t.Errorf("expected AmbiguousMatchError, got %T", err)
		}
	})

	t.Run("negative index returns AmbiguousMatchError", func(t *testing.T) {
		callback := func(matches []ProbeMatch) (int, error) {
			return -1, nil // invalid index
		}

		_, err := disambiguate("bat", closeMatches, priority, callback)
		if err == nil {
			t.Fatal("expected error for negative index")
		}

		_, ok := err.(*AmbiguousMatchError)
		if !ok {
			t.Errorf("expected AmbiguousMatchError, got %T", err)
		}
	})

	t.Run("callback not invoked for clear winner", func(t *testing.T) {
		clearWinnerMatches := []probeOutcome{
			{
				builderName: "crates.io",
				result: &builders.ProbeResult{
					Source:        "bat",
					Downloads:     10000,
					VersionCount:  10,
					HasRepository: true,
				},
			},
			{
				builderName: "npm",
				result: &builders.ProbeResult{
					Source:        "bat-cli",
					Downloads:     100,
					VersionCount:  3,
					HasRepository: true,
				},
			},
		}

		callbackInvoked := false
		callback := func(matches []ProbeMatch) (int, error) {
			callbackInvoked = true
			return 0, nil
		}

		result, err := disambiguate("bat", clearWinnerMatches, priority, callback)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if callbackInvoked {
			t.Error("callback should not be invoked for clear winner")
		}

		if result.Builder != "crates.io" {
			t.Errorf("result.Builder = %q, want %q", result.Builder, "crates.io")
		}
	})
}

// disambiguateTestError is a simple error type for testing.
type disambiguateTestError struct {
	msg string
}

func (e *disambiguateTestError) Error() string {
	return e.msg
}

func TestToProbeMatches(t *testing.T) {
	matches := []probeOutcome{
		{
			builderName: "crates.io",
			result: &builders.ProbeResult{
				Source:        "bat",
				Downloads:     1000,
				VersionCount:  10,
				HasRepository: true,
			},
		},
		{
			builderName: "npm",
			result: &builders.ProbeResult{
				Source:        "bat-cli",
				Downloads:     100,
				VersionCount:  5,
				HasRepository: false,
			},
		},
	}

	probeMatches := toProbeMatches(matches)

	if len(probeMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(probeMatches))
	}

	// Verify first match
	pm := probeMatches[0]
	if pm.Builder != "crates.io" {
		t.Errorf("first match Builder = %q, want %q", pm.Builder, "crates.io")
	}
	if pm.Source != "bat" {
		t.Errorf("first match Source = %q, want %q", pm.Source, "bat")
	}
	if pm.Downloads != 1000 {
		t.Errorf("first match Downloads = %d, want %d", pm.Downloads, 1000)
	}
	if pm.VersionCount != 10 {
		t.Errorf("first match VersionCount = %d, want %d", pm.VersionCount, 10)
	}
	if !pm.HasRepository {
		t.Error("first match should have repository")
	}

	// Verify second match
	pm = probeMatches[1]
	if pm.Builder != "npm" {
		t.Errorf("second match Builder = %q, want %q", pm.Builder, "npm")
	}
	if pm.HasRepository {
		t.Error("second match should not have repository")
	}
}

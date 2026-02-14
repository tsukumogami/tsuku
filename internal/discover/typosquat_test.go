package discover

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		// Identical strings
		{"", "", 0},
		{"a", "a", 0},
		{"bat", "bat", 0},
		{"ripgrep", "ripgrep", 0},

		// Single operations
		{"", "a", 1},       // insertion
		{"a", "", 1},       // deletion
		{"a", "b", 1},      // substitution
		{"ab", "a", 1},     // deletion
		{"a", "ab", 1},     // insertion
		{"bat", "bta", 2},  // two substitutions
		{"bat", "cat", 1},  // one substitution
		{"bat", "bart", 1}, // one insertion
		{"bart", "bat", 1}, // one deletion
		{"bat", "at", 1},   // one deletion at start
		{"at", "bat", 1},   // one insertion at start

		// Distance 2
		{"bat", "tab", 2}, // two operations
		{"abc", "cba", 2}, // two substitutions
		{"ab", "ba", 2},   // swap = 2 in Levenshtein (not Damerau)

		// Distance 3+
		{"abc", "xyz", 3},
		{"ripgrep", "rgiprep", 2}, // design doc example
		{"kubectl", "kubctl", 1},  // common typo
		{"terraform", "teraform", 1},

		// Unicode
		{"café", "cafe", 1},
		{"naïve", "naive", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshtein(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
			}
			// Verify symmetry
			gotReverse := levenshtein(tt.b, tt.a)
			if gotReverse != tt.expected {
				t.Errorf("levenshtein(%q, %q) = %d, want %d (symmetry check)", tt.b, tt.a, gotReverse, tt.expected)
			}
		})
	}
}

func TestCheckTyposquat(t *testing.T) {
	// Create a test registry with known tools
	registry := &DiscoveryRegistry{
		SchemaVersion: 1,
		Tools: map[string]RegistryEntry{
			"ripgrep": {Builder: "cargo", Source: "ripgrep"},
			"bat":     {Builder: "cargo", Source: "bat"},
			"kubectl": {Builder: "github", Source: "kubernetes/kubectl"},
			"fd":      {Builder: "cargo", Source: "fd-find"},
		},
	}
	registry.buildIndex()

	tests := []struct {
		name     string
		toolName string
		wantNil  bool
		wantDist int
		wantSim  string
	}{
		{
			name:     "no match - distance too high",
			toolName: "terraform",
			wantNil:  true,
		},
		{
			name:     "exact match - distance 0 returns nil",
			toolName: "ripgrep",
			wantNil:  true,
		},
		{
			name:     "exact match case insensitive - distance 0 returns nil",
			toolName: "RIPGREP",
			wantNil:  true,
		},
		{
			name:     "distance 1 - single typo",
			toolName: "rigrep", // missing 'p'
			wantNil:  false,
			wantDist: 1,
			wantSim:  "ripgrep",
		},
		{
			name:     "distance 2 - two typos",
			toolName: "rgiprep", // design doc example
			wantNil:  false,
			wantDist: 2,
			wantSim:  "ripgrep",
		},
		{
			name:     "distance 3 - no warning",
			toolName: "rgirep", // three changes from ripgrep
			wantNil:  true,
		},
		{
			name:     "short name distance 1",
			toolName: "bta", // bat -> bta
			wantNil:  false,
			wantDist: 2, // swap is 2 in Levenshtein
			wantSim:  "bat",
		},
		{
			name:     "short name distance 1 - cat vs bat",
			toolName: "cat",
			wantNil:  false,
			wantDist: 1,
			wantSim:  "bat",
		},
		{
			name:     "kubectl typo",
			toolName: "kubctl", // missing 'e'
			wantNil:  false,
			wantDist: 1,
			wantSim:  "kubectl",
		},
		{
			name:     "completely different name",
			toolName: "xyz123",
			wantNil:  true,
		},
		{
			name:     "empty string matches fd",
			toolName: "",
			wantNil:  false, // distance to "fd" is 2, which is within threshold
			wantDist: 2,
			wantSim:  "fd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckTyposquat(tt.toolName, registry)

			if tt.wantNil {
				if got != nil {
					t.Errorf("CheckTyposquat(%q) = %+v, want nil", tt.toolName, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("CheckTyposquat(%q) = nil, want warning", tt.toolName)
			}

			if got.Requested != tt.toolName {
				t.Errorf("Requested = %q, want %q", got.Requested, tt.toolName)
			}
			if got.Distance != tt.wantDist {
				t.Errorf("Distance = %d, want %d", got.Distance, tt.wantDist)
			}
			if got.Similar != tt.wantSim {
				t.Errorf("Similar = %q, want %q", got.Similar, tt.wantSim)
			}
			// Source should be builder:source format
			if got.Source == "" {
				t.Error("Source is empty")
			}
		})
	}
}

func TestCheckTyposquatNilRegistry(t *testing.T) {
	got := CheckTyposquat("anyname", nil)
	if got != nil {
		t.Errorf("CheckTyposquat with nil registry = %+v, want nil", got)
	}
}

func TestCheckTyposquatEmptyRegistry(t *testing.T) {
	registry := &DiscoveryRegistry{
		SchemaVersion: 1,
		Tools:         map[string]RegistryEntry{},
	}

	got := CheckTyposquat("anyname", registry)
	if got != nil {
		t.Errorf("CheckTyposquat with empty registry = %+v, want nil", got)
	}
}

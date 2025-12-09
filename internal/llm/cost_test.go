package llm

import (
	"testing"
)

func TestUsage_Cost(t *testing.T) {
	tests := []struct {
		name         string
		inputTokens  int
		outputTokens int
		wantCost     float64
	}{
		{
			name:         "zero usage",
			inputTokens:  0,
			outputTokens: 0,
			wantCost:     0.0,
		},
		{
			name:         "input only",
			inputTokens:  1_000_000,
			outputTokens: 0,
			wantCost:     3.0, // $3 per 1M input tokens
		},
		{
			name:         "output only",
			inputTokens:  0,
			outputTokens: 1_000_000,
			wantCost:     15.0, // $15 per 1M output tokens
		},
		{
			name:         "mixed usage",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			wantCost:     18.0, // $3 + $15
		},
		{
			name:         "typical recipe generation",
			inputTokens:  3500,
			outputTokens: 500,
			wantCost:     0.018, // ~$0.02 per issue spec
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := Usage{
				InputTokens:  tt.inputTokens,
				OutputTokens: tt.outputTokens,
			}
			got := u.Cost()
			// Allow small floating point tolerance
			tolerance := 0.0001
			if got < tt.wantCost-tolerance || got > tt.wantCost+tolerance {
				t.Errorf("Usage.Cost() = %v, want %v", got, tt.wantCost)
			}
		})
	}
}

func TestUsage_Add(t *testing.T) {
	u := Usage{InputTokens: 100, OutputTokens: 50}
	u.Add(Usage{InputTokens: 200, OutputTokens: 100})

	if u.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", u.InputTokens)
	}
	if u.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", u.OutputTokens)
	}
}

func TestUsage_String(t *testing.T) {
	u := Usage{InputTokens: 3500, OutputTokens: 500}
	got := u.String()
	want := "tokens: 3500 in / 500 out, cost: $0.0180"
	if got != want {
		t.Errorf("Usage.String() = %q, want %q", got, want)
	}
}

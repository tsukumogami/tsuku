package discover

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/builders"
)

func TestQualityFilter_Accept(t *testing.T) {
	f := NewQualityFilter()

	tests := []struct {
		name        string
		builderName string
		result      *builders.ProbeResult
		wantOK      bool
	}{
		{
			name:        "crates.io passes download threshold",
			builderName: "crates.io",
			result:      &builders.ProbeResult{Downloads: 500, VersionCount: 2},
			wantOK:      true,
		},
		{
			name:        "crates.io passes version threshold",
			builderName: "crates.io",
			result:      &builders.ProbeResult{Downloads: 10, VersionCount: 10},
			wantOK:      true,
		},
		{
			name:        "crates.io fails both thresholds",
			builderName: "crates.io",
			result:      &builders.ProbeResult{Downloads: 87, VersionCount: 3},
			wantOK:      false,
		},
		{
			name:        "crates.io boundary downloads",
			builderName: "crates.io",
			result:      &builders.ProbeResult{Downloads: 100, VersionCount: 1},
			wantOK:      true,
		},
		{
			name:        "crates.io boundary versions",
			builderName: "crates.io",
			result:      &builders.ProbeResult{Downloads: 0, VersionCount: 5},
			wantOK:      true,
		},
		{
			name:        "unknown builder fails open",
			builderName: "npm",
			result:      &builders.ProbeResult{Downloads: 0, VersionCount: 0},
			wantOK:      true,
		},
		{
			name:        "pypi fails open",
			builderName: "pypi",
			result:      &builders.ProbeResult{},
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := f.Accept(tt.builderName, tt.result)
			if ok != tt.wantOK {
				t.Errorf("Accept() = %v (reason: %s), want %v", ok, reason, tt.wantOK)
			}
			if reason == "" {
				t.Error("Accept() returned empty reason")
			}
		})
	}
}

package install

import (
	"testing"
)

func TestPinLevelFromRequested(t *testing.T) {
	tests := []struct {
		requested string
		want      PinLevel
	}{
		{"", PinLatest},
		{"latest", PinLatest},
		{"20", PinMajor},
		{"18", PinMajor},
		{"3", PinMajor},
		{"1.29", PinMinor},
		{"20.16", PinMinor},
		{"2024.01", PinMinor}, // CalVer maps to PinMinor (2 components)
		{"1.29.3", PinExact},
		{"20.16.0", PinExact},
		{"2024.01.15", PinExact}, // CalVer with 3 components
		{"1.2.3.4", PinExact},    // 4+ components still exact
		{"@lts", PinChannel},
		{"@stable", PinChannel},
	}

	for _, tt := range tests {
		t.Run(tt.requested, func(t *testing.T) {
			got := PinLevelFromRequested(tt.requested)
			if got != tt.want {
				t.Errorf("PinLevelFromRequested(%q) = %v, want %v", tt.requested, got, tt.want)
			}
		})
	}
}

func TestVersionMatchesPin(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		requested string
		want      bool
	}{
		// PinLatest matches everything
		{"empty matches anything", "22.3.0", "", true},
		{"latest matches anything", "22.3.0", "latest", true},

		// PinMajor
		{"major match", "18.2.1", "18", true},
		{"major exact", "18", "18", true},
		{"major mismatch", "22.3.0", "18", false},
		// Dot-boundary: "1" must NOT match "10.0.0"
		{"dot boundary prevents prefix collision", "10.0.0", "1", false},
		{"dot boundary prevents prefix collision 2", "18.0.0", "1", false},

		// PinMinor
		{"minor match", "1.29.3", "1.29", true},
		{"minor exact", "1.29", "1.29", true},
		{"minor mismatch", "1.30.0", "1.29", false},
		// Dot-boundary: "1.2" must NOT match "1.20.0"
		{"dot boundary minor", "1.20.0", "1.2", false},

		// PinExact
		{"exact match", "1.29.3", "1.29.3", true},
		{"exact mismatch", "1.29.4", "1.29.3", false},

		// PinChannel returns false (can't match by string)
		{"channel returns false", "18.20.0", "@lts", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VersionMatchesPin(tt.version, tt.requested)
			if got != tt.want {
				t.Errorf("VersionMatchesPin(%q, %q) = %v, want %v", tt.version, tt.requested, got, tt.want)
			}
		})
	}
}

func TestValidateRequested(t *testing.T) {
	// Valid inputs
	valid := []string{"", "18", "1.29", "1.29.3", "@lts", "@stable", "latest", "2024.01.15", "node-lts"}
	for _, v := range valid {
		t.Run("valid:"+v, func(t *testing.T) {
			if err := ValidateRequested(v); err != nil {
				t.Errorf("ValidateRequested(%q) returned unexpected error: %v", v, err)
			}
		})
	}

	// Invalid inputs
	invalid := []string{"../etc/passwd", "18/../../root", "1.29\\path", "$(cmd)", "1;rm -rf"}
	for _, v := range invalid {
		t.Run("invalid:"+v, func(t *testing.T) {
			if err := ValidateRequested(v); err == nil {
				t.Errorf("ValidateRequested(%q) should have returned an error", v)
			}
		})
	}
}

func TestPinLevelString(t *testing.T) {
	tests := []struct {
		level PinLevel
		want  string
	}{
		{PinLatest, "latest"},
		{PinMajor, "major"},
		{PinMinor, "minor"},
		{PinExact, "exact"},
		{PinChannel, "channel"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("PinLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

package toolchain

import (
	"errors"
	"testing"
)

func TestGetInfo(t *testing.T) {
	tests := []struct {
		ecosystem   string
		wantBinary  string
		wantName    string
		wantRecipe  string
		shouldExist bool
	}{
		{"crates.io", "cargo", "Cargo", "rust", true},
		{"rubygems", "gem", "gem", "ruby", true},
		{"pypi", "pipx", "pipx", "pipx", true},
		{"npm", "npm", "npm", "nodejs", true},
		{"unknown", "", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.ecosystem, func(t *testing.T) {
			info := GetInfo(tc.ecosystem)
			if tc.shouldExist {
				if info == nil {
					t.Fatalf("GetInfo(%q) returned nil, want non-nil", tc.ecosystem)
				}
				if info.Binary != tc.wantBinary {
					t.Errorf("Binary = %q, want %q", info.Binary, tc.wantBinary)
				}
				if info.Name != tc.wantName {
					t.Errorf("Name = %q, want %q", info.Name, tc.wantName)
				}
				if info.TsukuRecipe != tc.wantRecipe {
					t.Errorf("TsukuRecipe = %q, want %q", info.TsukuRecipe, tc.wantRecipe)
				}
			} else {
				if info != nil {
					t.Errorf("GetInfo(%q) = %+v, want nil", tc.ecosystem, info)
				}
			}
		})
	}
}

func TestCheckAvailable(t *testing.T) {
	// Save original and restore after test
	originalLookPath := LookPathFunc
	defer func() { LookPathFunc = originalLookPath }()

	tests := []struct {
		name        string
		ecosystem   string
		lookPathErr error
		wantErr     bool
		errContains string
	}{
		{
			name:      "cargo available",
			ecosystem: "crates.io",
			wantErr:   false,
		},
		{
			name:        "cargo not available",
			ecosystem:   "crates.io",
			lookPathErr: errors.New("not found"),
			wantErr:     true,
			errContains: "Cargo is required",
		},
		{
			name:      "gem available",
			ecosystem: "rubygems",
			wantErr:   false,
		},
		{
			name:        "gem not available",
			ecosystem:   "rubygems",
			lookPathErr: errors.New("not found"),
			wantErr:     true,
			errContains: "gem is required",
		},
		{
			name:      "pipx available",
			ecosystem: "pypi",
			wantErr:   false,
		},
		{
			name:        "pipx not available",
			ecosystem:   "pypi",
			lookPathErr: errors.New("not found"),
			wantErr:     true,
			errContains: "pipx is required",
		},
		{
			name:      "npm available",
			ecosystem: "npm",
			wantErr:   false,
		},
		{
			name:        "npm not available",
			ecosystem:   "npm",
			lookPathErr: errors.New("not found"),
			wantErr:     true,
			errContains: "npm is required",
		},
		{
			name:      "unknown ecosystem - no check",
			ecosystem: "unknown",
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			LookPathFunc = func(file string) (string, error) {
				if tc.lookPathErr != nil {
					return "", tc.lookPathErr
				}
				return "/usr/bin/" + file, nil
			}

			err := CheckAvailable(tc.ecosystem)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("CheckAvailable(%q) returned nil, want error", tc.ecosystem)
				}
				if tc.errContains != "" && !contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("CheckAvailable(%q) = %v, want nil", tc.ecosystem, err)
				}
			}
		})
	}
}

func TestIsAvailable(t *testing.T) {
	// Save original and restore after test
	originalLookPath := LookPathFunc
	defer func() { LookPathFunc = originalLookPath }()

	t.Run("available", func(t *testing.T) {
		LookPathFunc = func(file string) (string, error) {
			return "/usr/bin/" + file, nil
		}

		if !IsAvailable("crates.io") {
			t.Error("IsAvailable(\"crates.io\") = false, want true")
		}
	})

	t.Run("not available", func(t *testing.T) {
		LookPathFunc = func(file string) (string, error) {
			return "", errors.New("not found")
		}

		if IsAvailable("crates.io") {
			t.Error("IsAvailable(\"crates.io\") = true, want false")
		}
	})
}

func TestCheckAvailable_ErrorMessage(t *testing.T) {
	// Save original and restore after test
	originalLookPath := LookPathFunc
	defer func() { LookPathFunc = originalLookPath }()

	LookPathFunc = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	tests := []struct {
		ecosystem   string
		wantMessage string
	}{
		{
			ecosystem:   "crates.io",
			wantMessage: "Cargo is required to create recipes from crates.io. Install Rust or run: tsuku install rust",
		},
		{
			ecosystem:   "rubygems",
			wantMessage: "gem is required to create recipes from rubygems. Install Ruby or run: tsuku install ruby",
		},
		{
			ecosystem:   "pypi",
			wantMessage: "pipx is required to create recipes from pypi. Install Python or run: tsuku install pipx",
		},
		{
			ecosystem:   "npm",
			wantMessage: "npm is required to create recipes from npm. Install Node.js or run: tsuku install nodejs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.ecosystem, func(t *testing.T) {
			err := CheckAvailable(tc.ecosystem)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tc.wantMessage {
				t.Errorf("error message = %q\nwant = %q", err.Error(), tc.wantMessage)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

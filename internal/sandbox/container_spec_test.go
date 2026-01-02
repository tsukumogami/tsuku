package sandbox

import (
	"testing"
)

func TestDeriveContainerSpec(t *testing.T) {
	tests := []struct {
		name          string
		packages      map[string][]string
		wantFamily    string
		wantBaseImage string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "nil packages",
			packages:      nil,
			wantFamily:    "",
			wantBaseImage: "",
			wantErr:       false,
		},
		{
			name:          "empty packages",
			packages:      map[string][]string{},
			wantFamily:    "",
			wantBaseImage: "",
			wantErr:       false,
		},
		{
			name:          "debian family - apt",
			packages:      map[string][]string{"apt": {"curl", "jq"}},
			wantFamily:    "debian",
			wantBaseImage: "debian:bookworm-slim",
			wantErr:       false,
		},
		{
			name:          "rhel family - dnf",
			packages:      map[string][]string{"dnf": {"wget", "tar"}},
			wantFamily:    "rhel",
			wantBaseImage: "fedora:41",
			wantErr:       false,
		},
		{
			name:          "arch family - pacman",
			packages:      map[string][]string{"pacman": {"base-devel", "git"}},
			wantFamily:    "arch",
			wantBaseImage: "archlinux:base",
			wantErr:       false,
		},
		{
			name:          "alpine family - apk",
			packages:      map[string][]string{"apk": {"bash", "curl"}},
			wantFamily:    "alpine",
			wantBaseImage: "alpine:3.19",
			wantErr:       false,
		},
		{
			name:          "suse family - zypper",
			packages:      map[string][]string{"zypper": {"vim", "gcc"}},
			wantFamily:    "suse",
			wantBaseImage: "opensuse/leap:15",
			wantErr:       false,
		},
		{
			name:        "incompatible package managers - apt and dnf",
			packages:    map[string][]string{"apt": {"curl"}, "dnf": {"wget"}},
			wantErr:     true,
			errContains: "incompatible package managers",
		},
		{
			name:        "incompatible package managers - apt and pacman",
			packages:    map[string][]string{"apt": {"curl"}, "pacman": {"git"}},
			wantErr:     true,
			errContains: "incompatible package managers",
		},
		{
			name:        "unsupported package manager - brew",
			packages:    map[string][]string{"brew": {"node"}},
			wantErr:     true,
			errContains: "not applicable to Linux containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveContainerSpec(tt.packages)

			if tt.wantErr {
				if err == nil {
					t.Errorf("DeriveContainerSpec() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("DeriveContainerSpec() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("DeriveContainerSpec() unexpected error: %v", err)
				return
			}

			// nil/empty case
			if tt.wantFamily == "" {
				if got != nil {
					t.Errorf("DeriveContainerSpec() = %+v, want nil", got)
				}
				return
			}

			// Validate fields
			if got == nil {
				t.Fatalf("DeriveContainerSpec() = nil, want non-nil")
			}

			if got.LinuxFamily != tt.wantFamily {
				t.Errorf("LinuxFamily = %q, want %q", got.LinuxFamily, tt.wantFamily)
			}

			if got.BaseImage != tt.wantBaseImage {
				t.Errorf("BaseImage = %q, want %q", got.BaseImage, tt.wantBaseImage)
			}

			if got.Packages == nil {
				t.Errorf("Packages = nil, want non-nil")
			}

			if len(got.BuildCommands) == 0 {
				t.Errorf("BuildCommands is empty, want non-empty")
			}
		})
	}
}

func TestDeriveContainerSpec_BuildCommands(t *testing.T) {
	tests := []struct {
		name         string
		packages     map[string][]string
		wantCommands []string
	}{
		{
			name:     "debian - apt packages",
			packages: map[string][]string{"apt": {"curl", "jq"}},
			wantCommands: []string{
				"RUN apt-get update && apt-get install -y curl jq",
			},
		},
		{
			name:     "rhel - dnf packages",
			packages: map[string][]string{"dnf": {"wget", "tar"}},
			wantCommands: []string{
				"RUN dnf install -y tar wget",
			},
		},
		{
			name:     "arch - pacman packages",
			packages: map[string][]string{"pacman": {"git", "base-devel"}},
			wantCommands: []string{
				"RUN pacman -Sy --noconfirm base-devel git",
			},
		},
		{
			name:     "alpine - apk packages",
			packages: map[string][]string{"apk": {"bash", "curl"}},
			wantCommands: []string{
				"RUN apk add --no-cache bash curl",
			},
		},
		{
			name:     "suse - zypper packages",
			packages: map[string][]string{"zypper": {"vim", "gcc"}},
			wantCommands: []string{
				"RUN zypper install -y gcc vim",
			},
		},
		{
			name:     "single package",
			packages: map[string][]string{"apt": {"curl"}},
			wantCommands: []string{
				"RUN apt-get update && apt-get install -y curl",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := DeriveContainerSpec(tt.packages)
			if err != nil {
				t.Fatalf("DeriveContainerSpec() error = %v", err)
			}

			if len(spec.BuildCommands) != len(tt.wantCommands) {
				t.Errorf("BuildCommands count = %d, want %d", len(spec.BuildCommands), len(tt.wantCommands))
				t.Errorf("Got: %v", spec.BuildCommands)
				t.Errorf("Want: %v", tt.wantCommands)
				return
			}

			for i, cmd := range spec.BuildCommands {
				if cmd != tt.wantCommands[i] {
					t.Errorf("BuildCommands[%d] = %q, want %q", i, cmd, tt.wantCommands[i])
				}
			}
		})
	}
}

func TestDeriveContainerSpec_Determinism(t *testing.T) {
	// Package order in map iteration is random, but output should be deterministic
	packages := map[string][]string{
		"apt": {"zsh", "bash", "curl", "wget"},
	}

	spec1, err1 := DeriveContainerSpec(packages)
	if err1 != nil {
		t.Fatalf("First call error: %v", err1)
	}

	spec2, err2 := DeriveContainerSpec(packages)
	if err2 != nil {
		t.Fatalf("Second call error: %v", err2)
	}

	// Build commands should be identical across calls
	if len(spec1.BuildCommands) != len(spec2.BuildCommands) {
		t.Errorf("BuildCommands length mismatch: %d vs %d", len(spec1.BuildCommands), len(spec2.BuildCommands))
	}

	for i := range spec1.BuildCommands {
		if spec1.BuildCommands[i] != spec2.BuildCommands[i] {
			t.Errorf("BuildCommands[%d] differs:\n  Call 1: %s\n  Call 2: %s",
				i, spec1.BuildCommands[i], spec2.BuildCommands[i])
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

package main

import (
	"testing"
)

func TestValidateOS(t *testing.T) {
	tests := []struct {
		name    string
		os      string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"linux is valid", "linux", false},
		{"darwin is valid", "darwin", false},
		{"windows rejected", "windows", true},
		{"freebsd rejected", "freebsd", true},
		{"invalid os rejected", "invalid", true},
		{"ubuntu rejected", "ubuntu", true},
		{"macos rejected", "macos", true},
		{"win rejected", "win", true},
		{"path traversal rejected", "../etc", true},
		{"slash rejected", "linux/amd64", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOS(tt.os)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOS(%q) error = %v, wantErr %v", tt.os, err, tt.wantErr)
			}
		})
	}
}

func TestValidateArch(t *testing.T) {
	tests := []struct {
		name    string
		arch    string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"amd64 is valid", "amd64", false},
		{"arm64 is valid", "arm64", false},
		{"386 rejected", "386", true},
		{"arm rejected", "arm", true},
		{"invalid arch rejected", "invalid", true},
		{"x86_64 rejected", "x86_64", true},
		{"x64 rejected", "x64", true},
		{"i386 rejected", "i386", true},
		{"path traversal rejected", "../bin", true},
		{"slash rejected", "amd64/linux", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArch(tt.arch)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArch(%q) error = %v, wantErr %v", tt.arch, err, tt.wantErr)
			}
		})
	}
}

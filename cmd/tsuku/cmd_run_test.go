package main

import (
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// --- resolveMode tests ---

// The priority chain, and the origin each step records. The origin is the half
// the runner's elevation reads: it raises a mode whose origin is default and
// no other, so a step that recorded the wrong origin would either give a
// declaration a mode it must not raise or withhold one it should.
//
// This is D1-3's and D1-5's route half. Which routes produce which origin is
// settled here, because the flag, the environment variable and the
// configuration key are read here and nowhere else; what the runner then does
// with each origin is pinned in internal/autoinstall.
func TestResolveMode_ModeAndOrigin(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		env        string
		config     string
		wantMode   autoinstall.Mode
		wantOrigin autoinstall.Origin
	}{
		{
			name: "the flag wins over both", flag: "confirm", env: "suggest", config: "auto",
			wantMode: autoinstall.ModeConfirm, wantOrigin: autoinstall.OriginFlag,
		},
		{
			name: "the environment wins over the config", env: "suggest", config: "confirm",
			wantMode: autoinstall.ModeSuggest, wantOrigin: autoinstall.OriginEnvironment,
		},
		{
			name: "the config wins over the default", config: "suggest",
			wantMode: autoinstall.ModeSuggest, wantOrigin: autoinstall.OriginConfig,
		},
		{
			name:     "nothing set anywhere",
			wantMode: autoinstall.ModeConfirm, wantOrigin: autoinstall.OriginDefault,
		},
		// D1-3, the three routes an explicit suggest arrives by. The first two
		// are covered above by rows that also fix a precedence; this is the
		// third, and the set is what the criterion names.
		{
			name: "suggest by the flag", flag: "suggest",
			wantMode: autoinstall.ModeSuggest, wantOrigin: autoinstall.OriginFlag,
		},
		{
			name: "suggest by the environment", env: "suggest",
			wantMode: autoinstall.ModeSuggest, wantOrigin: autoinstall.OriginEnvironment,
		},
		// D1-5. The escalation restriction's output is a confirm the
		// environment produced, not a default one. Recorded as a default it
		// would be raised straight back to auto by any project declaration,
		// which is the reversal the restriction exists to prevent.
		{
			name: "an uncorroborated environment auto is lowered, and stays the environment's",
			env:  "auto", config: "confirm",
			wantMode: autoinstall.ModeConfirm, wantOrigin: autoinstall.OriginEnvironment,
		},
		{
			name:     "the same with no config at all",
			env:      "auto",
			wantMode: autoinstall.ModeConfirm, wantOrigin: autoinstall.OriginEnvironment,
		},
		{
			name: "a corroborated environment auto survives",
			env:  "auto", config: "auto",
			wantMode: autoinstall.ModeAuto, wantOrigin: autoinstall.OriginEnvironment,
		},
		{
			name: "the environment downgrades a config auto",
			env:  "confirm", config: "auto",
			wantMode: autoinstall.ModeConfirm, wantOrigin: autoinstall.OriginEnvironment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TSUKU_AUTO_INSTALL_MODE", tt.env)

			mode, origin, err := resolveMode(tt.flag, &userconfig.Config{AutoInstallMode: tt.config})
			if err != nil {
				t.Fatal(err)
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %v, want %v", mode, tt.wantMode)
			}
			if origin != tt.wantOrigin {
				t.Errorf("origin = %v, want %v", origin, tt.wantOrigin)
			}
		})
	}
}

func TestResolveMode_InvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		env    string
		config string
	}{
		{name: "flag", flag: "invalid"},
		{name: "environment variable", env: "invalid"},
		{name: "config key", config: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TSUKU_AUTO_INSTALL_MODE", tt.env)

			_, _, err := resolveMode(tt.flag, &userconfig.Config{AutoInstallMode: tt.config})
			if err == nil {
				t.Fatalf("resolveMode() error = nil, want one for an invalid %s", tt.name)
			}
		})
	}
}

// --- Command registration tests ---

func TestRunCmd_Registered(t *testing.T) {
	// Verify the run command is registered and has expected flags.
	cmd, _, err := rootCmd.Find([]string{"run"})
	if err != nil {
		t.Fatalf("run command not found: %v", err)
	}
	if cmd.Use != "run <command> [args...]" {
		t.Errorf("unexpected Use: %q", cmd.Use)
	}
	modeFlag := cmd.Flags().Lookup("mode")
	if modeFlag == nil {
		t.Fatal("--mode flag not registered")
	}
}

func TestRunCmd_HelpDocumentsAllModes(t *testing.T) {
	help := runCmd.Long

	for _, mode := range []string{"suggest", "confirm", "auto"} {
		if !strings.Contains(help, mode) {
			t.Errorf("help output should mention %q mode", mode)
		}
	}
	if !strings.Contains(help, "TSUKU_AUTO_INSTALL_MODE") {
		t.Error("help output should mention TSUKU_AUTO_INSTALL_MODE env var")
	}
	if !strings.Contains(help, "auto_install_mode") {
		t.Error("help output should mention auto_install_mode config key")
	}
	if !strings.Contains(help, "--") {
		t.Error("help output should document -- separator")
	}
}

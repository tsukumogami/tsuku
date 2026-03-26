package main

import (
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// --- resolveMode tests ---

func TestResolveMode_FlagWins(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "auto"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "suggest")

	m, err := resolveMode("confirm", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm", m)
	}
}

func TestResolveMode_EnvWinsOverConfig(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "confirm"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "suggest")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeSuggest {
		t.Errorf("got %v, want ModeSuggest", m)
	}
}

func TestResolveMode_ConfigWinsOverDefault(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "suggest"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeSuggest {
		t.Errorf("got %v, want ModeSuggest", m)
	}
}

func TestResolveMode_DefaultIsConfirm(t *testing.T) {
	cfg := &userconfig.Config{}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm", m)
	}
}

func TestResolveMode_EscalationRestriction_EnvAutoWithoutConfigAuto(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "confirm"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "auto")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm (escalation blocked)", m)
	}
}

func TestResolveMode_EscalationRestriction_EnvAutoWithEmptyConfig(t *testing.T) {
	cfg := &userconfig.Config{}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "auto")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm (escalation blocked)", m)
	}
}

func TestResolveMode_EscalationAllowed_EnvAutoWithConfigAuto(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "auto"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "auto")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeAuto {
		t.Errorf("got %v, want ModeAuto", m)
	}
}

func TestResolveMode_EnvDowngrade_ConfigAutoEnvConfirm(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "auto"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "confirm")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm (downgrade)", m)
	}
}

func TestResolveMode_InvalidFlag(t *testing.T) {
	cfg := &userconfig.Config{}
	_, err := resolveMode("invalid", cfg)
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
}

func TestResolveMode_InvalidEnvVar(t *testing.T) {
	cfg := &userconfig.Config{}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "invalid")

	_, err := resolveMode("", cfg)
	if err == nil {
		t.Fatal("expected error for invalid env var")
	}
}

func TestResolveMode_InvalidConfig(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "invalid"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "")

	_, err := resolveMode("", cfg)
	if err == nil {
		t.Fatal("expected error for invalid config value")
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
